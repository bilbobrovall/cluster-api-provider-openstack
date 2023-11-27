/*
Copyright 2019 The Kubernetes Authors.

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

package v1alpha7

import (
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// log is for logging in this package.
var openstackfloatingippoollog = logf.Log.WithName("openstackfloatingippool-resource")

func (r *OpenStackFloatingIPPool) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/mutate-infrastructure-cluster-x-k8s-io-v1alpha7-openstackfloatingippool,mutating=true,failurePolicy=fail,groups=infrastructure.cluster.x-k8s.io,resources=openstackfloatingippools,verbs=create;update,versions=v1alpha7,name=default.openstackfloatingippool.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1beta1
//+kubebuilder:webhook:verbs=create;update,path=/validate-infrastructure-cluster-x-k8s-io-v1alpha7-openstackfloatingippool,mutating=false,failurePolicy=fail,groups=infrastructure.cluster.x-k8s.io,resources=openstackfloatingippools,versions=v1alpha7,name=validation.openstackfloatingippool.infrastructure.cluster.x-k8s.io,sideEffects=None,admissionReviewVersions=v1beta1

var (
	_ webhook.Defaulter = &OpenStackFloatingIPPool{}
	_ webhook.Validator = &OpenStackFloatingIPPool{}
)

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *OpenStackFloatingIPPool) Default() {
	openstackfloatingippoollog.Info("default", "name", r.Name)
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackFloatingIPPool) ValidateCreate() (admission.Warnings, error) {
	openstackfloatingippoollog.Info("validate create", "name", r.Name)

	// TODO(user): fill in your validation logic upon object creation.
	return nil, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackFloatingIPPool) ValidateUpdate(old runtime.Object) (admission.Warnings, error) {
	openstackfloatingippoollog.Info("validate update", "name", r.Name)

	// TODO(user): fill in your validation logic upon object update.
	return nil, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *OpenStackFloatingIPPool) ValidateDelete() (admission.Warnings, error) {
	openstackfloatingippoollog.Info("validate delete", "name", r.Name)

	// TODO(user): fill in your validation logic upon object deletion.
	return nil, nil
}
