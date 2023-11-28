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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrav1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha7"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/cloud/services/networking"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/scope"
	ipamutils "sigs.k8s.io/cluster-api-provider-openstack/pkg/utils/ipam"
)

// IPAddressReconciler reconciles a IPAddress object.
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
	log := ctrl.LoggerFrom(ctx).WithValues("ipaddress", req.NamespacedName)

	ipAddress := &ipamv1.IPAddress{}
	if err := r.Client.Get(ctx, req.NamespacedName, ipAddress); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if ipAddress.Spec.PoolRef.Kind != openStackFloatingIPPool {
		log.Info("IPAddress is not associated with OpenStackFloatingIPPool")
		return ctrl.Result{}, nil
	}

	pool := &infrav1.OpenStackFloatingIPPool{}

	if err := r.Client.Get(ctx, client.ObjectKey{
		Namespace: ipAddress.Namespace,
		Name:      ipAddress.Spec.PoolRef.Name,
	}, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	log = log.WithValues("openStackFloatingIPPool", pool.Name)

	scope, err := r.ScopeFactory.NewClientScopeFromFloatingIPPool(ctx, r.Client, pool, r.CaCertificates, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	networkingService, err := networking.NewService(scope)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !ipAddress.ObjectMeta.DeletionTimestamp.IsZero() {
		// If the IPAddress has more than one finalizer, it has not been released by machine yet and we should not delete it.
		if controllerutil.ContainsFinalizer(ipAddress, infrav1.DeleteFloatingIPFinalizer) && len(ipAddress.GetFinalizers()) == 1 {
			if err = networkingService.DeleteFloatingIP(pool, ipAddress.Spec.Address); err != nil {
				return ctrl.Result{}, fmt.Errorf("delete floating IP %q: %w", ipAddress.Spec.Address, err)
			}
			controllerutil.RemoveFinalizer(ipAddress, infrav1.DeleteFloatingIPFinalizer)
			if err := r.Client.Update(ctx, ipAddress); err != nil {
				return ctrl.Result{}, err
			}
			// remove ip from pool status
			return ctrl.Result{}, nil
		}
		scope.Logger().Info("IPAddress is being deleted but has other finalizers, waiting for them to be removed")
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
