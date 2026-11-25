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

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestParseImageRef(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		ref  string
		want ImageRef
	}{
		{
			name: "empty",
			ref:  "",
			want: ImageRef{},
		},
		{
			name: "bare docker hub image",
			ref:  "nginx",
			want: ImageRef{RawName: "nginx", Host: "docker.io", Repo: "library/nginx"},
		},
		{
			name: "docker hub image with tag",
			ref:  "python:3.12-slim",
			want: ImageRef{RawName: "python", Host: "docker.io", Repo: "library/python", Tag: "3.12-slim", HasTag: true},
		},
		{
			name: "docker hub org image with tag and digest",
			ref:  "pytorch/pytorch:2.4@sha256:1111",
			want: ImageRef{
				RawName: "pytorch/pytorch", Host: "docker.io", Repo: "pytorch/pytorch",
				Tag: "2.4", HasTag: true, HasDigest: true,
			},
		},
		{
			name: "explicit docker.io bare image adds library prefix",
			ref:  "docker.io/nginx",
			want: ImageRef{RawName: "docker.io/nginx", Host: "docker.io", Repo: "library/nginx"},
		},
		{
			name: "uppercase DOCKER.IO normalizes to docker.io",
			ref:  "DOCKER.IO/nginx",
			want: ImageRef{RawName: "DOCKER.IO/nginx", Host: "docker.io", Repo: "library/nginx"},
		},
		{
			name: "index.docker.io normalizes to docker.io",
			ref:  "index.docker.io/busybox:latest",
			want: ImageRef{RawName: "index.docker.io/busybox", Host: "docker.io", Repo: "library/busybox", Tag: "latest", HasTag: true},
		},
		{
			name: "index.docker.io org image preserves org",
			ref:  "Index.Docker.IO/team/app:v1",
			want: ImageRef{RawName: "Index.Docker.IO/team/app", Host: "docker.io", Repo: "team/app", Tag: "v1", HasTag: true},
		},
		{
			name: "portless localhost registry preserved",
			ref:  "localhost/team/app:v1",
			want: ImageRef{RawName: "localhost/team/app", Host: "localhost", Repo: "team/app", Tag: "v1", HasTag: true},
		},
		{
			name: "uppercase portless LOCALHOST normalized to localhost",
			ref:  "LOCALHOST/team/app:v1",
			want: ImageRef{RawName: "LOCALHOST/team/app", Host: "localhost", Repo: "team/app", Tag: "v1", HasTag: true},
		},
		{
			name: "localhost with port and no tag",
			ref:  "localhost:5000/team/app",
			want: ImageRef{RawName: "localhost:5000/team/app", Host: "localhost:5000", Repo: "team/app"},
		},
		{
			name: "IPv6 bracketed host with port and tag",
			ref:  "[fd00::1]:5000/obs/collector:v1",
			want: ImageRef{RawName: "[fd00::1]:5000/obs/collector", Host: "[fd00::1]:5000", Repo: "obs/collector", Tag: "v1", HasTag: true},
		},
		{
			name: "name with numeric tag and no slash",
			ref:  "myreg:5000",
			want: ImageRef{RawName: "myreg", Host: "docker.io", Repo: "library/myreg", Tag: "5000", HasTag: true},
		},
		{
			name: "localhost with port, tag, and digest",
			ref:  "localhost:5000/team/app:v2@sha256:abcdef",
			want: ImageRef{
				RawName: "localhost:5000/team/app", Host: "localhost:5000", Repo: "team/app",
				Tag: "v2", HasTag: true, HasDigest: true,
			},
		},
		{
			name: "uppercase localhost with port, DNS host, and IPv6 host normalize Host to lowercase",
			ref:  "LOCALHOST:5000/team/app:v1",
			want: ImageRef{RawName: "LOCALHOST:5000/team/app", Host: "localhost:5000", Repo: "team/app", Tag: "v1", HasTag: true},
		},
		{
			name: "mixed-case registry host with port normalizes Host while preserving RawName",
			ref:  "Harbor.Local:8443/team-a/app:v1",
			want: ImageRef{RawName: "Harbor.Local:8443/team-a/app", Host: "harbor.local:8443", Repo: "team-a/app", Tag: "v1", HasTag: true},
		},
		{
			name: "uppercase GAR host normalizes Host to lowercase",
			ref:  "US-DOCKER.PKG.DEV/p/r/img:v1",
			want: ImageRef{RawName: "US-DOCKER.PKG.DEV/p/r/img", Host: "us-docker.pkg.dev", Repo: "p/r/img", Tag: "v1", HasTag: true},
		},
		{
			name: "uppercase IPv6 host normalizes Host to lowercase",
			ref:  "[FD00::1]:5000/obs/collector:v1",
			want: ImageRef{RawName: "[FD00::1]:5000/obs/collector", Host: "[fd00::1]:5000", Repo: "obs/collector", Tag: "v1", HasTag: true},
		},
		{
			name: "non-docker.io single-segment repo does not add library prefix",
			ref:  "localhost:5000/app:v1",
			want: ImageRef{RawName: "localhost:5000/app", Host: "localhost:5000", Repo: "app", Tag: "v1", HasTag: true},
		},
		{
			name: "bare docker hub image containing dot without slash",
			ref:  "my.app:v1",
			want: ImageRef{RawName: "my.app", Host: "docker.io", Repo: "library/my.app", Tag: "v1", HasTag: true},
		},
		{
			name: "bare image named localhost without slash defaults to docker.io",
			ref:  "localhost:v1",
			want: ImageRef{RawName: "localhost", Host: "docker.io", Repo: "library/localhost", Tag: "v1", HasTag: true},
		},
		{
			name: "GAR image with digest only",
			ref:  "us-docker.pkg.dev/proj/repo/server@sha256:1234",
			want: ImageRef{
				RawName: "us-docker.pkg.dev/proj/repo/server", Host: "us-docker.pkg.dev", Repo: "proj/repo/server",
				HasDigest: true,
			},
		},
		{
			name: "trailing colon sets HasTag with empty Tag",
			ref:  "us-docker.pkg.dev/proj/repo/server:",
			want: ImageRef{
				RawName: "us-docker.pkg.dev/proj/repo/server", Host: "us-docker.pkg.dev", Repo: "proj/repo/server",
				Tag: "", HasTag: true,
			},
		},
		{
			name: "trailing at-sign sets HasDigest",
			ref:  "us-docker.pkg.dev/proj/repo/server@",
			want: ImageRef{
				RawName: "us-docker.pkg.dev/proj/repo/server", Host: "us-docker.pkg.dev", Repo: "proj/repo/server",
				HasDigest: true,
			},
		},
		{
			name: "tag only with empty name leaves Host and Repo empty",
			ref:  ":latest",
			want: ImageRef{Tag: "latest", HasTag: true},
		},
		{
			name: "digest only with empty name leaves Host and Repo empty",
			ref:  "@sha256:abcdef",
			want: ImageRef{HasDigest: true},
		},
		{
			name: "registry host with empty repo leaves Host and Repo empty",
			ref:  "us-docker.pkg.dev/",
			want: ImageRef{RawName: "us-docker.pkg.dev/"},
		},
		{
			name: "double slash after registry host leaves Host and Repo empty",
			ref:  "us-docker.pkg.dev//proj/img",
			want: ImageRef{RawName: "us-docker.pkg.dev//proj/img"},
		},
		{
			name: "leading slash leaves Host and Repo empty",
			ref:  "/nginx",
			want: ImageRef{RawName: "/nginx"},
		},
		{
			name: "trailing slash leaves Host and Repo empty",
			ref:  "myorg/",
			want: ImageRef{RawName: "myorg/"},
		},
		{
			name: "slash after colon is not treated as tag",
			ref:  "us-docker.pkg.dev/proj/server:bad/path",
			want: ImageRef{RawName: "us-docker.pkg.dev/proj/server:bad/path", Host: "us-docker.pkg.dev", Repo: "proj/server:bad/path"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ParseImageRef(tc.ref)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("ParseImageRef(%q) mismatch (-want +got):\n%s", tc.ref, diff)
			}
		})
	}
}
