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

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"
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
			serverImage: "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server@sha256:abcdef",
			want:        "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server@sha256:abcdef",
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

func TestGenerateSPSClientManifest(t *testing.T) {
	orc := NewGKEOrchestrator()
	inst := spsInstance{
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

	t.Run("in-cluster mode renders proxy initContainer and workload-container", func(t *testing.T) {
		job := orchestrator.JobDefinition{
			WorkloadName:            "sps-client-cluster",
			NumSlices:               2,
			TtlSecondsAfterFinished: 3600,
			CommandToRun:            "python3 train.py",
			Pathways: orchestrator.PathwaysJobDefinition{
				RunSPSWorkload: "sps-j6080103",
			},
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
			Pathways: orchestrator.PathwaysJobDefinition{
				RunSPSWorkload: "sps-j6080103",
			},
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
			Pathways: orchestrator.PathwaysJobDefinition{
				RunSPSWorkload: "sps-j6080103",
			},
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
			},
			nil,
		)
	})
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

func TestReadSPSInstance_RejectsNonRunningHeadPod(t *testing.T) {
	orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{
		"kubectl get jobset": {{ExitCode: 0, Stdout: testSPSJobSetJSON}},
		"kubectl get pods":   {{ExitCode: 0, Stdout: "Pending"}},
	}))
	_, err := orc.readSPSInstance("sps-j6080103", "default")
	if err == nil || !strings.Contains(err.Error(), `SPS instance "sps-j6080103" has no running pathways-head pod`) {
		t.Errorf("got err = %v, want no running pathways-head pod error", err)
	}
}

func TestRunSPSWorkload_Validation(t *testing.T) {
	baseResponses := func() map[string][]shell.CommandResult {
		return map[string][]shell.CommandResult{
			"gcloud container clusters get-credentials": {{ExitCode: 0}},
			"kubectl get jobset":                        {{ExitCode: 0, Stdout: testSPSJobSetJSON}},
			"kubectl get pods":                          {{ExitCode: 0, Stdout: "Running"}},
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

func TestReadySPSProxyPod(t *testing.T) {
	t.Run("returns empty until Running pod logs IFRT proxy server started", func(t *testing.T) {
		orc := newTestGKEOrchestrator(NewMockExecutor(map[string][]shell.CommandResult{
			"kubectl get pods": {
				{ExitCode: 0, Stdout: "proxy-pod-0 Running\n"},
				{ExitCode: 0, Stdout: "proxy-pod-0 Running\n"},
			},
			"kubectl logs": {
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
			"kubectl get pods": {{ExitCode: 0, Stdout: "proxy-pod-0 Failed\n"}},
			"kubectl logs":     {{ExitCode: 0, Stderr: "F1105 invalid flag --bad_arg\n"}},
		}))
		_, err := orc.readySPSProxyPod("default", "jobset.sigs.k8s.io/jobset-name=sps-client")
		if err == nil || !strings.Contains(err.Error(), "SPS proxy pod proxy-pod-0 exited (Failed)") || !strings.Contains(err.Error(), "invalid flag --bad_arg") {
			t.Errorf("got err = %v, want Failed proxy pod error with logs", err)
		}
	})
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
		inst.ProxyImage = ""
		_, err := resolveSPSClientPathways(orchestrator.PathwaysJobDefinition{}, inst)
		if err == nil || !strings.Contains(err.Error(), "pass --pathways-proxy-server-image") {
			t.Errorf("got err = %v, want --pathways-proxy-server-image error", err)
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
