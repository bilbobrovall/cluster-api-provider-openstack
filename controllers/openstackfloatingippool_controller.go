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
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/cloud/services/networking"
	"sigs.k8s.io/cluster-api-provider-openstack/pkg/scope"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	ipamv1 "sigs.k8s.io/cluster-api/exp/ipam/api/v1alpha1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrav1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha7"
)

const (
	openStackFloatingIPPool = "OpenStackFloatingIPPool"
)

// OpenStackFloatingIPPoolReconciler reconciles a OpenStackFloatingIPPool object
type OpenStackFloatingIPPoolReconciler struct {
	Client           client.Client
	Recorder         record.EventRecorder
	WatchFilterValue string
	ScopeFactory     scope.Factory
	CaCertificates   []byte // PEM encoded ca certificates.

	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=openstackfloatingippools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=openstackfloatingippools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddressclaims;ipaddressclaims/status,verbs=get;list;watch;update;create;delete
// +kubebuilder:rbac:groups=ipam.cluster.x-k8s.io,resources=ipaddresses;ipaddresses/status,verbs=get;list;watch;create;update;delete

// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.6.4/pkg/reconcile
func (r *OpenStackFloatingIPPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	_ = context.Background()
	log := ctrl.LoggerFrom(ctx)

	pool := &infrav1.OpenStackFloatingIPPool{}
	if err := r.Client.Get(context.Background(), req.NamespacedName, pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if pool.ObjectMeta.DeletionTimestamp.IsZero() {
		// Add finalizer if it does not exist
		if controllerutil.AddFinalizer(pool, infrav1.OpenStackFloatingIPPoolFinalizer) {
			return ctrl.Result{Requeue: true}, r.Client.Update(context.Background(), pool)
		}
	} else {
		// Handle deletion
		return r.reconcileDelete(ctx, pool)
	}

	if err := r.setIPStatuses(ctx, pool); err != nil {
		return ctrl.Result{}, err
	}

	patchHelper, err := patch.NewHelper(pool, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	defer func() {
		if err := patchHelper.Patch(ctx, pool); err != nil {
			if reterr == nil {
				reterr = fmt.Errorf("error patching OpenStackFloatingIPPool %s/%s: %w", pool.Namespace, pool.Name, err)
			}
		}
	}()

	claims := &ipamv1.IPAddressClaimList{}
	if err := r.Client.List(context.Background(), claims, client.InNamespace(req.Namespace), client.MatchingFields{infrav1.OpenStackFloatingIPPoolNameIndex: pool.Name}); err != nil {
		return ctrl.Result{}, err
	}

	for _, claim := range claims.Items {

		cluster, err := util.GetClusterFromMetadata(ctx, r.Client, claim.ObjectMeta)
		if err != nil {
			log.Info("IPAddressClaim is missing cluster label or cluster does not exist")
			return ctrl.Result{}, nil
		}

		log = log.WithValues("claim", claim.Name)

		infraCluster, err := r.getInfraCluster(ctx, cluster, &claim)
		if err != nil {
			return ctrl.Result{}, errors.New("error getting infra provider cluster")
		}
		if infraCluster == nil {
			log.Info("OpenStackCluster is not ready yet")
			return ctrl.Result{}, nil
		}

		if claim.Status.AddressRef.Name == "" {
			clusterName := fmt.Sprintf("%s-%s", claim.ObjectMeta.Labels[clusterv1.ClusterNameLabel], claim.Namespace)
			ip, err := r.getIP(ctx, pool, infraCluster, clusterName, log)
			if err != nil {
				return ctrl.Result{}, err
			}
			// TODO: handle status when error between getting ip and assigning it to claim

			// Create IP address
			// TODO: make shit nicer
			ipaddress := &ipamv1.IPAddress{
				ObjectMeta: ctrl.ObjectMeta{
					Name:      claim.Name,
					Namespace: claim.Namespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: claim.APIVersion,
							Kind:       claim.Kind,
							Name:       claim.Name,
							UID:        claim.UID,
						},
					},
				},
				Spec: ipamv1.IPAddressSpec{
					ClaimRef: corev1.LocalObjectReference{
						Name: claim.Name,
					},
					PoolRef: corev1.TypedLocalObjectReference{
						APIGroup: pointer.String(infrav1.GroupVersion.Group),
						Kind:     pool.Kind,
						Name:     pool.Name,
					},
					Address: ip,
				},
			}

			if err = r.Client.Create(ctx, ipaddress); err != nil {
				return ctrl.Result{}, err
			}

			claim.Status.AddressRef.Name = ipaddress.Name
			if err = r.Client.Status().Update(ctx, &claim); err != nil {
				log.Error(err, "Failed to update IPAddressClaim status", "claim", claim.Name, "ip", ip)
				return ctrl.Result{}, err
			}
			log.Info("IPAddressClaim status updated")
		}
	}
	err = r.setIPStatuses(ctx, pool)
	if err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *OpenStackFloatingIPPoolReconciler) reconcileDelete(ctx context.Context, pool *infrav1.OpenStackFloatingIPPool) (ctrl.Result, error) {
	// If the OpenStackMachine doesn't have our finalizer, add it.
	log := ctrl.LoggerFrom(ctx)
	if controllerutil.RemoveFinalizer(pool, infrav1.OpenStackFloatingIPPoolFinalizer) {
		// Delete all IPAdress ? :thinking: They've should have a finalizer from the OpenStackMachine
		log.Info("Removing finalizer from OpenStackFloatingIPPool")
		return ctrl.Result{Requeue: true}, r.Client.Update(context.Background(), pool)
	}
	return ctrl.Result{}, nil
}

func (r *OpenStackFloatingIPPoolReconciler) reconcileClaim(ctx context.Context, claim *ipamv1.IPAddressClaim) error {
	return nil
}

func (r *OpenStackFloatingIPPoolReconciler) setIPStatuses(ctx context.Context, pool *infrav1.OpenStackFloatingIPPool) error {
	ips := &ipamv1.IPAddressList{}
	if err := r.Client.List(ctx, ips, client.InNamespace(pool.Namespace), client.MatchingFields{infrav1.OpenStackFloatingIPPoolNameIndex: pool.Name}); err != nil {
		return err
	}
	pool.Status.AvailableIPs = []string{}
	pool.Status.ClaimedIPs = []string{}

	for _, ip := range ips.Items {
		pool.Status.ClaimedIPs = append(pool.Status.ClaimedIPs, ip.Spec.Address)
	}

	for _, ip := range pool.Spec.PreAllocatedFloatingIPs {
		if !contains(pool.Status.IPs, ip) {
			pool.Status.IPs = append(pool.Status.IPs, ip)
		}
	}

	for _, ip := range pool.Status.IPs {
		if !contains(pool.Status.ClaimedIPs, ip) {
			pool.Status.AvailableIPs = append(pool.Status.AvailableIPs, ip)
		}
	}
	return nil
}

func (r *OpenStackFloatingIPPoolReconciler) getIP(ctx context.Context, pool *infrav1.OpenStackFloatingIPPool, openStackCluster *infrav1.OpenStackCluster, clusterName string, logger logr.Logger) (string, error) {
	log := ctrl.LoggerFrom(ctx)

	if len(pool.Status.AvailableIPs) > 0 {
		ip := pool.Status.AvailableIPs[0]
		pool.Status.AvailableIPs = pool.Status.AvailableIPs[1:]
		pool.Status.ClaimedIPs = append(pool.Status.ClaimedIPs, ip)
		if err := r.Client.Status().Update(ctx, pool); err != nil {
			return "", err
		}
		return ip, nil
	}

	scope, err := r.ScopeFactory.NewClientScopeFromCluster(ctx, r.Client, openStackCluster, r.CaCertificates, log)
	if err != nil {
		return "", err
	}

	networkingService, err := networking.NewService(scope)
	if err != nil {
		log.Error(err, "Failed to create networking service") // TODO Remove log
		return "", err
	}

	fp, err := networkingService.GetOrCreateFloatingIP(pool, openStackCluster, clusterName, "")
	if err != nil {
		//conditions.MarkFalse(openStackMachine, infrav1.APIServerIngressReadyCondition, infrav1.FloatingIPErrorReason, clusterv1.ConditionSeverityError, "Floating IP cannot be obtained or created: %v", err)
		return "", fmt.Errorf("get or create floating IP: %w", err)
	}
	ip := fp.FloatingIP

	// TODO: setStatus should probably solve this stuff
	pool.Status.ClaimedIPs = append(pool.Status.ClaimedIPs, ip)
	pool.Status.IPs = append(pool.Status.IPs, ip)
	if err := r.Client.Status().Update(ctx, pool); err != nil {
		log.Error(err, "Failed to update OpenStackFloatingIPPool status", "pool", pool.Name, "ip", ip)
		return "", err
	}
	return ip, nil
}

func (r *OpenStackFloatingIPPoolReconciler) getInfraCluster(ctx context.Context, cluster *clusterv1.Cluster, claim *ipamv1.IPAddressClaim) (*infrav1.OpenStackCluster, error) {
	openStackCluster := &infrav1.OpenStackCluster{}
	openStackClusterName := client.ObjectKey{
		Namespace: claim.Namespace,
		Name:      cluster.Spec.InfrastructureRef.Name,
	}
	if err := r.Client.Get(ctx, openStackClusterName, openStackCluster); err != nil {
		return nil, err
	}
	return openStackCluster, nil
}

// TODO: combine theese ///////////////////////////////////////////////////////////////////////////////////////////////

func (r *OpenStackFloatingIPPoolReconciler) iPAddressClaimToPoolMapper(ctx context.Context, o client.Object) []ctrl.Request {
	claim, ok := o.(*ipamv1.IPAddressClaim)
	if !ok {
		panic(fmt.Sprintf("Expected a IPAddressClaim but got a %T", o))
	}

	if claim.Spec.PoolRef.Kind != openStackFloatingIPPool {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      claim.Spec.PoolRef.Name,
				Namespace: claim.Namespace,
			},
		},
	}
}

func (r *OpenStackFloatingIPPoolReconciler) ipAddressToPoolMapper(ctx context.Context, o client.Object) []ctrl.Request {
	ip, ok := o.(*ipamv1.IPAddress)
	if !ok {
		panic(fmt.Sprintf("Expected a IPAddress but got a %T", o))
	}

	if ip.Spec.PoolRef.Kind != openStackFloatingIPPool {
		return nil
	}

	return []ctrl.Request{
		{
			NamespacedName: client.ObjectKey{
				Name:      ip.Spec.PoolRef.Name,
				Namespace: ip.Namespace,
			},
		},
	}
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

// SetupWithManager sets up the controller with the Manager.
func (r *OpenStackFloatingIPPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// setup index for pool name
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ipamv1.IPAddressClaim{}, infrav1.OpenStackFloatingIPPoolNameIndex, func(rawObj client.Object) []string {
		claim := rawObj.(*ipamv1.IPAddressClaim)
		return []string{claim.Spec.PoolRef.Name}
	}); err != nil {
		return err
	}

	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &ipamv1.IPAddress{}, infrav1.OpenStackFloatingIPPoolNameIndex, func(rawObj client.Object) []string {
		ip := rawObj.(*ipamv1.IPAddress)
		return []string{ip.Spec.PoolRef.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.OpenStackFloatingIPPool{}).
		Watches(
			&ipamv1.IPAddressClaim{},
			handler.EnqueueRequestsFromMapFunc(r.iPAddressClaimToPoolMapper),
		).
		WithOptions(controller.Options{
			// To avoid race conditions when allocating IP Addresses, we explicitly set this to 1
			MaxConcurrentReconciles: 1,
		}).
		Watches(
			&ipamv1.IPAddress{},
			handler.EnqueueRequestsFromMapFunc(r.ipAddressToPoolMapper),
		).
		Complete(r)
}
