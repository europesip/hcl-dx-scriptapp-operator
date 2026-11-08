/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DXDamCollectionSpec is a collection of the DX Digital Asset Management with the files of a source.
type DXDamCollectionSpec struct {
	// ConnectionRef: the DXConnection to use.
	ConnectionRef LocalObjectReference `json:"connectionRef"`
	// CollectionName in the DAM (default: the name of the resource). The key that finds the same collection.
	// +optional
	CollectionName string `json:"collectionName,omitempty"`
	// Description of the collection.
	// +optional
	Description string `json:"description,omitempty"`
	// Source of the files: every file becomes an asset, named after its path (subfolders flattened with "-").
	Source Source `json:"source"`
	// Access: who can read the assets (default: all authenticated users; anonymous for public images).
	// +optional
	Access Access `json:"access,omitempty"`
	// Prune: delete the assets of the collection that are no longer in the source (default false).
	// +optional
	Prune bool `json:"prune,omitempty"`
	// DriftPolicy: Restore (default) uploads again what was deleted or replaced in DX; Report only reports it.
	// +kubebuilder:default=Restore
	// +optional
	DriftPolicy DriftPolicy `json:"driftPolicy,omitempty"`
	// DeletionPolicy: Retain (default) leaves the collection in the DAM when the resource is deleted; Delete
	// removes it with its assets.
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

// DAMAssetStatus is one asset of the collection.
type DAMAssetStatus struct {
	// Name of the asset (the file name).
	Name string `json:"name"`
	// ID of the asset in the DAM.
	ID string `json:"id"`
	// URL of its original rendition, relative to the DX host (usable in pages and Script Applications).
	// +optional
	URL string `json:"url,omitempty"`
	// SHA256 of the uploaded file.
	SHA256 string `json:"sha256"`
}

// DXDamCollectionStatus is what the operator knows about the collection.
type DXDamCollectionStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// CollectionID in the DAM.
	// +optional
	CollectionID string `json:"collectionId,omitempty"`
	// SourceRevision that was uploaded.
	// +optional
	SourceRevision string `json:"sourceRevision,omitempty"`
	// Assets of the collection managed by the operator.
	// +optional
	Assets []DAMAssetStatus `json:"assets,omitempty"`
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dxdam
// +kubebuilder:printcolumn:name="Collection",type=string,JSONPath=`.spec.collectionName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DXDamCollection is a collection of the HCL DX Digital Asset Management, kept as the resource declares.
type DXDamCollection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              DXDamCollectionSpec   `json:"spec"`
	Status            DXDamCollectionStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DXDamCollectionList contains a list of DXDamCollection.
type DXDamCollectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DXDamCollection `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DXDamCollection{}, &DXDamCollectionList{})
		return nil
	})
}
