/*
 * Copyright 2022 The OpenYurt Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package service

import (
	"context"
	"fmt"
	"reflect"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
	"github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/controllers/util"
)

// ServiceReconciler reconciles a Endpoint object
type ServiceReconciler struct {
	client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.7.2/pkg/reconcile
func (r *ServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.V(4).Info("started reconciling Endpoint", "name", req.Name, "namespace", req.Namespace)
	defer func() {
		log.V(4).Info("finished reconciling Endpoint", "name", req.Name, "namespace", req.Namespace)
	}()

	var gatewayList v1alpha1.GatewayList
	if err := r.List(ctx, &gatewayList); err != nil {
		err = fmt.Errorf("unable to list gateways: %s", err)
		return ctrl.Result{}, err
	}

	if err := r.reconcileService(ctx, req, &gatewayList); err != nil {
		err = fmt.Errorf("unable to reconcile service: %s", err)
		return ctrl.Result{}, err
	}

	if err := r.reconcileEndpoint(ctx, req, &gatewayList); err != nil {
		err = fmt.Errorf("unable to reconcile endpoint: %s", err)
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *ServiceReconciler) reconcileService(ctx context.Context, req ctrl.Request, gatewayList *v1alpha1.GatewayList) error {
	for _, gw := range gatewayList.Items {
		if util.IsGatewayExposeByLB(&gw) {
			return r.ensureService(ctx, req)
		}
	}
	return r.cleanService(ctx, req)
}

func (r *ServiceReconciler) cleanService(ctx context.Context, req ctrl.Request) error {
	if err := r.Delete(ctx, &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}); err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ServiceReconciler) ensureService(ctx context.Context, req ctrl.Request) error {
	log := log.FromContext(ctx)
	var service corev1.Service
	if err := r.Get(ctx, req.NamespacedName, &service); err != nil {
		if apierrs.IsNotFound(err) {
			log.V(2).Info("create service", "name", req.Name, "namespace", req.Namespace)
			return r.Create(ctx, &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      req.Name,
					Namespace: req.Namespace,
				},
				Spec: corev1.ServiceSpec{
					Ports: []corev1.ServicePort{
						{
							Port:       4500,
							Protocol:   corev1.ProtocolUDP,
							TargetPort: intstr.FromInt(4500),
						},
					},
					Type:                  corev1.ServiceTypeLoadBalancer,
					ExternalTrafficPolicy: corev1.ServiceExternalTrafficPolicyTypeLocal,
				},
			})

		}
	}
	return nil
}

func (r *ServiceReconciler) reconcileEndpoint(ctx context.Context, req ctrl.Request, gatewayList *v1alpha1.GatewayList) error {
	exposedByLB := false
	for _, gw := range gatewayList.Items {
		if util.IsGatewayExposeByLB(&gw) {
			exposedByLB = true
			if gw.Status.ActiveEndpoint != nil {
				var node corev1.Node
				if err := r.Get(ctx, types.NamespacedName{
					Name: gw.Status.ActiveEndpoint.NodeName,
				}, &node); err != nil {
					return err
				}
				return r.ensureEndpoint(ctx, req, node)
			}
		}
	}
	if !exposedByLB {
		return r.cleanEndpoint(ctx, req)
	}
	return nil
}

func (r *ServiceReconciler) cleanEndpoint(ctx context.Context, req ctrl.Request) error {
	if err := r.Delete(ctx, &corev1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}); err != nil && !apierrs.IsNotFound(err) {
		return err
	}
	return nil
}

func (r *ServiceReconciler) ensureEndpoint(ctx context.Context, req ctrl.Request, node corev1.Node) error {
	log := log.FromContext(ctx)
	var serviceEndpoint corev1.Endpoints
	newSubnets := []corev1.EndpointSubset{
		{
			Addresses: []corev1.EndpointAddress{
				{
					IP:       util.GetNodeInternalIP(node),
					NodeName: func(n corev1.Node) *string { return &n.Name }(node),
				},
			},
			Ports: []corev1.EndpointPort{
				{
					Port:     4500,
					Protocol: corev1.ProtocolUDP,
				},
			},
		},
	}
	if err := r.Get(ctx, req.NamespacedName, &serviceEndpoint); err != nil {
		if apierrs.IsNotFound(err) {
			log.V(2).Info("create endpoint", "name", req.Name, "namespace", req.Namespace)
			return r.Create(ctx, &corev1.Endpoints{
				ObjectMeta: metav1.ObjectMeta{
					Name:      req.Name,
					Namespace: req.Namespace,
				},
				Subsets: newSubnets,
			})
		}
		return err
	}

	if !reflect.DeepEqual(serviceEndpoint.Subsets, newSubnets) {
		log.V(2).Info("update endpoint", "name", req.Name, "namespace", req.Namespace)
		serviceEndpoint.Subsets = newSubnets
		return r.Update(ctx, &serviceEndpoint)
	}
	log.V(2).Info("skip to update endpoint", "name", req.Name, "namespace", req.Namespace)
	return nil
}

// mapGatewayToRequest maps the given Gateway object to reconcile.Request.
func (r *ServiceReconciler) mapGatewayToRequest(object client.Object) []reconcile.Request {
	gw := object.(*v1alpha1.Gateway)
	if util.IsGatewayExposeByLB(gw) {
		return []reconcile.Request{
			{
				NamespacedName: v1alpha1.ServiceNamespacedName,
			},
		}
	}
	return []reconcile.Request{}
}

// mapEndpointToRequest maps the given Endpoint object to reconcile.Request.
func (r *ServiceReconciler) mapEndpointToRequest(object client.Object) []reconcile.Request {
	ep := object.(*corev1.Endpoints)
	if ep.Name == v1alpha1.ServiceNamespacedName.Name && ep.Namespace == v1alpha1.ServiceNamespacedName.Namespace {
		return []reconcile.Request{
			{
				NamespacedName: v1alpha1.ServiceNamespacedName,
			},
		}
	}
	return []reconcile.Request{}
}

// SetupWithManager sets up the controller with the Manager.
func (r *ServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.recorder = mgr.GetEventRecorderFor("Service")
	return ctrl.NewControllerManagedBy(mgr).
		For(
			&corev1.Service{},
			builder.WithPredicates(ServicePredicates{log: r.Log})).
		Watches(
			&source.Kind{Type: &v1alpha1.Gateway{}},
			handler.EnqueueRequestsFromMapFunc(r.mapGatewayToRequest),
		).
		Watches(
			&source.Kind{Type: &corev1.Endpoints{}},
			handler.EnqueueRequestsFromMapFunc(r.mapEndpointToRequest),
		).Complete(r)
}
