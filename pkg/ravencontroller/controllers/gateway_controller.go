/*
Copyright 2022 The OpenYurt Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"net"
	"reflect"
	"sort"
	"strings"

	"github.com/go-logr/logr"
	combinations "github.com/mxschmitt/golang-combinations"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	calicov3 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/calico/v3"
	ravenv1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
)

// GatewayReconciler reconciles a Gateway object
type GatewayReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=raven.openyurt.io,resources=gateways/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=nodes,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=crd.projectcalico.org,resources=blockaffinities,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(4).Info("started reconciling Gateway", "name", req.Name)
	defer func() {
		log.V(4).Info("finished reconciling Gateway", "name", req.Name)
	}()
	var gw ravenv1alpha1.Gateway
	if err := r.Get(ctx, req.NamespacedName, &gw); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// get all managed nodes
	var nodeList corev1.NodeList
	nodeSelector, err := labels.Parse(fmt.Sprintf(ravenv1alpha1.LabelCurrentGateway+"=%s", gw.Name))
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.List(ctx, &nodeList, &client.ListOptions{
		LabelSelector: nodeSelector,
	})
	if err != nil {
		err = fmt.Errorf("unable to list nodes: %s", err)
		return ctrl.Result{}, err
	}

	// get all agent pods
	var podList corev1.PodList
	podSelector, err := labels.Parse(fmt.Sprintf(ravenv1alpha1.LabelAgentPod+"=%s", "true"))
	if err != nil {
		return ctrl.Result{}, err
	}
	err = r.List(ctx, &podList, &client.ListOptions{
		LabelSelector: podSelector,
	})
	if err != nil {
		err = fmt.Errorf("unable to list pods: %s", err)
		return ctrl.Result{}, err
	}

	// 1. try to elect active endpoints if possible
	activeEps, healthyAepNum := r.electActiveEndpoints(nodeList, podList, &gw)
	if healthyAepNum == 0 {
		return ctrl.Result{Requeue: true}, nil
	}
	r.recordEndpointEvent(ctx, &gw, gw.Status.ActiveEndpoints, activeEps)
	gw.Status.ActiveEndpoints = activeEps

	// 2. get nodeInfo list of nodes managed by the Gateway
	nodes := make([]ravenv1alpha1.NodeInfo, 0, len(nodeList.Items))
	for _, v := range nodeList.Items {
		podCIDRs, err := r.getPodCIDRs(ctx, v)
		if err != nil {
			log.Error(err, "unable to get podCIDR")
			return ctrl.Result{}, err
		}
		node := ravenv1alpha1.NodeInfo{
			NodeName:  v.Name,
			PrivateIP: getNodeInternalIP(v),
			Subnets:   podCIDRs,
		}
		if r.assignNodeToActiveEndpoint(&node, activeEps) {
			continue
		}
		nodes = append(nodes, node)
	}
	log.V(4).Info("managed node info list", "nodes", nodes)

	// 3. assign nodes to healthy active endpoints
	dividedNodes := divideNodes(nodes, healthyAepNum)
	current := 0
	for i := range activeEps {
		if activeEps[i].Healthy && current < len(dividedNodes) {
			activeEps[i].Nodes = append(activeEps[i].Nodes, dividedNodes[current]...)
			current++
		}
	}

	// 4. decide central gateway
	var gatewayList ravenv1alpha1.GatewayList
	err = r.List(ctx, &gatewayList, &client.ListOptions{})
	if err != nil {
		err = fmt.Errorf("unable to list gateways: %s", err)
		return ctrl.Result{}, err
	}
	central := r.findCentralGateway(&gatewayList)

	// 5. reconcile central gateway
	if central != nil {
		result, err := r.reconcileCentralGateway(ctx, central, &gw, &gatewayList)
		if err != nil {
			return result, err
		}
	}
	// 6. update current gateway
	if central == nil || central.Name != gw.Name {
		gw.Status.Central = false
		err = r.Status().Update(ctx, &gw)
		if err != nil {
			log.Error(err, "unable to Update Gateway.status")
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) reconcileCentralGateway(ctx context.Context, centralGw *ravenv1alpha1.Gateway, currentGw *ravenv1alpha1.Gateway, gwList *ravenv1alpha1.GatewayList) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(4).Info("started reconciling central Gateway", "name", centralGw.Name)
	defer func() {
		log.V(4).Info("finished reconciling central Gateway", "name", centralGw.Name)
	}()
	if centralGw.Name == currentGw.Name {
		centralGw.Status.ActiveEndpoints = currentGw.Status.ActiveEndpoints
	}
	centralGw.Status.Central = true

	var gateways []ravenv1alpha1.Gateway
	for _, v := range gwList.Items {
		if v.Name != centralGw.Name {
			gateways = append(gateways, v)
		}
	}

	// 1. compute how many healthy active endpoint in central gateway
	chunkNum := 0
	for _, aep := range centralGw.Status.ActiveEndpoints {
		if aep.Healthy {
			chunkNum++
		}
	}

	// 2. divide forward vpn connections by healthy active endpoint number
	dividedForwards := divideForwards(generateForwards(gateways), chunkNum)
	current := 0
	for i := range centralGw.Status.ActiveEndpoints {
		centralGw.Status.ActiveEndpoints[i].Forwards = make([]ravenv1alpha1.Forward, 0)
		if centralGw.Status.ActiveEndpoints[i].Healthy && current < len(dividedForwards) {
			for _, v := range dividedForwards[current] {
				centralGw.Status.ActiveEndpoints[i].Forwards = append(centralGw.Status.ActiveEndpoints[i].Forwards, v, ravenv1alpha1.Forward{
					From: v.To,
					To:   v.From,
				})
			}
			current++
		}
	}

	err := r.Status().Update(ctx, centralGw)
	if err != nil {
		log.Error(err, "unable to Update Central Gateway.status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) findCentralGateway(gatewayList *ravenv1alpha1.GatewayList) *ravenv1alpha1.Gateway {
	candidates := make([]ravenv1alpha1.Gateway, 0)
	for _, v := range gatewayList.Items {
		if v.Spec.Central {
			candidates = append(candidates, v)
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// select the early one
	sort.Slice(candidates, func(i, j int) bool {
		timestamp1 := candidates[i].GetCreationTimestamp()
		timestamp2 := candidates[j].GetCreationTimestamp()
		return timestamp1.Before(&timestamp2)
	})
	return &candidates[0]
}

func (r *GatewayReconciler) assignNodeToActiveEndpoint(node *ravenv1alpha1.NodeInfo, activeEps []*ravenv1alpha1.ActiveEndpoint) bool {
	for _, aep := range activeEps {
		if aep.Endpoint.NodeName == node.NodeName {
			aep.Nodes = append(aep.Nodes, *node)
			return true
		}
	}
	return false
}

func (r *GatewayReconciler) recordEndpointEvent(ctx context.Context, sourceObj *ravenv1alpha1.Gateway, previous, current []*ravenv1alpha1.ActiveEndpoint) {
	log := log.FromContext(ctx)
	if len(current) != 0 && !reflect.DeepEqual(previous, current) {
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeNormal,
			ravenv1alpha1.EventActiveEndpointsElected,
			fmt.Sprintf("the new active endpoints were elected in gateway: %s", sourceObj.Name))
		log.V(2).Info("elected new active endpoints", "gwName", sourceObj.Name)
		return
	}
	if len(current) == 0 && len(previous) != 0 {
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeWarning,
			ravenv1alpha1.EventActiveEndpointsLost,
			fmt.Sprintf("the active endpoints were lost in gateway: %s", sourceObj.Name))
		log.V(2).Info("active endpoints were lost", "gwName", sourceObj.Name)
		return
	}
}

// electActiveEndpoints trys to elect active Endpoints.
// If the current active endpoint remains valid, then we don't change it.
// Otherwise, try to elect a new one.
func (r *GatewayReconciler) electActiveEndpoints(nodeList corev1.NodeList, podList corev1.PodList, gw *ravenv1alpha1.Gateway) ([]*ravenv1alpha1.ActiveEndpoint, int) {
	// get all ready nodes referenced by endpoints
	readyNodes := make(map[string]corev1.Node)
	for _, v := range nodeList.Items {
		if isNodeReady(v) {
			readyNodes[v.Name] = v
		}
	}
	// get all ready agent pods referenced by endpoints
	readyPods := make(map[string]corev1.Pod)
	for _, p := range podList.Items {
		if _, ok := readyNodes[p.Spec.NodeName]; ok && isPodReady(p) {
			readyPods[p.Spec.NodeName] = p
		}
	}
	// checkActive check if the given endpoint is able to become the active endpoint.
	checkActive := func(ep *ravenv1alpha1.Endpoint) bool {
		if ep == nil {
			return false
		}
		// check if the agent pod status is ready
		if _, ok := readyPods[ep.NodeName]; ok {
			// check if ep is in the Endpoint list
			for _, v := range gw.Spec.Endpoints {
				if reflect.DeepEqual(v, *ep) {
					return true
				}
			}
		}
		return false
	}

	replicas := *(gw.Spec.Replicas)
	activeEndpoints := make(map[string]*ravenv1alpha1.ActiveEndpoint)

	current := 0
	// 1. the current active endpoint is still competent.
	for _, aep := range gw.Status.ActiveEndpoints {
		if current == replicas {
			break
		}
		if checkActive(aep.Endpoint) {
			aep := aep.DeepCopy()
			activeEndpoints[aep.Endpoint.NodeName] = &ravenv1alpha1.ActiveEndpoint{
				Endpoint: aep.Endpoint,
				Healthy:  true,
			}
			current++
		}
	}

	// 2. try to elect new healthy active endpoints
	for _, ep := range gw.Spec.Endpoints {
		if current == replicas {
			break
		}
		if _, ok := activeEndpoints[ep.NodeName]; ok {
			continue
		}
		ep := ep.DeepCopy()
		if checkActive(ep) {
			activeEndpoints[ep.NodeName] = &ravenv1alpha1.ActiveEndpoint{
				Endpoint: ep,
				Healthy:  true,
			}
			current++
		}
	}

	healthyAepNum := len(activeEndpoints)

	// 3. if not enough healthy active endpoint, use unhealthy one
	for _, ep := range gw.Spec.Endpoints {
		if current == replicas {
			break
		}
		if _, ok := activeEndpoints[ep.NodeName]; ok {
			continue
		}
		ep := ep.DeepCopy()
		activeEndpoints[ep.NodeName] = &ravenv1alpha1.ActiveEndpoint{
			Endpoint: ep,
			Healthy:  false,
		}
		current++
	}

	res := make([]*ravenv1alpha1.ActiveEndpoint, 0, len(activeEndpoints))
	for _, v := range activeEndpoints {
		res = append(res, v)
	}

	return res, healthyAepNum
}

// mapPodToRequest maps the given Agent Pod object to reconcile.Request.
func (r *GatewayReconciler) mapPodToRequest(object client.Object) []reconcile.Request {
	pod := object.(*corev1.Pod)
	nodeName := pod.Spec.NodeName
	if nodeName == "" {
		return []reconcile.Request{}
	}
	var node corev1.Node
	err := r.Get(context.TODO(), types.NamespacedName{
		Name: nodeName,
	}, &node)
	if err != nil {
		r.Log.Error(err, "unable to get node")
		return []reconcile.Request{}
	}
	gwName, ok := node.Labels[ravenv1alpha1.LabelCurrentGateway]
	if !ok || gwName == "" {
		return []reconcile.Request{}
	}
	var gw ravenv1alpha1.Gateway
	err = r.Get(context.TODO(), types.NamespacedName{
		Name: gwName,
	}, &gw)
	if apierrs.IsNotFound(err) {
		r.Log.Info("gateway not found", "name", gwName)
		return []reconcile.Request{}
	}
	if err != nil {
		r.Log.Error(err, "unable to get Gateway")
		return []reconcile.Request{}
	}
	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: "",
				Name:      gwName,
			},
		},
	}
}

// divideNodes divides the given nodes into lists
func divideNodes(nodes []ravenv1alpha1.NodeInfo, chunkNum int) [][]ravenv1alpha1.NodeInfo {
	if len(nodes) == 0 || chunkNum == 0 {
		return [][]ravenv1alpha1.NodeInfo{}
	}
	chunkSize := len(nodes) / chunkNum
	if len(nodes)%chunkNum != 0 {
		chunkSize++
	}
	chunks := make([][]ravenv1alpha1.NodeInfo, chunkNum)
	for i := 0; i < chunkNum; i++ {
		chunks[i] = make([]ravenv1alpha1.NodeInfo, 0, chunkSize)
	}
	for i, v := range nodes {
		chunks[i%chunkNum] = append(chunks[i%chunkNum], v)
	}
	return chunks
}

// generateForwards generate all connections that need to forward by central gateway
func generateForwards(gateways []ravenv1alpha1.Gateway) []ravenv1alpha1.Forward {
	forwards := make([]ravenv1alpha1.Forward, 0)
	gwMap := make(map[string]ravenv1alpha1.Gateway)
	gwName := make([]string, 0)

	for _, v := range gateways {
		gwMap[v.Name] = v
		gwName = append(gwName, v.Name)
	}

	needForward := func(gw1 ravenv1alpha1.Gateway, gw2 ravenv1alpha1.Gateway) bool {
		forward := true
		for _, v := range append(gw1.Spec.Endpoints, gw2.Spec.Endpoints...) {
			if !v.UnderNAT {
				forward = false
				break
			}
		}
		return forward
	}

	if len(gwName) > 1 {
		for _, v := range combinations.Combinations(gwName, 2) {
			if needForward(gwMap[v[0]], gwMap[v[1]]) {
				forwards = append(forwards, ravenv1alpha1.Forward{
					From: v[0],
					To:   v[1],
				})
			}
		}
	}
	return forwards
}

// divideForwards divides the given connections that need to forward.
func divideForwards(forwards []ravenv1alpha1.Forward, chunkNum int) [][]ravenv1alpha1.Forward {
	if len(forwards) == 0 || chunkNum == 0 {
		return [][]ravenv1alpha1.Forward{}
	}
	chunkSize := len(forwards) / chunkNum
	if len(forwards)%chunkNum != 0 {
		chunkSize++
	}
	chunks := make([][]ravenv1alpha1.Forward, chunkNum)
	for i := 0; i < chunkNum; i++ {
		chunks[i] = make([]ravenv1alpha1.Forward, 0, chunkSize)
	}

	for i, v := range forwards {
		chunks[i%chunkNum] = append(chunks[i%chunkNum], v)
	}
	return chunks
}

// isNodeReady checks if the `node` is `corev1.NodeReady`
func isNodeReady(node corev1.Node) bool {
	_, nc := getNodeCondition(&node.Status, corev1.NodeReady)
	// GetNodeCondition will return nil and -1 if the condition is not present
	return nc != nil && nc.Status == corev1.ConditionTrue
}

// isPodReady checks if the `Pod` is `corev1.PodReady`
func isPodReady(pod corev1.Pod) bool {
	_, pc := getPodCondition(&pod.Status, corev1.PodReady)
	// GetPodCondition will return nil and -1 if the condition is not present
	return pc != nil && pc.Status == corev1.ConditionTrue
}

// getNodeCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func getNodeCondition(status *corev1.NodeStatus, conditionType corev1.NodeConditionType) (int, *corev1.NodeCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

// getPodCondition extracts the provided condition from the given status and returns that.
// Returns nil and -1 if the condition is not present, and the index of the located condition.
func getPodCondition(status *corev1.PodStatus, conditionType corev1.PodConditionType) (int, *corev1.PodCondition) {
	if status == nil {
		return -1, nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

// getNodeInternalIP returns internal ip of the given `node`.
func getNodeInternalIP(node corev1.Node) string {
	var ip string
	for _, addr := range node.Status.Addresses {
		if addr.Type == corev1.NodeInternalIP && net.ParseIP(addr.Address) != nil {
			ip = addr.Address
			break
		}
	}
	return ip
}

// getPodCIDRs returns the pod IP ranges assigned to the node.
func (r *GatewayReconciler) getPodCIDRs(ctx context.Context, node corev1.Node) ([]string, error) {
	podCIDRs := make([]string, 0)
	for key := range node.Annotations {
		if strings.Contains(key, "projectcalico.org") {
			var blockAffinityList calicov3.BlockAffinityList
			err := r.List(ctx, &blockAffinityList)
			if err != nil {
				err = fmt.Errorf("unable to list calico blockaffinity: %s", err)
				return nil, err
			}
			for _, v := range blockAffinityList.Items {
				if v.Spec.Node != node.Name || v.Spec.State != "confirmed" {
					continue
				}
				podCIDRs = append(podCIDRs, v.Spec.CIDR)
			}
			return podCIDRs, nil
		}
	}
	return append(podCIDRs, node.Spec.PodCIDR), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("Gateway")
	return ctrl.NewControllerManagedBy(mgr).For(&ravenv1alpha1.Gateway{}).
		Watches(
			&source.Kind{Type: &corev1.Pod{}},
			handler.EnqueueRequestsFromMapFunc(r.mapPodToRequest),
			builder.WithPredicates(PodChangedPredicates{log: r.Log}),
		).Complete(r)
}
