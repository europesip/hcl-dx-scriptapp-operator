/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// ThemeBase is the DX theme the new theme starts from: its files are copied from the WebDAV store of the same DX and
// its registration is copied with XMLAccess.
type ThemeBase struct {
	// Folder of the base theme in the WebDAV store (default: Portal8.5, the HCL DX Portal 8.5 theme).
	// +kubebuilder:default=Portal8.5
	// +optional
	Folder string `json:"folder,omitempty"`
	// UniqueName of the base theme registration (default: ibm.portal.85Theme).
	// +kubebuilder:default=ibm.portal.85Theme
	// +optional
	UniqueName string `json:"uniqueName,omitempty"`
	// CopyFiles: true (default) copies the files of the base theme first and puts the source on top; false uploads only
	// the files of the source (a complete theme of its own), and the base is used only as the model of the registration.
	// +optional
	CopyFiles *bool `json:"copyFiles,omitempty"`
}

// DXThemeSpec is a DX theme (WebDAV based): the files of a source (a complete theme of its own, or files added to a
// copy of a base theme of the same DX) and optional HTML added to the head of its templates, registered with XMLAccess.
type DXThemeSpec struct {
	// ConnectionRef: the DXConnection to use.
	ConnectionRef LocalObjectReference `json:"connectionRef"`
	// Folder of the theme in the WebDAV store (dav:fs-type1/themes/<folder>/). Default: the name of the resource.
	// +optional
	Folder string `json:"folder,omitempty"`
	// UniqueName of the theme registration (default dxop.<namespace>.<name>). Pages refer to it (DXPage spec.theme).
	// +optional
	UniqueName string `json:"uniqueName,omitempty"`
	// Titles by locale (for example {en: "My theme", es: "Mi tema"}). At least one.
	// +kubebuilder:validation:MinProperties=1
	Titles map[string]string `json:"titles"`
	// Description of the theme.
	// +optional
	Description string `json:"description,omitempty"`
	// Base theme copied first (default: Portal 8.5).
	// +optional
	Base ThemeBase `json:"base,omitempty"`
	// Source: files added to (or replacing in) the copy of the base theme, at the same paths (for example
	// css/my.css or images/logo.png). Optional.
	// +optional
	Source *Source `json:"source,omitempty"`
	// Profile: resource aggregation profile of the theme, relative to its folder (for example
	// profiles/profile_europesip.json). Default: the one of the base theme.
	// +optional
	Profile string `json:"profile,omitempty"`
	// Layout: default layout template of the theme, relative to its folder (for example layout-templates/1Column).
	// Default: the one of the base theme, in the new folder.
	// +optional
	Layout string `json:"layout,omitempty"`
	// HeadHTML: HTML inserted before </head> in every theme template (theme.html, theme_sidenav.html and their language
	// versions): a <style>, a <link> to a file of the source (/wps/mycontenthandler/dav/fs-type1/themes/<folder>/...),
	// a <script>...
	// +optional
	HeadHTML string `json:"headHTML,omitempty"`
	// DriftPolicy: Restore (default) uploads and registers it again if the theme or its folder was deleted in DX;
	// Report only reports it.
	// +kubebuilder:default=Restore
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`
	// DeletionPolicy: Retain (default) leaves the theme in DX when the resource is deleted; Delete removes its
	// registration and its folder (pages that use it fall back to the default theme).
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

// DXThemeStatus is what the operator knows about the theme.
type DXThemeStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// UniqueName of the registered theme.
	// +optional
	UniqueName string `json:"uniqueName,omitempty"`
	// AppliedHash: hash of what was uploaded and registered (also kept as the theme parameter dxop.hash).
	// +optional
	AppliedHash string `json:"appliedHash,omitempty"`
	// SourceRevision of the files that were added.
	// +optional
	SourceRevision string `json:"sourceRevision,omitempty"`
	// Files uploaded to the WebDAV folder.
	// +optional
	Files int `json:"files,omitempty"`
	// +optional
	LastApplyTime *metav1.Time `json:"lastApplyTime,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dxtheme
// +kubebuilder:printcolumn:name="Unique name",type=string,JSONPath=`.status.uniqueName`
// +kubebuilder:printcolumn:name="Files",type=integer,JSONPath=`.status.files`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DXTheme is a theme of HCL DX Compose (WebDAV based), kept in DX as the resource declares.
type DXTheme struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              DXThemeSpec   `json:"spec"`
	Status            DXThemeStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DXThemeList contains a list of DXTheme.
type DXThemeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DXTheme `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DXTheme{}, &DXThemeList{})
		return nil
	})
}
