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

// Code generated by client-gen. DO NOT EDIT.

package fake

import (
	"context"

	v1alpha1 "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/apis/raven/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	labels "k8s.io/apimachinery/pkg/labels"
	schema "k8s.io/apimachinery/pkg/runtime/schema"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	testing "k8s.io/client-go/testing"
)

// FakeGateways implements GatewayInterface
type FakeGateways struct {
	Fake *FakeRavenV1alpha1
}

var gatewaysResource = schema.GroupVersionResource{Group: "raven", Version: "v1alpha1", Resource: "gateways"}

var gatewaysKind = schema.GroupVersionKind{Group: "raven", Version: "v1alpha1", Kind: "Gateway"}

// Get takes name of the gateway, and returns the corresponding gateway object, and an error if there is any.
func (c *FakeGateways) Get(ctx context.Context, name string, options v1.GetOptions) (result *v1alpha1.Gateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootGetAction(gatewaysResource, name), &v1alpha1.Gateway{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Gateway), err
}

// List takes label and field selectors, and returns the list of Gateways that match those selectors.
func (c *FakeGateways) List(ctx context.Context, opts v1.ListOptions) (result *v1alpha1.GatewayList, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootListAction(gatewaysResource, gatewaysKind, opts), &v1alpha1.GatewayList{})
	if obj == nil {
		return nil, err
	}

	label, _, _ := testing.ExtractFromListOptions(opts)
	if label == nil {
		label = labels.Everything()
	}
	list := &v1alpha1.GatewayList{ListMeta: obj.(*v1alpha1.GatewayList).ListMeta}
	for _, item := range obj.(*v1alpha1.GatewayList).Items {
		if label.Matches(labels.Set(item.Labels)) {
			list.Items = append(list.Items, item)
		}
	}
	return list, err
}

// Watch returns a watch.Interface that watches the requested gateways.
func (c *FakeGateways) Watch(ctx context.Context, opts v1.ListOptions) (watch.Interface, error) {
	return c.Fake.
		InvokesWatch(testing.NewRootWatchAction(gatewaysResource, opts))
}

// Create takes the representation of a gateway and creates it.  Returns the server's representation of the gateway, and an error, if there is any.
func (c *FakeGateways) Create(ctx context.Context, gateway *v1alpha1.Gateway, opts v1.CreateOptions) (result *v1alpha1.Gateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootCreateAction(gatewaysResource, gateway), &v1alpha1.Gateway{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Gateway), err
}

// Update takes the representation of a gateway and updates it. Returns the server's representation of the gateway, and an error, if there is any.
func (c *FakeGateways) Update(ctx context.Context, gateway *v1alpha1.Gateway, opts v1.UpdateOptions) (result *v1alpha1.Gateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateAction(gatewaysResource, gateway), &v1alpha1.Gateway{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Gateway), err
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().
func (c *FakeGateways) UpdateStatus(ctx context.Context, gateway *v1alpha1.Gateway, opts v1.UpdateOptions) (*v1alpha1.Gateway, error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootUpdateSubresourceAction(gatewaysResource, "status", gateway), &v1alpha1.Gateway{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Gateway), err
}

// Delete takes name of the gateway and deletes it. Returns an error if one occurs.
func (c *FakeGateways) Delete(ctx context.Context, name string, opts v1.DeleteOptions) error {
	_, err := c.Fake.
		Invokes(testing.NewRootDeleteAction(gatewaysResource, name), &v1alpha1.Gateway{})
	return err
}

// DeleteCollection deletes a collection of objects.
func (c *FakeGateways) DeleteCollection(ctx context.Context, opts v1.DeleteOptions, listOpts v1.ListOptions) error {
	action := testing.NewRootDeleteCollectionAction(gatewaysResource, listOpts)

	_, err := c.Fake.Invokes(action, &v1alpha1.GatewayList{})
	return err
}

// Patch applies the patch and returns the patched gateway.
func (c *FakeGateways) Patch(ctx context.Context, name string, pt types.PatchType, data []byte, opts v1.PatchOptions, subresources ...string) (result *v1alpha1.Gateway, err error) {
	obj, err := c.Fake.
		Invokes(testing.NewRootPatchSubresourceAction(gatewaysResource, name, pt, data, subresources...), &v1alpha1.Gateway{})
	if obj == nil {
		return nil, err
	}
	return obj.(*v1alpha1.Gateway), err
}
