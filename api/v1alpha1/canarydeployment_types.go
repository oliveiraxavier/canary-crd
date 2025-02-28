/*
Copyright 2025.

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

type CanaryDeploymentSpec struct {
	// +kubebuilder:validation:MinLength=1
	AppName string `json:"appName"`

	// +kubebuilder:validation:MinLength=1

	Stable string `json:"stable"`

	// +kubebuilder:validation:MinLength=1
	Canary string `json:"canary"`

	// +kubebuilder:validation:listType=map
	// +kubebuilder:validation:uniqueItems=true
	// +kubebuilder:validation:items={"$ref":"#/definitions/Step"}
	// +kubebuilder:validation:default=[{"setWeight":20,"pause":[{"minutes":120}]}]
	// +kubebuilder:validation:required
	Steps []Step `json:"steps"`
}
type Step struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	SetWeight int32 `json:"setWeight"`

	Pause Pause `json:"pause,omitempty"`
}

type Pause struct {
	Seconds int32 `json:"seconds,omitempty"`
	Minutes int32 `json:"minutes,omitempty"`
	Hours   int32 `json:"hours,omitempty"`
}

// +kubebuilder:object:root=true
// CanaryDeployment is the Schema for the canarydeployments API.
type CanaryDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +kubebuilder:default=0
	ActualStep int32                `json:"actualStep,omitempty"`
	Spec       CanaryDeploymentSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true
// CanaryDeploymentList contains a list of CanaryDeployment.
type CanaryDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []CanaryDeployment `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CanaryDeployment{}, &CanaryDeploymentList{})
}
