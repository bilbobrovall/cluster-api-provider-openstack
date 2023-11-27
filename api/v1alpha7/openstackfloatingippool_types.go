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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.
const (
	// OpenStackFloatingIPPoolFinalizer allows ReconcileOpenStackFloatingIPPool to clean up resources associated with OpenStackFloatingIPPool before
	// removing it from the apiserver.
	OpenStackFloatingIPPoolFinalizer = "openstackfloatingippool.infrastructure.cluster.x-k8s.io"

	OpenStackFloatingIPPoolNameIndex = "spec.poolRef.name"

	// OpenStackFloatingIPPoolIP
	DeleteFloatingIPFinalizer = "openstackfloatingippool.infrastructure.cluster.x-k8s.io/delete-floating-ip"

	// OpenStackFloatingIPPoolClaimFinalizer allows ReconcileOpenStackFloatingIPPool to clean up resources associated with OpenStackFloatingIPPoolClaim before
	// removing it from the apiserver.
)

// OpenStackFloatingIPPoolSpec defines the desired state of OpenStackFloatingIPPool
type OpenStackFloatingIPPoolSpec struct {
	PreAllocatedFloatingIPs []string `json:"preAllocatedFloatingIPs,omitempty"`

	// IdentityRef is a reference to a identity to be used when reconciling this pool.
	// +optional
	IdentityRef *OpenStackIdentityReference `json:"identityRef,omitempty"`

	// The name of the cloud to use from the clouds secret
	// +optional
	CloudName string `json:"cloudName"`
}

// OpenStackFloatingIPPoolStatus defines the observed state of OpenStackFloatingIPPool
type OpenStackFloatingIPPoolStatus struct {
	// +kubebuilder:default={}
	// +optional
	ClaimedIPs []string `json:"claimedIPs,omitempty"`
	// +kubebuilder:default={}
	// +optional
	AvailableIPs []string `json:"availableIPs,omitempty"`
	// +kubebuilder:default={}
	// +optional
	IPs []string `json:"ips,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// OpenStackFloatingIPPool is the Schema for the openstackfloatingippools API
type OpenStackFloatingIPPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenStackFloatingIPPoolSpec   `json:"spec,omitempty"`
	Status OpenStackFloatingIPPoolStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// OpenStackFloatingIPPoolList contains a list of OpenStackFloatingIPPool
type OpenStackFloatingIPPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenStackFloatingIPPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenStackFloatingIPPool{}, &OpenStackFloatingIPPoolList{})
}
