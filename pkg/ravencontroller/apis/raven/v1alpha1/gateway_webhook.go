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

package v1alpha1

import (
	"fmt"
	"net"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var defaultBackend = BackendLibreswan

// log is for logging in this package.
var gatewaylog = logf.Log.WithName("gateway-resource")

func (g *Gateway) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(g).
		Complete()
}

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

//+kubebuilder:webhook:path=/mutate-raven-openyurt-io-v1alpha1-gateway,mutating=true,failurePolicy=fail,sideEffects=None,groups=raven.openyurt.io,resources=gateways,verbs=create;update,versions=v1alpha1,name=mgateway.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Defaulter = &Gateway{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (g *Gateway) Default() {
	gatewaylog.V(4).Info("default", "name", g.Name)
	g.Spec.NodeSelector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			LabelCurrentGateway: g.Name,
		},
	}
	if g.Spec.Backend == "" {
		g.Spec.Backend = defaultBackend
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
//+kubebuilder:webhook:path=/validate-raven-openyurt-io-v1alpha1-gateway,mutating=false,failurePolicy=fail,sideEffects=None,groups=raven.openyurt.io,resources=gateways,verbs=create;update,versions=v1alpha1,name=vgateway.kb.io,admissionReviewVersions={v1,v1beta1}

var _ webhook.Validator = &Gateway{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (g *Gateway) ValidateCreate() error {
	gatewaylog.V(4).Info("validate create", "name", g.Name)
	return g.validateGateway()
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (g *Gateway) ValidateUpdate(old runtime.Object) error {
	gatewaylog.V(4).Info("validate update", "name", g.Name)
	return g.validateGateway()
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (g *Gateway) ValidateDelete() error {
	gatewaylog.V(4).Info("validate delete", "name", g.Name)
	return nil
}

func (r *Gateway) validateGateway() error {
	var errList field.ErrorList
	for i, ep := range r.Spec.Endpoints {
		if err := validateIP(ep.PublicIP); err != nil {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("publicIP")
			errList = append(errList, field.Invalid(fldPath, ep.PublicIP, fmt.Sprintf("endpoints[%d].publicIP must be a validate IP address", i)))
		}
		if err := validateIP(ep.PrivateIP); err != nil {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("privateIP")
			errList = append(errList, field.Invalid(fldPath, ep.PrivateIP, fmt.Sprintf("endpoints[%d].privateIP must be a validate IP address", i)))
		}
		if len(ep.NodeName) == 0 {
			fldPath := field.NewPath("spec").Child(fmt.Sprintf("endpoints[%d]", i)).Child("nodeName")
			errList = append(errList, field.Invalid(fldPath, ep.NodeName, fmt.Sprintf("endpoints[%d].nodeName cannot be empty", i)))
		}
	}
	if errList != nil {
		return apierrors.NewInvalid(
			schema.GroupKind{Group: SchemeGroupVersion.Group, Kind: r.Kind},
			r.Name, errList)
	}
	return nil
}

func validateIP(ip string) error {
	s := net.ParseIP(ip)
	if s.To4() != nil || s.To16() != nil {
		return nil
	}
	return fmt.Errorf("invalid ip address: %s", ip)
}
