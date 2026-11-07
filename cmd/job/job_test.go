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

package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func writeTestKubeconfigFile(t *testing.T, ctxName, clusterEntry, server string) string {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[clusterEntry] = &clientcmdapi.Cluster{Server: server}
	cfg.AuthInfos["admin"] = &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")}
	cfg.Contexts[ctxName] = &clientcmdapi.Context{Cluster: clusterEntry, AuthInfo: "admin", Namespace: "ml-team"}
	cfg.CurrentContext = ctxName
	raw, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(p, raw, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestJobCmd_TypeK8sRequiresOnlyKubeconfigAndZeroGCloud(t *testing.T) {
	setupSubmitTestEnv(t)
	t.Setenv("HOME", t.TempDir())
	kcPath := writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443")
	t.Setenv("KUBECONFIG", kcPath)
	useFileStore(t)

	var executed []string
	origExec := shell.ExecuteCommandWithTimeout
	t.Cleanup(func() { shell.ExecuteCommandWithTimeout = origExec })
	shell.ExecuteCommandWithTimeout = func(_ time.Duration, name string, args ...string) shell.CommandResult {
		call := strings.Join(append([]string{name}, args...), " ")
		executed = append(executed, call)
		if name != "kubectl" {
			t.Errorf("unexpected non-kubectl prerequisite binary %q invoked in TypeK8s mode: %s", name, call)
			return shell.CommandResult{ExitCode: 99, Stderr: "forbidden binary"}
		}
		return shell.CommandResult{ExitCode: 0, Stdout: `{"clientVersion":{"gitVersion":"v1.32.0"}}`}
	}

	origADC := getADCSetupCommandFunc
	t.Cleanup(func() { getADCSetupCommandFunc = origADC })
	getADCSetupCommandFunc = func() string {
		t.Errorf("getADCSetupCommandFunc must not run in TypeK8s mode")
		return "gcloud auth application-default login"
	}

	var gotListCluster string
	mockOrc := &mockOrchestrator{
		listJobsFunc: func(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
			gotListCluster = opts.ClusterName
			return nil, nil
		},
	}
	origFactory := gkeOrchestratorFactory
	t.Cleanup(func() { gkeOrchestratorFactory = origFactory })
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

	if _, err := executeCommand(JobCmd, "list"); err != nil {
		t.Fatalf("gcluster job list in TypeK8s mode failed: %v", err)
	}
	if len(executed) != 1 || !strings.HasPrefix(executed[0], "kubectl version --client") {
		t.Errorf("executed prereq commands = %v, want only [kubectl version --client ...]", executed)
	}
	if gotListCluster != "onprem-k8s" || mockOrc.InitClusterName != "onprem-k8s" || mockOrc.InitLocation != "" || mockOrc.InitProjectID != "" {
		t.Errorf("got list=%q init=(%q,%q,%q), want onprem-k8s with empty location/project", gotListCluster, mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID)
	}
}

func TestJobCmd_MissingContextAndRequiredGKEFlags(t *testing.T) {
	t.Run("empty KUBECONFIG and zero flags returns actionable KUBECONFIG/GKE guidance", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
		if _, err := executeCommand(JobCmd, "list"); err == nil || !strings.Contains(err.Error(), "KUBECONFIG") {
			t.Fatalf("expected actionable KUBECONFIG error, got: %v", err)
		}
	})

	t.Run("unknown --cluster without -l/-p returns actionable context error", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Setenv("HOME", t.TempDir())
		t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
		if _, err := executeCommand(JobCmd, "list", "--cluster", "unknown-ctx"); err == nil || !strings.Contains(err.Error(), "unknown-ctx") {
			t.Fatalf("expected unknown-ctx error, got: %v", err)
		}
	})

	t.Run("partial GKE flags preserve required flag error messages", func(t *testing.T) {
		for _, tc := range []struct {
			args []string
			want string
		}{
			{args: []string{"list", "--project", "my-proj", "--location", "us-central1"}, want: "cluster name is required"},
			{args: []string{"list", "--cluster", "my-clus", "--project", "my-proj"}, want: "location is required"},
			{args: []string{"list", "--cluster", "my-clus", "--location", "us-central1"}, want: "project ID is required"},
		} {
			setupSubmitTestEnv(t)
			t.Setenv("HOME", t.TempDir())
			t.Setenv("KUBECONFIG", filepath.Join(t.TempDir(), "missing"))
			if _, err := executeCommand(JobCmd, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("args %v: got err=%v, want substring %q", tc.args, err, tc.want)
			}
		}
	})

	t.Run("invalid GCLUSTER_ORCHESTRATOR fails fast", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Setenv("GCLUSTER_ORCHESTRATOR", "bogus")
		if _, err := executeCommand(JobCmd, "list"); err == nil || !strings.Contains(err.Error(), "GCLUSTER_ORCHESTRATOR") {
			t.Fatalf("expected GCLUSTER_ORCHESTRATOR error, got: %v", err)
		}
	})
}

func TestJobCmd_MissingClusterNotMaskedByActiveGKEContext(t *testing.T) {
	setupSubmitTestEnv(t)
	t.Setenv("HOME", t.TempDir())
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["gke_my-proj_us-central1_existing"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
	cfg.AuthInfos["gke-user"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}
	cfg.Contexts["gke_my-proj_us-central1_existing"] = &clientcmdapi.Context{
		Cluster:  "gke_my-proj_us-central1_existing",
		AuthInfo: "gke-user",
	}
	cfg.CurrentContext = "gke_my-proj_us-central1_existing"
	raw, err := clientcmd.Write(*cfg)
	if err != nil {
		t.Fatal(err)
	}
	kcPath := filepath.Join(t.TempDir(), "kubeconfig")
	if err := os.WriteFile(kcPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kcPath)

	if _, err := executeCommand(JobCmd, "list", "--project", "my-proj", "--location", "us-central1"); err == nil || !strings.Contains(err.Error(), "cluster name is required") {
		t.Fatalf("expected 'cluster name is required' when --cluster is omitted in GKE mode, got: %v", err)
	}
}

func TestJobCmd_TypeK8sSubmitPrebuiltImageZeroFlags(t *testing.T) {
	setupSubmitTestEnv(t)
	t.Setenv("HOME", t.TempDir())
	kcPath := writeTestKubeconfigFile(t, "admin@k8s-local", "k8s-local", "https://10.128.0.68:6443")
	t.Setenv("KUBECONFIG", kcPath)
	useFileStore(t)

	origExec := shell.ExecuteCommandWithTimeout
	t.Cleanup(func() { shell.ExecuteCommandWithTimeout = origExec })
	shell.ExecuteCommandWithTimeout = func(_ time.Duration, name string, args ...string) shell.CommandResult {
		if name != "kubectl" {
			t.Errorf("unexpected non-kubectl binary %q invoked in TypeK8s submit", name)
			return shell.CommandResult{ExitCode: 99, Stderr: "forbidden binary"}
		}
		return shell.CommandResult{ExitCode: 0, Stdout: `{"clientVersion":{"gitVersion":"v1.32.0"}}`}
	}

	mockOrc := &mockOrchestrator{}
	origFactory := gkeOrchestratorFactory
	t.Cleanup(func() { gkeOrchestratorFactory = origFactory })
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "k8s-prebuilt",
		"--image", "10.128.0.68:5000/jax:v5p",
		"--command", "python3 train.py",
		"--compute-type", "v5p-8",
	)
	if err != nil {
		t.Fatalf("TypeK8s zero-flag prebuilt submit failed: %v", err)
	}
	if mockOrc.SubmittedJob.WorkloadName != "k8s-prebuilt" || mockOrc.SubmittedJob.ClusterName != "k8s-local" {
		t.Errorf("unexpected SubmittedJob: %+v", mockOrc.SubmittedJob)
	}
}

func writeSavedGKEContext(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	if err := saveContext(Context{
		ProjectID:   "saved-gke-proj",
		ClusterName: "saved-gke-cluster",
		Location:    "us-central1-c",
	}); err != nil {
		t.Fatalf("saveContext: %v", err)
	}
}

func TestJobCmd_SavedGKEConfigDoesNotBreakTypeK8sOrEnvOverride(t *testing.T) {
	tests := []struct {
		name         string
		envOverride  string
		withK8sCfg   bool
		args         []string
		wantCluster  string
		wantLocation string
		wantProject  string
	}{
		{
			name:        "GCLUSTER_ORCHESTRATOR=k8s ignores saved GKE context and uses KUBECONFIG current-context",
			envOverride: "k8s",
			withK8sCfg:  true,
			args:        []string{"list"},
			wantCluster: "onprem-k8s",
		},
		{
			name:        "explicit --cluster targeting TypeK8s clears non-explicit saved GKE project and location",
			withK8sCfg:  true,
			args:        []string{"list", "--cluster", "onprem-k8s"},
			wantCluster: "onprem-k8s",
		},
		{
			name:         "explicit --project and --location flags are preserved in TypeK8s mode",
			withK8sCfg:   true,
			args:         []string{"list", "--cluster", "onprem-k8s", "--location", "europe-north1", "--project", "explicit-gar-proj"},
			wantCluster:  "onprem-k8s",
			wantLocation: "europe-north1",
			wantProject:  "explicit-gar-proj",
		},
		{
			name:         "zero flags without GCLUSTER_ORCHESTRATOR preserves saved GKE context defaults",
			args:         []string{"list"},
			wantCluster:  "saved-gke-cluster",
			wantLocation: "us-central1-c",
			wantProject:  "saved-gke-proj",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			writeSavedGKEContext(t, t.TempDir())
			if tc.withK8sCfg {
				t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443"))
			}
			if tc.envOverride != "" {
				t.Setenv("GCLUSTER_ORCHESTRATOR", tc.envOverride)
			}

			oldStore := store
			t.Cleanup(func() { store = oldStore })
			store = &MockPrereqStore{
				State: PrereqState{
					LastCheckedTimestamp:         time.Now(),
					LastCheckedProjectID:         "saved-gke-proj",
					GCloudSDKInstalled:           true,
					GCloudProjectConfigured:      true,
					GCloudAuthenticated:          true,
					ADCConfigured:                true,
					KubectlInstalled:             true,
					GKEGCloudAuthPluginInstalled: true,
				},
			}

			mockOrc := &mockOrchestrator{}
			origFactory := gkeOrchestratorFactory
			t.Cleanup(func() { gkeOrchestratorFactory = origFactory })
			gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

			if _, err := executeCommand(JobCmd, tc.args...); err != nil {
				t.Fatalf("executeCommand(%v) failed: %v", tc.args, err)
			}
			if mockOrc.InitClusterName != tc.wantCluster || mockOrc.InitLocation != tc.wantLocation || mockOrc.InitProjectID != tc.wantProject {
				t.Errorf("init = (%q, %q, %q), want (%q, %q, %q)",
					mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID,
					tc.wantCluster, tc.wantLocation, tc.wantProject)
			}
		})
	}
}
