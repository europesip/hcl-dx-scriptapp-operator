/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// PostgresDatabaseSpec is a database and its owner in an existing PostgreSQL server, for the backend of an
// application.
type PostgresDatabaseSpec struct {
	// Host of the PostgreSQL server, for example dx-pg-rw.dx-shared.svc.cluster.local.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// Port (default 5432).
	// +kubebuilder:default=5432
	// +optional
	Port int32 `json:"port,omitempty"`
	// SSLMode: disable, prefer (default), require, verify-ca or verify-full.
	// +kubebuilder:validation:Enum=disable;prefer;require;verify-ca;verify-full
	// +kubebuilder:default=prefer
	// +optional
	SSLMode string `json:"sslMode,omitempty"`
	// SuperuserSecret: Secret with "username" and "password" of a user that can create roles and databases.
	// It may be in another namespace only if the operator is allowed to read it there (superuserSecretNamespace).
	SuperuserSecret LocalObjectReference `json:"superuserSecret"`
	// SuperuserSecretNamespace: namespace of the superuser Secret (default: the namespace of the resource).
	// +optional
	SuperuserSecretNamespace string `json:"superuserSecretNamespace,omitempty"`
	// Database name (default: the name of the resource, with "-" replaced by "_").
	// +optional
	Database string `json:"database,omitempty"`
	// Owner: role that owns the database (default: the database name).
	// +optional
	Owner string `json:"owner,omitempty"`
	// Extensions to create in the database, for example [pgcrypto, vector].
	// +optional
	Extensions []string `json:"extensions,omitempty"`
	// SecretName: Secret the operator writes for the backend, with username, password, host, port, database,
	// uri (postgresql://...) and jdbcUrl. The password is generated once and kept. Default: <name>-db.
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// DeletionPolicy: Retain (default) keeps the database and the role when the resource is deleted; Delete
	// drops them (the data is lost).
	// +kubebuilder:default=Retain
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// PostgresDatabaseStatus is what the operator knows about the database.
type PostgresDatabaseStatus struct {
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// SecretName written for the backend.
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// +optional
	LastCheckTime *metav1.Time `json:"lastCheckTime,omitempty"`
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=pgdb
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.host`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.status.secretName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PostgresDatabase is a database for the backend of an application, created and kept by the operator.
type PostgresDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              PostgresDatabaseSpec   `json:"spec"`
	Status            PostgresDatabaseStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PostgresDatabaseList contains a list of PostgresDatabase.
type PostgresDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PostgresDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &PostgresDatabase{}, &PostgresDatabaseList{})
		return nil
	})
}
