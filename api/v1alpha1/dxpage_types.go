/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DXPageSpec is a DX page that shows one or more Script Applications.
type DXPageSpec struct {
	// ConnectionRef: the DXConnection to use (its user must be a DX administrator: pages use XMLAccess).
	ConnectionRef LocalObjectReference `json:"connectionRef"`
	// UniqueName of the page in DX: the key that makes an update find the same page.
	// Default: dxop.<namespace>.<name>. Do not change it after the page is created.
	// +optional
	UniqueName string `json:"uniqueName,omitempty"`
	// Parent: unique name of the parent page (default ibm.portal.Home).
	// +kubebuilder:default="ibm.portal.Home"
	// +optional
	Parent string `json:"parent,omitempty"`
	// FriendlyName of the page in its URL (default: the name of the resource).
	// +optional
	FriendlyName string `json:"friendlyName,omitempty"`
	// Titles by locale, for example {en: "Time off", es: "Vacaciones"}.
	// +kubebuilder:validation:MinProperties=1
	Titles map[string]string `json:"titles"`
	// Applications shown on the page, one under the other: names of ScriptApplication resources in the same
	// namespace (each one in a Web Content Viewer). They become dependencies of the page.
	// +kubebuilder:validation:MinItems=1
	Applications []LocalObjectReference `json:"applications"`
	// Ordinal of the page among its siblings (optional).
	// +optional
	Ordinal *int32 `json:"ordinal,omitempty"`
	// LayoutTemplate of the theme (default: the 1 column layout of the Portal 8.5 theme).
	// +kubebuilder:default="dav:fs-type1/themes/Portal8.5/layout-templates/1Column"
	// +optional
	LayoutTemplate string `json:"layoutTemplate,omitempty"`
	// Skin of the viewers (default ibm.portal.85NoSkin).
	// +kubebuilder:default="ibm.portal.85NoSkin"
	// +optional
	Skin string `json:"skin,omitempty"`
	// Theme: unique name of a theme for this page (default: inherited).
	// +optional
	Theme string `json:"theme,omitempty"`
	// Access: who can see the page (default: all authenticated users).
	// +optional
	Access Access `json:"access,omitempty"`
	// DriftPolicy: Restore (default) recreates the page if it was deleted in DX; Report only reports it.
	// +kubebuilder:default=Restore
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`
	// DeletionPolicy: Retain (default) leaves the page in DX when the resource is deleted; Delete removes it.
	// +kubebuilder:default=Retain
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
	// DependsOn: other objects that must be ready first.
	// +optional
	DependsOn []Dependency `json:"dependsOn,omitempty"`
	// Suspend: stop reconciling.
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// DXPageStatus is what the operator knows about the page.
type DXPageStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// UniqueName of the page in DX.
	// +optional
	UniqueName string `json:"uniqueName,omitempty"`
	// URL of the page (for authenticated users), when the parent is the Home page.
	// +optional
	URL string `json:"url,omitempty"`
	// AppliedHash: hash of the settings applied with XMLAccess.
	// +optional
	AppliedHash string `json:"appliedHash,omitempty"`
	// LayoutHash: hash of the list of applications the layout was built for. The layout (the viewers) is only
	// rebuilt when that list changes, so the settings people make on the page survive other updates.
	// +optional
	LayoutHash string `json:"layoutHash,omitempty"`
	// +optional
	LastApplyTime *metav1.Time `json:"lastApplyTime,omitempty"`
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dxp
// +kubebuilder:printcolumn:name="Unique name",type=string,JSONPath=`.status.uniqueName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`,priority=1
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DXPage is a page of HCL DX that shows Script Applications, kept in DX as the resource declares.
type DXPage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              DXPageSpec   `json:"spec"`
	Status            DXPageStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DXPageList contains a list of DXPage.
type DXPageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DXPage `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DXPage{}, &DXPageList{})
		return nil
	})
}
