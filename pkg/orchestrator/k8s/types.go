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
	"github.com/google/go-containerregistry/pkg/authn"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// NodeGVR identifies core/v1 Node resources for dynamic Kubernetes queries.
var NodeGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "nodes",
}

// SecretGVR identifies core/v1 Secret resources for dynamic Kubernetes queries.
var SecretGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "secrets",
}

// ServiceAccountGVR identifies core/v1 ServiceAccount resources for dynamic Kubernetes queries.
var ServiceAccountGVR = schema.GroupVersionResource{
	Group:    "",
	Version:  "v1",
	Resource: "serviceaccounts",
}

// ResourceLister lists unstructured Kubernetes resources for cluster and secret discovery.
type ResourceLister interface {
	ListResources(gvr schema.GroupVersionResource, namespace, labelSelector, fieldSelector string) ([]unstructured.Unstructured, error)
}

// ResourceReader lists and gets unstructured Kubernetes resources.
type ResourceReader interface {
	ResourceLister
	GetResource(gvr schema.GroupVersionResource, namespace, name string) (*unstructured.Unstructured, error)
}

// dockerConfigJSON is a .dockerconfigjson payload. authn.AuthConfig decodes "auth" with the kubelet's own rules
// (base64 of "user:pass"), so a payload the kubelet would reject fails to unmarshal here as well.
type dockerConfigJSON struct {
	Auths map[string]authn.AuthConfig `json:"auths"`
}
