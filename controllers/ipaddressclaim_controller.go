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
	infrav1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha7"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/cloud/services/networking"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/scope"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"errors"
)

// IPAddressClaimReconciler reconciles a IPAddressClaim object
type IPAddressClaimReconciler struct {
	Client           client.Client
	Recorder         record.EventRecorder
	WatchFilterValue string
	ScopeFactory     scope.Factory
	CaCertificates   []byte // PEM encoded ca certificates.

	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=ipam.cluster.x-k8x.io.cluster.x-k8s.io,resources=ipaddressclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=ipam.cluster.x-k8x.io.cluster.x-k8s.io,resources=ipaddressclaims/status,verbs=get;update;patch

func (r *IPAddressClaimReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Reconciling IPAddressClaim")

	claim := &ipamv1.IPAddressClaim{}
	if err := r.Client.Get(ctx, req.NamespacedName, claim); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if claim.Status.AddressRef.Name == "" {
		// TODO: Should we requeue here? Because It's a status update. backoff? or just requeue?
		return ctrl.Result{}, nil
	}

	openStackMachine, err := r.getOpenStackMachineForClaim(ctx, claim)
	if err != nil {
		return ctrl.Result{}, err
	}

	//TODO? REMOVE
	log.Info("Found OpenStackMachine for IPAddressClaim", "OpenStackMachine", openStackMachine.Name)

	ip := &ipamv1.IPAddress{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: claim.Status.AddressRef.Name}, ip); err != nil {
		return ctrl.Result{}, err
	}

	scope, err := r.ScopeFactory.NewClientScopeFromMachine(ctx, r.Client, openStackMachine, r.CaCertificates, log)
	if err != nil {
		return reconcile.Result{}, err
	}

	networkingService, err := networking.NewService(scope)
	if err != nil {
		return ctrl.Result{}, err
	}

	fip, err := networkingService.GetFloatingIP(ip.Spec.Address)
	if err != nil {
		return ctrl.Result{}, err
	}

	if fip == nil {
		// set conditions etc
		return ctrl.Result{}, errors.New("floating ip not found")
	}

	//computeService, err := compute.NewService(scope)
	//if err != nil {
	//	return ctrl.Result{}, err
	//}

	//instanceStatus, err := computeService.GetInstanceStatusByName(openStackMachine, openStackMachine.Name)
	//if err != nil {
	//	return ctrl.Result{}, err
	//}

	//	port, err := computeService.GetManagementPort(openStackCluster, instanceStatus)
	//	if err != nil {
	//		return ctrl.Result{}, err
	//	}
	//
	//	if err = networkingService.AssociateFloatingIP(openStackMachine, fip, port.ID); err != nil {
	//		return ctrl.Result{}, err
	//	}
	//
	//	log.Info("Successfully associated floating IP to port yolo", "name", openStackMachine.Name) // TODO Remove log
	//
	//	//TODO: Make double sure the finalizer is added
	//	if controllerutil.AddFinalizer(claim, infrav1.IPClaimMachineFinalizer) {
	//		if err := r.Client.Update(ctx, claim); err != nil {
	//			log.Error(err, "failed to add finalizer to IPAddressClaim", "name", claim.Name)
	//		}
	//	}

	// Create the scope.

	return ctrl.Result{}, nil
}

func (r *IPAddressClaimReconciler) getOpenStackMachineForClaim(ctx context.Context, claim *ipamv1.IPAddressClaim) (*infrav1.OpenStackMachine, error) {
	refs := claim.GetOwnerReferences()
	var openStackMachineRef metav1.OwnerReference

	for _, ref := range refs {
		if ref.Kind == "OpenStackMachine" {
			openStackMachineRef = ref
		}
	}

	if openStackMachineRef.Name == "" {
		return nil, errors.New("claim has no owner reference to a OpenStackMachine")
	}

	openStackMachine := &infrav1.OpenStackMachine{}
	if err := r.Client.Get(ctx, client.ObjectKey{Namespace: claim.Namespace, Name: openStackMachineRef.Name}, openStackMachine); err != nil {
		return nil, err
	}
	return openStackMachine, nil
}

func (r *IPAddressClaimReconciler) openStackMachineToIPAddressClaim(ctx context.Context) handler.MapFunc {
	return func(_ context.Context, o client.Object) []reconcile.Request {
		openStackMachine, ok := o.(*infrav1.OpenStackMachine)
		if !ok {
			return nil
		}
		requests := make([]reconcile.Request, len(openStackMachine.Spec.FloatingAddressesFromPools))
		for i, pool := range openStackMachine.Spec.FloatingAddressesFromPools {
			requests[i] = reconcile.Request{
				NamespacedName: client.ObjectKey{
					Namespace: openStackMachine.Namespace,
					Name:      pool.Name,
				},
			}
		}
		return requests
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *IPAddressClaimReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		For(&ipamv1.IPAddressClaim{}).
		Watches(
			&infrav1.OpenStackMachine{},
			handler.EnqueueRequestsFromMapFunc(r.openStackMachineToIPAddressClaim(ctx)),
		).
		Complete(r)
}
