/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

// Types shared by the resources of the operator.

// LocalObjectReference names an object in the same namespace.
type LocalObjectReference struct {
	// Name of the object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// SecretKeySelector selects a key of a Secret in the same namespace.
type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// ConfigMapKeySelector selects a key of a ConfigMap in the same namespace.
type ConfigMapKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// Source says where the files of a Script Application or of a DAM collection come from. Exactly one of
// image, configMap or url must be set.
// +kubebuilder:validation:XValidation:rule="(has(self.image) ? 1 : 0) + (has(self.configMap) ? 1 : 0) + (has(self.url) ? 1 : 0) == 1",message="set exactly one of image, configMap or url"
type Source struct {
	// Image: an OCI image (a "bundle image") that carries the files. The operator reads its layers directly
	// from the registry: nothing runs in the cluster. Use a digest (repo@sha256:...) for immutable deployments.
	// +optional
	Image *ImageSource `json:"image,omitempty"`
	// ConfigMap: every key of the ConfigMap is a file (data and binaryData). Handy for small applications:
	// a ConfigMap and its Secret are limited to 1 MiB. Keys cannot contain "/", so a key "assets__app.js"
	// becomes the file "assets/app.js".
	// +optional
	ConfigMap *ConfigMapSource `json:"configMap,omitempty"`
	// URL: a .tar.gz, .tgz or .zip archive downloaded over HTTP(S), for example a CI artifact or a GitHub
	// release asset.
	// +optional
	URL *URLSource `json:"url,omitempty"`
}

// ImageSource is an OCI image with the files.
type ImageSource struct {
	// Reference of the image, for example ghcr.io/acme/my-app:1.2.0 or ghcr.io/acme/my-app@sha256:...
	// +kubebuilder:validation:MinLength=1
	Ref string `json:"ref"`
	// Path inside the image where the files are (default /app).
	// +kubebuilder:default="/app"
	// +optional
	Path string `json:"path,omitempty"`
	// PullSecrets: Secrets of type kubernetes.io/dockerconfigjson for a private registry.
	// +optional
	PullSecrets []LocalObjectReference `json:"pullSecrets,omitempty"`
}

// ConfigMapSource is a ConfigMap with the files.
type ConfigMapSource struct {
	// Name of the ConfigMap.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// URLSource is an archive downloaded over HTTP(S).
type URLSource struct {
	// URL of a .tar.gz, .tgz or .zip archive.
	// +kubebuilder:validation:Pattern=`^https?://`
	URL string `json:"url"`
	// SHA256 of the archive (hex). Recommended: the download is rejected if it does not match, and the
	// operator only downloads it again when this value changes.
	// +optional
	SHA256 string `json:"sha256,omitempty"`
	// Path inside the archive where the files are (default: the root).
	// +optional
	Path string `json:"path,omitempty"`
}

// Dependency is an object that must be ready before the resource is reconciled.
type Dependency struct {
	// APIVersion of the object, for example apps/v1 (default) or dx.europesip.com/v1alpha1.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Kind: Deployment, StatefulSet, Job, DXScriptApplication, DXPage, DXDamCollection, DXDatabase or DXTheme.
	// +kubebuilder:validation:Enum=Deployment;StatefulSet;Job;DXScriptApplication;DXPage;DXDamCollection;DXDatabase;DXTheme
	Kind string `json:"kind"`
	// Name of the object, in the same namespace.
	Name string `json:"name"`
}

// DriftPolicy says what to do when the state in DX no longer matches the resource.
// +kubebuilder:validation:Enum=Restore;Report
type DriftPolicy string

const (
	// DriftRestore puts DX back as the resource says (the default: Git wins).
	DriftRestore DriftPolicy = "Restore"
	// DriftReport only reports the drift in the status (condition Drifted) and an event.
	DriftReport DriftPolicy = "Report"
)

// DeletionPolicy says what to do in DX when the resource is deleted.
// +kubebuilder:validation:Enum=Retain;Delete
type DeletionPolicy string

const (
	// DeletionRetain leaves the object in DX (the default: pages may still use it).
	DeletionRetain DeletionPolicy = "Retain"
	// DeletionDelete removes the object from DX.
	DeletionDelete DeletionPolicy = "Delete"
)

// Access: who can read a DX object (a page or a DAM collection).
type Access struct {
	// Anonymous: visitors who are not signed in.
	// +optional
	Anonymous bool `json:"anonymous,omitempty"`
	// Authenticated: all authenticated portal users (default true).
	// +kubebuilder:default=true
	// +optional
	Authenticated *bool `json:"authenticated,omitempty"`
}

// Condition types used by every resource.
const (
	// ConditionReady: the resource is in the state it declares.
	ConditionReady = "Ready"
	// ConditionDrifted: the state in DX differs from the resource (only with driftPolicy Report, or while
	// it is being restored).
	ConditionDrifted = "Drifted"
	// ConditionDependenciesReady: every dependency is ready.
	ConditionDependenciesReady = "DependenciesReady"
)

// Annotations understood by the operator.
const (
	// AnnotationReconcile: change its value (for example to a date) to force a full reconciliation now,
	// including a push or an upload even if nothing changed.
	AnnotationReconcile = "dx.europesip.com/reconcile"
)
