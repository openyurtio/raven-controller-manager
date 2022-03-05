/*
Copyright 2020 The OpenYurt Authors.
Copyright 2020 The Kruise Authors.

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

package webhook

import (
	"fmt"
	"time"

	clientset "k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	webhookcontroller "github.com/openyurtio/raven-controller-manager/pkg/ravencontroller/webhook/util/controller"
)

// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=get;list;watch;create;update;patch;delete

// Initialize initialize webhook controllers.
func Initialize(mgr ctrl.Manager, stopCh <-chan struct{}) error {
	cli, err := client.NewDelegatingClient(client.NewDelegatingClientInput{
		CacheReader: mgr.GetAPIReader(),
		Client:      mgr.GetClient(),
	})
	if err != nil {
		return err
	}
	kubeCli, err := clientset.NewForConfig(mgr.GetConfig())
	if err != nil {
		return err
	}
	c, err := webhookcontroller.New(cli, kubeCli)
	if err != nil {
		return err
	}
	go func() {
		c.Start(stopCh)
	}()

	timer := time.NewTimer(time.Second * 5)
	defer timer.Stop()
	select {
	case <-webhookcontroller.Inited():
		return nil
	case <-timer.C:
		return fmt.Errorf("failed to start webhook controller for waiting more than 5s")
	}
}
