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
	"net"
	"net/url"
	"path"
	"slices"
	"strings"

	"hpc-toolkit/pkg/logging"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// IsForbiddenError reports whether err indicates a 403 Forbidden RBAC error.
func IsForbiddenError(err error) bool {
	return err != nil && (apierrors.IsForbidden(err) || strings.Contains(strings.ToLower(err.Error()), "forbidden"))
}

// DiscoverImagePullSecrets returns the comma-separated names of the kubernetes.io/dockerconfigjson Secrets
// in namespace whose auths entries match images, using the same host/path matching as the kubelet's
// credential keyring. It returns "" when the client is unavailable, Secrets cannot be listed, or the pods'
// ServiceAccount already lists imagePullSecrets, and skips an image when several secrets match it.
func DiscoverImagePullSecrets(client ResourceReader, namespace, serviceAccount string, images []string) string {
	if client == nil || len(images) == 0 {
		return ""
	}
	if namespace == "" {
		namespace = "default"
	}
	if deferToServiceAccount(client, namespace, serviceAccount) {
		return ""
	}
	secrets, err := client.ListResources(SecretGVR, namespace, "", "type="+string(corev1.SecretTypeDockerConfigJson))
	if err != nil {
		if IsForbiddenError(err) {
			logging.Info("Skipping automatic imagePullSecrets discovery in namespace %q (lacking permission to list Secrets); pass --image-pull-secret if pulling from a private registry.", namespace)
		} else {
			logging.Warn("failed to list Secrets in namespace %q for automatic imagePullSecrets discovery: %v", namespace, err)
		}
		return ""
	}
	var selected []string
	for _, img := range images {
		matched := matchingSecretNames(secrets, img)
		switch {
		case len(matched) > 1:
			logging.Warn("multiple dockerconfigjson secrets in namespace %q match image %q (%s); skipping auto-injection for this image. Pass --image-pull-secret with all pull secrets required by the workload to disambiguate (specifying --image-pull-secret disables auto-discovery).", namespace, img, strings.Join(matched, ", "))
		case len(matched) == 1 && !slices.Contains(selected, matched[0]):
			selected = append(selected, matched[0])
			logging.Info("Auto-discovered imagePullSecret %q in namespace %q for image %q", matched[0], namespace, img)
		}
	}
	return strings.Join(selected, ",")
}

// deferToServiceAccount reports whether discovery must leave the pods' imagePullSecrets empty: the ServiceAccount
// admission plugin copies the ServiceAccount's imagePullSecrets only into pods that declare none, so attaching
// discovered secrets would silently drop them. An unreadable ServiceAccount is treated the same way.
func deferToServiceAccount(client ResourceReader, namespace, name string) bool {
	if name == "" {
		name = "default"
	}
	sa, err := client.GetResource(ServiceAccountGVR, namespace, name)
	switch {
	case apierrors.IsNotFound(err):
		return false
	case err != nil:
		logging.Info("Skipping automatic imagePullSecrets discovery: cannot read ServiceAccount %s/%s: %v", namespace, name, err)
		return true
	}
	if refs, _, _ := unstructured.NestedSlice(sa.Object, "imagePullSecrets"); len(refs) > 0 {
		logging.Info("Skipping automatic imagePullSecrets discovery: ServiceAccount %s/%s already lists imagePullSecrets.", namespace, name)
		return true
	}
	return false
}

// matchingSecretNames returns the sorted names of the secrets holding credentials for img.
func matchingSecretNames(secrets []unstructured.Unstructured, img string) []string {
	imgHost, imgPath, ok := splitImageRef(img)
	if !ok {
		return nil
	}
	var names []string
	for i := range secrets {
		if slices.ContainsFunc(dockerConfigAuthKeys(&secrets[i]), func(key string) bool {
			host, prefix, ok := authKeyHostAndPath(key)
			return ok && authHostMatches(host, imgHost) && strings.HasPrefix(imgPath, prefix)
		}) {
			names = append(names, secrets[i].GetName())
		}
	}
	slices.Sort(names)
	return names
}

// dockerConfigAuthKeys decodes the secret's .dockerconfigjson payload and returns the auths keys that give the
// kubelet credentials. A payload the kubelet cannot parse yields none: attaching it would fail every image pull in
// the pod, not just the pulls from that registry.
func dockerConfigAuthKeys(secret *unstructured.Unstructured) []string {
	raw, _, _ := unstructured.NestedString(secret.Object, "data", corev1.DockerConfigJsonKey)
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return nil
	}
	var cfg dockerConfigJSON
	if err := json.Unmarshal(decoded, &cfg); err != nil {
		return nil
	}
	var keys []string
	for key, e := range cfg.Auths {
		// The kubelet reads only username and password (decoded from "auth" when set) from pull secrets.
		if e.Username != "" || e.Password != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

// splitImageRef returns the registry host and repository path of an image reference as the container
// runtime normalizes it (Docker Hub shorthand becomes docker.io/library/<name>), without tag or digest.
func splitImageRef(ref string) (string, string, bool) {
	name, _, _ := strings.Cut(ref, "@")
	host, repo, hasSlash := strings.Cut(name, "/")
	if !hasSlash || (!strings.ContainsAny(host, ".:") && host != "localhost") {
		host, repo = "docker.io", name
	} else if host == "index.docker.io" {
		host = "docker.io"
	}
	lastSeg := strings.LastIndex(repo, "/") + 1
	if tag := strings.Index(repo[lastSeg:], ":"); tag >= 0 {
		repo = repo[:lastSeg+tag]
	}
	if repo == "" {
		return "", "", false
	}
	if host == "docker.io" && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	return host, repo, true
}

// authKeyHostAndPath normalizes a dockerconfigjson auths key the way the kubelet keyring does: an optional
// scheme, host[:port], and a path whose leading /v1/ or /v2/ segment is dropped. A path-less
// index.docker.io key is the Docker Hub default and applies to every docker.io image.
func authKeyHostAndPath(key string) (string, string, bool) {
	if !strings.HasPrefix(key, "https://") && !strings.HasPrefix(key, "http://") {
		key = "https://" + key
	}
	u, err := url.Parse(key)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	p := u.Path
	if strings.HasPrefix(p, "/v1/") || strings.HasPrefix(p, "/v2/") {
		p = p[3:]
	}
	p = strings.Trim(p, "/")
	if u.Host == "index.docker.io" && p == "" {
		return "docker.io", "", true
	}
	return u.Host, p, true
}

// authHostMatches mirrors the kubelet's URL matching: equal ports, the same number of dot-separated labels,
// and every auth label glob-matching the image label (so *.gcr.io matches us.gcr.io).
func authHostMatches(authHost, imgHost string) bool {
	authName, authPort := splitHostPort(authHost)
	imgName, imgPort := splitHostPort(imgHost)
	if authPort != imgPort {
		return false
	}
	authLabels, imgLabels := strings.Split(authName, "."), strings.Split(imgName, ".")
	if len(authLabels) != len(imgLabels) {
		return false
	}
	for i, pattern := range authLabels {
		if ok, err := path.Match(pattern, imgLabels[i]); err != nil || !ok {
			return false
		}
	}
	return true
}

func splitHostPort(hostport string) (string, string) {
	if host, port, err := net.SplitHostPort(hostport); err == nil {
		return host, port
	}
	return hostport, ""
}
