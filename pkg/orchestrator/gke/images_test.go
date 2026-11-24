// Copyright 2026 "Google LLC"
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

package gke

import (
	"strings"
	"testing"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
)

const (
	testPublicRepo    = "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways"
	testPublicSidecar = testPublicRepo + "/colocated_python_sidecar"
	testDefaultServer = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest"
	testDefaultProxy  = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest"
	testCustomSidecar = "us-docker.pkg.dev/my-proj/my-repo/colocated-sidecar:jax-0.11.2"
)

type pathwaysImages struct{ Server, Proxy, Worker string }

func containerImage(spec *corev1.PodSpec, name string) string {
	if spec == nil {
		return ""
	}
	if c := findContainer(spec, name); c != nil {
		return c.Image
	}
	return ""
}

func publicImages(tag string) pathwaysImages {
	return pathwaysImages{testPublicRepo + "/server:" + tag, testPublicRepo + "/proxy_server:" + tag, testPublicRepo + "/server:" + tag}
}

func TestResolvePathwaysImages(t *testing.T) {
	for _, tc := range []struct {
		name     string
		in       orchestrator.PathwaysJobDefinition
		want     pathwaysImages
		wantMsg  []string // substrings of the message; none means no message
		notInMsg []string
		wantWarn bool
	}{
		{
			name: "no sidecar keeps gcluster's defaults",
			want: pathwaysImages{testDefaultServer, testDefaultProxy, testDefaultServer},
		},
		{
			name: "no sidecar keeps explicit images",
			in:   orchestrator.PathwaysJobDefinition{ServerImage: "server:v1", ProxyServerImage: "proxy:v1"},
			want: pathwaysImages{"server:v1", "proxy:v1", "server:v1"},
		},
		{
			name:    "public sidecar with a jax tag",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2"},
			want:    publicImages("jax-0.11.2"),
			wantMsg: []string{testPublicRepo + "/server:jax-0.11.2", testPublicRepo + "/proxy_server:jax-0.11.2"},
		},
		{
			name:    "public sidecar with a dated tag",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":20261124-jax_0.11.2"},
			want:    publicImages("20261124-jax_0.11.2"),
			wantMsg: []string{testPublicRepo + "/server:20261124-jax_0.11.2"},
		},
		{
			name:    "public sidecar tagged latest",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":latest"},
			want:    publicImages("latest"),
			wantMsg: []string{testPublicRepo + "/server:latest"},
		},
		{
			name:    "public sidecar without a tag pulls latest",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar},
			want:    publicImages("latest"),
			wantMsg: []string{testPublicRepo + "/server:latest"},
		},
		{
			name:    "public sidecar with a tag and a digest uses the tag",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2@sha256:abcdef"},
			want:    publicImages("jax-0.11.2"),
			wantMsg: []string{testPublicRepo + "/server:jax-0.11.2"},
		},
		{
			name:     "public sidecar pinned by digest only",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef"},
			want:     publicImages("latest"),
			wantMsg:  []string{testPublicRepo + "/server:latest", "to pin"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with a tag that is not a release tag",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":update-available-abcdef"},
			want:     publicImages("latest"),
			wantMsg:  []string{testPublicRepo + "/server:latest", "to pin"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with only the server image set",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ServerImage: testDefaultServer},
			want:     pathwaysImages{testDefaultServer, testPublicRepo + "/proxy_server:jax-0.11.2", testDefaultServer},
			wantMsg:  []string{testPublicRepo + "/proxy_server:jax-0.11.2", "--pathways-server-image", "--pathways-proxy-server-image"},
			notInMsg: []string{testPublicRepo + "/server:"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with only the proxy image set",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ProxyServerImage: testDefaultProxy},
			want:     pathwaysImages{testPublicRepo + "/server:jax-0.11.2", testDefaultProxy, testPublicRepo + "/server:jax-0.11.2"},
			wantMsg:  []string{testPublicRepo + "/server:jax-0.11.2", "--pathways-server-image", "--pathways-proxy-server-image"},
			notInMsg: []string{testPublicRepo + "/proxy_server:"},
			wantWarn: true,
		},
		{
			name: "public sidecar with both images set",
			in:   orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ServerImage: "server:v1", ProxyServerImage: "proxy:v1"},
			want: pathwaysImages{"server:v1", "proxy:v1", "server:v1"},
		},
		{
			name:    "public sidecar keeps an explicit worker image",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", WorkerImage: "worker:v1"},
			want:    pathwaysImages{testPublicRepo + "/server:jax-0.11.2", testPublicRepo + "/proxy_server:jax-0.11.2", "worker:v1"},
			wantMsg: []string{testPublicRepo + "/server:jax-0.11.2"},
		},
		{
			name:     "deploy-SPS with the public sidecar names only the server, the head runs no proxy",
			in:       orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2"},
			want:     pathwaysImages{testPublicRepo + "/server:jax-0.11.2", testDefaultProxy, testPublicRepo + "/server:jax-0.11.2"},
			wantMsg:  []string{testPublicRepo + "/server:jax-0.11.2"},
			notInMsg: []string{"proxy"},
		},
		{
			name: "deploy-SPS with the public sidecar and an explicit server image",
			in:   orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ServerImage: "server:v1"},
			want: pathwaysImages{"server:v1", testDefaultProxy, "server:v1"},
		},
		{
			name:     "custom sidecar on gcluster's default images",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testCustomSidecar},
			want:     pathwaysImages{testDefaultServer, testDefaultProxy, testDefaultServer},
			wantMsg:  []string{testCustomSidecar, testDefaultServer, testDefaultProxy, "should ideally come from the same Pathways release", "cannot verify"},
			wantWarn: true,
		},
		{
			name:     "custom sidecar with only the server image set",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testCustomSidecar, ServerImage: "server:v1"},
			want:     pathwaysImages{"server:v1", testDefaultProxy, "server:v1"},
			wantMsg:  []string{testDefaultProxy},
			notInMsg: []string{testDefaultServer},
			wantWarn: true,
		},
		{
			name: "custom sidecar with both images set",
			in:   orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testCustomSidecar, ServerImage: "server:v1", ProxyServerImage: "proxy:v1"},
			want: pathwaysImages{"server:v1", "proxy:v1", "server:v1"},
		},
		{
			name: "deploy-SPS with a custom sidecar and an explicit server image",
			in:   orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testCustomSidecar, ServerImage: "server:v1"},
			want: pathwaysImages{"server:v1", testDefaultProxy, "server:v1"},
		},
		{
			name:     "a look-alike repository is a custom sidecar",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + "_dev:jax-0.11.2"},
			want:     pathwaysImages{testDefaultServer, testDefaultProxy, testDefaultServer},
			wantMsg:  []string{"should ideally come from the same Pathways release"},
			wantWarn: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, msg, warn := resolvePathwaysImages(tc.in)
			if diff := cmp.Diff(tc.want, pathwaysImages{got.ServerImage, got.ProxyServerImage, got.WorkerImage}); diff != "" {
				t.Errorf("images mismatch (-want +got):\n%s", diff)
			}
			if got.ColocatedPythonSidecarImage != tc.in.ColocatedPythonSidecarImage {
				t.Errorf("sidecar image = %q, want it unchanged (%q)", got.ColocatedPythonSidecarImage, tc.in.ColocatedPythonSidecarImage)
			}
			if len(tc.wantMsg) == 0 {
				if msg != "" {
					t.Errorf("message = %q, want none", msg)
				}
				return
			}
			for _, s := range tc.wantMsg {
				if !strings.Contains(msg, s) {
					t.Errorf("message %q does not contain %q", msg, s)
				}
			}
			for _, s := range tc.notInMsg {
				if strings.Contains(msg, s) {
					t.Errorf("message %q contains %q", msg, s)
				}
			}
			if warn != tc.wantWarn {
				t.Errorf("warning = %v, want %v", warn, tc.wantWarn)
			}
		})
	}
}

// TestGeneratePathwaysManifest_PublicSidecarImages pins the wiring in every mode that renders the sidecar: the
// pathways-rm, pathways-proxy and pathways-worker containers run the cloud-pathways images of the sidecar's release,
// and gcluster logs the choice.
func TestGeneratePathwaysManifest_PublicSidecarImages(t *testing.T) {
	const sidecar = testPublicSidecar + ":jax-0.11.2"
	want := publicImages("jax-0.11.2")
	for _, tc := range []struct {
		name      string
		headless  bool
		deploySPS bool
		wantProxy bool
	}{
		{name: "batch", wantProxy: true},
		{name: "headless", headless: true, wantProxy: true},
		{name: "deploy-SPS", deploySPS: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, restore := logging.CaptureForTesting(t)
			manifest, err := generatePathwaysTestManifest(t, "", tc.headless, sidecar, "", nil, func(job *orchestrator.JobDefinition) {
				job.Pathways.ServerImage, job.Pathways.ProxyServerImage, job.Pathways.WorkerImage = "", "", ""
				if tc.deploySPS {
					job.Pathways.DeploySPS = true
					job.CommandToRun = ""
				}
			})
			restore()
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			specs := pathwaysPodSpecs(t, manifest)
			got := map[string]string{
				"pathways-rm":              containerImage(specs["pathways-head"], "pathways-rm"),
				"pathways-worker":          containerImage(specs["worker"], "pathways-worker"),
				"colocated-python-sidecar": containerImage(specs["worker"], "colocated-python-sidecar"),
			}
			wantImages := map[string]string{"pathways-rm": want.Server, "pathways-worker": want.Worker, "colocated-python-sidecar": sidecar}
			if tc.wantProxy {
				got["pathways-proxy"] = containerImage(specs["pathways-head"], "pathways-proxy")
				wantImages["pathways-proxy"] = want.Proxy
			}
			if diff := cmp.Diff(wantImages, got); diff != "" {
				t.Errorf("container images mismatch (-want +got):\n%s", diff)
			}
			infoOut, errOut := snapshot()
			if !strings.Contains(infoOut, want.Server) || strings.Contains(errOut, want.Server) {
				t.Errorf("want the chosen server image %q on the info log only; info log %q, warning log %q", want.Server, infoOut, errOut)
			}
		})
	}
}

// TestGeneratePathwaysManifest_SidecarImageWarning pins that a resolver warning reaches the warning log, not the
// info log.
func TestGeneratePathwaysManifest_SidecarImageWarning(t *testing.T) {
	snapshot, restore := logging.CaptureForTesting(t)
	_, err := generatePathwaysTestManifest(t, "", false, testCustomSidecar, "", nil, func(job *orchestrator.JobDefinition) {
		job.Pathways.ServerImage, job.Pathways.ProxyServerImage, job.Pathways.WorkerImage = "", "", ""
	})
	restore()
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}
	infoOut, errOut := snapshot()
	const want = "should ideally come from the same Pathways release"
	if !strings.Contains(errOut, want) || strings.Contains(infoOut, want) {
		t.Errorf("want the custom-sidecar notice on the warning log only; warning log %q, info log %q", errOut, infoOut)
	}
}

func TestProxyImageFor(t *testing.T) {
	tests := []struct {
		serverImage string
		want        string
	}{
		{
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest",
			want:        "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest",
		},
		{
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways/server:jax-0.11.2",
			want:        "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways/proxy_server:jax-0.11.2",
		},
		// Pathways server and proxy images never share a digest, so a digest-pinned server has no derivable proxy.
		{
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server@sha256:abcdef",
			want:        "",
		},
		{
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest@sha256:abcdef",
			want:        "",
		},
		{
			serverImage: "custom-registry.io/my-rm:v1",
			want:        "",
		},
	}

	for _, tc := range tests {
		if got := proxyImageFor(tc.serverImage); got != tc.want {
			t.Errorf("proxyImageFor(%q) = %q, want %q", tc.serverImage, got, tc.want)
		}
	}
}
