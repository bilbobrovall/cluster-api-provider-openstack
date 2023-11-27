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

package controllers

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	infrav1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha7"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/scope"
	ipamutils "sigs.k8s.io/cluster-api-provider-openstack/pkg/utils/ipam"
)

// IPAddressReconciler reconciles a IPAddress object
type IPAddressReconciler struct {
	Client           client.Client
	Recorder         record.EventRecorder
	WatchFilterValue string
	ScopeFactory     scope.Factory
	CaCertificates   []byte // PEM encoded ca certificates.

	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8x.io.cluster.x-k8s.io,resources=ipaddresses,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8x.io.cluster.x-k8s.io,resources=ipaddresses/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the IPAddress object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *IPAddressReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {

	// Get IPAddress
	ipAddress := &ipamv1.IPAddress{}
	if err := r.Client.Get(ctx, req.NamespacedName, ipAddress); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// If its deleting and has no finalizer, exit.
	if !ipAddress.ObjectMeta.DeletionTimestamp.IsZero() {

		// Verify that it has the finalizer, and that its the only one left before deleting IPs.
		if controllerutil.ContainsFinalizer(ipAddress, infrav1.DeleteFloatingIPFinalizer) && len(ipAddress.GetFinalizers()) == 1 {

		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAddressReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ipamv1.IPAddress{}, builder.WithPredicates(
			predicate.And(
				ipamutils.AddressReferencesPoolKind(
					metav1.GroupKind{
						Group: infrav1.GroupVersion.Group,
						Kind:  openStackFloatingIPPool,
					}),
				ipamutils.HasFinalizerAndIsDeleting(infrav1.DeleteFloatingIPFinalizer),
			)),
		).
		Complete(r)
}
