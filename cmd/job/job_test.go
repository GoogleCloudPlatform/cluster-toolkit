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
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

func writeKubeconfig(t *testing.T, cfg *clientcmdapi.Config) string {
	t.Helper()
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

func writeTestKubeconfigFile(t *testing.T, ctxName, clusterEntry, server string) string {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters[clusterEntry] = &clientcmdapi.Cluster{Server: server}
	cfg.AuthInfos["admin"] = &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")}
	cfg.Contexts[ctxName] = &clientcmdapi.Context{Cluster: clusterEntry, AuthInfo: "admin", Namespace: "ml-team"}
	cfg.CurrentContext = ctxName
	return writeKubeconfig(t, cfg)
}

func stubUnexpectedSubprocesses(t *testing.T, allowKubectl bool) *[]string {
	t.Helper()
	executed := new([]string)
	origExec := shell.ExecuteCommandWithTimeout
	t.Cleanup(func() { shell.ExecuteCommandWithTimeout = origExec })
	shell.ExecuteCommandWithTimeout = func(_ time.Duration, name string, args ...string) shell.CommandResult {
		call := strings.Join(append([]string{name}, args...), " ")
		*executed = append(*executed, call)
		if !allowKubectl || name != "kubectl" {
			t.Errorf("unexpected subprocess %q invoked: %s", name, call)
			return shell.CommandResult{ExitCode: 99, Stderr: "forbidden binary"}
		}
		return shell.CommandResult{ExitCode: 0, Stdout: `{"clientVersion":{"gitVersion":"v1.32.0"}}`}
	}
	return executed
}

func TestJobCmd_TypeK8sRequiresOnlyKubeconfigAndZeroGCloud(t *testing.T) {
	setupSubmitTestEnv(t)
	kcPath := writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443")
	t.Setenv("KUBECONFIG", kcPath)
	useFileStore(t)

	executed := stubUnexpectedSubprocesses(t, true)
	origADC := getADCSetupCommandFunc
	t.Cleanup(func() { getADCSetupCommandFunc = origADC })
	getADCSetupCommandFunc = func() string {
		t.Errorf("getADCSetupCommandFunc must not run in TypeK8s mode")
		return "gcloud auth application-default login"
	}

	var gotListCluster, gotLogsJob, gotLogsCluster, gotCancelJob, gotCancelCluster, gotInspectCluster string
	mockOrc := &mockOrchestrator{
		listJobsFunc: func(opts orchestrator.ListOptions) ([]orchestrator.JobStatus, error) {
			gotListCluster = opts.ClusterName
			return nil, nil
		},
		getJobLogsFunc: func(name string, opts orchestrator.LogsOptions) (string, error) {
			gotLogsJob, gotLogsCluster = name, opts.ClusterName
			return "ok\n", nil
		},
		cancelJobFunc: func(name string, opts orchestrator.CancelOptions) error {
			gotCancelJob, gotCancelCluster = name, opts.ClusterName
			return nil
		},
		inspectClusterFunc: func(opts orchestrator.InspectOptions) error {
			gotInspectCluster = opts.ClusterName
			return nil
		},
	}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

	for _, cmdArgs := range [][]string{{"list"}, {"logs", "job-1"}, {"cancel", "job-1"}, {"inspect"}} {
		if _, err := executeCommand(JobCmd, cmdArgs...); err != nil {
			t.Fatalf("executeCommand(%v) error = %v, want nil", cmdArgs, err)
		}
	}
	if len(*executed) != 1 || !strings.HasPrefix((*executed)[0], "kubectl version --client") {
		t.Errorf("executed prereq commands = %v, want cached single [kubectl version --client ...]", *executed)
	}
	assertTypeK8sDispatchedTargets(t, mockOrc, gotListCluster, gotLogsJob, gotLogsCluster, gotCancelJob, gotCancelCluster, gotInspectCluster)
}

func assertTypeK8sDispatchedTargets(t *testing.T, mockOrc *mockOrchestrator, gotList, gotLogsJob, gotLogsCluster, gotCancelJob, gotCancelCluster, gotInspect string) {
	t.Helper()
	if mockOrc.InitClusterName != "onprem-k8s" || mockOrc.InitLocation != "" || mockOrc.InitProjectID != "" {
		t.Errorf("init = (%q,%q,%q), want (\"onprem-k8s\",\"\",\"\")", mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID)
	}
	for label, got := range map[string]string{
		"list.ClusterName":    gotList,
		"logs.ClusterName":    gotLogsCluster,
		"cancel.ClusterName":  gotCancelCluster,
		"inspect.ClusterName": gotInspect,
	} {
		if got != "onprem-k8s" {
			t.Errorf("%s = %q, want \"onprem-k8s\"", label, got)
		}
	}
	if gotLogsJob != "job-1" || gotCancelJob != "job-1" {
		t.Errorf("dispatched job names = (logs=%q, cancel=%q), want (\"job-1\", \"job-1\")", gotLogsJob, gotCancelJob)
	}
}

func TestJobCmd_RealRegistryWiringOnList(t *testing.T) {
	setupSubmitTestEnv(t)
	var apiHits atomic.Int32
	var gotPath, gotAuth atomic.Value
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiHits.Add(1)
		gotPath.Store(r.URL.Path)
		gotAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apiVersion":"jobset.x-k8s.io/v1alpha2","kind":"JobSetList","items":[]}`))
	}))
	t.Cleanup(srv.Close)

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["unrelated-clus"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:1"}
	cfg.Contexts["unrelated-ctx"] = &clientcmdapi.Context{Cluster: "unrelated-clus", Namespace: "wrong-ns"}
	cfg.Clusters["onprem-k8s"] = &clientcmdapi.Cluster{Server: srv.URL, InsecureSkipTLSVerify: true}
	cfg.AuthInfos["sa"] = &clientcmdapi.AuthInfo{Token: "sa-jwt"}
	cfg.Contexts["admin@onprem-k8s"] = &clientcmdapi.Context{Cluster: "onprem-k8s", AuthInfo: "sa", Namespace: "ml-team"}
	cfg.CurrentContext = "unrelated-ctx"
	t.Setenv("KUBECONFIG", writeKubeconfig(t, cfg))
	t.Setenv(shell.AirGappedEnvVar, "true")

	useFileStore(t)
	stubUnexpectedSubprocesses(t, true)
	gkeOrchestratorFactory = nil

	if _, err := executeCommand(JobCmd, "list", "--cluster", "onprem-k8s"); err != nil {
		t.Fatalf("executeCommand(list --cluster onprem-k8s) via real provider registry error = %v, want nil", err)
	}
	if got := apiHits.Load(); got != 1 {
		t.Errorf("JobSet API list hits = %d, want 1", got)
	}
	if p, _ := gotPath.Load().(string); p != "/apis/jobset.x-k8s.io/v1alpha2/namespaces/ml-team/jobsets" {
		t.Errorf("request path = %q, want /apis/jobset.x-k8s.io/v1alpha2/namespaces/ml-team/jobsets", p)
	}
	if a, _ := gotAuth.Load().(string); a != "Bearer sa-jwt" {
		t.Errorf("Authorization header = %q, want \"Bearer sa-jwt\"", a)
	}
}

func TestJobCmd_TypeK8sSubmitFastFailsBeforePrereqsAndLookup(t *testing.T) {
	tests := []struct {
		name        string
		emptyConfig bool
		envOverride string
		args        func(t *testing.T) []string
	}{
		{
			name: "prebuilt_image_submit_rejected_in_k8s_mode",
			args: func(*testing.T) []string {
				return []string{
					"submit",
					"--name", "k8s-prebuilt",
					"--image", "10.128.0.68:5000/jax:v5p",
					"--command", "python3 train.py",
					"--compute-type", "v5p-8",
				}
			},
		},
		{
			name: "build_context_submit_rejected_before_submit_prerun",
			args: func(t *testing.T) []string {
				return []string{
					"submit",
					"--cluster", "k8s-local",
					"--project", "ar-proj",
					"--location", "us-central1",
					"--name", "k8s-build",
					"--base-image", "python:3.11-slim",
					"--build-context", t.TempDir(),
					"--command", "python3 train.py",
					"--compute-type", "n2-standard-4",
				}
			},
		},
		{
			name:        "forced_k8s_env_on_empty_kubeconfig_rejects_submit",
			emptyConfig: true,
			envOverride: "k8s",
			args: func(*testing.T) []string {
				return []string{
					"submit",
					"--name", "k8s-forced",
					"--image", "busybox",
					"--command", "echo hi",
					"--compute-type", "n2-standard-4",
				}
			},
		},
	}

	wantErr := orchestrator.UnsupportedK8sSubmitError().Error()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			if !tc.emptyConfig {
				t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@k8s-local", "k8s-local", "https://10.128.0.68:6443"))
			}
			if tc.envOverride != "" {
				t.Setenv(orchestrator.EnvVar, tc.envOverride)
			}
			useFileStore(t)
			executed := stubUnexpectedSubprocesses(t, false)
			lookupArtifactRegistryRepos = func(context.Context, string, string) ([]string, bool) {
				t.Errorf("lookupArtifactRegistryRepos must not run when submit is rejected in TypeK8s mode")
				return nil, false
			}
			gkeOrchestratorFactory = nil

			_, err := executeCommand(JobCmd, tc.args(t)...)
			if err == nil || err.Error() != wantErr {
				t.Fatalf("executeCommand(submit) err = %v, want %q", err, wantErr)
			}
			if len(*executed) != 0 {
				t.Errorf("executed subprocesses = %v, want 0", *executed)
			}
		})
	}
}

func TestJobCmd_MissingContextAndRequiredGKEFlags(t *testing.T) {
	tests := []struct {
		name        string
		envOverride string
		cfg         func() *clientcmdapi.Config
		args        []string
		wantSubstr  []string
		forbid      string
	}{
		{
			name:       "empty_kubeconfig_and_zero_flags",
			args:       []string{"list"},
			wantSubstr: []string{"no Kubernetes context found in KUBECONFIG"},
		},
		{
			name: "dangling_current_context_names_missing_context",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.CurrentContext = "deleted-ctx"
				return c
			},
			args:       []string{"list"},
			wantSubstr: []string{`"deleted-ctx"`, "was not found"},
		},
		{
			name:        "forced_k8s_empty_kubeconfig_omits_gke_flag_advice",
			envOverride: "k8s",
			args:        []string{"list"},
			wantSubstr:  []string{"no Kubernetes current-context is configured in KUBECONFIG"},
			forbid:      "--location",
		},
		{
			name:        "forced_k8s_mode_missing_context_omits_gke_flag_advice",
			envOverride: "k8s",
			args:        []string{"list", "--cluster", "unknown-ctx"},
			wantSubstr:  []string{`"unknown-ctx"`},
			forbid:      "--location",
		},
		{
			name:       "unforced_missing_cluster_mentions_k8s_and_gke_advice",
			args:       []string{"list", "--cluster", "unknown-ctx"},
			wantSubstr: []string{`"unknown-ctx"`, "--location and --project"},
		},
		{
			name:       "fresh_workstation_submit_missing_flags_reports_gke_required_flags",
			args:       []string{"submit", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"},
			wantSubstr: []string{"cluster name is required"},
			forbid:     "not yet supported",
		},
		{
			name: "gke_context_missing_flags_on_submit_omits_k8s_override_hint",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.Clusters["gke_my-proj_us-central1_existing"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
				c.AuthInfos["gke-user"] = &clientcmdapi.AuthInfo{Token: "static-token"}
				c.Contexts["gke_my-proj_us-central1_existing"] = &clientcmdapi.Context{
					Cluster:  "gke_my-proj_us-central1_existing",
					AuthInfo: "gke-user",
				}
				c.CurrentContext = "gke_my-proj_us-central1_existing"
				return c
			},
			args:       []string{"submit", "--cluster", "gke_my-proj_us-central1_existing", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"},
			wantSubstr: []string{"location is required"},
			forbid:     "GCLUSTER_ORCHESTRATOR=k8s",
		},
		{
			name:        "forced_gke_mode_missing_flags_omits_k8s_override_hint",
			envOverride: "gke",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.Clusters["gke_my-proj_us-central1_existing"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
				c.AuthInfos["gke-user"] = &clientcmdapi.AuthInfo{Token: "static-token"}
				c.Contexts["gke_my-proj_us-central1_existing"] = &clientcmdapi.Context{
					Cluster:  "gke_my-proj_us-central1_existing",
					AuthInfo: "gke-user",
				}
				c.CurrentContext = "gke_my-proj_us-central1_existing"
				return c
			},
			args:       []string{"list", "--cluster", "gke_my-proj_us-central1_existing"},
			wantSubstr: []string{"location is required"},
			forbid:     "GCLUSTER_ORCHESTRATOR=k8s",
		},
		{
			name: "gke_context_missing_flags_mentions_k8s_override_hint",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.Clusters["gke_my-proj_us-central1_existing"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
				c.AuthInfos["gke-user"] = &clientcmdapi.AuthInfo{Token: "static-token"}
				c.Contexts["gke_my-proj_us-central1_existing"] = &clientcmdapi.Context{
					Cluster:  "gke_my-proj_us-central1_existing",
					AuthInfo: "gke-user",
				}
				c.CurrentContext = "gke_my-proj_us-central1_existing"
				return c
			},
			args:       []string{"list", "--cluster", "gke_my-proj_us-central1_existing"},
			wantSubstr: []string{"location is required", "GCLUSTER_ORCHESTRATOR=k8s"},
		},
		{
			name:       "missing_cluster_flag_for_gke",
			args:       []string{"list", "--project", "my-proj", "--location", "us-central1"},
			wantSubstr: []string{"cluster name is required"},
		},
		{
			name:       "missing_location_flag_for_gke",
			args:       []string{"list", "--cluster", "my-clus", "--project", "my-proj"},
			wantSubstr: []string{"location is required"},
		},
		{
			name:       "missing_project_flag_for_gke",
			args:       []string{"list", "--cluster", "my-clus", "--location", "us-central1"},
			wantSubstr: []string{"project ID is required"},
		},
		{
			name:        "invalid_gcluster_orchestrator_fails_fast",
			envOverride: "bogus",
			args:        []string{"list"},
			wantSubstr:  []string{orchestrator.EnvVar, `"bogus"`, `"gke", "k8s"`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			if tc.cfg != nil {
				t.Setenv("KUBECONFIG", writeKubeconfig(t, tc.cfg()))
			}
			if tc.envOverride != "" {
				t.Setenv(orchestrator.EnvVar, tc.envOverride)
			}
			_, err := executeCommand(JobCmd, tc.args...)
			if err == nil {
				t.Fatalf("executeCommand(%v) err = nil, want error containing %v", tc.args, tc.wantSubstr)
			}
			assertErrorSubstrings(t, tc.args, err.Error(), tc.wantSubstr, tc.forbid)
		})
	}
}

func assertErrorSubstrings(t *testing.T, args []string, msg string, wantSubstr []string, forbid string) {
	t.Helper()
	for _, want := range wantSubstr {
		if !strings.Contains(msg, want) {
			t.Errorf("executeCommand(%v) err = %q, want substring %q", args, msg, want)
		}
	}
	if forbid != "" && strings.Contains(msg, forbid) {
		t.Errorf("executeCommand(%v) err = %q, must not contain %q", args, msg, forbid)
	}
}

func TestJobCmd_MissingClusterNotMaskedByActiveGKEContext(t *testing.T) {
	setupSubmitTestEnv(t)
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["gke_my-proj_us-central1_existing"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
	cfg.AuthInfos["gke-user"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}
	cfg.Contexts["gke_my-proj_us-central1_existing"] = &clientcmdapi.Context{
		Cluster:  "gke_my-proj_us-central1_existing",
		AuthInfo: "gke-user",
	}
	cfg.CurrentContext = "gke_my-proj_us-central1_existing"
	t.Setenv("KUBECONFIG", writeKubeconfig(t, cfg))

	if _, err := executeCommand(JobCmd, "list", "--project", "my-proj", "--location", "us-central1"); err == nil || !strings.Contains(err.Error(), "cluster name is required") {
		t.Fatalf("executeCommand(list -p -l) err = %v, want 'cluster name is required'", err)
	}

	// Symmetry check: an active non-GKE current-context (e.g. kind-kind) must also NOT mask missing --cluster
	// when --project/--location are passed on cancel/list or submit.
	t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "kind-kind", "kind-kind", "https://127.0.0.1:6443"))
	cancelCalled := false
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{
			cancelJobFunc: func(string, orchestrator.CancelOptions) error {
				cancelCalled = true
				return nil
			},
		}
	}
	if _, err := executeCommand(JobCmd, "cancel", "my-job", "--project", "my-proj", "--location", "us-central1"); err == nil || !strings.Contains(err.Error(), "cluster name is required") || !strings.Contains(err.Error(), "GCLUSTER_ORCHESTRATOR=k8s") {
		t.Fatalf("executeCommand(cancel -p -l with non-GKE current-context) err = %v, want 'cluster name is required' + k8s hint", err)
	}
	if cancelCalled {
		t.Fatal("cancel executed against unrelated non-GKE current-context when --cluster was omitted")
	}
	if _, err := executeCommand(JobCmd, "submit", "--project", "my-proj", "--location", "us-central1", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"); err == nil || !strings.Contains(err.Error(), "cluster name is required") || strings.Contains(err.Error(), "not yet supported") {
		t.Fatalf("executeCommand(submit -p -l with non-GKE current-context) err = %v, want 'cluster name is required' (not UnsupportedK8sSubmitError)", err)
	}
}

func writeSavedGKEContext(t *testing.T, home string) {
	t.Helper()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
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
		wantType     orchestrator.Type
		wantCluster  string
		wantLocation string
		wantProject  string
	}{
		{
			name:        "env_override_k8s_ignores_saved_gke_context",
			envOverride: "k8s",
			withK8sCfg:  true,
			args:        []string{"list"},
			wantType:    orchestrator.TypeK8s,
			wantCluster: "onprem-k8s",
		},
		{
			name:        "explicit_k8s_cluster_clears_saved_gke_project_and_location",
			withK8sCfg:  true,
			args:        []string{"list", "--cluster", "onprem-k8s"},
			wantType:    orchestrator.TypeK8s,
			wantCluster: "onprem-k8s",
		},
		{
			name:         "explicit_project_and_location_preserved_in_k8s_mode",
			withK8sCfg:   true,
			args:         []string{"list", "--cluster", "onprem-k8s", "--location", "europe-north1", "--project", "explicit-gar-proj"},
			wantType:     orchestrator.TypeK8s,
			wantCluster:  "onprem-k8s",
			wantLocation: "europe-north1",
			wantProject:  "explicit-gar-proj",
		},
		{
			name:         "zero_flags_preserve_saved_gke_context_defaults",
			args:         []string{"list"},
			wantType:     orchestrator.TypeGKE,
			wantCluster:  "saved-gke-cluster",
			wantLocation: "us-central1-c",
			wantProject:  "saved-gke-proj",
		},
		{
			name:         "env_override_gke_preserves_saved_gke_context",
			envOverride:  "GKE",
			withK8sCfg:   true,
			args:         []string{"list"},
			wantType:     orchestrator.TypeGKE,
			wantCluster:  "saved-gke-cluster",
			wantLocation: "us-central1-c",
			wantProject:  "saved-gke-proj",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			stubUnexpectedSubprocesses(t, false)
			writeSavedGKEContext(t, t.TempDir())
			if tc.withK8sCfg {
				t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443"))
			}
			if tc.envOverride != "" {
				t.Setenv(orchestrator.EnvVar, tc.envOverride)
			}

			store = &MockPrereqStore{
				State: PrereqState{
					LastCheckedTimestamp: time.Now(),
					LastCheckedProjectID: "saved-gke-proj",
				},
			}
			mockOrc := &mockOrchestrator{}
			gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

			snapLogs, restoreLogs := logging.CaptureForTesting()
			defer restoreLogs()
			if _, err := executeCommand(JobCmd, tc.args...); err != nil {
				t.Fatalf("executeCommand(%v) error = %v, want nil", tc.args, err)
			}
			infoOut, _ := snapLogs()
			wantLog := "Orchestrator: " + string(tc.wantType) + " ("
			if !strings.Contains(infoOut, wantLog) {
				t.Errorf("executeCommand(%v) info logs = %q, want %q", tc.args, infoOut, wantLog)
			}
			if mockOrc.InitClusterName != tc.wantCluster || mockOrc.InitLocation != tc.wantLocation || mockOrc.InitProjectID != tc.wantProject {
				t.Errorf("executeCommand(%v) init = (%q, %q, %q), want (%q, %q, %q)",
					tc.args,
					mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID,
					tc.wantCluster, tc.wantLocation, tc.wantProject)
			}
		})
	}
}

func TestJobCmd_SequentialInvocationsDoNotLeakTargetFlags(t *testing.T) {
	setupSubmitTestEnv(t)
	stubUnexpectedSubprocesses(t, false)
	t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443"))
	mockOrc := &mockOrchestrator{
		initializeFunc: func(_, loc, _ string) (string, error) {
			if loc != "" {
				return "us-central1-a", nil
			}
			return "", nil
		},
	}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

	if _, err := executeCommand(JobCmd, "list", "--cluster", "gke-clus", "--location", "us-central1", "--project", "test-project"); err != nil {
		t.Fatalf("first executeCommand(GKE list) error = %v", err)
	}
	// Reset only Cobra Changed bits (simulating a fresh CLI flag parse without calling resetJobFlags on globals).
	JobCmd.PersistentFlags().Lookup("cluster").Changed = false
	JobCmd.PersistentFlags().Lookup("location").Changed = false
	JobCmd.PersistentFlags().Lookup("project").Changed = false

	if _, err := executeCommand(JobCmd, "list"); err != nil {
		t.Fatalf("second executeCommand(TypeK8s list) error = %v", err)
	}
	if mockOrc.InitClusterName != "onprem-k8s" || mockOrc.InitLocation != "" || mockOrc.InitProjectID != "" {
		t.Errorf("second invocation init = (%q, %q, %q), want (\"onprem-k8s\", \"\", \"\")",
			mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID)
	}
}
