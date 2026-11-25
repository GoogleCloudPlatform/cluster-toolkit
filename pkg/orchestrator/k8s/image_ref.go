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
// repository path, raw name, tag, and digest presence. Unlike go-containerregistry's
// name.ParseReference, it preserves tags on tag+digest refs and accepts synthetic digests.
func ParseImageRef(ref string) ImageRef {
	name, _, hasDigest := strings.Cut(ref, "@")
	lastSlash := strings.LastIndex(name, "/")
	rawName := name
	var tag string
	var hasTag bool
	if seg, parsedTag, ok := strings.Cut(name[lastSlash+1:], ":"); ok {
		rawName = name[:lastSlash+1+len(seg)]
		tag = parsedTag
		hasTag = true
	}

	res := ImageRef{
		RawName:   rawName,
		Tag:       tag,
		HasTag:    hasTag,
		HasDigest: hasDigest,
	}

	host, repo, hasSlash := strings.Cut(rawName, "/")
	// Bare names and org/repo shorthands have no '.', ':', or 'localhost' in the first segment.
	if !hasSlash || (!strings.ContainsAny(host, ".:") && !strings.EqualFold(host, "localhost")) {
		host, repo = "docker.io", rawName
	} else {
		host = strings.ToLower(host)
		if host == "index.docker.io" {
			host = "docker.io"
		}
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
