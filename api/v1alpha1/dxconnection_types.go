/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DXConnectionSpec is how the operator reaches an HCL DX server.
type DXConnectionSpec struct {
	// URL of DX as the operator reaches it, for example https://dx.example.com (normally the public host).
	// +kubebuilder:validation:Pattern=`^https?://[^/]+/?$`
	URL string `json:"url"`
	// CredentialsSecret: Secret with the keys "username" and "password" of a DX user. Script Applications
	// need Editor access to the Script Application Library; pages (XMLAccess) and DAM need a DX administrator.
	CredentialsSecret LocalObjectReference `json:"credentialsSecret"`
	// CASecret: Secret with the key "ca.crt", for a self-signed certificate or a private CA.
	// +optional
	CASecret *LocalObjectReference `json:"caSecret,omitempty"`
	// InsecureSkipTLSVerify: do not verify the certificate at all (labs only).
	// +optional
	InsecureSkipTLSVerify bool `json:"insecureSkipTLSVerify,omitempty"`
	// ContentHandlerPath of DX (default /wps/mycontenthandler).
	// +kubebuilder:default="/wps/mycontenthandler"
	// +optional
	ContentHandlerPath string `json:"contentHandlerPath,omitempty"`
	// VirtualPortalContext: push Script Applications into this virtual portal.
	// +optional
	VirtualPortalContext string `json:"virtualPortalContext,omitempty"`
	// ResyncInterval: how often every resource that uses this connection is checked against DX, to detect
	// and correct drift (default 10m, minimum 1m).
	// +kubebuilder:default="10m"
	// +optional
	ResyncInterval *metav1.Duration `json:"resyncInterval,omitempty"`
}

// DXConnectionStatus is the last check of the connection.
type DXConnectionStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// LastCheckTime of the connection.
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dxconn
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.spec.url`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Message",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DXConnection is a connection to an HCL DX server, shared by the resources of a namespace.
type DXConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              DXConnectionSpec   `json:"spec"`
	Status            DXConnectionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DXConnectionList contains a list of DXConnection.
type DXConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DXConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DXConnection{}, &DXConnectionList{})
		return nil
	})
}
