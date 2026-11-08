/*
Copyright 2026 EUROPESIP.

Licensed under the MIT License (see LICENSE).
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// DatabaseEngine is the database server type.
// +kubebuilder:validation:Enum=PostgreSQL;Oracle;SQLServer
type DatabaseEngine string

const (
	EnginePostgreSQL DatabaseEngine = "PostgreSQL"
	EngineOracle     DatabaseEngine = "Oracle"
	EngineSQLServer  DatabaseEngine = "SQLServer"
)

// OracleOptions are the settings that only apply to Oracle.
type OracleOptions struct {
	// Service name of the pluggable database to connect to, for example FREEPDB1 or ORCLPDB1.
	// +kubebuilder:validation:MinLength=1
	Service string `json:"service"`
	// Tablespace for the quota of the user (default USERS).
	// +kubebuilder:default=USERS
	// +optional
	Tablespace string `json:"tablespace,omitempty"`
}

// SQLServerOptions are the settings that only apply to Microsoft SQL Server.
type SQLServerOptions struct {
	// Encrypt (TLS of the connection): disable, optional, mandatory (default) or strict (TDS 8).
	// +kubebuilder:validation:Enum=disable;optional;mandatory;strict
	// +kubebuilder:default=mandatory
	// +optional
	Encrypt string `json:"encrypt,omitempty"`
	// TrustServerCertificate: accept the certificate of the server without checking it (self-signed servers in labs).
	// +optional
	TrustServerCertificate bool `json:"trustServerCertificate,omitempty"`
}

// DXDatabaseSpec is a database (and the user that owns it) in an existing database server, for the backend of an
// application. The operator creates them and writes a Secret with the connection details; the password is generated
// once and kept.
type DXDatabaseSpec struct {
	// Engine of the server: PostgreSQL (default), Oracle or SQLServer.
	// +kubebuilder:default=PostgreSQL
	// +optional
	Engine DatabaseEngine `json:"engine,omitempty"`
	// Host of the database server, for example dx-pg-rw.dx-shared.svc.cluster.local.
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	// Port (default: 5432 PostgreSQL, 1521 Oracle, 1433 SQL Server).
	// +optional
	Port int32 `json:"port,omitempty"`
	// AdminSecret: Secret with "username" and "password" of an administrator that can create users and databases
	// (PostgreSQL: a superuser or CREATEROLE + CREATEDB; Oracle: CREATE USER and GRANT; SQL Server: sysadmin or
	// dbcreator + securityadmin). In another namespace only if the operator may read it there (adminSecretNamespace).
	AdminSecret LocalObjectReference `json:"adminSecret"`
	// AdminSecretNamespace: namespace of the admin Secret (default: the namespace of the resource).
	// +optional
	AdminSecretNamespace string `json:"adminSecretNamespace,omitempty"`
	// Database name (default: the name of the resource, with "-" replaced by "_"). With Oracle it is the schema: a
	// user with that name.
	// +optional
	Database string `json:"database,omitempty"`
	// Owner: the user that owns the database and that the backend signs in with (default: the database name). With
	// Oracle it is always the database (schema) name.
	// +optional
	Owner string `json:"owner,omitempty"`
	// SSLMode (PostgreSQL only): disable, prefer (default), require, verify-ca or verify-full.
	// +kubebuilder:validation:Enum=disable;prefer;require;verify-ca;verify-full
	// +optional
	SSLMode string `json:"sslMode,omitempty"`
	// Extensions (PostgreSQL only) to create in the database, for example [pgcrypto, vector].
	// +optional
	Extensions []string `json:"extensions,omitempty"`
	// Oracle settings (required with engine Oracle).
	// +optional
	Oracle *OracleOptions `json:"oracle,omitempty"`
	// SQLServer settings (engine SQLServer).
	// +optional
	SQLServer *SQLServerOptions `json:"sqlServer,omitempty"`
	// SecretName: Secret the operator writes for the backend, with engine, username, password, host, port, database,
	// uri and jdbcUrl. Default: <name>-db.
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// DeletionPolicy: Retain (default) keeps the database and the user when the resource is deleted; Delete drops
	// them (the data is lost).
	// +kubebuilder:default=Retain
	// +optional
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// DXDatabaseStatus is what the operator knows about the database.
type DXDatabaseStatus struct {
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
// +kubebuilder:resource:shortName=dxdb
// +kubebuilder:printcolumn:name="Engine",type=string,JSONPath=`.spec.engine`
// +kubebuilder:printcolumn:name="Host",type=string,JSONPath=`.spec.host`
// +kubebuilder:printcolumn:name="Secret",type=string,JSONPath=`.status.secretName`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// DXDatabase is a database for the backend of an application (PostgreSQL, Oracle or SQL Server), created and kept by
// the operator.
type DXDatabase struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitzero"`
	Spec              DXDatabaseSpec   `json:"spec"`
	Status            DXDatabaseStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// DXDatabaseList contains a list of DXDatabase.
type DXDatabaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []DXDatabase `json:"items"`
}

func init() {
	SchemeBuilder.Register(func(s *runtime.Scheme) error {
		s.AddKnownTypes(SchemeGroupVersion, &DXDatabase{}, &DXDatabaseList{})
		return nil
	})
}
