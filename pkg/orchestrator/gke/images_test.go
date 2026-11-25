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
	"hpc-toolkit/pkg/shell"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
		wantErr  []string
		notInErr []string
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
			name:     "public sidecar with a tag and a digest uses the tag and warns",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2@sha256:abcdef"},
			want:     publicImages("jax-0.11.2"),
			wantMsg:  []string{testPublicRepo + "/server:jax-0.11.2", testPublicRepo + "/proxy_server:jax-0.11.2", "pinned by digest", "to pin matching images"},
			wantWarn: true,
		},
		{
			name:    "public sidecar pinned by digest only returns an error",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef"},
			wantErr: []string{"pinned by digest without a release tag", "--pathways-server-image and --pathways-proxy-server-image"},
		},
		{
			name:    "public sidecar pinned by digest with only the server image set returns an error",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef", ServerImage: testDefaultServer},
			wantErr: []string{"pinned by digest without a release tag", "--pathways-server-image and --pathways-proxy-server-image"},
		},
		{
			name:    "public sidecar pinned by digest with an unrecognized tag returns an error",
			in:      orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":update-available-abcdef@sha256:abcdef"},
			wantErr: []string{"pinned by digest without a release tag", "--pathways-server-image and --pathways-proxy-server-image"},
		},
		{
			name: "public sidecar pinned by digest with both server and proxy images set succeeds",
			in:   orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef", ServerImage: "server:v1", ProxyServerImage: "proxy:v1"},
			want: pathwaysImages{"server:v1", "proxy:v1", "server:v1"},
		},
		{
			name:     "public sidecar with a tag that is not a release tag",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":update-available-abcdef"},
			want:     publicImages("latest"),
			wantMsg:  []string{testPublicRepo + "/server:latest", "to pin"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with a malformed dot-only version tag falls back to latest",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-.."},
			want:     publicImages("latest"),
			wantMsg:  []string{testPublicRepo + "/server:latest", "names no release tag"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with only the server image set",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ServerImage: testDefaultServer},
			want:     pathwaysImages{testDefaultServer, testPublicRepo + "/proxy_server:jax-0.11.2", testDefaultServer},
			wantMsg:  []string{testPublicRepo + "/proxy_server:jax-0.11.2", "that image from the --pathways-server-image", "--pathways-proxy-server-image"},
			notInMsg: []string{testPublicRepo + "/server:"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with only the proxy image set",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ProxyServerImage: testDefaultProxy},
			want:     pathwaysImages{testPublicRepo + "/server:jax-0.11.2", testDefaultProxy, testPublicRepo + "/server:jax-0.11.2"},
			wantMsg:  []string{testPublicRepo + "/server:jax-0.11.2", "that image from the --pathways-proxy-server-image", "--pathways-server-image"},
			notInMsg: []string{testPublicRepo + "/proxy_server:"},
			wantWarn: true,
		},
		{
			name:     "public sidecar with an unrecognized tag and only the server image set does not duplicate flag advice",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":update-available-abcdef", ServerImage: testDefaultServer},
			want:     pathwaysImages{testDefaultServer, testPublicRepo + "/proxy_server:latest", testDefaultServer},
			wantMsg:  []string{testPublicRepo + "/proxy_server:latest", "make sure the images work together.", "names no release tag", "to pin the images."},
			notInMsg: []string{"yourself"},
			wantWarn: true,
		},
		{
			name: "public sidecar with both images set",
			in:   orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", ServerImage: "server:v1", ProxyServerImage: "proxy:v1"},
			want: pathwaysImages{"server:v1", "proxy:v1", "server:v1"},
		},
		{
			name:     "public sidecar keeps an explicit worker image and warns",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2", WorkerImage: "worker:v1"},
			want:     pathwaysImages{testPublicRepo + "/server:jax-0.11.2", testPublicRepo + "/proxy_server:jax-0.11.2", "worker:v1"},
			wantMsg:  []string{testPublicRepo + "/server:jax-0.11.2", "those images from the --pathways-worker-image"},
			wantWarn: true,
		},
		{
			name:     "deploy-SPS with the public sidecar names only the server, the head runs no proxy",
			in:       orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2"},
			want:     pathwaysImages{testPublicRepo + "/server:jax-0.11.2", testDefaultProxy, testPublicRepo + "/server:jax-0.11.2"},
			wantMsg:  []string{testPublicRepo + "/server:jax-0.11.2"},
			notInMsg: []string{"proxy"},
		},
		{
			name:     "deploy-SPS with the public sidecar pinned by digest only errors and omits proxy flag",
			in:       orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef"},
			wantErr:  []string{"pinned by digest without a release tag", "--pathways-server-image"},
			notInErr: []string{"proxy"},
		},
		{
			name: "deploy-SPS with the public sidecar pinned by digest and explicit server image succeeds",
			in:   orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef", ServerImage: "server:v1"},
			want: pathwaysImages{"server:v1", testDefaultProxy, "server:v1"},
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
			name:     "custom sidecar with only the proxy image set",
			in:       orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testCustomSidecar, ProxyServerImage: "proxy:v1"},
			want:     pathwaysImages{testDefaultServer, "proxy:v1", testDefaultServer},
			wantMsg:  []string{testDefaultServer},
			notInMsg: []string{testDefaultProxy},
			wantWarn: true,
		},
		{
			name: "custom sidecar with both images set",
			in:   orchestrator.PathwaysJobDefinition{ColocatedPythonSidecarImage: testCustomSidecar, ServerImage: "server:v1", ProxyServerImage: "proxy:v1"},
			want: pathwaysImages{"server:v1", "proxy:v1", "server:v1"},
		},
		{
			name:     "deploy-SPS with a custom sidecar and no server image omits proxy from warning",
			in:       orchestrator.PathwaysJobDefinition{DeploySPS: true, ColocatedPythonSidecarImage: testCustomSidecar},
			want:     pathwaysImages{testDefaultServer, testDefaultProxy, testDefaultServer},
			wantMsg:  []string{testDefaultServer, "The server image should ideally come from the same Pathways release", "--pathways-server-image"},
			notInMsg: []string{"proxy"},
			wantWarn: true,
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
			got, msg, warn, err := resolvePathwaysImages(tc.in)
			if len(tc.wantErr) > 0 {
				assertResolvePathwaysImagesErr(t, err, tc.wantErr, tc.notInErr)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, pathwaysImages{got.ServerImage, got.ProxyServerImage, got.WorkerImage}); diff != "" {
				t.Errorf("images mismatch (-want +got):\n%s", diff)
			}
			if got.ColocatedPythonSidecarImage != tc.in.ColocatedPythonSidecarImage {
				t.Errorf("sidecar image = %q, want it unchanged (%q)", got.ColocatedPythonSidecarImage, tc.in.ColocatedPythonSidecarImage)
			}
			if warn != tc.wantWarn {
				t.Errorf("warning = %v, want %v (message: %q)", warn, tc.wantWarn, msg)
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
		})
	}
}

func assertResolvePathwaysImagesErr(t *testing.T, err error, want, unwanted []string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %v, got nil", want)
	}
	for _, s := range want {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("error %q does not contain %q", err.Error(), s)
		}
	}
	for _, s := range unwanted {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error %q unexpectedly contains %q", err.Error(), s)
		}
	}
}

// TestSubmitJob_DigestOnlyPublicSidecarFailsPreflight pins that a digest-only public sidecar without matching
// server/proxy image flags fails fast locally before any cluster or image-build commands run.
func TestSubmitJob_DigestOnlyPublicSidecarFailsPreflight(t *testing.T) {
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{})
	orc := newTestGKEOrchestrator(mockExec)
	job := orchestrator.JobDefinition{
		WorkloadName:    "pw-digest",
		ClusterName:     "test-cluster",
		ClusterLocation: "us-central1-a",
		ProjectID:       "test-project",
		ComputeType:     "v6e-16",
		BaseImage:       "python:3.12",
		BuildContext:    t.TempDir(),
		IsPathwaysJob:   true,
		Pathways: orchestrator.PathwaysJobDefinition{
			GCSLocation:                 "gs://bucket/pw",
			ColocatedPythonSidecarImage: testPublicSidecar + "@sha256:abcdef",
		},
	}
	err := orc.SubmitJob(job)
	if err == nil || !strings.Contains(err.Error(), "pinned by digest without a release tag") {
		t.Fatalf("SubmitJob() = %v, want digest-only sidecar error", err)
	}
	if len(mockExec.calls) != 0 {
		t.Errorf("expected 0 shell calls before local preflight failure, got %v", mockExec.calls)
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
			} else if c := findContainer(specs["pathways-head"], "pathways-proxy"); c != nil {
				t.Errorf("deploy-SPS must not render pathways-proxy container, got %+v", c)
			}
			if diff := cmp.Diff(wantImages, got); diff != "" {
				t.Errorf("container images mismatch (-want +got):\n%s", diff)
			}
			infoOut, errOut := snapshot()
			if !strings.Contains(infoOut, want.Server) || strings.Contains(errOut, want.Server) {
				t.Errorf("want the chosen server image %q on the info log only; info log %q, warning log %q", want.Server, infoOut, errOut)
			}
			if tc.wantProxy && !strings.Contains(infoOut, want.Proxy) {
				t.Errorf("want the chosen proxy image %q on the info log; got %q", want.Proxy, infoOut)
			}
			if !tc.wantProxy && strings.Contains(infoOut, "proxy") {
				t.Errorf("deploy-SPS info log should not mention proxy; got %q", infoOut)
			}
		})
	}
}

// TestGeneratePathwaysManifest_PublicSidecarPullSecretDiscovery pins that resolvePathwaysImages runs before
// resolveImagePullSecrets so repo-scoped secrets match the auto-selected cloud-pathways server and proxy images.
func TestGeneratePathwaysManifest_PublicSidecarPullSecretDiscovery(t *testing.T) {
	setupMockMachineConfig(t)
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"gcloud compute machine-types describe n2-standard-2 --zone=us-central1-a --format=json": {{ExitCode: 0, Stdout: `{"guestCpus": 2}`}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.projectID = "mock-project"
	orc.clusterZones = []string{"us-central1-a"}
	orc.clusterDesc.NodePools = []gkeJobNodePool{
		{Name: "default-pool", Config: gkeNodePoolConfig{MachineType: "n2-standard-2"}},
	}
	orc.kubeClient = &MockKubeClient{
		Objects: map[string][]unstructured.Unstructured{
			"secrets": {
				makeDockerConfigSecret("cloud-pw-server-sec", testPublicRepo+"/server"),
				makeDockerConfigSecret("cloud-pw-proxy-sec", testPublicRepo+"/proxy_server"),
			},
		},
	}
	job := orchestrator.JobDefinition{
		WorkloadName:    "pw-pullsec",
		ComputeType:     "n2-standard-2",
		ClusterLocation: "us-central1",
		NumSlices:       1,
		NodesPerSlice:   1,
		CommandToRun:    "python train.py",
		IsPathwaysJob:   true,
		Pathways: orchestrator.PathwaysJobDefinition{
			GCSLocation:                 "gs://bucket/pw",
			HeadNodePool:                "cpu-np",
			ColocatedPythonSidecarImage: testPublicSidecar + ":jax-0.11.2",
		},
	}
	profile, isDyn, isStat, err := orc.resolveHardwareRequirements(&job)
	if err != nil {
		t.Fatalf("resolveHardwareRequirements failed: %v", err)
	}
	manifest, err := orc.GeneratePathwaysManifest(job, "registry.local/workload:v1", profile, isDyn, isStat)
	if err != nil {
		t.Fatalf("GeneratePathwaysManifest failed: %v", err)
	}
	specs := pathwaysPodSpecs(t, manifest)
	for _, podName := range []string{"pathways-head", "worker"} {
		var got []string
		for _, s := range specs[podName].ImagePullSecrets {
			got = append(got, s.Name)
		}
		if diff := cmp.Diff([]string{"cloud-pw-server-sec", "cloud-pw-proxy-sec"}, got); diff != "" {
			t.Errorf("%s imagePullSecrets mismatch (-want +got):\n%s", podName, diff)
		}
	}
}

// TestGeneratePathwaysManifest_SidecarImageWarning pins that a resolver warning reaches the warning log, not the
// info log, and that no-sidecar submissions emit neither notice.
func TestGeneratePathwaysManifest_SidecarImageWarning(t *testing.T) {
	for _, tc := range []struct {
		name    string
		sidecar string
		want    string
	}{
		{name: "custom sidecar warns", sidecar: testCustomSidecar, want: "should ideally come from the same Pathways release"},
		{name: "public sidecar tag@digest warns", sidecar: testPublicSidecar + ":jax-0.11.2@sha256:abcdef", want: "pinned by digest"},
		{name: "no sidecar is silent", sidecar: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, restore := logging.CaptureForTesting(t)
			_, err := generatePathwaysTestManifest(t, "", false, tc.sidecar, "", nil, func(job *orchestrator.JobDefinition) {
				job.Pathways.ServerImage, job.Pathways.ProxyServerImage, job.Pathways.WorkerImage = "", "", ""
			})
			restore()
			if err != nil {
				t.Fatalf("GeneratePathwaysManifest failed: %v", err)
			}
			infoOut, errOut := snapshot()
			if tc.want == "" {
				if strings.Contains(infoOut, "Using the Pathways") || strings.Contains(errOut, "colocated Python sidecar") {
					t.Errorf("expected no sidecar image notice; info log %q, warning log %q", infoOut, errOut)
				}
				return
			}
			if !strings.Contains(errOut, tc.want) || strings.Contains(infoOut, tc.want) {
				t.Errorf("want notice containing %q on the warning log only; warning log %q, info log %q", tc.want, errOut, infoOut)
			}
		})
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
		{
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways/server",
			want:        "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways/proxy_server:latest",
		},
		// Empty trailing tag and leading-slash refs must not be rewritten.
		{
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/cloud-pathways/server:",
			want:        "",
		},
		{
			serverImage: "/server",
			want:        "",
		},
		// Intermediate path segments containing /server: must not be rewritten as the image name.
		{
			serverImage: "registry.local/server:8080/my-rm:v1",
			want:        "",
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
