// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package k8s

import "strings"

// ParseImageRef splits an OCI image reference into its normalized registry host,
// repository path, raw name, tag, and digest components.
func ParseImageRef(ref string) ImageRef {
	name, digest, hasDigest := strings.Cut(ref, "@")
	lastSlash := strings.LastIndex(name, "/")
	rawName := name
	var tag string
	var hasTag bool
	if colon := strings.Index(name[lastSlash+1:], ":"); colon >= 0 {
		rawName = name[:lastSlash+1+colon]
		tag = name[lastSlash+1+colon+1:]
		hasTag = true
	}

	res := ImageRef{
		RawName:   rawName,
		Tag:       tag,
		HasTag:    hasTag,
		Digest:    digest,
		HasDigest: hasDigest,
	}

	host, repo, hasSlash := strings.Cut(rawName, "/")
	switch {
	case !hasSlash || (!strings.ContainsAny(host, ".:") && !strings.EqualFold(host, "localhost")):
		host, repo = "docker.io", rawName
	case strings.EqualFold(host, "index.docker.io") || strings.EqualFold(host, "docker.io"):
		host = "docker.io"
	case strings.EqualFold(host, "localhost"):
		host = "localhost"
	}
	if repo == "" || strings.HasPrefix(repo, "/") || strings.HasSuffix(repo, "/") {
		return res
	}
	if host == "docker.io" && !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	res.Host = host
	res.Repo = repo
	return res
}
