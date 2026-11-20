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
	"encoding/base64"
	"encoding/pem"
	"fmt"
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

func TestJobCmd_TypeK8sSubmitDispatchesAndValidatesFlags(t *testing.T) {
	t.Run("prebuilt_image_submit_dispatches_in_k8s_mode_without_gcloud", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@k8s-local", "k8s-local", "https://10.128.0.68:6443"))
		useFileStore(t)
		executed := stubUnexpectedSubprocesses(t, true)
		mockOrc := &mockOrchestrator{}
		gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

		_, err := executeCommand(JobCmd,
			"submit",
			"--name", "k8s-prebuilt",
			"--image", "10.128.0.68:5000/jax:v5p",
			"--command", "python3 train.py",
			"--k8s-label-scheme", "tpu-slice",
		)
		if err != nil {
			t.Fatalf("executeCommand(TypeK8s submit) err = %v, want nil", err)
		}
		gotJob := mockOrc.SubmittedJob
		wantSummary := "k8s-local|k8s-prebuilt|tpu-slice|"
		gotSummary := strings.Join([]string{gotJob.ClusterName, gotJob.WorkloadName, gotJob.K8sLabelScheme, gotJob.ComputeType}, "|")
		if gotSummary != wantSummary {
			t.Errorf("SubmitJob received %q, want %q", gotSummary, wantSummary)
		}
		if len(*executed) != 1 || !strings.HasPrefix((*executed)[0], "kubectl version") {
			t.Errorf("executed subprocesses = %v, want only kubectl version prereq check", *executed)
		}
	})

	t.Run("invalid_k8s_label_scheme_fails_before_submit", func(t *testing.T) {
		setupSubmitTestEnv(t)
		t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@k8s-local", "k8s-local", "https://10.128.0.68:6443"))
		useFileStore(t)
		stubUnexpectedSubprocesses(t, true)
		mockOrc := &mockOrchestrator{}
		gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

		_, err := executeCommand(JobCmd,
			"submit",
			"--name", "k8s-bad-scheme",
			"--image", "busybox",
			"--command", "echo hi",
			"--k8s-label-scheme", "invalid-scheme",
		)
		if err == nil || !strings.Contains(err.Error(), "invalid --k8s-label-scheme") {
			t.Fatalf("executeCommand(invalid --k8s-label-scheme) err = %v, want invalid scheme error", err)
		}
		if mockOrc.SubmittedJob.WorkloadName != "" {
			t.Errorf("SubmitJob must not run when --k8s-label-scheme is invalid")
		}
	})
}

func TestJobCmd_ForcedK8sEnvOnEmptyKubeconfigReportsMissingContext(t *testing.T) {
	setupSubmitTestEnv(t)
	t.Setenv(orchestrator.EnvVar, "k8s")
	useFileStore(t)
	executed := stubUnexpectedSubprocesses(t, false)
	gkeOrchestratorFactory = nil

	_, err := executeCommand(JobCmd,
		"submit",
		"--name", "k8s-forced",
		"--image", "busybox",
		"--command", "echo hi",
		"--compute-type", "n2-standard-4",
	)
	if err == nil || !strings.Contains(err.Error(), "no current-context set in kubeconfig") {
		t.Fatalf("executeCommand(forced k8s submit on empty kubeconfig) err = %v, want missing current-context error", err)
	}
	if len(*executed) != 0 {
		t.Errorf("executed subprocesses = %v, want 0", *executed)
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
			wantSubstr: []string{"no kubeconfig context found and no GKE flags provided"},
		},
		{
			name: "dangling_current_context_names_missing_context",
			cfg: func() *clientcmdapi.Config {
				c := clientcmdapi.NewConfig()
				c.CurrentContext = "deleted-ctx"
				return c
			},
			args:       []string{"list"},
			wantSubstr: []string{`"deleted-ctx"`, "not found"},
		},
		{
			name:        "forced_k8s_empty_kubeconfig_omits_gke_flag_advice",
			envOverride: "k8s",
			args:        []string{"list"},
			wantSubstr:  []string{"no current-context set in kubeconfig"},
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
			name:       "fresh_workstation_submit_missing_flags_reports_missing_context_or_gke_flags",
			args:       []string{"submit", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"},
			wantSubstr: []string{"no kubeconfig context found and no GKE flags provided"},
		},
		{
			name: "gke_context_missing_flags_on_submit_mentions_k8s_override_hint",
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
			wantSubstr: []string{"location is required", "GCLUSTER_ORCHESTRATOR=k8s"},
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
	// when --project, --location, or whitespace --cluster is passed on cancel/list or submit.
	assertNonGKECurrentContextDoesNotMaskMissingCluster(t)
}

func assertNonGKECurrentContextDoesNotMaskMissingCluster(t *testing.T) {
	t.Helper()
	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "project_and_location", args: []string{"cancel", "my-job", "--project", "my-proj", "--location", "us-central1"}},
		{name: "project_only", args: []string{"cancel", "my-job", "--project", "my-proj"}},
		{name: "location_only", args: []string{"cancel", "my-job", "--location", "us-central1"}},
		{name: "whitespace_cluster_with_project", args: []string{"cancel", "my-job", "--cluster", "   ", "--project", "my-proj", "--location", "us-central1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
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
			if _, err := executeCommand(JobCmd, tc.args...); err == nil || !strings.Contains(err.Error(), "cluster name is required") || !strings.Contains(err.Error(), "GCLUSTER_ORCHESTRATOR=k8s") {
				t.Fatalf("executeCommand(%v) err = %v, want 'cluster name is required' + k8s hint", tc.args, err)
			}
			if cancelCalled {
				t.Fatalf("cancel executed against unrelated non-GKE current-context for %v", tc.args)
			}
		})
	}

	setupSubmitTestEnv(t)
	t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "kind-kind", "kind-kind", "https://127.0.0.1:6443"))
	if _, err := executeCommand(JobCmd, "submit", "--project", "my-proj", "--location", "us-central1", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"); err == nil || !strings.Contains(err.Error(), "cluster name is required") || strings.Contains(err.Error(), "not yet supported") {
		t.Fatalf("executeCommand(submit -p -l with non-GKE current-context) err = %v, want 'cluster name is required' (not UnsupportedK8sSubmitError)", err)
	}

	// Forced GCLUSTER_ORCHESTRATOR=k8s with --project and no --cluster uses current-context without requiring --cluster.
	setupSubmitTestEnv(t)
	stubUnexpectedSubprocesses(t, true)
	t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "kind-kind", "kind-kind", "https://127.0.0.1:6443"))
	t.Setenv(orchestrator.EnvVar, "k8s")
	useFileStore(t)
	var forcedCancelCluster string
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
		return &mockOrchestrator{
			cancelJobFunc: func(_ string, opts orchestrator.CancelOptions) error {
				forcedCancelCluster = opts.ClusterName
				return nil
			},
		}
	}
	if _, err := executeCommand(JobCmd, "cancel", "my-job", "--project", "ar-proj"); err != nil || forcedCancelCluster != "kind-kind" {
		t.Fatalf("executeCommand(forced k8s cancel -p) = (%v, %q), want (nil, \"kind-kind\")", err, forcedCancelCluster)
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
		name                   string
		envOverride            string
		refreshEnv             bool
		withK8sCfg             bool
		args                   []string
		wantType               orchestrator.Type
		wantCluster            string
		wantLocation           string
		wantProject            string
		wantSavedLog           bool
		wantIgnoredWarnContext string
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
			name:                   "explicit_k8s_cluster_clears_saved_gke_project_and_location",
			withK8sCfg:             true,
			args:                   []string{"list", "--cluster", "onprem-k8s"},
			wantType:               orchestrator.TypeK8s,
			wantCluster:            "onprem-k8s",
			wantIgnoredWarnContext: "admin@onprem-k8s",
		},
		{
			name:                   "explicit_k8s_cluster_with_refresh_credentials_env_stays_k8s",
			refreshEnv:             true,
			withK8sCfg:             true,
			args:                   []string{"list", "--cluster", "onprem-k8s"},
			wantType:               orchestrator.TypeK8s,
			wantCluster:            "onprem-k8s",
			wantIgnoredWarnContext: "admin@onprem-k8s",
		},
		{
			name:         "explicit_uncached_gke_cluster_falls_back_to_saved_project_and_location",
			withK8sCfg:   true,
			args:         []string{"list", "--cluster", "other-gke-cluster"},
			wantType:     orchestrator.TypeGKE,
			wantCluster:  "other-gke-cluster",
			wantLocation: "us-central1-c",
			wantProject:  "saved-gke-proj",
			wantSavedLog: true,
		},
		{
			name:         "explicit_project_and_location_select_gke_over_bare_k8s_context",
			withK8sCfg:   true,
			args:         []string{"list", "--cluster", "onprem-k8s", "--location", "europe-north1", "--project", "explicit-gar-proj"},
			wantType:     orchestrator.TypeGKE,
			wantCluster:  "onprem-k8s",
			wantLocation: "europe-north1",
			wantProject:  "explicit-gar-proj",
		},
		{
			name:         "forced_k8s_preserves_explicit_project_and_location",
			envOverride:  "k8s",
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
			wantSavedLog: true,
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
			wantSavedLog: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			stubUnexpectedSubprocesses(t, tc.wantType == orchestrator.TypeK8s)
			writeSavedGKEContext(t, t.TempDir())
			if tc.withK8sCfg {
				t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "admin@onprem-k8s", "onprem-k8s", "https://10.128.0.68:6443"))
			}
			if tc.envOverride != "" {
				t.Setenv(orchestrator.EnvVar, tc.envOverride)
			}
			if tc.refreshEnv {
				t.Setenv("GCLUSTER_REFRESH_CREDENTIALS", "1")
			}

			lastProj := "saved-gke-proj"
			if tc.wantProject != "" {
				lastProj = tc.wantProject
			}
			store = &MockPrereqStore{
				State: PrereqState{
					GCloudSDKInstalled:           true,
					GCloudProjectConfigured:      true,
					GCloudAuthenticated:          true,
					ADCConfigured:                true,
					KubectlInstalled:             true,
					GKEGCloudAuthPluginInstalled: true,
					LastCheckedTimestamp:         time.Now(),
					LastCheckedProjectID:         lastProj,
				},
			}
			mockOrc := &mockOrchestrator{}
			gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

			snapLogs, restoreLogs := logging.CaptureForTesting(t)
			defer restoreLogs()
			if _, err := executeCommand(JobCmd, tc.args...); err != nil {
				t.Fatalf("executeCommand(%v) error = %v, want nil", tc.args, err)
			}
			infoOut, errOut := snapLogs()
			assertSavedGKEConfigLogs(t, tc.args, tc.wantType, tc.wantSavedLog, tc.wantIgnoredWarnContext, infoOut, errOut)
			if mockOrc.InitClusterName != tc.wantCluster || mockOrc.InitLocation != tc.wantLocation || mockOrc.InitProjectID != tc.wantProject {
				t.Errorf("executeCommand(%v) init = (%q, %q, %q), want (%q, %q, %q)",
					tc.args,
					mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID,
					tc.wantCluster, tc.wantLocation, tc.wantProject)
			}
		})
	}

	assertPartialAndK8sSavedDefaultsNotice(t)
}

func assertSavedGKEConfigLogs(t *testing.T, args []string, wantType orchestrator.Type, wantSavedLog bool, wantIgnoredWarnContext, infoOut, errOut string) {
	t.Helper()
	wantLog := "Orchestrator: " + string(wantType) + " ("
	if !strings.Contains(infoOut, wantLog) {
		t.Errorf("executeCommand(%v) info logs = %q, want %q", args, infoOut, wantLog)
	}
	if strings.Contains(infoOut, "Using saved job config defaults") != wantSavedLog {
		t.Errorf("executeCommand(%v) saved-defaults log present = %v, want %v (logs=%q)", args, !wantSavedLog, wantSavedLog, infoOut)
	}
	if wantIgnoredWarnContext == "" {
		if strings.Contains(errOut, "Ignoring saved job config") {
			t.Errorf("executeCommand(%v) unexpected ignored-defaults warning: %q", args, errOut)
		}
		return
	}
	for _, wantSubstr := range []string{"Ignoring saved job config project/location defaults", wantIgnoredWarnContext, "--location/--project", orchestrator.EnvVar + "=gke"} {
		if !strings.Contains(errOut, wantSubstr) {
			t.Errorf("executeCommand(%v) warning logs = %q, want substring %q", args, errOut, wantSubstr)
		}
	}
}

func assertPartialAndK8sSavedDefaultsNotice(t *testing.T) {
	t.Helper()
	// 1. Partial saved GKE defaults (cluster unset, project/location saved) must emit the saved-defaults notice
	// before failing with "cluster name is required" so users see how to clear ~/.gcluster/context.json.
	setupSubmitTestEnv(t)
	t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "kind-kind", "kind-kind", "https://127.0.0.1:6443"))
	if err := saveContext(Context{ProjectID: "saved-proj", Location: "us-central1"}); err != nil {
		t.Fatalf("saveContext(partial): %v", err)
	}
	snapPartial, restorePartial := logging.CaptureForTesting(t)
	defer restorePartial()
	_, err := executeCommand(JobCmd, "list")
	partialInfo, _ := snapPartial()
	restorePartial()
	if err == nil || !strings.Contains(err.Error(), "cluster name is required") {
		t.Fatalf("partial saved config list err = %v, want 'cluster name is required'", err)
	}
	if !strings.Contains(partialInfo, "Using saved job config defaults") {
		t.Errorf("partial saved config info logs = %q, want saved-defaults notice before error", partialInfo)
	}
	assertPartialSavedGKEExplicitK8sClusterWarns(t)

	// 2. Saved cluster pointing to a K8s context overrides current-context and must emit the saved-defaults notice.
	setupSubmitTestEnv(t)
	stubUnexpectedSubprocesses(t, true)
	useFileStore(t)
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["active-clus"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:6443"}
	cfg.Clusters["saved-k8s-clus"] = &clientcmdapi.Cluster{Server: "https://10.0.0.2:6443"}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert")}
	cfg.Contexts["active-ctx"] = &clientcmdapi.Context{Cluster: "active-clus", AuthInfo: "u"}
	cfg.Contexts["saved-k8s-ctx"] = &clientcmdapi.Context{Cluster: "saved-k8s-clus", AuthInfo: "u"}
	cfg.CurrentContext = "active-ctx"
	t.Setenv("KUBECONFIG", writeKubeconfig(t, cfg))
	if err := saveContext(Context{ClusterName: "saved-k8s-ctx"}); err != nil {
		t.Fatalf("saveContext(k8s cluster): %v", err)
	}
	mockOrc := &mockOrchestrator{}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }

	snapK8s, restoreK8s := logging.CaptureForTesting(t)
	defer restoreK8s()
	if _, err := executeCommand(JobCmd, "list"); err != nil {
		t.Fatalf("saved K8s cluster list err = %v, want nil", err)
	}
	k8sInfo, _ := snapK8s()
	restoreK8s()
	if mockOrc.InitClusterName != "saved-k8s-ctx" || !strings.Contains(k8sInfo, "Using saved job config defaults") {
		t.Errorf("saved K8s cluster init=%q logs=%q, want saved-k8s-ctx + saved-defaults notice", mockOrc.InitClusterName, k8sInfo)
	}
}

func assertPartialSavedGKEExplicitK8sClusterWarns(t *testing.T) {
	t.Helper()
	setupSubmitTestEnv(t)
	stubUnexpectedSubprocesses(t, true)
	useFileStore(t)
	t.Setenv("KUBECONFIG", writeTestKubeconfigFile(t, "kind-kind", "kind-kind", "https://127.0.0.1:6443"))
	if err := saveContext(Context{ProjectID: "saved-proj", Location: "us-central1"}); err != nil {
		t.Fatalf("saveContext(partial): %v", err)
	}
	mockOrc := &mockOrchestrator{}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }
	snapLogs, restoreLogs := logging.CaptureForTesting(t)
	defer restoreLogs()
	if _, err := executeCommand(JobCmd, "list", "--cluster", "kind-kind"); err != nil {
		t.Fatalf("partial saved config list -c kind-kind err = %v, want nil", err)
	}
	infoOut, errOut := snapLogs()
	assertSavedGKEConfigLogs(t, []string{"list", "--cluster", "kind-kind"}, orchestrator.TypeK8s, false, "kind-kind", infoOut, errOut)
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

func TestJobCmd_ExplicitEmptyClusterFlagRejected(t *testing.T) {
	gkeCfg := func() *clientcmdapi.Config {
		c := clientcmdapi.NewConfig()
		c.Clusters["gke_my-proj_us-central1_existing"] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
		c.AuthInfos["gke-user"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}
		c.Contexts["gke_my-proj_us-central1_existing"] = &clientcmdapi.Context{
			Cluster:  "gke_my-proj_us-central1_existing",
			AuthInfo: "gke-user",
		}
		c.CurrentContext = "gke_my-proj_us-central1_existing"
		return c
	}
	k8sCfg := func() *clientcmdapi.Config {
		c := clientcmdapi.NewConfig()
		c.Clusters["kind-kind"] = &clientcmdapi.Cluster{Server: "https://127.0.0.1:6443"}
		c.AuthInfos["admin"] = &clientcmdapi.AuthInfo{ClientCertificateData: []byte("cert-pem")}
		c.Contexts["kind-kind"] = &clientcmdapi.Context{Cluster: "kind-kind", AuthInfo: "admin"}
		c.Contexts["saved-k8s-ctx"] = &clientcmdapi.Context{Cluster: "kind-kind", AuthInfo: "admin"}
		c.CurrentContext = "kind-kind"
		return c
	}

	tests := []struct {
		name         string
		envOverride  string
		savedCluster string
		cfg          func() *clientcmdapi.Config
		args         []string
		wantSubstr   []string
		forbid       string
	}{
		{
			name:       "empty_cluster_with_active_k8s_context",
			cfg:        k8sCfg,
			args:       []string{"cancel", "my-job", "--cluster", ""},
			wantSubstr: []string{"cluster name is required", "omit --cluster to use kubeconfig current-context"},
		},
		{
			name:       "whitespace_cluster_with_active_k8s_context",
			cfg:        k8sCfg,
			args:       []string{"list", "--cluster", "   "},
			wantSubstr: []string{"cluster name is required", "omit --cluster to use kubeconfig current-context"},
		},
		{
			name:        "empty_cluster_under_forced_k8s_env",
			envOverride: "k8s",
			cfg:         k8sCfg,
			args:        []string{"cancel", "my-job", "--cluster", "", "--project", "ar-proj"},
			wantSubstr:  []string{"cluster name is required", "omit --cluster to use kubeconfig current-context"},
		},
		{
			name:         "empty_cluster_overrides_saved_k8s_cluster_and_fails",
			savedCluster: "saved-k8s-ctx",
			cfg:          k8sCfg,
			args:         []string{"list", "--cluster", ""},
			wantSubstr:   []string{"cluster name is required", "omit --cluster to use kubeconfig current-context"},
		},
		{
			name:       "empty_cluster_with_active_gke_context",
			cfg:        gkeCfg,
			args:       []string{"cancel", "my-job", "--cluster", ""},
			wantSubstr: []string{"cluster name is required"},
			forbid:     "omit --cluster to use kubeconfig current-context",
		},
		{
			name:        "empty_cluster_under_forced_gke_env",
			envOverride: "gke",
			cfg:         k8sCfg,
			args:        []string{"list", "--cluster", ""},
			wantSubstr:  []string{"cluster name is required"},
			forbid:      "omit --cluster to use kubeconfig current-context",
		},
		{
			name:       "empty_cluster_on_submit",
			cfg:        k8sCfg,
			args:       []string{"submit", "--cluster", "", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"},
			wantSubstr: []string{"cluster name is required when --cluster is set", "omit --cluster to use kubeconfig current-context"},
		},
		{
			name:        "empty_cluster_on_forced_k8s_submit",
			envOverride: "k8s",
			cfg:         k8sCfg,
			args:        []string{"submit", "--cluster", "", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "n2-standard-4"},
			wantSubstr:  []string{"cluster name is required when --cluster is set", "omit --cluster to use kubeconfig current-context"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			setupSubmitTestEnv(t)
			stubUnexpectedSubprocesses(t, false)
			t.Setenv("KUBECONFIG", writeKubeconfig(t, tc.cfg()))
			if tc.envOverride != "" {
				t.Setenv(orchestrator.EnvVar, tc.envOverride)
			}
			if tc.savedCluster != "" {
				if err := saveContext(Context{ClusterName: tc.savedCluster}); err != nil {
					t.Fatalf("saveContext: %v", err)
				}
			}
			called := false
			gkeOrchestratorFactory = func() orchestrator.JobOrchestrator {
				return &mockOrchestrator{
					listJobsFunc:  func(orchestrator.ListOptions) ([]orchestrator.JobStatus, error) { called = true; return nil, nil },
					cancelJobFunc: func(string, orchestrator.CancelOptions) error { called = true; return nil },
				}
			}
			_, err := executeCommand(JobCmd, tc.args...)
			if err == nil {
				t.Fatalf("executeCommand(%v) err = nil, want error containing %v", tc.args, tc.wantSubstr)
			}
			if called {
				t.Fatalf("executeCommand(%v) dispatched to orchestrator despite empty --cluster", tc.args)
			}
			assertErrorSubstrings(t, tc.args, err.Error(), tc.wantSubstr, tc.forbid)
		})
	}
}

func newTestGKEAndOnPremServers(t *testing.T) (*httptest.Server, *httptest.Server, []byte, string, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	var onPremHits, gkeDeletes atomic.Int32
	onPremSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		onPremHits.Add(1)
		t.Errorf("unexpected Kubernetes API request sent to on-prem 'prod' server: %s %s", r.Method, r.URL.Path)
		http.Error(w, "wrong cluster", http.StatusTeapot)
	}))
	t.Cleanup(onPremSrv.Close)

	gkeSrv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/apis/jobset.x-k8s.io/v1alpha2/namespaces/default/jobsets/myjob":
			_, _ = w.Write([]byte(`{"apiVersion":"jobset.x-k8s.io/v1alpha2","kind":"JobSet","metadata":{"name":"myjob","namespace":"default"}}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/apis/jobset.x-k8s.io/v1alpha2/namespaces/default/jobsets/myjob":
			gkeDeletes.Add(1)
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"Status","status":"Success"}`))
		default:
			_, _ = w.Write([]byte(`{"apiVersion":"v1","kind":"PersistentVolumeClaimList","items":[]}`))
		}
	}))
	t.Cleanup(gkeSrv.Close)

	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: gkeSrv.Certificate().Raw})
	caB64 := base64.StdEncoding.EncodeToString(caPEM)
	return onPremSrv, gkeSrv, caPEM, caB64, &onPremHits, &gkeDeletes
}

func stubGKECredsAndDescribeForProd(t *testing.T, kcPath, gkeURL string, caPEM []byte, caB64 string) {
	t.Helper()
	gkeHost := strings.TrimPrefix(gkeURL, "https://")
	describeJSON := fmt.Sprintf(`{"endpoint":%q,"masterAuth":{"clusterCaCertificate":%q}}`, gkeHost, caB64)
	origExec := shell.ExecuteCommandWithTimeout
	t.Cleanup(func() { shell.ExecuteCommandWithTimeout = origExec })
	shell.ExecuteCommandWithTimeout = func(_ time.Duration, name string, args ...string) shell.CommandResult {
		joined := strings.Join(args, " ")
		switch {
		case name == "gcloud" && strings.Contains(joined, "clusters describe prod"):
			return shell.CommandResult{ExitCode: 0, Stdout: describeJSON}
		case name == "gcloud" && strings.Contains(joined, "clusters get-credentials prod"):
			cfg, err := clientcmd.LoadFromFile(kcPath)
			if err != nil {
				t.Fatalf("LoadFromFile: %v", err)
			}
			const gkeCtx = "gke_my-proj_us-central1_prod"
			cfg.Clusters[gkeCtx] = &clientcmdapi.Cluster{Server: gkeURL, CertificateAuthorityData: caPEM}
			cfg.AuthInfos[gkeCtx] = &clientcmdapi.AuthInfo{Token: "gke-jwt"}
			cfg.Contexts[gkeCtx] = &clientcmdapi.Context{Cluster: gkeCtx, AuthInfo: gkeCtx}
			if err := clientcmd.WriteToFile(*cfg, kcPath); err != nil {
				t.Fatalf("WriteToFile: %v", err)
			}
			return shell.CommandResult{ExitCode: 0}
		case name == "kubectl" && strings.Contains(joined, "--context gke_my-proj_us-central1_prod get --raw /version"):
			return shell.CommandResult{ExitCode: 0, Stdout: `{"gitVersion":"v1.32.2-gke.1182000"}`}
		default:
			t.Errorf("unexpected subprocess %s %s", name, joined)
			return shell.CommandResult{ExitCode: 1, Stderr: "unexpected command"}
		}
	}
}

func writeOnPremProdContextKubeconfig(t *testing.T, ctxName, onPremURL string) string {
	t.Helper()
	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["prod"] = &clientcmdapi.Cluster{Server: onPremURL, InsecureSkipTLSVerify: true}
	cfg.AuthInfos["onprem-sa"] = &clientcmdapi.AuthInfo{Token: "onprem-jwt"}
	cfg.Contexts[ctxName] = &clientcmdapi.Context{Cluster: "prod", AuthInfo: "onprem-sa"}
	cfg.CurrentContext = ctxName
	return writeKubeconfig(t, cfg)
}

func writeBareOnPremProdKubeconfig(t *testing.T, onPremURL string) string {
	t.Helper()
	return writeOnPremProdContextKubeconfig(t, "prod", onPremURL)
}

func TestJobCmd_GKEFlagsOverrideBareK8sContextCollision(t *testing.T) {
	t.Run("bare_onprem_prod_only_routes_flags_and_saved_config_to_gke", assertBareOnPremProdRoutesToGKE)
	t.Run("both_gke_and_bare_onprem_prod_with_saved_config_prefers_gke_context", assertBothGKEAndBareProdPrefersGKEContext)
}

func assertBareOnPremProdRoutesToGKE(t *testing.T) {
	onPremSrv, gkeSrv, caPEM, caB64, onPremHits, gkeDeletes := newTestGKEAndOnPremServers(t)
	setupSubmitTestEnv(t)
	t.Setenv(shell.AirGappedEnvVar, "true")
	kcPath := writeBareOnPremProdKubeconfig(t, onPremSrv.URL)
	t.Setenv("KUBECONFIG", kcPath)
	stubGKECredsAndDescribeForProd(t, kcPath, gkeSrv.URL, caPEM, caB64)
	gkeOrchestratorFactory = nil

	if _, err := executeCommand(JobCmd, "cancel", "myjob", "-c", "prod", "-l", "us-central1", "-p", "my-proj", "--skip-prereqs"); err != nil {
		t.Fatalf("cancel with explicit -c/-l/-p err = %v, want nil", err)
	}
	if gkeDeletes.Load() != 1 || onPremHits.Load() != 0 {
		t.Fatalf("explicit -c/-l/-p hits: gkeDeletes=%d onPremHits=%d, want (1, 0)", gkeDeletes.Load(), onPremHits.Load())
	}

	// Reset kubeconfig and Cobra flag state back to bare on-prem "prod" only, then test all three values coming from `job config`.
	resetJobFlags()
	kcPath2 := writeBareOnPremProdKubeconfig(t, onPremSrv.URL)
	t.Setenv("KUBECONFIG", kcPath2)
	stubGKECredsAndDescribeForProd(t, kcPath2, gkeSrv.URL, caPEM, caB64)
	if err := saveContext(Context{ProjectID: "my-proj", Location: "us-central1", ClusterName: "prod"}); err != nil {
		t.Fatalf("saveContext: %v", err)
	}
	if _, err := executeCommand(JobCmd, "cancel", "myjob", "--skip-prereqs"); err != nil {
		t.Fatalf("cancel with saved job config err = %v, want nil", err)
	}
	if gkeDeletes.Load() != 2 || onPremHits.Load() != 0 {
		t.Fatalf("saved config hits: gkeDeletes=%d onPremHits=%d, want (2, 0)", gkeDeletes.Load(), onPremHits.Load())
	}

	assertExplicitSavedClusterFlagRoutesToGKE(t, onPremSrv.URL, gkeSrv.URL, caPEM, caB64, onPremHits, gkeDeletes)
	assertSubmitWithCollidingBareProdRoutesToGKE(t, onPremSrv.URL, onPremHits)
	assertExplicitGKEContextAndLoadErrWithSavedGKE(t)
}

func assertExplicitSavedClusterFlagRoutesToGKE(t *testing.T, onPremURL, gkeURL string, caPEM []byte, caB64 string, onPremHits, gkeDeletes *atomic.Int32) {
	t.Helper()
	wantDeletes := gkeDeletes.Load()
	for _, tc := range []struct {
		ctxName    string
		refreshEnv string
	}{
		{ctxName: "prod", refreshEnv: ""},
		{ctxName: "admin@prod", refreshEnv: "1"},
	} {
		resetJobFlags()
		kcPath := writeOnPremProdContextKubeconfig(t, tc.ctxName, onPremURL)
		t.Setenv("KUBECONFIG", kcPath)
		t.Setenv("GCLUSTER_REFRESH_CREDENTIALS", tc.refreshEnv)
		stubGKECredsAndDescribeForProd(t, kcPath, gkeURL, caPEM, caB64)
		gkeOrchestratorFactory = nil
		if _, err := executeCommand(JobCmd, "cancel", "myjob", "-c", "prod", "--skip-prereqs"); err != nil {
			t.Fatalf("cancel -c prod matching saved cluster (ctx=%q refresh=%q) err = %v, want nil", tc.ctxName, tc.refreshEnv, err)
		}
		wantDeletes++
		if gkeDeletes.Load() != wantDeletes || onPremHits.Load() != 0 {
			t.Fatalf("cancel -c prod (ctx=%q refresh=%q) hits: gkeDeletes=%d onPremHits=%d, want (%d, 0)",
				tc.ctxName, tc.refreshEnv, gkeDeletes.Load(), onPremHits.Load(), wantDeletes)
		}
	}
	t.Setenv("GCLUSTER_REFRESH_CREDENTIALS", "")
}

func assertSubmitWithCollidingBareProdRoutesToGKE(t *testing.T, onPremURL string, onPremHits *atomic.Int32) {
	t.Helper()
	for _, tc := range []struct {
		workload   string
		ctxName    string
		refreshEnv string
		savedCtx   Context
		args       []string
		wantLoc    string
		wantProj   string
	}{
		{
			workload: "j1",
			ctxName:  "prod",
			savedCtx: Context{ProjectID: "my-proj", Location: "us-central1", ClusterName: "prod"},
			args:     []string{"submit", "-c", "prod", "-l", "us-central1", "-p", "my-proj", "--skip-prereqs", "--name", "j1", "--image", "busybox", "--command", "echo hi", "--compute-type", "a3-highgpu-8g"},
			wantLoc:  "us-central1",
			wantProj: "my-proj",
		},
		{
			workload: "j2",
			ctxName:  "prod",
			savedCtx: Context{ProjectID: "my-proj", Location: "us-central1", ClusterName: "prod"},
			args:     []string{"submit", "--skip-prereqs", "--name", "j2", "--image", "busybox", "--command", "echo hi", "--compute-type", "a3-highgpu-8g"},
			wantLoc:  "us-central1",
			wantProj: "my-proj",
		},
		{
			workload: "j3",
			ctxName:  "prod",
			savedCtx: Context{ProjectID: "my-proj", Location: "us-central1"},
			args:     []string{"submit", "-c", "prod", "--skip-prereqs", "--name", "j3", "--image", "busybox", "--command", "echo hi", "--compute-type", "a3-highgpu-8g"},
		},
		{
			workload:   "j4",
			ctxName:    "admin@prod",
			refreshEnv: "1",
			savedCtx:   Context{ProjectID: "my-proj", Location: "us-central1", ClusterName: "other-saved-gke"},
			args:       []string{"submit", "-c", "prod", "--skip-prereqs", "--name", "j4", "--image", "busybox", "--command", "echo hi", "--compute-type", "a3-highgpu-8g"},
		},
	} {
		resetJobFlags()
		t.Setenv("KUBECONFIG", writeOnPremProdContextKubeconfig(t, tc.ctxName, onPremURL))
		t.Setenv("GCLUSTER_REFRESH_CREDENTIALS", tc.refreshEnv)
		if err := saveContext(tc.savedCtx); err != nil {
			t.Fatalf("saveContext(%+v): %v", tc.savedCtx, err)
		}
		submitMock := &mockOrchestrator{}
		gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return submitMock }
		if _, err := executeCommand(JobCmd, tc.args...); err != nil {
			t.Fatalf("executeCommand(%v) with colliding %q context err = %v, want nil", tc.args, tc.ctxName, err)
		}
		if submitMock.SubmittedJob.WorkloadName != tc.workload || submitMock.InitClusterName != "prod" || submitMock.InitLocation != tc.wantLoc || submitMock.InitProjectID != tc.wantProj {
			t.Fatalf("submit dispatch = (workload=%q, init=%q/%q/%q), want (%s, prod/%s/%s)",
				submitMock.SubmittedJob.WorkloadName, submitMock.InitClusterName, submitMock.InitLocation, submitMock.InitProjectID, tc.workload, tc.wantLoc, tc.wantProj)
		}
	}
	t.Setenv("GCLUSTER_REFRESH_CREDENTIALS", "")
	if onPremHits.Load() != 0 {
		t.Fatalf("submit hit on-prem server %d times, want 0", onPremHits.Load())
	}
}

func assertExplicitGKEContextAndLoadErrWithSavedGKE(t *testing.T) {
	t.Helper()
	resetJobFlags()
	cfg := clientcmdapi.NewConfig()
	const gkeCtx = "gke_other-proj_europe-west4_cached"
	cfg.Clusters[gkeCtx] = &clientcmdapi.Cluster{Server: "https://34.118.224.10"}
	cfg.AuthInfos["u"] = &clientcmdapi.AuthInfo{Exec: &clientcmdapi.ExecConfig{Command: "gke-gcloud-auth-plugin"}}
	cfg.Contexts[gkeCtx] = &clientcmdapi.Context{Cluster: gkeCtx, AuthInfo: "u"}
	validKC := writeKubeconfig(t, cfg)
	t.Setenv("KUBECONFIG", validKC)

	mockOrc := &mockOrchestrator{}
	gkeOrchestratorFactory = func() orchestrator.JobOrchestrator { return mockOrc }
	if _, err := executeCommand(JobCmd, "list", "-c", gkeCtx, "--skip-prereqs"); err != nil {
		t.Fatalf("list -c %s with saved GKE defaults err = %v, want nil", gkeCtx, err)
	}
	if mockOrc.InitClusterName != gkeCtx || mockOrc.InitLocation != "us-central1" || mockOrc.InitProjectID != "my-proj" {
		t.Fatalf("list -c %s init = (%q,%q,%q), want (%q, us-central1, my-proj)", gkeCtx, mockOrc.InitClusterName, mockOrc.InitLocation, mockOrc.InitProjectID, gkeCtx)
	}

	resetJobFlags()
	corruptKC := filepath.Join(t.TempDir(), "corrupt-kubeconfig")
	if err := os.WriteFile(corruptKC, []byte("clusters: [unclosed\n"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	t.Setenv("KUBECONFIG", validKC+string(os.PathListSeparator)+corruptKC)
	if _, err := executeCommand(JobCmd, "list", "-c", "onprem-only", "--skip-prereqs"); err == nil || !strings.Contains(err.Error(), "failed to load kubeconfig") {
		t.Fatalf("list with corrupt multi-file KUBECONFIG err = %v, want 'failed to load kubeconfig'", err)
	}
}

func assertBothGKEAndBareProdPrefersGKEContext(t *testing.T) {
	onPremSrv, gkeSrv, caPEM, caB64, onPremHits, gkeDeletes := newTestGKEAndOnPremServers(t)
	setupSubmitTestEnv(t)
	t.Setenv(shell.AirGappedEnvVar, "true")

	cfg := clientcmdapi.NewConfig()
	cfg.Clusters["prod"] = &clientcmdapi.Cluster{Server: onPremSrv.URL, InsecureSkipTLSVerify: true}
	cfg.AuthInfos["onprem-sa"] = &clientcmdapi.AuthInfo{Token: "onprem-jwt"}
	cfg.Contexts["prod"] = &clientcmdapi.Context{Cluster: "prod", AuthInfo: "onprem-sa"}
	const gkeCtx = "gke_my-proj_us-central1_prod"
	cfg.Clusters[gkeCtx] = &clientcmdapi.Cluster{Server: gkeSrv.URL, CertificateAuthorityData: caPEM}
	cfg.AuthInfos[gkeCtx] = &clientcmdapi.AuthInfo{Token: "gke-jwt"}
	cfg.Contexts[gkeCtx] = &clientcmdapi.Context{Cluster: gkeCtx, AuthInfo: gkeCtx}
	cfg.CurrentContext = "prod"
	kcPath := writeKubeconfig(t, cfg)
	t.Setenv("KUBECONFIG", kcPath)
	stubGKECredsAndDescribeForProd(t, kcPath, gkeSrv.URL, caPEM, caB64)
	if err := saveContext(Context{ProjectID: "my-proj", Location: "us-central1", ClusterName: "prod"}); err != nil {
		t.Fatalf("saveContext: %v", err)
	}

	for _, args := range [][]string{
		{"cancel", "myjob", "--skip-prereqs"},
		{"cancel", "myjob", "-c", "prod", "--skip-prereqs"},
	} {
		resetJobFlags()
		gkeOrchestratorFactory = nil
		if _, err := executeCommand(JobCmd, args...); err != nil {
			t.Fatalf("executeCommand(%v) err = %v, want nil", args, err)
		}
	}
	if gkeDeletes.Load() != 2 || onPremHits.Load() != 0 {
		t.Fatalf("both-contexts hits: gkeDeletes=%d onPremHits=%d, want (2, 0)", gkeDeletes.Load(), onPremHits.Load())
	}
}
