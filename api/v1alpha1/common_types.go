// NOTE: Boilerplate only.  Ignore this file.

// Package v1alpha1 contains API Schema definitions for the fabric v1alpha1 API group
// +k8s:deepcopy-gen=package,register
// +groupName=fabric.hyperledger.org
package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// The corresponding msp structure for node such as orderer or peer
type MSP struct {
	// Administrator's certificates
	AdminCerts []string `json:"adminCerts,omitempty"`
	// CA certificates
	CaCerts []string `json:"caCerts,required"`
	// Node private key
	KeyStore string `json:"keyStore,required"`
	// Node certificate
	SignCerts string `json:"signCerts,required"`
	// ca tls certificates
	TLSCacerts []string `json:"tlsCacerts,omitempty"`
	ConfigFile string   `json:"configFile,required"`
}

type TLS struct {
	// Node tls certificate
	TLSCert string `json:"tlsCert,required"`
	// Node tls private key
	TLSKey string `json:"tlsKey,required"`
	// trusted root certificate
	TLSCaCert string `json:"tlsCaCert,required"`
}

type NodeSpec struct {
	Image        string                      `json:"image"`
	ConfigParams []ConfigParam               `json:"configParams"`
	Ports        []int                       `json:"ports,omitempty"`
	Resources    corev1.ResourceRequirements `json:"resources,omitempty"`
	StorageClass string                      `json:"storageClass,omitempty"`
	StorageSize  string                      `json:"storageSize,omitempty"`
}

type NodeStatus struct {
	AccessPoint  string `json:"accessPoint"`
	ExternalPort int    `json:"externalPort"`
}

type CommonSpec struct {
	MSP      `json:"msp"`
	TLS      `json:"tls"`
	NodeSpec `json:"nodeSpec,omitempty"`
}

type ConfigParam struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}
