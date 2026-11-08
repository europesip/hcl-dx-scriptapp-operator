/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DXScriptApplicationMode is how the application reaches DX.
// +kubebuilder:validation:Enum=Push;Verify
type DXScriptApplicationMode string

const (
	// ModePush: the operator pushes the application with DXClient (create or update) and keeps it as declared.
	ModePush DXScriptApplicationMode = "Push"
	// ModeVerify: the operator never pushes. It only checks that the application exists in DX (for example in
	// an environment that receives content by syndication) and reports it.
	ModeVerify DXScriptApplicationMode = "Verify"
)

// WCMSettings are the settings of the Script Application in WCM.
type WCMSettings struct {
	// Name of the WCM content: the key that makes a push an update. Default: the name of the resource.
	// Do not change it after the first push (a new name creates a second application).
	// +optional
	Name string `json:"name,omitempty"`
	// Title shown in DX (default: the WCM name).
	// +optional
	Title string `json:"title,omitempty"`
	// SiteArea where it is created (default "Script Application Library/Script Applications/").
	// +kubebuilder:default="Script Application Library/Script Applications/"
	// +optional
	SiteArea string `json:"siteArea,omitempty"`
	// MainHTMLFile of the application (default index.html).
	// +kubebuilder:default="index.html"
	// +optional
	MainHTMLFile string `json:"mainHtmlFile,omitempty"`
	// ProjectContext: push into this WCM project instead of publishing at once (the project must exist).
	// +optional
	ProjectContext string `json:"projectContext,omitempty"`
}

// Substitution replaces ${NAME} in the text files of the application (.html, .js, .mjs, .css, .json, .txt,
// .svg) before the push. It lets the same build run in every environment: URLs of the backend, of DAM
// assets, feature flags...
type Substitution struct {
	// Name of the placeholder: ${NAME} in the files.
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	Name string `json:"name"`
	// Value, as is.
	// +optional
	Value string `json:"value,omitempty"`
	// ValueFrom: a key of a ConfigMap or a Secret, or the URL of an asset of a DXDamCollection (which then
	// becomes a dependency).
	// +optional
	ValueFrom *SubstitutionSource `json:"valueFrom,omitempty"`
}

// SubstitutionSource is where the value of a substitution comes from (one of them).
type SubstitutionSource struct {
	// +optional
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`
	// SecretKeyRef: remember that the value ends up in a file every visitor can download.
	// +optional
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
	// DAMAsset: the URL of an asset of a DXDamCollection in the same namespace.
	// +optional
	DAMAsset *DAMAssetSelector `json:"damAsset,omitempty"`
}

// DAMAssetSelector selects an asset of a DXDamCollection.
type DAMAssetSelector struct {
	// Collection: name of the DXDamCollection resource.
	Collection string `json:"collection"`
	// Asset: file name of the asset in the collection.
	Asset string `json:"asset"`
}

// DXScriptApplicationSpec is a Script Application as it must be in DX.
type DXScriptApplicationSpec struct {
	// ConnectionRef: the DXConnection to use.
	ConnectionRef LocalObjectReference `json:"connectionRef"`
	// Source of the built application (the content of dist/).
	Source Source `json:"source"`
	// WCM settings.
	// +optional
	WCM WCMSettings `json:"wcm,omitempty"`
	// Substitutions of ${NAME} placeholders in the text files.
	// +optional
	Substitutions []Substitution `json:"substitutions,omitempty"`
	// Mode: Push (default) or Verify.
	// +kubebuilder:default=Push
	// +optional
	Mode DXScriptApplicationMode `json:"mode,omitempty"`
	// DriftPolicy: Restore (default) pushes again if the application was deleted or edited in DX; Report only
	// reports it.
	// +kubebuilder:default=Restore
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`
	// DeletionPolicy: Retain (default) leaves the application in DX when the resource is deleted; Delete
	// removes it (pages that use it then show an error).
	// +kubebuilder:default=Retain
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
	// DependsOn: objects that must be ready first (for example the Deployment of the backend).
	// +optional
	DependsOn []Dependency `json:"dependsOn,omitempty"`
	// Suspend: stop reconciling (nothing is pushed or checked).
	// +optional
	Suspend bool `json:"suspend,omitempty"`
}

// DXScriptApplicationStatus is what the operator knows about the application in DX.
type DXScriptApplicationStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ContentID of the application in WCM.
	// +optional
	ContentID string `json:"contentId,omitempty"`
	// SourceRevision: digest of the source that was pushed (image digest, ConfigMap hash or archive SHA256).
	// +optional
	SourceRevision string `json:"sourceRevision,omitempty"`
	// AppliedHash: hash of everything pushed (files after substitutions and WCM settings).
	// +optional
	AppliedHash string `json:"appliedHash,omitempty"`
	// WCMLastModified: last modification date of the content in WCM after the operator's push. A different
	// date means somebody changed it in DX.
	// +optional
	WCMLastModified string `json:"wcmLastModified,omitempty"`
	// Files and Size of the last push.
	// +optional
	Files int `json:"files,omitempty"`
	// +optional
	Size string `json:"size,omitempty"`
	// LastPushTime and LastCheckTime.
	// +optional
	LastPushTime *metav1.Time `json:"lastPushTime,omitempty"`
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// History of the last pushes (newest first, at most 10): to go back, set the source of one of them.
	// +optional
	History []PushRecord `json:"history,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// PushRecord is one push of the application.
type PushRecord struct {
	Time           metav1.Time `json:"time"`
	SourceRevision string      `json:"sourceRevision"`
	Reason         string      `json:"reason"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dxapp;dxsa
// +kubebuilder:printcolumn:name="WCM name",type=string,JSONPath=`.spec.wcm.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Content ID",type=string,JSONPath=`.status.contentId`,priority=1
// +kubebuilder:printcolumn:name="Last push",type=date,JSONPath=`.status.lastPushTime`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DXScriptApplication is an HCL DX Script Application, kept in DX as the resource declares.
type DXScriptApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              DXScriptApplicationSpec   `json:"spec"`
	Status            DXScriptApplicationStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DXScriptApplicationList contains a list of DXScriptApplication.
type DXScriptApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DXScriptApplication `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DXScriptApplication{}, &DXScriptApplicationList{})
		return nil
	})
}
