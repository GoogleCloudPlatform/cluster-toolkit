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
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type stubResourceLister struct {
	items []unstructured.Unstructured
	err   error
	gotNS string
	gotFS string
	sa    *unstructured.Unstructured
	saErr error
	gotSA string
}

func (s *stubResourceLister) GetResource(gvr schema.GroupVersionResource, _, name string) (*unstructured.Unstructured, error) {
	s.gotSA = name
	if s.saErr != nil || s.sa != nil {
		return s.sa, s.saErr
	}
	return nil, apierrors.NewNotFound(gvr.GroupResource(), name)
}

func (s *stubResourceLister) ListResources(_ schema.GroupVersionResource, ns, _, fs string) ([]unstructured.Unstructured, error) {
	s.gotNS = ns
	s.gotFS = fs
	return s.items, s.err
}

func rawDockerSecret(name string, dockerConfigJSON string) unstructured.Unstructured {
	return unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata":   map[string]any{"name": name},
			"type":       string(corev1.SecretTypeDockerConfigJson),
			"data":       map[string]any{corev1.DockerConfigJsonKey: dockerConfigJSON},
		},
	}
}

func makeTestDockerSecret(name string, authKeys ...string) unstructured.Unstructured {
	auths := make(map[string]any, len(authKeys))
	for _, k := range authKeys {
		auths[k] = map[string]any{"auth": base64.StdEncoding.EncodeToString([]byte("u:p"))}
	}
	raw, _ := json.Marshal(map[string]any{"auths": auths})
	return rawDockerSecret(name, base64.StdEncoding.EncodeToString(raw))
}

func TestSplitImageRef(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ref, wantHost, wantPath string
		wantOK                  bool
	}{
		{"", "", "", false},
		{":latest", "", "", false},
		{"alpine:3.20", "docker.io", "library/alpine", true},
		{"myreg:5000", "docker.io", "library/myreg", true},
		{"pytorch/pytorch:2.4@sha256:1111", "docker.io", "pytorch/pytorch", true},
		{"docker.io/nginx", "docker.io", "library/nginx", true},
		{"index.docker.io/team/app:v1", "docker.io", "team/app", true},
		{"gcr.io/proj/img@sha256:abc", "gcr.io", "proj/img", true},
		{"localhost:5000/repo/img:tag", "localhost:5000", "repo/img", true},
		{"[fd00::1]:5000/obs/collector:v1", "[fd00::1]:5000", "obs/collector", true},
	}
	for _, tc := range cases {
		gotHost, gotPath, gotOK := splitImageRef(tc.ref)
		if gotHost != tc.wantHost || gotPath != tc.wantPath || gotOK != tc.wantOK {
			t.Errorf("splitImageRef(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.ref, gotHost, gotPath, gotOK, tc.wantHost, tc.wantPath, tc.wantOK)
		}
	}
}

func TestAuthKeyHostAndPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		key, wantHost, wantPath string
		wantOK                  bool
	}{
		{"", "", "", false},
		{"https://", "", "", false},
		{"/team-a", "", "", false},
		{"https://%zz", "", "", false},
		{"https://index.docker.io/v1/", "docker.io", "", true},
		{"index.docker.io", "docker.io", "", true},
		{"docker.io", "docker.io", "", true},
		{"registry-1.docker.io", "registry-1.docker.io", "", true},
		{"https://index.docker.io/v1/access-token", "index.docker.io", "access-token", true},
		{"https://gcr.io/v2/proj/", "gcr.io", "proj", true},
		{"gcr.io/v1", "gcr.io", "v1", true},
		{"harbor.local:8443/team-a", "harbor.local:8443", "team-a", true},
		{"http://[fd00::1]:5000", "[fd00::1]:5000", "", true},
		{"*.pkg.dev", "*.pkg.dev", "", true},
	}
	for _, tc := range cases {
		gotHost, gotPath, gotOK := authKeyHostAndPath(tc.key)
		if gotHost != tc.wantHost || gotPath != tc.wantPath || gotOK != tc.wantOK {
			t.Errorf("authKeyHostAndPath(%q) = (%q, %q, %v), want (%q, %q, %v)", tc.key, gotHost, gotPath, gotOK, tc.wantHost, tc.wantPath, tc.wantOK)
		}
	}
}

func TestAuthHostMatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		authHost, imgHost string
		want              bool
	}{
		{"gcr.io", "gcr.io", true},
		{"gcr.io", "us.gcr.io", false},
		{"*.pkg.dev", "us-docker.pkg.dev", true},
		{"*.pkg.dev", "sub.us-docker.pkg.dev", false},
		{"*-docker.gcr.io", "eu-docker.gcr.io", true},
		{"*.pkg.dev:5000", "us-docker.pkg.dev", false},
		{"*.local:5000", "harbor.local:5000", true},
		{"[fd00::1]:5000", "[fd00::1]:5000", true},
		{"[fd00::1]:5000", "[fd00::1]:6000", false},
		{"[", "x", false},
	}
	for _, tc := range cases {
		if got := authHostMatches(tc.authHost, tc.imgHost); got != tc.want {
			t.Errorf("authHostMatches(%q, %q) = %v, want %v", tc.authHost, tc.imgHost, got, tc.want)
		}
	}
}

func TestAuthPathMatches(t *testing.T) {
	t.Parallel()
	cases := []struct {
		prefix, imgPath string
		want            bool
	}{
		{"", "anything", true},
		{"", "team-a/app", true},
		{"team-a", "team-a", true},
		{"team-a", "team-a/app", true},
		{"team-a", "team-a/sub/app", true},
		{"team-a", "team-alpha", false},
		{"team-a", "team-alpha/app", false},
		{"team-a/", "team-a/app", true},
		{"team-a/", "team-alpha", false},
		{"team-a/", "team-alpha/app", false},
		{"/team-a/", "team-a/app", true},
		{"proj", "proj/image", true},
		{"proj", "proj-two/image", false},
	}
	for _, tc := range cases {
		if got := authPathMatches(tc.prefix, tc.imgPath); got != tc.want {
			t.Errorf("authPathMatches(%q, %q) = %v, want %v", tc.prefix, tc.imgPath, got, tc.want)
		}
	}
}

func TestIsForbiddenError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"typed 403", apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("forbidden")), true},
		{"kubectl stderr", errors.New("Error from server (Forbidden): secrets is forbidden: User cannot list"), true},
		{"other error", errors.New("connection refused"), false},
	}
	for _, tc := range cases {
		if got := IsForbiddenError(tc.err); got != tc.want {
			t.Errorf("%s: IsForbiddenError(%v) = %v, want %v", tc.name, tc.err, got, tc.want)
		}
	}
}

func TestDiscoverImagePullSecrets_ListErrorsAndNamespaceDefault(t *testing.T) {
	t.Parallel()
	if got := DiscoverImagePullSecrets(nil, "ns", "", []string{"alpine:3"}); got != "" {
		t.Errorf("expected empty on nil lister, got %q", got)
	}
	noImages := &stubResourceLister{err: errors.New("must not list")}
	if got := DiscoverImagePullSecrets(noImages, "ns", "", nil); got != "" || noImages.gotNS != "" {
		t.Errorf("expected no list call without images, got %q (ns=%q)", got, noImages.gotNS)
	}
	forbidden := apierrors.NewForbidden(schema.GroupResource{Resource: "secrets"}, "", errors.New("forbidden"))
	forbiddenLister := &stubResourceLister{err: forbidden}
	if got := DiscoverImagePullSecrets(forbiddenLister, "", "", []string{"registry.local/app:v1"}); got != "" || forbiddenLister.gotNS != "default" {
		t.Errorf("got (%q, ns=%q), want (\"\", \"default\")", got, forbiddenLister.gotNS)
	}
	timeoutLister := &stubResourceLister{err: errors.New("context deadline exceeded")}
	if got := DiscoverImagePullSecrets(timeoutLister, "team-ns", "", []string{"registry.local/app:v1"}); got != "" {
		t.Errorf("expected empty on non-RBAC list error, got %q", got)
	}
	okLister := &stubResourceLister{items: []unstructured.Unstructured{makeTestDockerSecret("sec-1", "registry.local")}}
	if got := DiscoverImagePullSecrets(okLister, "team-ns", "", []string{"registry.local/app:v1"}); got != "sec-1" || okLister.gotNS != "team-ns" || okLister.gotFS != "type=kubernetes.io/dockerconfigjson" {
		t.Errorf("got (%q, ns=%q, fs=%q), want (\"sec-1\", \"team-ns\", \"type=kubernetes.io/dockerconfigjson\")", got, okLister.gotNS, okLister.gotFS)
	}
}

func TestDiscoverImagePullSecrets_MatchingSemantics(t *testing.T) {
	t.Parallel()
	credsStoreStub, _ := json.Marshal(map[string]any{"credsStore": "osxkeychain", "auths": map[string]any{"https://index.docker.io/v1/": map[string]any{}}})
	userPass, _ := json.Marshal(map[string]any{"auths": map[string]any{"quay.io": map[string]any{"username": "u", "password": "p"}}})
	noData := rawDockerSecret("no-data", "")
	delete(noData.Object, "data")
	secrets := []unstructured.Unstructured{
		noData,
		rawDockerSecret("empty-data", ""),
		rawDockerSecret("bad-base64", "%%%"),
		rawDockerSecret("bad-json", base64.StdEncoding.EncodeToString([]byte("{not-json"))),
		rawDockerSecret("creds-store-stub", base64.StdEncoding.EncodeToString(credsStoreStub)),
		rawDockerSecret("quay-userpass", base64.StdEncoding.EncodeToString(userPass)),
		makeTestDockerSecret("hub-default", "https://index.docker.io/v1/", ""),
		makeTestDockerSecret("hub-token-only", "https://index.docker.io/v1/access-token"),
		makeTestDockerSecret("pkg-wildcard", "https://*.pkg.dev"),
		makeTestDockerSecret("pkg-wildcard-port", "https://*.pkg.dev:5000"),
		makeTestDockerSecret("scoped-v2", "https://harbor.local/v2/team-a"),
		makeTestDockerSecret("ipv6-sec", "http://[fd00::1]:5000"),
		rawDockerSecret("auth-not-userpass", base64.StdEncoding.EncodeToString([]byte(`{"auths":{"tok.local":{"auth":"`+base64.StdEncoding.EncodeToString([]byte("token"))+`"}}}`))),
		rawDockerSecret("identitytoken-only", base64.StdEncoding.EncodeToString([]byte(`{"auths":{"idt.local":{"identitytoken":"t"}}}`))),
		makeTestDockerSecret("z-dup", "dup.local"),
		makeTestDockerSecret("a-dup", "https://dup.local"),
	}
	cases := []struct {
		image string
		want  []string
	}{
		{"", nil},
		{"nginx:1.27", []string{"hub-default"}},
		{"docker.io/library/nginx", []string{"hub-default"}},
		{"quay.io/org/img:v1", []string{"quay-userpass"}},
		{"us-docker.pkg.dev/p/r/img:v1", []string{"pkg-wildcard"}},
		{"us-docker.pkg.dev:5000/p/r/img:v1", []string{"pkg-wildcard-port"}},
		{"sub.us-docker.pkg.dev/p/r/img:v1", nil},
		{"harbor.local/team-a/trainer:v1", []string{"scoped-v2"}},
		// Path prefix scoping matches on path segment boundaries; team-alpha does not match team-a.
		{"harbor.local/team-alpha/x:v1", nil},
		{"harbor.local/other/x:v1", nil},
		{"[fd00::1]:5000/obs/collector:v1", []string{"ipv6-sec"}},
		{"dup.local/app:v1", []string{"a-dup", "z-dup"}},
		// The kubelet rejects the whole secret when "auth" is not base64("user:pass"), failing every pull in the pod.
		{"tok.local/app:v1", nil},
		// The kubelet ignores identitytoken/registrytoken in pull secrets.
		{"idt.local/app:v1", nil},
	}
	for _, tc := range cases {
		if got := matchingSecretNames(secrets, tc.image); !slices.Equal(got, tc.want) {
			t.Errorf("matchingSecretNames(%q) = %v, want %v", tc.image, got, tc.want)
		}
	}

	lister := &stubResourceLister{items: secrets}
	images := []string{"nginx:1.27", "dup.local/app:v1", "harbor.local/team-a/trainer:v1", "harbor.local/team-a/proxy:v1", "unknown.example.com/img:v1"}
	if got := DiscoverImagePullSecrets(lister, "ml-ns", "", images); got != "hub-default,scoped-v2" {
		t.Errorf("DiscoverImagePullSecrets = %q, want ambiguous dup.local skipped and scoped-v2 listed once", got)
	}
}

func TestDiscoverImagePullSecrets_DefersToServiceAccount(t *testing.T) {
	t.Parallel()
	saWith := func(refs ...any) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]any{"imagePullSecrets": refs}}
	}
	images := []string{"registry.local/app:v1"}
	cases := []struct {
		name, sa, wantSA, want string
		obj                    *unstructured.Unstructured
		err                    error
	}{
		{name: "missing ServiceAccount", wantSA: "default", want: "sec-1"},
		{name: "ServiceAccount without pull secrets", sa: "team-sa", wantSA: "team-sa", obj: saWith(), want: "sec-1"},
		// Pod-level secrets would stop admission from copying the ServiceAccount's own.
		{name: "ServiceAccount with pull secrets", sa: "team-sa", wantSA: "team-sa", obj: saWith(map[string]any{"name": "sa-cred"})},
		{name: "unreadable ServiceAccount", wantSA: "default", err: apierrors.NewForbidden(schema.GroupResource{Resource: "serviceaccounts"}, "default", errors.New("forbidden"))},
	}
	for _, tc := range cases {
		lister := &stubResourceLister{items: []unstructured.Unstructured{makeTestDockerSecret("sec-1", "registry.local")}, sa: tc.obj, saErr: tc.err}
		if got := DiscoverImagePullSecrets(lister, "team-ns", tc.sa, images); got != tc.want || lister.gotSA != tc.wantSA {
			t.Errorf("%s: got (%q, sa=%q), want (%q, sa=%q)", tc.name, got, lister.gotSA, tc.want, tc.wantSA)
		}
	}
}
