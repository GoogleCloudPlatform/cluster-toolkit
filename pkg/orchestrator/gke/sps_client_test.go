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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"github.com/google/go-cmp/cmp"
)

const testSPSJobSetJSON = `{
	"metadata": {
		"labels": {"kueue.x-k8s.io/queue-name": "cluster-queue"}
	},
	"spec": {
		"replicatedJobs": [
			{
				"name": "pathways-head",
				"replicas": 1,
				"template": {"spec": {"template": {"spec": {
					"nodeSelector": {"cloud.google.com/gke-nodepool": "cpu-np"},
					"containers": [{
						"name": "pathways-rm",
						"image": "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20261004",
						"args": [
							"--gcs_scratch_location", "gs://my-bucket/sps",
							"--instance_type=tpu7x:2x2x1",
							"--instance_count=2"
						]
					}]
				}}}}
			},
			{
				"name": "worker",
				"replicas": 2,
				"template": {"spec": {"template": {"spec": {
					"initContainers": [{"name": "colocated-python-sidecar", "image": "sidecar:tag"}],
					"containers": [{"name": "pathways-worker", "image": "custom-registry.io/custom-worker:v1"}]
				}}}}
			}
		]
	}
}`

func TestSPSInstanceRef(t *testing.T) {
	tests := []struct {
		ref          string
		wantName     string
		wantEndpoint string
	}{
		{
			ref:          "sps-v5p",
			wantName:     "sps-v5p",
			wantEndpoint: "sps-v5p-pathways-head-0-0.sps-v5p:29001",
		},
		{
			ref:          "sps-v5p-pathways-head-0-0.sps-v5p:29001",
			wantName:     "sps-v5p",
			wantEndpoint: "sps-v5p-pathways-head-0-0.sps-v5p:29001",
		},
		{
			ref:          "grpc://sps-v5p-pathways-head-0-0.sps-v5p:29001",
			wantName:     "sps-v5p",
			wantEndpoint: "sps-v5p-pathways-head-0-0.sps-v5p:29001",
		},
		{
			ref:          "sps-v5p-pathways-head-0-0.sps-v5p",
			wantName:     "sps-v5p",
			wantEndpoint: "sps-v5p-pathways-head-0-0.sps-v5p:29001",
		},
	}

	for _, tc := range tests {
		gotName, gotEndpoint := spsInstanceRef(tc.ref)
		if gotName != tc.wantName || gotEndpoint != tc.wantEndpoint {
			t.Errorf("spsInstanceRef(%q) = (%q, %q), want (%q, %q)", tc.ref, gotName, gotEndpoint, tc.wantName, tc.wantEndpoint)
		}
	}
}

func TestParseSPSClientTarget(t *testing.T) {
	inst, err := parseSPSClientTarget("sps-j6080103", []byte(testSPSJobSetJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := spsInstance{
		Name:           "sps-j6080103",
		KueueQueueName: "cluster-queue",
		HeadNodePool:   "cpu-np",
		ServerImage:    "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20261004",
		GCSLocation:    "gs://my-bucket/sps",
		InstanceType:   "tpu7x:2x2x1",
		InstanceCount:  2,
		HasSidecar:     true,
	}
	if inst != want {
		t.Errorf("parseSPSClientTarget() = %+v, want %+v", inst, want)
	}
}

func TestParseSPSClientTarget_RejectsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		jobName string
		raw     string
		wantErr string
	}{
		{
			name:    "rejects regular Pathways JobSet running pathways-proxy",
			jobName: "regular-pw",
			raw: `{
				"spec": {
					"replicatedJobs": [{
						"name": "pathways-head",
						"template": {"spec": {"template": {"spec": {
							"initContainers": [
								{"name": "pathways-rm", "image": "server:latest", "args": ["--instance_type=tpuv6e:2x4"]},
								{"name": "pathways-proxy", "image": "proxy:latest"}
							]
						}}}}
					}]
				}
			}`,
			wantErr: "its pathways-head runs pathways-proxy",
		},
		{
			name:    "rejects non-Pathways JobSet",
			jobName: "plain-job",
			raw:     `{"spec": {"replicatedJobs": [{"name": "main-job"}]}}`,
			wantErr: "needs a pathways-head job running pathways-rm",
		},
		{
			name:    "rejects JobSet with pathways-head but no worker job",
			jobName: "head-only",
			raw: `{
				"spec": {
					"replicatedJobs": [{
						"name": "pathways-head",
						"template": {"spec": {"template": {"spec": {
							"containers": [{"name": "pathways-rm", "image": "server:latest", "args": ["--instance_type=tpuv6e:2x4"]}]
						}}}}
					}]
				}
			}`,
			wantErr: "and a worker job running pathways-worker",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSPSClientTarget(tc.jobName, []byte(tc.raw))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("got err = %v, want substring %q", err, tc.wantErr)
			}
		})
	}
}

func assertManifestStrings(t *testing.T, manifest string, want, unwanted []string) {
	t.Helper()
	for _, s := range want {
		if !strings.Contains(manifest, s) {
			t.Errorf("manifest missing %q:\n%s", s, manifest)
		}
	}
	for _, s := range unwanted {
		if strings.Contains(manifest, s) {
			t.Errorf("manifest unexpectedly contains %q:\n%s", s, manifest)
		}
	}
}

// testSPSInstance is a two-instance tpu7x SPS deployment with the colocated Python sidecar.
func testSPSInstance() spsInstance {
	return spsInstance{
		Name:           "sps-j6080103",
		Endpoint:       "sps-j6080103-pathways-head-0-0.sps-j6080103:29001",
		KueueQueueName: "",
		HeadNodePool:   "cpu-np",
		ServerImage:    "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20261004",
		ProxyImage:     "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:20261004",
		GCSLocation:    "gs://my-bucket/sps",
		InstanceType:   "tpu7x:2x2x1",
		InstanceCount:  2,
		HasSidecar:     true,
	}
}

func mustResolveSPSPathways(t *testing.T, p orchestrator.PathwaysJobDefinition, inst spsInstance) orchestrator.PathwaysJobDefinition {
	t.Helper()
	got, err := resolveSPSClientPathways(p, inst)
	if err != nil {
		t.Fatalf("resolveSPSClientPathways failed: %v", err)
	}
	return got
}

func TestGenerateSPSClientManifest(t *testing.T) {
	orc := NewGKEOrchestrator()
	inst := testSPSInstance()

	t.Run("in-cluster mode renders proxy initContainer and workload-container", func(t *testing.T) {
		job := orchestrator.JobDefinition{
			WorkloadName:            "sps-client-cluster",
			NumSlices:               2,
			TtlSecondsAfterFinished: 3600,
			CommandToRun:            "python3 train.py",
			Pathways: mustResolveSPSPathways(t, orchestrator.PathwaysJobDefinition{
				RunSPSWorkload: "sps-j6080103",
			}, inst),
		}
		manifest, err := orc.generateSPSClientManifest(job, inst, "us-docker.pkg.dev/my-proj/img:latest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertManifestStrings(t, manifest,
			[]string{
				"name: sps-client-cluster",
				"cloud.google.com/gke-nodepool: cpu-np",
				"name: pathways-proxy",
				`"--resource_manager_address=sps-j6080103-pathways-head-0-0.sps-j6080103:29001"`,
				"--virtual_slices=tpu7x:2x2x1,tpu7x:2x2x1",
				"--sidecar_name=external",
				"name: workload-container",
				"value: grpc://localhost:29000",
			},
			[]string{
				"kueue.x-k8s.io/queue-name",
				"name: pathways-rm",
				"name: pathways-worker",
			},
		)
		if got := strings.Count(manifest, "gcluster.google.com/workload: sps-client-cluster"); got != 2 {
			t.Errorf("gcluster.google.com/workload label count = %d, want 2 (JobSet metadata + pathways-head pod template):\n%s", got, manifest)
		}
	})

	t.Run("terminal mode renders proxy-only pod with inherited Kueue queue", func(t *testing.T) {
		instWithQueue := inst
		instWithQueue.KueueQueueName = "cluster-queue"
		job := orchestrator.JobDefinition{
			WorkloadName:            "sps-client-local",
			KueueQueueName:          instWithQueue.KueueQueueName,
			NumSlices:               1,
			TtlSecondsAfterFinished: 3600,
			CommandToRun:            "python3 train.py",
			Pathways: mustResolveSPSPathways(t, orchestrator.PathwaysJobDefinition{
				RunSPSWorkload: "sps-j6080103",
			}, instWithQueue),
		}
		manifest, err := orc.generateSPSClientManifest(job, instWithQueue, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertManifestStrings(t, manifest,
			[]string{
				"kueue.x-k8s.io/queue-name: cluster-queue",
				"name: pathways-proxy",
				"--virtual_slices=tpu7x:2x2x1",
				"--sidecar_name=external",
			},
			[]string{"workload-container"},
		)
	})

	t.Run("in-cluster mode mounts RawMounts onto workload-container", func(t *testing.T) {
		job := orchestrator.JobDefinition{
			WorkloadName:            "sps-client-mount",
			NumSlices:               1,
			TtlSecondsAfterFinished: 3600,
			CommandToRun:            "python3 train.py",
			RawMounts:               []string{"gs://my-bucket;/mnt/data;rw"},
			ImagePullSecrets:        "my-reg-secret",
			RestartOnExitCodes:      []int{42, 43},
			MLDiagnosticsEnabled:    true,
			Pathways: mustResolveSPSPathways(t, orchestrator.PathwaysJobDefinition{
				RunSPSWorkload: "sps-j6080103",
			}, inst),
		}
		manifest, err := orc.generateSPSClientManifest(job, inst, "us-docker.pkg.dev/my-proj/img:latest")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertManifestStrings(t, manifest,
			[]string{
				`gke-gcsfuse/volumes: "true"`,
				"mountPath: /mnt/data",
				"driver: gcsfuse.csi.storage.gke.io",
				"bucketName: my-bucket",
				"name: my-reg-secret",
				"podFailurePolicy:",
				`managed-mldiagnostics-gke: "true"`,
			},
			nil,
		)
	})
}

// renderSPSClientEnvManifest renders the SPS client JobSet for testSPSInstance and a job mutated by mutate,
// without touching a cluster or a kubeconfig.
func renderSPSClientEnvManifest(t *testing.T, templatesPath, image string, mutate func(*orchestrator.JobDefinition)) string {
	t.Helper()
	orc := newTestGKEOrchestrator(NewMockExecutor(nil))
	orc.gkeCustomTemplatesPath = templatesPath
	inst := testSPSInstance()
	job := orchestrator.JobDefinition{
		WorkloadName:              "sps-env-client",
		NumSlices:                 2,
		CommandToRun:              "python3 train.py",
		ImagePullSecretsSpecified: true,
		Pathways:                  mustResolveSPSPathways(t, orchestrator.PathwaysJobDefinition{RunSPSWorkload: inst.Name}, inst),
	}
	mutate(&job)
	manifest, err := orc.generateSPSClientManifest(job, inst, image)
	if err != nil {
		t.Fatalf("generateSPSClientManifest failed: %v", err)
	}
	return manifest
}

// TestGenerateSPSClientManifest_UserEnvReplacesEveryTemplateDefault is the SPS client counterpart of
// TestGeneratePathwaysManifest_UserEnvReplacesEveryTemplateDefault, for the in-cluster and terminal modes.
func TestGenerateSPSClientManifest_UserEnvReplacesEveryTemplateDefault(t *testing.T) {
	for _, tc := range []struct{ name, image string }{
		{name: "in-cluster workload", image: "us-docker.pkg.dev/my-proj/img:latest"},
		{name: "terminal mode proxy only"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assertUserEnvReplacesDefaults(t, func(t *testing.T, mutate func(*orchestrator.JobDefinition)) map[string]map[string]string {
				return envByContainer(t, pathwaysPodSpecs(t, renderSPSClientEnvManifest(t, "", tc.image, mutate)))
			}, workloadEnvList, proxyEnvList)
		})
	}
}

// TestGenerateSPSClientManifest_LegacyTemplateRendersUserEnv renders the SPS client with a pathways_jobset.tmpl
// extracted before the env guards and user env vars that collide with no default: each must be added once to the
// container its list feeds and nothing else may change.
func TestGenerateSPSClientManifest_LegacyTemplateRendersUserEnv(t *testing.T) {
	customDir := writeLegacyPathwaysTemplate(t)
	assertUserEnvAddedOnce(t, func(t *testing.T, mutate func(*orchestrator.JobDefinition)) map[string]map[string]string {
		return envByContainer(t, pathwaysPodSpecs(t, renderSPSClientEnvManifest(t, customDir, "us-docker.pkg.dev/my-proj/img:latest", mutate)))
	}, workloadEnvList, proxyEnvList)
}

func TestReadForwardedPort(t *testing.T) {
	t.Run("extracts local port from IPv4 and IPv6 kubectl port-forward output", func(t *testing.T) {
		for _, line := range []string{
			"Forwarding from 127.0.0.1:41955 -> 29000\n",
			"Forwarding from [::1]:41955 -> 29000\n",
		} {
			var errOut strings.Builder
			portCh := make(chan int, 1)
			pipeDone := make(chan struct{})
			readForwardedPort(strings.NewReader(line), &errOut, portCh, pipeDone)
			if got, ok := <-portCh; !ok || got != 41955 {
				t.Errorf("for %q: got (%d, %v), want (41955, true)", line, got, ok)
			}
			<-pipeDone
		}
	})

	t.Run("captures error output when port-forward fails", func(t *testing.T) {
		var errOut strings.Builder
		portCh := make(chan int, 1)
		pipeDone := make(chan struct{})
		readForwardedPort(strings.NewReader("error: unable to forward port\n"), &errOut, portCh, pipeDone)
		if _, ok := <-portCh; ok {
			t.Errorf("expected closed channel on failure")
		}
		<-pipeDone
		if !strings.Contains(errOut.String(), "unable to forward port") {
			t.Errorf("errOut = %q, want failure message", errOut.String())
		}
	})
}

// execEnv returns the variables, among those in want, that a process started by os/exec with this Env sees.
func execEnv(env []string, want map[string]string) map[string]string {
	vars := map[string]string{}
	for _, kv := range (&exec.Cmd{Env: env}).Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if _, ok := want[k]; ok {
			vars[k] = v
		}
	}
	return vars
}

func TestLocalCommandEnv(t *testing.T) {
	shellEnv := []string{"PATH=/usr/bin", "JAX_PLATFORMS=cpu", "JAX_BACKEND_TARGET=grpc://10.0.0.1:29000"}
	const target = "grpc://127.0.0.1:43210"
	for _, tc := range []struct {
		name string
		env  map[string]string
		want map[string]string
	}{
		{
			name: "defaults replace the shell values",
			want: map[string]string{"PATH": "/usr/bin", "JAX_PLATFORMS": "proxy", "JAX_BACKEND_TARGET": target},
		},
		{
			name: "--env JAX_PLATFORMS replaces the default",
			env:  map[string]string{"JAX_PLATFORMS": "proxy,cpu", "FOO": "bar"},
			want: map[string]string{"PATH": "/usr/bin", "JAX_PLATFORMS": "proxy,cpu", "FOO": "bar", "JAX_BACKEND_TARGET": target},
		},
		{
			name: "--env JAX_PLATFORMS empty string still replaces the default",
			env:  map[string]string{"JAX_PLATFORMS": ""},
			want: map[string]string{"PATH": "/usr/bin", "JAX_PLATFORMS": "", "JAX_BACKEND_TARGET": target},
		},
		{
			name: "JAX_BACKEND_TARGET always points at the port-forward",
			env:  map[string]string{"JAX_BACKEND_TARGET": "grpc://10.0.0.2:29000"},
			want: map[string]string{"PATH": "/usr/bin", "JAX_PLATFORMS": "proxy", "JAX_BACKEND_TARGET": target},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, execEnv(localCommandEnv(shellEnv, tc.env, 43210), tc.want)); diff != "" {
				t.Errorf("environment of the local command mismatch (-want +got):\n%s", diff)
			}
		})
	}
	t.Run("runLocalCommand warns when ignoring --env JAX_BACKEND_TARGET", func(t *testing.T) {
		if _, err := exec.LookPath("bash"); err != nil {
			t.Skip("bash not found in PATH")
		}
		snapshot, restore := logging.CaptureForTesting(t)
		err := runLocalCommand("true", 43210, map[string]string{"JAX_BACKEND_TARGET": "grpc://10.0.0.2:29000"}, nil)
		restore()
		if err != nil {
			t.Fatalf("runLocalCommand failed: %v", err)
		}
		_, errOut := snapshot()
		if !strings.Contains(errOut, "Ignoring --env JAX_BACKEND_TARGET") || !strings.Contains(errOut, "127.0.0.1:43210") {
			t.Errorf("expected JAX_BACKEND_TARGET warning, got %q", errOut)
		}
	})
}

func TestReadSPSInstance_RejectsNonRunningHeadPod(t *testing.T) {
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get jobset sps-j6080103 -n custom-ns -o json --request-timeout=10s": {{ExitCode: 0, Stdout: testSPSJobSetJSON}},
		"kubectl get pods -n custom-ns -l jobset.sigs.k8s.io/jobset-name=sps-j6080103,jobset.sigs.k8s.io/replicatedjob-name=pathways-head -o jsonpath={.items[*].status.phase} --request-timeout=10s": {
			{ExitCode: 0, Stdout: "Pending"},
		},
	})
	orc := newTestGKEOrchestrator(mockExec)
	_, err := orc.readSPSInstance("sps-j6080103", "custom-ns")
	if err == nil || !strings.Contains(err.Error(), `SPS instance "sps-j6080103" has no running pathways-head pod`) {
		t.Errorf("got err = %v, want no running pathways-head pod error", err)
	}
	if len(mockExec.calls) != 2 {
		t.Errorf("kubectl calls = %d, want 2", len(mockExec.calls))
	}
}

func TestRunSPSWorkload_Validation(t *testing.T) {
	baseResponses := func() map[string][]shell.CommandResult {
		return map[string][]shell.CommandResult{
			"gcloud container clusters get-credentials":                                {{ExitCode: 0}},
			"kubectl get jobset sps-j6080103 -n default -o json --request-timeout=10s": {{ExitCode: 0, Stdout: testSPSJobSetJSON}},
			"kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=sps-j6080103,jobset.sigs.k8s.io/replicatedjob-name=pathways-head -o jsonpath={.items[*].status.phase} --request-timeout=10s": {
				{ExitCode: 0, Stdout: "Running"},
			},
		}
	}

	t.Run("rejects workload name equal to SPS instance name", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(baseResponses()))
		job := orchestrator.JobDefinition{
			WorkloadName:    "sps-j6080103",
			ClusterName:     "test-cluster",
			ClusterLocation: "us-central1-a",
			ProjectID:       "test-project",
			NumSlices:       1,
			Pathways:        orchestrator.PathwaysJobDefinition{RunSPSWorkload: "sps-j6080103"},
		}
		err := orc.runSPSWorkload(job)
		if err == nil || !strings.Contains(err.Error(), `must differ from the target SPS instance "sps-j6080103"`) {
			t.Errorf("got err = %v, want self-collision error", err)
		}
	})

	t.Run("rejects num-slices exceeding SPS instance slice count", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(baseResponses()))
		job := orchestrator.JobDefinition{
			WorkloadName:    "sps-client",
			ClusterName:     "test-cluster",
			ClusterLocation: "us-central1-a",
			ProjectID:       "test-project",
			NumSlices:       3,
			Pathways:        orchestrator.PathwaysJobDefinition{RunSPSWorkload: "sps-j6080103"},
		}
		err := orc.runSPSWorkload(job)
		if err == nil || !strings.Contains(err.Error(), `--num-slices 3 is more than the 2 slices of SPS instance "sps-j6080103"`) {
			t.Errorf("got err = %v, want slice count error", err)
		}
	})
}

func TestSubmitSPSClusterWorkload_ValidatesClusterMounts(t *testing.T) {
	const pvcJSONCmd = "kubectl get pvc -n default --ignore-not-found --request-timeout=10s -o json -- train-pvc"
	inst := spsInstance{Name: "sps-inst"}
	baseJob := orchestrator.JobDefinition{
		WorkloadName:  "sps-mount-check",
		ImageName:     "gcr.io/proj/img:latest",
		RawMounts:     []string{"train-pvc;/data"},
		NumSlices:     2,
		NodesPerSlice: 1,
		IsPathwaysJob: true,
		Pathways:      orchestrator.PathwaysJobDefinition{RunSPSWorkload: "sps-inst"},
	}

	t.Run("missing PVC fails fast before manifest generation", func(t *testing.T) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{pvcJSONCmd: {{ExitCode: 0, Stdout: ""}}})
		orc := newTestGKEOrchestrator(mockExec)
		err := orc.submitSPSClusterWorkload(baseJob, inst)
		if err == nil || !strings.Contains(err.Error(), `PersistentVolumeClaim "train-pvc"`) {
			t.Fatalf("submitSPSClusterWorkload() = %v, want missing PVC error", err)
		}
		if len(mockExec.calls) != 1 {
			t.Errorf("expected exactly 1 kubectl call before fast-fail, got %d: %v", len(mockExec.calls), mockExec.calls)
		}
	})

	t.Run("single-pod SPS client allows ReadWriteOnce PVC past ValidateClusterMounts and caches PVC metadata", func(t *testing.T) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			pvcJSONCmd: {{ExitCode: 0, Stdout: `{"spec":{"volumeName":"pv-sps-rwo","accessModes":["ReadWriteOnce"]}}`}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		job := baseJob
		job.ImageName = ""
		err := orc.submitSPSClusterWorkload(job, inst)
		if err == nil || !strings.Contains(err.Error(), "either --image or --base-image must be provided") {
			t.Fatalf("expected RWO PVC to pass ValidateClusterMounts and reach BuildContainerImage, got: %v", err)
		}
		if got := orc.inspectedPVCs["default/train-pvc"].volumeName; got != "pv-sps-rwo" {
			t.Errorf("expected inspectedPVCs[default/train-pvc].volumeName = pv-sps-rwo, got %q", got)
		}
		if len(mockExec.calls) != 1 {
			t.Errorf("expected 1 PVC inspection call, got %d: %v", len(mockExec.calls), mockExec.calls)
		}
	})

	t.Run("single-pod SPS client rejects ReadOnlyMany PVC mounted rw", func(t *testing.T) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			pvcJSONCmd: {{ExitCode: 0, Stdout: `{"spec":{"accessModes":["ReadOnlyMany"]}}`}},
		})
		orc := newTestGKEOrchestrator(mockExec)
		job := baseJob
		job.RawMounts = []string{"train-pvc;/data;rw"}
		err := orc.submitSPSClusterWorkload(job, inst)
		if err == nil || !strings.Contains(err.Error(), "read-only accessModes") {
			t.Fatalf("expected ReadOnlyMany + rw error on single-pod SPS client, got: %v", err)
		}
	})

	t.Run("off-cloud profile mount hits ValidateClusterMounts before RunStorageProfilePreflight", func(t *testing.T) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{})
		orc := newTestGKEOrchestrator(mockExec)
		job := baseJob
		job.RawMounts = []string{"gs://my-bkt;/data;profile=training"}
		err := orc.submitSPSClusterWorkload(job, inst)
		if err == nil || !strings.Contains(err.Error(), "Cloud URI") {
			t.Fatalf("expected Cloud URI error from ValidateClusterMounts before storage-profile preflight, got: %v", err)
		}
		if len(mockExec.calls) != 0 {
			t.Errorf("expected 0 kubectl calls when off-cloud Cloud URI fails fast, got: %v", mockExec.calls)
		}
	})
}

// TestRunSPSWorkload_ResolvesPathwaysBeforeMountsAndBuild pins that runSPSWorkload resolves client Pathways
// settings once right after reading the SPS instance, so a digest-pinned server fails fast in both local and
// in-cluster mode before cluster mount checks or container image builds.
func TestRunSPSWorkload_ResolvesPathwaysBeforeMountsAndBuild(t *testing.T) {
	t.Setenv("GCLUSTER_IMAGE_REPO", "")
	digestJobSetJSON := strings.Replace(testSPSJobSetJSON, ":20261004", "@sha256:abcdef", 1)
	newOrchestrator := func() (*GKEOrchestrator, *MockExecutor) {
		mockExec := NewMockExecutor(map[string][]shell.CommandResult{
			"gcloud container clusters get-credentials":                                {{ExitCode: 0}},
			"kubectl get jobset sps-j6080103 -n default -o json --request-timeout=10s": {{ExitCode: 0, Stdout: digestJobSetJSON}},
			"kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=sps-j6080103,jobset.sigs.k8s.io/replicatedjob-name=pathways-head -o jsonpath={.items[*].status.phase} --request-timeout=10s": {
				{ExitCode: 0, Stdout: "Running"},
			},
		})
		orc := newTestGKEOrchestrator(mockExec)
		orc.dynClient = &mockDynamicClient{}
		return orc, mockExec
	}
	baseJob := func() orchestrator.JobDefinition {
		return orchestrator.JobDefinition{
			WorkloadName:    "sps-build-check",
			ClusterName:     "test-cluster",
			ClusterLocation: "us-central1-a",
			ProjectID:       "test-project",
			NumSlices:       1,
			NodesPerSlice:   1,
			IsPathwaysJob:   true,
			Pathways:        orchestrator.PathwaysJobDefinition{RunSPSWorkload: "sps-j6080103"},
		}
	}

	for _, tc := range []struct {
		name   string
		mutate func(*orchestrator.JobDefinition)
	}{
		{
			name: "local terminal mode fails fast on digest-pinned server",
		},
		{
			name: "in-cluster mode fails fast before cluster mount checks and image build",
			mutate: func(j *orchestrator.JobDefinition) {
				j.BaseImage = "python:3.12"
				j.BuildContext = t.TempDir()
				j.RawMounts = []string{"train-pvc;/data"}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			orc, mockExec := newOrchestrator()
			job := baseJob()
			if tc.mutate != nil {
				tc.mutate(&job)
			}
			err := orc.runSPSWorkload(job)
			if err == nil || !strings.Contains(err.Error(), "different digests") {
				t.Fatalf("runSPSWorkload() = %v, want the digest error", err)
			}
			for _, call := range mockExec.calls {
				if strings.Contains(call, "get pvc") {
					t.Errorf("unexpected PVC check before digest error: %q", call)
				}
			}
		})
	}

	t.Run("positive control advances to BuildContainerImage when proxy image is given", func(t *testing.T) {
		orc, _ := newOrchestrator()
		job := baseJob()
		job.BaseImage = "python:3.12"
		job.BuildContext = t.TempDir()
		job.Pathways.ProxyServerImage = "proxy:pinned"
		if err := orc.runSPSWorkload(job); err == nil || !strings.Contains(err.Error(), "crane-based image build failed") {
			t.Fatalf("with explicit proxy image, expected BuildContainerImage error, got: %v", err)
		}
	})
}

func TestReadySPSProxyPod(t *testing.T) {
	const listCmd = `kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=sps-client -o jsonpath={range .items[*]}{.metadata.name}{" "}{.status.phase}{"\n"}{end} --request-timeout=10s`

	t.Run("returns empty until Running pod logs IFRT proxy server started", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{
			listCmd: {
				{ExitCode: 0, Stdout: "proxy-pod-0 Running\n"},
				{ExitCode: 0, Stdout: "proxy-pod-0 Running\n"},
			},
			"kubectl logs -n default proxy-pod-0 -c pathways-proxy --request-timeout=10s": {
				{ExitCode: 0, Stdout: "starting up...\n"},
				{ExitCode: 0, Stdout: "I1105 IFRT proxy server started on 29000\n"},
			},
		}))
		pod, err := orc.readySPSProxyPod("default", "jobset.sigs.k8s.io/jobset-name=sps-client")
		if err != nil || pod != "" {
			t.Fatalf("first poll = (%q, %v), want (\"\", nil)", pod, err)
		}
		pod, err = orc.readySPSProxyPod("default", "jobset.sigs.k8s.io/jobset-name=sps-client")
		if err != nil || pod != "proxy-pod-0" {
			t.Fatalf("second poll = (%q, %v), want (\"proxy-pod-0\", nil)", pod, err)
		}
	})

	t.Run("fails fast when proxy pod enters Failed", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{
			listCmd: {{ExitCode: 0, Stdout: "proxy-pod-0 Failed\n"}},
			"kubectl logs -n default proxy-pod-0 -c pathways-proxy --tail=50 --request-timeout=10s": {
				{ExitCode: 0, Stderr: "F1105 invalid flag --bad_arg\n"},
			},
		}))
		_, err := orc.readySPSProxyPod("default", "jobset.sigs.k8s.io/jobset-name=sps-client")
		if err == nil || !strings.Contains(err.Error(), "SPS proxy pod proxy-pod-0 exited (Failed)") || !strings.Contains(err.Error(), "invalid flag --bad_arg") {
			t.Errorf("got err = %v, want Failed proxy pod error with logs", err)
		}
	})
}

func TestValidateSPSClientManifest(t *testing.T) {
	const endpoint = "sps-j6080103-pathways-head-0-0.sps-j6080103:29001"

	t.Run("rejects worker replicatedJob from stale custom template", func(t *testing.T) {
		manifest := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: pathways-head
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-proxy
              args: ["--resource_manager_address=` + endpoint + `"]
  - name: worker
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-worker
`
		err := validateSPSClientManifest(manifest, endpoint)
		if err == nil || !strings.Contains(err.Error(), "has a worker replicatedJob") {
			t.Fatalf("validateSPSClientManifest() error = %v, want worker replicatedJob error", err)
		}
	})

	t.Run("rejects pathways-rm container in pathways-head", func(t *testing.T) {
		manifest := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: pathways-head
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-rm
            - name: pathways-proxy
              args: ["--resource_manager_address=` + endpoint + `"]
`
		err := validateSPSClientManifest(manifest, endpoint)
		if err == nil || !strings.Contains(err.Error(), `runs container "pathways-rm"`) {
			t.Fatalf("validateSPSClientManifest() error = %v, want pathways-rm error", err)
		}
	})

	t.Run("rejects pathways-proxy not targeting endpoint", func(t *testing.T) {
		manifest := `apiVersion: jobset.x-k8s.io/v1alpha2
kind: JobSet
spec:
  replicatedJobs:
  - name: pathways-head
    template:
      spec:
        template:
          spec:
            containers:
            - name: pathways-proxy
              args: ["--resource_manager_address=$(PATHWAYS_HEAD):29001"]
`
		err := validateSPSClientManifest(manifest, endpoint)
		if err == nil || !strings.Contains(err.Error(), "does not target") {
			t.Fatalf("validateSPSClientManifest() error = %v, want endpoint mismatch error", err)
		}
	})
}

func TestDeleteSPSClient_PassesTimeout(t *testing.T) {
	mockExec := NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl delete jobset sps-client -n custom-ns --ignore-not-found --wait=false --request-timeout=10s": {{ExitCode: 0}},
	})
	orc := newTestGKEOrchestrator(mockExec)
	orc.deleteSPSClient("sps-client", "custom-ns")
	if len(mockExec.calls) != 1 {
		t.Fatalf("deleteSPSClient calls = %d, want 1", len(mockExec.calls))
	}
}

func TestResolveSPSClientPathways(t *testing.T) {
	baseInst := spsInstance{
		Name:         "sps-v5p",
		HeadNodePool: "cpu-np",
		ServerImage:  "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest",
		ProxyImage:   "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest",
		GCSLocation:  "gs://my-bucket/sps",
	}

	t.Run("fails when HeadNodePool is empty", func(t *testing.T) {
		inst := baseInst
		inst.HeadNodePool = ""
		_, err := resolveSPSClientPathways(orchestrator.PathwaysJobDefinition{}, inst)
		if err == nil || !strings.Contains(err.Error(), "pass --pathways-head-np") {
			t.Errorf("got err = %v, want --pathways-head-np error", err)
		}
	})

	t.Run("fails when ProxyServerImage is empty", func(t *testing.T) {
		inst := baseInst
		inst.ServerImage = "custom-registry.io/my-rm:v1"
		inst.ProxyImage = ""
		_, err := resolveSPSClientPathways(orchestrator.PathwaysJobDefinition{}, inst)
		if err == nil || !strings.Contains(err.Error(), "pass --pathways-proxy-server-image") || strings.Contains(err.Error(), "different digests") {
			t.Errorf("got err = %v, want generic --pathways-proxy-server-image error (without digest text)", err)
		}
	})

	t.Run("fails when GCSLocation is empty", func(t *testing.T) {
		inst := baseInst
		inst.GCSLocation = ""
		_, err := resolveSPSClientPathways(orchestrator.PathwaysJobDefinition{}, inst)
		if err == nil || !strings.Contains(err.Error(), "pass --pathways-gcs-location") {
			t.Errorf("got err = %v, want --pathways-gcs-location error", err)
		}
	})
}

func TestResolveSPSClientPathways_DigestPinnedServer(t *testing.T) {
	for _, srv := range []string{
		"us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server@sha256:abcdef",
		"us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest@sha256:abcdef",
	} {
		inst := spsInstance{
			Name:         "sps-inst",
			HeadNodePool: "cpu-np",
			ServerImage:  srv,
			ProxyImage:   proxyImageFor(srv),
			GCSLocation:  "gs://my-bucket/sps",
		}
		_, err := resolveSPSClientPathways(orchestrator.PathwaysJobDefinition{}, inst)
		if err == nil || !strings.Contains(err.Error(), "different digests") || !strings.Contains(err.Error(), "pass --pathways-proxy-server-image") {
			t.Errorf("server %q: got err = %v, want a digest explanation and a --pathways-proxy-server-image hint", srv, err)
		}
		got, err := resolveSPSClientPathways(orchestrator.PathwaysJobDefinition{ProxyServerImage: "proxy:pinned"}, inst)
		if err != nil || got.ProxyServerImage != "proxy:pinned" {
			t.Errorf("server %q with --pathways-proxy-server-image: got (%q, %v), want (\"proxy:pinned\", nil)", srv, got.ProxyServerImage, err)
		}
	}
}

func TestRunSPSWorkload_StripsQueueWhenKueueNotInstalled(t *testing.T) {
	cases := []struct {
		name             string
		jobQueue         string
		kueueAPIRes      shell.CommandResult
		wantQueueSubstr  string
		wantOmitQueue    bool
		wantKueueEnabled bool
	}{
		{
			name:             "explicit_queue_stripped_when_kueue_absent",
			jobQueue:         "my-queue",
			kueueAPIRes:      shell.CommandResult{ExitCode: 1, Stderr: "Error from server (NotFound): not found"},
			wantOmitQueue:    true,
			wantKueueEnabled: false,
		},
		{
			name:             "inherited_instance_queue_stripped_when_kueue_absent",
			jobQueue:         "",
			kueueAPIRes:      shell.CommandResult{ExitCode: 1, Stderr: "Error from server (NotFound): not found"},
			wantOmitQueue:    true,
			wantKueueEnabled: false,
		},
		{
			name:             "whitespace_queue_inherits_instance_queue_when_kueue_installed",
			jobQueue:         "   ",
			kueueAPIRes:      shell.CommandResult{ExitCode: 0, Stdout: "{}"},
			wantQueueSubstr:  "kueue.x-k8s.io/queue-name: cluster-queue",
			wantKueueEnabled: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dryOut := filepath.Join(t.TempDir(), "sps-client.yaml")
			mockExec := NewMockExecutor(map[string][]shell.CommandResult{
				"gcloud container clusters get-credentials": {{ExitCode: 0}},
				"kubectl get jobset sps-j6080103 -n default -o json --request-timeout=10s": {
					{ExitCode: 0, Stdout: testSPSJobSetJSON},
				},
				"kubectl get pods -n default -l jobset.sigs.k8s.io/jobset-name=sps-j6080103,jobset.sigs.k8s.io/replicatedjob-name=pathways-head -o jsonpath={.items[*].status.phase} --request-timeout=10s": {
					{ExitCode: 0, Stdout: "Running"},
				},
				"kubectl get --raw /apis/kueue.x-k8s.io": {tc.kueueAPIRes},
			})
			orc := newTestGKEOrchestrator(mockExec)
			orc.kubeconfigLoader = staticLoader(kubeconfigWith("https://10.0.0.1:6443", nil, gkeExec(), "gke_test-project_us-central1-a_test-cluster"))
			job := orchestrator.JobDefinition{
				WorkloadName:    "sps-client-unqueued",
				ClusterName:     "test-cluster",
				ClusterLocation: "us-central1-a",
				ProjectID:       "test-project",
				ImageName:       "gcr.io/proj/img:v1",
				CommandToRun:    "python3 train.py",
				KueueQueueName:  tc.jobQueue,
				NumSlices:       1,
				IsPathwaysJob:   true,
				DryRunManifest:  dryOut,
				Pathways:        orchestrator.PathwaysJobDefinition{RunSPSWorkload: "sps-j6080103"},
			}
			if err := orc.runSPSWorkload(job); err != nil {
				t.Fatalf("runSPSWorkload() err = %v", err)
			}
			raw, err := os.ReadFile(dryOut)
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantOmitQueue && strings.Contains(string(raw), "kueue.x-k8s.io/queue-name") {
				t.Errorf("SPS client manifest must omit queue-name:\n%s", string(raw))
			}
			if tc.wantQueueSubstr != "" && !strings.Contains(string(raw), tc.wantQueueSubstr) {
				t.Errorf("SPS client manifest missing %q:\n%s", tc.wantQueueSubstr, string(raw))
			}
			if orc.kueueEnabled() != tc.wantKueueEnabled {
				t.Errorf("orc.kueueEnabled() = %v, want %v", orc.kueueEnabled(), tc.wantKueueEnabled)
			}
		})
	}
}
