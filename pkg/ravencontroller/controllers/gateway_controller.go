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
	"reflect"

	"github.com/go-logr/logr"
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

	// 1. try to elect an active endpoint if possible
	activeEp := r.electActiveEndpoint(nodeList, &gw)
	r.recordEndpointEvent(ctx, &gw, gw.Status.ActiveEndpoint, activeEp)
	gw.Status.ActiveEndpoint = activeEp

	// 2. get subnet list of all nodes managed by the Gateway
	var subnets []string
	for _, v := range nodeList.Items {
		subnets = append(subnets, v.Spec.PodCIDR)
	}
	log.V(4).Info("managed subnet list", "subnets", subnets)
	gw.Status.Subnets = subnets

	// 3. add region labels to all managed nodes.
	err = r.ensureRegionLabelForNodes(ctx, nodeList, &gw)
	if err != nil {
		return ctrl.Result{}, err
	}

	err = r.Status().Update(ctx, &gw)
	if err != nil {
		log.Error(err, "unable to Update Gateway.status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GatewayReconciler) recordEndpointEvent(ctx context.Context, sourceObj *ravenv1alpha1.Gateway, previous, current *ravenv1alpha1.Endpoint) {
	log := log.FromContext(ctx)
	if current != nil && !reflect.DeepEqual(previous, current) {
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeNormal,
			ravenv1alpha1.EventActiveEndpointElected,
			fmt.Sprintf("The endpoint hosted by node %s has been elected active endpoint, privateIP: %s, publicIP: %s", current.NodeName, current.PrivateIP, current.PublicIP))
		log.V(2).Info("elected new active endpoint", "nodeName", current.NodeName, "privateIP", current.PrivateIP, "publicIP", current.PublicIP)
		return
	}
	if current == nil && previous != nil {
		r.recorder.Event(sourceObj.DeepCopy(), corev1.EventTypeWarning,
			ravenv1alpha1.EventActiveEndpointLost,
			fmt.Sprintf("The active endpoint hosted by node %s was lost, privateIP: %s, publicIP: %s", previous.NodeName, previous.PrivateIP, previous.PublicIP))
		log.V(2).Info("active endpoint lost", "nodeName", previous.NodeName, "privateIP", previous.PrivateIP, "publicIP", previous.PublicIP)
		return
	}
}

// ensureTopologyLabels ensures the topology labels source are consist with the given topologies.
func ensureTopologyLabels(source map[string]string, topologies map[string]string) {
	// cleanup first
	for k := range source {
		if ravenv1alpha1.IsTopologyLabel(k) {
			delete(source, k)
		}
	}
	// refill
	for k, v := range topologies {
		source[k] = v
	}
}

// electActiveEndpoint trys to elect an active Endpoint.
// If the current active endpoint remains valid, then we don't change it.
// Otherwise, try to elect a new one.
func (r *GatewayReconciler) electActiveEndpoint(nodeList corev1.NodeList, gw *ravenv1alpha1.Gateway) (ep *ravenv1alpha1.Endpoint) {
	// get all ready nodes referenced by endpoints
	readyNodes := make(map[string]corev1.Node)
	for _, v := range nodeList.Items {
		if isNodeReady(v) {
			readyNodes[v.Name] = v
		}
	}
	// checkActive check if the given endpoint is able to become the active endpoint.
	checkActive := func(ep *ravenv1alpha1.Endpoint) bool {
		if ep == nil {
			return false
		}
		// check if the node status is ready
		if _, ok := readyNodes[ep.NodeName]; ok {
			var inList bool
			// check if ep is in the Endpoint list
			for _, v := range gw.Spec.Endpoints {
				if reflect.DeepEqual(v, *ep) {
					inList = true
					break
				}
			}
			return inList
		}
		return false
	}

	// the current active endpoint is still competent.
	if checkActive(gw.Status.ActiveEndpoint) {
		return gw.Status.ActiveEndpoint.DeepCopy()
	}

	// try to elect an active endpoint.
	for _, v := range gw.Spec.Endpoints {
		if checkActive(&v) {
			return v.DeepCopy()
		}
	}
	return
}

// ensureRegionLabelForNodes ensure the region labels of nodes that are managed by the Gateway are consist with the Gateway。
func (r *GatewayReconciler) ensureRegionLabelForNodes(ctx context.Context, nodeList corev1.NodeList, gw *ravenv1alpha1.Gateway) error {
	topologies := make(map[string]string)
	for k, v := range gw.Labels {
		if ravenv1alpha1.IsTopologyLabel(k) {
			topologies[k] = gw.Labels[v]
		}
	}
	for k := range nodeList.Items {
		n := &nodeList.Items[k]
		if n.Labels == nil {
			n.Labels = make(map[string]string)
		}
		lb := n.Labels
		ensureTopologyLabels(lb, topologies)
		err := r.Update(ctx, n)
		if err != nil {
			return fmt.Errorf("unable to update node: %s", err)
		}
	}
	return nil
}

// mapNodeToRequest maps the given Node object to reconcile.Request.
func (r *GatewayReconciler) mapNodeToRequest(object client.Object) []reconcile.Request {
	node := object.(*corev1.Node)
	gwName, ok := node.Labels[ravenv1alpha1.LabelCurrentGateway]
	if !ok || gwName == "" {
		return []reconcile.Request{}
	}
	var gw ravenv1alpha1.Gateway
	err := r.Get(context.TODO(), types.NamespacedName{
		Name: gwName,
	}, &gw)
	if apierrs.IsNotFound(err) {
		r.Log.Info("gateway not found", "name", gwName)
		return []reconcile.Request{}
	}
	if err != nil {
		r.Log.Error(err, "unable to get Gateway")
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

// isNodeReady checks if the `node` is `corev1.NodeReady`
func isNodeReady(node corev1.Node) bool {
	_, nc := getNodeCondition(&node.Status, corev1.NodeReady)
	// GetNodeCondition will return nil and -1 if the condition is not present
	return nc != nil && nc.Status == corev1.ConditionTrue
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

// SetupWithManager sets up the controller with the Manager.
func (r *GatewayReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("Gateway")
	return ctrl.NewControllerManagedBy(mgr).For(&ravenv1alpha1.Gateway{}).
		Watches(
			&source.Kind{Type: &corev1.Node{}},
			handler.EnqueueRequestsFromMapFunc(r.mapNodeToRequest),
			builder.WithPredicates(NodeChangedPredicates{log: r.Log}),
		).Complete(r)
}
