// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SecretGVR identifies core/v1 Secret resources for dynamic Kubernetes queries.
var SecretGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "secrets",
}

// ResourceLister lists unstructured Kubernetes resources for cluster and secret discovery.
type ResourceLister interface {
	ListResources(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error)
}

type dockerConfigJSON struct {
	Auths map[string]dockerConfigEntry `json:"auths"`
}

type dockerConfigEntry struct {
	Username      string `json:"username"`
	Password      string `json:"password"`
	Auth          string `json:"auth"`
	IdentityToken string `json:"identitytoken"`
	RegistryToken string `json:"registrytoken"`
}
