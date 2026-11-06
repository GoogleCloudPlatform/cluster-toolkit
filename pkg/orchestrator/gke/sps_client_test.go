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
)

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
	t.Run("parses SPS JobSet with sidecar, space-separated RM args, and Kueue label", func(t *testing.T) {
		raw := `{
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
							"containers": [{"name": "pathways-worker", "image": "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:20261004"}]
						}}}}
					}
				]
			}
		}`

		inst, err := parseSPSClientTarget("sps-j6080103", []byte(raw))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if inst.KueueQueueName != "cluster-queue" {
			t.Errorf("KueueQueueName = %q, want %q", inst.KueueQueueName, "cluster-queue")
		}
		if inst.HeadNodePool != "cpu-np" {
			t.Errorf("HeadNodePool = %q, want %q", inst.HeadNodePool, "cpu-np")
		}
		if inst.GCSLocation != "gs://my-bucket/sps" {
			t.Errorf("GCSLocation = %q, want %q", inst.GCSLocation, "gs://my-bucket/sps")
		}
		if inst.InstanceType != "tpu7x:2x2x1" {
			t.Errorf("InstanceType = %q, want %q", inst.InstanceType, "tpu7x:2x2x1")
		}
		if inst.InstanceCount != 2 {
			t.Errorf("InstanceCount = %d, want 2", inst.InstanceCount)
		}
		if !inst.HasSidecar {
			t.Errorf("HasSidecar = false, want true")
		}
	})

	t.Run("rejects regular Pathways JobSet running pathways-proxy", func(t *testing.T) {
		raw := `{
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
		}`
		_, err := parseSPSClientTarget("regular-pw", []byte(raw))
		if err == nil || !strings.Contains(err.Error(), "its pathways-head runs pathways-proxy") {
			t.Errorf("got err = %v, want pathways-proxy rejection", err)
		}
	})

	t.Run("rejects non-Pathways JobSet", func(t *testing.T) {
		raw := `{"spec": {"replicatedJobs": [{"name": "main-job"}]}}`
		_, err := parseSPSClientTarget("plain-job", []byte(raw))
		if err == nil || !strings.Contains(err.Error(), "needs a pathways-head job running pathways-rm") {
			t.Errorf("got err = %v, want non-SPS rejection", err)
		}
	})
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
		for _, want := range []string{
			"name: sps-client-cluster",
			"cloud.google.com/gke-nodepool: cpu-np",
			"name: pathways-proxy",
			`"--resource_manager_address=sps-j6080103-pathways-head-0-0.sps-j6080103:29001"`,
			"--virtual_slices=tpu7x:2x2x1,tpu7x:2x2x1",
			"--sidecar_name=external",
			"name: workload-container",
			"value: grpc://localhost:29000",
		} {
			if !strings.Contains(manifest, want) {
				t.Errorf("manifest missing %q:\n%s", want, manifest)
			}
		}
		for _, unwanted := range []string{
			"kueue.x-k8s.io/queue-name",
			"name: pathways-rm",
			"name: pathways-worker",
		} {
			if strings.Contains(manifest, unwanted) {
				t.Errorf("manifest unexpectedly contains %q:\n%s", unwanted, manifest)
			}
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
		for _, want := range []string{
			"kueue.x-k8s.io/queue-name: cluster-queue",
			"name: pathways-proxy",
			"--virtual_slices=tpu7x:2x2x1",
			"--sidecar_name=external",
		} {
			if !strings.Contains(manifest, want) {
				t.Errorf("manifest missing %q:\n%s", want, manifest)
			}
		}
		if strings.Contains(manifest, "workload-container") {
			t.Errorf("terminal-mode manifest should not contain workload-container:\n%s", manifest)
		}
	})
}
