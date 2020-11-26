/*


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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

type CACerts struct {
	Cert    string `json:"cert,omitempty"`
	Key     string `json:"key,omitempty"`
	TLSCert string `json:"tlsCert,omitempty"`
	TLSKey  string `json:"tlsKey,omitempty"`
}

// CASpec defines the desired state of CA
type CASpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of CA. Edit CA_types.go to remove/update
	Admin         string   `json:"admin"`
	AdminPassword string   `json:"adminPassword"`
	Certs         *CACerts `json:"certs,omitempty"`
	Config        string   `json:"config,required"`
	NodeSpec      `json:"nodeSpec,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CA is the Schema for the cas API
type CA struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   CASpec     `json:"spec,omitempty"`
	Status NodeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// CAList contains a list of CA
type CAList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CA `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CA{}, &CAList{})
}
