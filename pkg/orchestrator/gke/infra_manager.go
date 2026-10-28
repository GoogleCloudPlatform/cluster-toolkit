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

package gke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/shell"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// Keep in sync with the jobset.version default in modules/management/kubectl-apply/variables.tf (no 'v' prefix there).
const defaultJobSetVersion = "v0.12.0"

const (
	jobSetNamespace             = "jobset-system"
	jobSetControllerManagerName = "jobset-controller-manager"
	jobSetHelmControllerName    = "jobset-controller"
	jobSetWebhookServiceName    = "jobset-webhook-service"
	jobSetMetricsServiceName    = "jobset-controller-manager-metrics-service"
	jobSetCRDName               = "jobsets.jobset.x-k8s.io"
	// jobSetWebhookPollAttempts caps the endpoint reads, so the wait also ends when tests remove the poll interval.
	jobSetWebhookPollAttempts = 100
	// jobSetWaitLogInterval is how often the endpoint wait reports that it is still waiting.
	jobSetWaitLogInterval = 30 * time.Second
	// kubectlReadTimeout bounds one kubectl read made while waiting for, or diagnosing, a webhook.
	kubectlReadTimeout = "--request-timeout=10s"
)

var (
	// jobSetWebhookWaitTimeout bounds the whole wait for a ready JobSet webhook endpoint, including slow
	// kubectl reads; tests shorten it.
	jobSetWebhookWaitTimeout = 5 * time.Minute
	// jobSetWebhookPollInterval is the pause between JobSet webhook endpoint reads; tests shorten it.
	jobSetWebhookPollInterval = 3 * time.Second
)

// errEndpointsUnreadable marks a failed endpoint read that callers may retry.
var errEndpointsUnreadable = errors.New("endpoints could not be read")

// jobSetInstallPermissionChecks are the cluster-scoped permissions needed to install JobSet.
var jobSetInstallPermissionChecks = []permissionCheck{
	{"create", "namespaces"},
	{"create", "customresourcedefinitions.apiextensions.k8s.io"},
	{"create", "clusterroles.rbac.authorization.k8s.io"},
	{"create", "clusterrolebindings.rbac.authorization.k8s.io"},
	{"create", "mutatingwebhookconfigurations.admissionregistration.k8s.io"},
	{"create", "validatingwebhookconfigurations.admissionregistration.k8s.io"},
}

// jobSetObjectsWithSelectors are the controller Deployments and Services in jobset-system whose
// selectors or pods an install would collide with if any of them already existed.
var jobSetObjectsWithSelectors = []struct {
	kind string
	gvr  schema.GroupVersionResource
	name string
}{
	{"Deployment", deploymentGVR, jobSetControllerManagerName},
	{"Deployment", deploymentGVR, jobSetHelmControllerName},
	{"Service", serviceGVR, jobSetWebhookServiceName},
	{"Service", serviceGVR, jobSetMetricsServiceName},
}

// checkAndInstallJobSetCRD installs JobSet only when neither its CRD nor its webhook Service exists.
// When both exist it waits for the webhook to become ready. It never modifies an existing or partial
// installation: re-applying manifests over one can leave the webhook without endpoints, which blocks
// every JobSet in the cluster.
func (g *GKEOrchestrator) checkAndInstallJobSetCRD() error {
	crdExists, crdForbidden, err := g.checkResourceExists("crd", jobSetCRDName, "")
	if err != nil {
		return fmt.Errorf("failed to check for the JobSet CRD: %w", err)
	}
	if crdExists && !crdForbidden {
		return g.verifyOrWaitForJobSetWithCRD()
	}
	serviceExists, _, err := g.checkResourceExists("service", jobSetWebhookServiceName, jobSetNamespace)
	if err != nil {
		return fmt.Errorf("failed to check for the JobSet webhook Service: %w", err)
	}
	switch {
	case crdExists && serviceExists:
		if err := g.waitForJobSetWebhookEndpoints(); err != nil {
			return fmt.Errorf("JobSet is installed but its webhook is not ready, and gcluster did not modify the existing JobSet installation: %w", err)
		}
		return nil
	case !crdExists && !serviceExists:
		return g.installJobSetOnFreshCluster()
	default:
		return g.partialJobSetInstallError(crdExists)
	}
}

// verifyOrWaitForJobSetWithCRD handles the common case where the JobSet CRD is confirmed present.
// It checks the webhook EndpointSlice first so a healthy cluster requires only two kubectl reads
// (CRD + EndpointSlice) and skips the separate Service existence read.
func (g *GKEOrchestrator) verifyOrWaitForJobSetWithCRD() error {
	start := time.Now()
	ready, epForbidden, epErr := g.checkServiceEndpointsReady(jobSetNamespace, jobSetWebhookServiceName)
	if epErr != nil && !errors.Is(epErr, errEndpointsUnreadable) {
		return epErr
	}
	if ready && !epForbidden {
		return nil
	}
	serviceExists, _, err := g.checkResourceExists("service", jobSetWebhookServiceName, jobSetNamespace)
	if err != nil {
		return fmt.Errorf("failed to check for the JobSet webhook Service: %w", err)
	}
	if !serviceExists {
		return g.partialJobSetInstallError(true)
	}
	if ready {
		return nil
	}
	logging.Info("Service %s/%s has no ready endpoint yet; the JobSet controller may be starting or restarting. Waiting up to %s...",
		jobSetNamespace, jobSetWebhookServiceName, jobSetWebhookWaitTimeout)
	if err := g.pollJobSetWebhookEndpoints(1, start, epErr == nil, epErr); err != nil {
		return fmt.Errorf("JobSet is installed but its webhook is not ready, and gcluster did not modify the existing JobSet installation: %w", err)
	}
	return nil
}

// checkResourceExists reports whether a Kubernetes object exists. A Forbidden read counts as existing, so a
// user who cannot read the object never triggers an installation.
func (g *GKEOrchestrator) checkResourceExists(kind, name, namespace string) (exists, forbidden bool, err error) {
	args := []string{"get", kind, name}
	if namespace != "" {
		args = append(args, "-n", namespace)
	}
	res := g.kubectl(append(args, "--ignore-not-found", "-o", "name")...)
	if res.ExitCode == 0 {
		return strings.TrimSpace(res.Stdout) != "", false, nil
	}
	err = fmt.Errorf("kubectl get %s %s failed: %s", kind, name, strings.TrimSpace(res.Stderr+"\n"+res.Stdout))
	if isForbiddenError(err) {
		logging.Warn("Insufficient RBAC permissions to read %s %s (403 Forbidden). Assuming it exists.", kind, name)
		return true, true, nil
	}
	return false, false, err
}

func (g *GKEOrchestrator) installJobSetOnFreshCluster() error {
	logging.Info("JobSet CRD %s and Service %s/%s not found. Checking the permissions needed to install JobSet...", jobSetCRDName, jobSetNamespace, jobSetWebhookServiceName)
	if missing := g.missingPermissions(jobSetInstallPermissionChecks); len(missing) > 0 {
		return fmt.Errorf("JobSet is not installed, and your account cannot install it: missing RBAC permissions [%s]. gcluster made no changes. "+
			"Ask your cluster administrator to install JobSet (for example with jobset.install: true in the kubectl-apply module) or to run this command once", strings.Join(missing, ", "))
	}
	logging.Info("Installing JobSet %s cluster-wide in namespace %s...", defaultJobSetVersion, jobSetNamespace)
	if err := g.installJobSetCRD(fmt.Sprintf("https://github.com/kubernetes-sigs/jobset/releases/download/%s/manifests.yaml", defaultJobSetVersion)); err != nil {
		return fmt.Errorf("failed to install JobSet: %w", err)
	}
	logging.Info("JobSet %s is installed.", defaultJobSetVersion)
	return nil
}

// partialJobSetInstallError explains a JobSet installation that has only one of its CRD and webhook
// Service. Its fixes never re-apply a JobSet release older than the installed one, or over a
// controller Deployment of another release: either can leave the webhook without endpoints.
func (g *GKEOrchestrator) partialJobSetInstallError(crdExists bool) error {
	kubectl := func(args ...string) string { return kubectlCommandLine(g.kubeContext, args...) }
	if !crdExists {
		return fmt.Errorf("JobSet is only partly installed: Service %s/%s exists (or could not be read; see the warning above), but CRD %s was not found. gcluster made no changes.\n"+
			"No JobSet can exist without its CRD, so reinstalling JobSet loses nothing: remove what is left with '%s' (or uninstall its Helm release), "+
			"then re-run this command, or re-run gcluster deploy if JobSet is installed with jobset.install: true in the kubectl-apply module.\n"+
			"If the warning above says your account cannot read the Service, ask your cluster administrator to check the JobSet installation",
			jobSetNamespace, jobSetWebhookServiceName, jobSetCRDName, kubectl("delete", "namespace", jobSetNamespace))
	}
	return fmt.Errorf("JobSet is only partly installed: CRD %[1]s exists (or could not be read; see the warning above), but Service %[2]s/%[3]s was not found. gcluster made no changes.\n"+
		"While that Service is missing, any remaining JobSet webhook configurations reject every JobSet create in the cluster. Find the case that applies:\n"+
		"  - If the warning above says your account cannot read the CRD, JobSet may simply not be installed: ask your cluster administrator to install it, for example with jobset.install: true in the kubectl-apply module.\n"+
		"  - If JobSet is being installed right now, wait a minute and re-run this command.\n"+
		"  - If JobSet was installed with Helm (for example with jobset.install: true) and then uninstalled, which keeps the CRD, re-run gcluster deploy or reinstall the Helm release.\n"+
		"  - If JobSet was installed with kubectl or by gcluster and its namespace or Service was deleted, re-apply the same JobSet release with server-side apply (Helm cannot take over the objects that are left):\n"+
		"      %[4]s\n"+
		"    If Deployment jobset-controller-manager still exists, use the version of its image (%[5]s); otherwise use the installed version or a newer one (this gcluster installs %[6]s), never an older one.\n"+
		"  - If a gcluster run was interrupted while installing JobSet, run the command it printed to finish. If that output is lost and '%[7]s' lists no JobSets, "+
		"run '%[8]s' and '%[9]s' and re-run this command to install JobSet from scratch. Never delete the CRD while JobSets exist: that deletes them.\n"+
		"  - If JobSet runs in another namespace: gcluster job submit currently requires JobSet in namespace %[2]s",
		jobSetCRDName, jobSetNamespace, jobSetWebhookServiceName,
		kubectl("apply", "--server-side", "--force-conflicts", "-f", "https://github.com/kubernetes-sigs/jobset/releases/download/<version>/manifests.yaml"),
		kubectl("-n", jobSetNamespace, "get", "deployments", "-o", "wide"),
		defaultJobSetVersion,
		kubectl("get", "jobsets", "-A"),
		kubectl("delete", "crd", jobSetCRDName),
		kubectl("delete", "namespace", jobSetNamespace))
}

func (g *GKEOrchestrator) installJobSetCRD(jobSetManifestsURL string) error {
	if err := g.checkNoLiveJobSetObjects(); err != nil {
		return err
	}

	manifestBytes, err := g.downloadManifests(jobSetManifestsURL)
	if err != nil {
		return err
	}

	cleanedManifests, err := g.cleanJobSetManifests(manifestBytes)
	if err != nil {
		return err
	}

	manifestPath, err := writeGeneratedManifest(cleanedManifests, jobSetManifestFile(g.kubeContext))
	if err != nil {
		return err
	}
	if err := g.applyManifestFile(manifestPath); err != nil {
		return fmt.Errorf("%w\nJobSet may now be partly installed, and later runs of gcluster will not modify it. To finish the installation, run:\n  %s",
			err, kubectlCommandLine(g.kubeContext, "apply", "-f", manifestPath))
	}

	logging.Info("JobSet components applied successfully.")

	if err := g.waitForJobSetWebhook(); err != nil {
		return fmt.Errorf("%w\nThe JobSet manifests were applied. Later runs of gcluster will wait for the JobSet controller and will not apply the manifests again; "+
			"re-run this command once the pods in namespace %s are ready", err, jobSetNamespace)
	}
	return nil
}

// checkNoLiveJobSetObjects fails unless every JobSet controller Deployment and Service in jobset-system is
// NotFound, so gcluster never applies manifests over leftover or concurrently created objects whose
// selectors could differ.
func (g *GKEOrchestrator) checkNoLiveJobSetObjects() error {
	if g.needsDynamicClientInit() {
		if _, err := g.getDynamicClient(); err != nil {
			return err
		}
	}
	for _, o := range jobSetObjectsWithSelectors {
		_, err := g.kubeClient.GetResource(o.gvr, jobSetNamespace, o.name)
		switch {
		case apierrors.IsNotFound(err):
		case err == nil:
			return fmt.Errorf("JobSet is only partly installed: %s %s/%s already exists, so applying the JobSet manifest could change existing selectors or controller pods. gcluster made no changes. "+
				"Remove the remaining objects in namespace %s (for example with '%s', or by uninstalling its Helm release) before re-running, or ask your cluster administrator",
				o.kind, jobSetNamespace, o.name, jobSetNamespace, kubectlCommandLine(g.kubeContext, "delete", "namespace", jobSetNamespace))
		default:
			return fmt.Errorf("failed to check whether JobSet %s %s/%s already exists (gcluster made no changes): %w", o.kind, jobSetNamespace, o.name, err)
		}
	}
	return nil
}

// jobSetManifestFile names the generated JobSet manifest after the kube context, so an install on another
// cluster cannot overwrite the file that the recovery command of a failed install applies.
func jobSetManifestFile(kubeContext string) string {
	if kubeContext == "" {
		return "jobset.yaml"
	}
	return "jobset-" + strings.Map(func(r rune) rune {
		if r == '.' || r == '-' || r == '_' || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9') {
			return r
		}
		return '_'
	}, kubeContext) + ".yaml"
}

// waitForJobSetWebhook waits for the controller that installJobSetCRD applied, then for a ready webhook endpoint.
func (g *GKEOrchestrator) waitForJobSetWebhook() error {
	logging.Info("Waiting up to 10 minutes for the JobSet controller to roll out...")
	res := g.kubectl("rollout", "status", "deployment/"+jobSetControllerManagerName, "-n", jobSetNamespace, "--timeout=600s")
	if res.ExitCode != 0 {
		return fmt.Errorf("the JobSet controller did not become ready: %s\n%s", strings.TrimSpace(res.Stderr+"\n"+res.Stdout), g.diagnoseJobSetWebhook())
	}
	return g.waitForJobSetWebhookEndpoints()
}

// waitForJobSetWebhookEndpoints polls the JobSet webhook Service until it has a ready endpoint. It works
// for any installation method because it does not depend on the controller Deployment's name.
func (g *GKEOrchestrator) waitForJobSetWebhookEndpoints() error {
	return g.pollJobSetWebhookEndpoints(0, time.Now(), false, nil)
}

func (g *GKEOrchestrator) pollJobSetWebhookEndpoints(startAttempt int, start time.Time, everRead bool, lastErr error) error {
	lastLog := start
	for attempt := startAttempt; attempt < jobSetWebhookPollAttempts; attempt++ {
		if attempt > 0 {
			if time.Since(start)+jobSetWebhookPollInterval >= jobSetWebhookWaitTimeout {
				break
			}
			time.Sleep(jobSetWebhookPollInterval)
		}
		ready, _, err := g.checkServiceEndpointsReady(jobSetNamespace, jobSetWebhookServiceName)
		if err != nil && !errors.Is(err, errEndpointsUnreadable) {
			return err
		}
		if ready {
			if attempt > 0 {
				logging.Info("JobSet webhook is ready after %s.", time.Since(start).Round(time.Second))
			}
			return nil
		}
		everRead = everRead || err == nil
		lastErr = err
		if attempt == 0 {
			logging.Info("Service %s/%s has no ready endpoint yet; the JobSet controller may be starting or restarting. Waiting up to %s...",
				jobSetNamespace, jobSetWebhookServiceName, jobSetWebhookWaitTimeout)
		} else if time.Since(lastLog) >= jobSetWaitLogInterval {
			lastLog = time.Now()
			logging.Info("Still waiting for a ready endpoint on Service %s/%s (%s elapsed)%s",
				jobSetNamespace, jobSetWebhookServiceName, time.Since(start).Round(time.Second), lastReadFailure(err))
		}
	}
	return g.jobSetWebhookTimeoutError(time.Since(start), everRead, lastErr)
}

// jobSetWebhookTimeoutError explains why the wait for a ready JobSet webhook endpoint ended. When no read
// ever succeeded, it skips the diagnosis, whose reads would most likely fail the same way.
func (g *GKEOrchestrator) jobSetWebhookTimeoutError(elapsed time.Duration, everRead bool, lastErr error) error {
	elapsed = elapsed.Round(time.Second)
	if !everRead {
		return fmt.Errorf("could not read the endpoints of Service %s/%s for %s (last error: %v). Check your connection to the cluster and your credentials, then re-run the command",
			jobSetNamespace, jobSetWebhookServiceName, elapsed, lastErr)
	}
	return fmt.Errorf("the JobSet webhook Service %s/%s has no ready endpoint after %s%s. Until it has one, the JobSet admission webhooks (failurePolicy: Fail) "+
		"reject every JobSet create or update and every JobSet pod creation in the cluster, including restarts of running JobSets.\n%s",
		jobSetNamespace, jobSetWebhookServiceName, elapsed, lastReadFailure(lastErr), g.diagnoseJobSetWebhook())
}

func lastReadFailure(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("; the last read failed: %v", err)
}

// checkServiceEndpointsReady reports whether a Service has a ready endpoint. A Forbidden read counts as
// ready, like the other RBAC-tolerant checks; other failed reads wrap errEndpointsUnreadable.
func (g *GKEOrchestrator) checkServiceEndpointsReady(namespace, service string) (ready, forbidden bool, err error) {
	res := g.kubectl("get", "endpointslice", "-l", "kubernetes.io/service-name="+service, "-n", namespace, "-o", "json", kubectlReadTimeout)
	if res.ExitCode != 0 {
		err = fmt.Errorf("%w: %s", errEndpointsUnreadable, strings.TrimSpace(res.Stderr+"\n"+res.Stdout))
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to read %s endpoints in %s (403 Forbidden). Skipping endpoint readiness check.", service, namespace)
			return true, true, nil
		}
		return false, false, err
	}
	var eps k8sEndpointSliceList
	if err := json.Unmarshal([]byte(res.Stdout), &eps); err != nil {
		return false, false, fmt.Errorf("failed to parse EndpointSlices of Service %s/%s: %w", namespace, service, err)
	}
	return eps.HasReadyEndpoint(), false, nil
}

// diagnoseJobSetWebhook explains, on a best-effort basis, why the JobSet webhook Service has no ready endpoint.
func (g *GKEOrchestrator) diagnoseJobSetWebhook() string {
	res := g.kubectl("get", "service", jobSetWebhookServiceName, "-n", jobSetNamespace, "-o", "jsonpath={.spec.selector}", kubectlReadTimeout)
	if res.ExitCode != 0 {
		return fmt.Sprintf("(diagnostics unavailable: %s)", strings.TrimSpace(res.Stderr))
	}
	selector := map[string]string{}
	if out := strings.TrimSpace(res.Stdout); out != "" {
		if err := json.Unmarshal([]byte(out), &selector); err != nil {
			return fmt.Sprintf("(diagnostics unavailable: %v)", err)
		}
	}
	if len(selector) == 0 {
		return fmt.Sprintf("Diagnostics: Service %s/%s has no selector.", jobSetNamespace, jobSetWebhookServiceName)
	}
	res = g.kubectl("get", "pods", "-n", jobSetNamespace, "-o", "json", kubectlReadTimeout)
	if res.ExitCode != 0 {
		return fmt.Sprintf("(diagnostics unavailable: %s)", strings.TrimSpace(res.Stderr))
	}
	var pods corev1.PodList
	if err := json.Unmarshal([]byte(res.Stdout), &pods); err != nil {
		return fmt.Sprintf("(diagnostics unavailable: %v)", err)
	}
	return fmt.Sprintf("%s\nTo investigate, run:\n  %s\n  %s", describeJobSetPods(selector, pods.Items),
		kubectlCommandLine(g.kubeContext, "-n", jobSetNamespace, "get", "deployments,services,pods", "-o", "wide", "--show-labels"),
		kubectlCommandLine(g.kubeContext, "-n", jobSetNamespace, "describe", "pods"))
}

// describeJobSetPods reports which pods in the JobSet namespace match the webhook Service selector.
func describeJobSetPods(selector map[string]string, pods []corev1.Pod) string {
	sel := labels.SelectorFromSet(selector)
	matched := 0
	for _, p := range pods {
		if sel.Matches(labels.Set(p.Labels)) {
			matched++
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Diagnostics: Service %s/%s selects %s; %d of %d pods in %s match.", jobSetNamespace, jobSetWebhookServiceName, sel, matched, len(pods), jobSetNamespace)
	for _, p := range pods {
		podLabels := labels.Set(p.Labels)
		problem := podProblem(p)
		if problem != "" {
			problem += "; "
		}
		fmt.Fprintf(&b, "\n  - %s: %sphase=%s, ready=%t, matches selector=%t, labels=%s", p.Name, problem, p.Status.Phase, podReady(p), sel.Matches(podLabels), podLabels)
	}
	switch {
	case len(pods) == 0:
		fmt.Fprintf(&b, "\nNo pods are running in namespace %s.", jobSetNamespace)
	case matched == 0:
		b.WriteString("\nNo pod matches the Service selector. This usually means a JobSet manifest was only partly applied (the Service selector changed but the controller Deployment did not). " +
			"A cluster administrator can restore the webhook by setting the selector of the JobSet Services to the controller Deployment's selector.")
	}
	return b.String()
}

func podReady(p corev1.Pod) bool {
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

// podProblem names the first waiting init or app container with its image, or else why the pod is not scheduled.
func podProblem(p corev1.Pod) string {
	if msg := waitingContainerProblem("init container", p.Status.InitContainerStatuses); msg != "" {
		return msg
	}
	if msg := waitingContainerProblem("container", p.Status.ContainerStatuses); msg != "" {
		return msg
	}
	for _, c := range p.Status.Conditions {
		if c.Type == corev1.PodScheduled && c.Status == corev1.ConditionFalse {
			return fmt.Sprintf("%s (%s)", c.Reason, c.Message)
		}
	}
	return ""
}

func waitingContainerProblem(kind string, statuses []corev1.ContainerStatus) string {
	for _, cs := range statuses {
		if w := cs.State.Waiting; w != nil && w.Reason != "" {
			detail := fmt.Sprintf("%s %s, image %s", kind, cs.Name, cs.Image)
			if w.Message != "" {
				detail += ": " + w.Message
			}
			return fmt.Sprintf("%s (%s)", w.Reason, detail)
		}
	}
	return ""
}

// missingPermissions returns the checks that `kubectl auth can-i` denies, formatted as 'verb resource'.
// The checked resources are cluster-scoped, so the checks ask about all namespaces: otherwise a
// RoleBinding in the current namespace would wrongly count as permission to create them.
func (g *GKEOrchestrator) missingPermissions(checks []permissionCheck) []string {
	var missing []string
	for _, c := range checks {
		res := g.kubectl("auth", "can-i", c.verb, c.resource, "--all-namespaces")
		if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "yes" {
			missing = append(missing, fmt.Sprintf("'%s %s'", c.verb, c.resource))
		}
	}
	return missing
}

func (g *GKEOrchestrator) getHTTPClient() HTTPClient {
	g.httpOnce.Do(func() {
		if g.httpClient == nil {
			g.httpClient = &http.Client{Timeout: 30 * time.Second}
		}
	})
	return g.httpClient
}

func (g *GKEOrchestrator) downloadManifests(url string) ([]byte, error) {
	logging.Info("Downloading manifests from %s", url)
	resp, err := g.getHTTPClient().Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download manifests: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download manifests: received status code %d", resp.StatusCode)
	}

	manifestBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifests: %w", err)
	}
	return manifestBytes, nil
}

func (g *GKEOrchestrator) cleanJobSetManifests(manifestBytes []byte) ([]byte, error) {
	logging.Info("Cleaning JobSet manifests (removing description fields)...")
	return g.cleanAndProcessManifests(manifestBytes, func(data map[interface{}]interface{}) {
		g.injectTolerationsAndLabels(data)
	})
}

func (g *GKEOrchestrator) cleanAndProcessManifests(manifestBytes []byte, processFn func(map[interface{}]interface{})) ([]byte, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(manifestBytes))
	var cleanedManifests bytes.Buffer

	for {
		var doc interface{}
		if err := decoder.Decode(&doc); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode YAML document: %w", err)
		}

		if doc == nil {
			continue
		}

		if data, ok := doc.(map[interface{}]interface{}); ok {
			g.removeDescriptionFields(data)
			if processFn != nil {
				processFn(data)
			}
			cleanedBytes, err := yaml.Marshal(data)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal cleaned YAML: %w", err)
			}
			cleanedManifests.Write(cleanedBytes)
			cleanedManifests.WriteString("---\n")
		} else {
			cleanedBytes, err := yaml.Marshal(doc)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal YAML document: %w", err)
			}
			cleanedManifests.Write(cleanedBytes)
			cleanedManifests.WriteString("---\n")
		}
	}
	return cleanedManifests.Bytes(), nil
}

func (g *GKEOrchestrator) injectTolerationsAndLabels(data map[interface{}]interface{}) {
	kind, ok := data["kind"].(string)
	if !ok || kind != "Deployment" {
		return
	}

	meta, ok := data["metadata"].(map[interface{}]interface{})
	if !ok {
		return
	}
	name, ok := meta["name"].(string)
	if !ok || (name != jobSetControllerManagerName && name != jobSetHelmControllerName) {
		return
	}

	spec, ok := data["spec"].(map[interface{}]interface{})
	if !ok {
		return
	}
	template, ok := spec["template"].(map[interface{}]interface{})
	if !ok {
		return
	}
	podSpec, ok := template["spec"].(map[interface{}]interface{})
	if !ok {
		return
	}

	tolerations := []interface{}{
		map[interface{}]interface{}{
			"key":      "nvidia.com/gpu",
			"operator": "Exists",
			"effect":   "NoSchedule",
		},
		map[interface{}]interface{}{
			"key":      "components.gke.io/gke-managed-components",
			"operator": "Exists",
			"effect":   "NoSchedule",
		},
	}

	if existingTolerations, ok := podSpec["tolerations"].([]interface{}); ok {
		podSpec["tolerations"] = append(existingTolerations, tolerations...)
	} else {
		podSpec["tolerations"] = tolerations
	}

	if podMeta, ok := template["metadata"].(map[interface{}]interface{}); ok {
		labels, ok := podMeta["labels"].(map[interface{}]interface{})
		if !ok {
			labels = make(map[interface{}]interface{})
			podMeta["labels"] = labels
		}
		labels["app.kubernetes.io/instance"] = "jobset"
		labels["app.kubernetes.io/name"] = "jobset"
		labels["control-plane"] = "controller-manager"
		labels["app.kubernetes.io/component"] = "controller-manager"
	}
}

// applyManifests saves manifests under ~/.gcluster/generated and applies them.
func (g *GKEOrchestrator) applyManifests(manifests []byte, filename string) error {
	logging.Info("Applying manifests for %s...", filename)
	filePath, err := writeGeneratedManifest(manifests, filename)
	if err != nil {
		return err
	}
	return g.applyManifestFile(filePath)
}

// writeGeneratedManifest saves manifests as ~/.gcluster/generated/<filename> and returns the file's path.
func writeGeneratedManifest(manifests []byte, filename string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get user home directory: %w", err)
	}

	stateDir := filepath.Join(homeDir, ".gcluster", "generated")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory for generated manifests at %q. Please check your file system permissions for this path: %w", stateDir, err)
	}

	filePath := filepath.Join(stateDir, filename)
	if err := os.WriteFile(filePath, manifests, 0644); err != nil {
		return "", fmt.Errorf("failed to write manifests to %s: %w", filePath, err)
	}
	logging.Info("Manifests saved to %s", filePath)
	return filePath, nil
}

func (g *GKEOrchestrator) applyManifestFile(filePath string) error {
	res := g.kubectl("apply", "-f", filePath)
	if res.ExitCode != 0 {
		return fmt.Errorf("kubectl apply failed with exit code %d: %s\n%s", res.ExitCode, res.Stderr, res.Stdout)
	}
	logging.Info("Manifests applied successfully.")
	return nil
}

func (g *GKEOrchestrator) removeDescriptionFields(data map[interface{}]interface{}) {
	for key, value := range data {
		if key == "description" {
			delete(data, key)
			continue
		}
		if subMap, ok := value.(map[interface{}]interface{}); ok {
			g.removeDescriptionFields(subMap)
		} else if subList, ok := value.([]interface{}); ok {
			for _, item := range subList {
				if itemMap, ok := item.(map[interface{}]interface{}); ok {
					g.removeDescriptionFields(itemMap)
				}
			}
		}
	}
}

// ValidateClusterState runs all cluster-specific validations to fail early on invalid state.
func (g *GKEOrchestrator) ValidateClusterState(job *orchestrator.JobDefinition) error {
	validators := []func() error{
		func() error {
			return g.validateTargetNamespaceExists(job.ClusterName, job.ClusterLocation, job.ProjectID)
		},
		func() error { return g.CheckAndInstallKueue("", job.ClusterName, job.ClusterLocation) },
		g.checkAndInstallJobSetCRD,
	}

	if job.PriorityClassName != "" {
		validators = append(validators, func() error {
			return g.ensureAndValidatePriorityClass(job.PriorityClassName)
		})
	}

	for _, validate := range validators {
		if err := validate(); err != nil {
			return err
		}
	}
	return nil
}

func (g *GKEOrchestrator) getKubeClient() KubeClient {
	g.syncKubeClient()
	return g.kubeClient
}

// getCurrentNamespace is the single source of truth for the target namespace.
// It returns the cached namespace (seeded from --gke-namespace at every
// orchestrator entry point) or, if unset, resolves and caches the namespace of
// the pinned kubeconfig context, falling back to a lookup by cluster name.
func (g *GKEOrchestrator) getCurrentNamespace(clusterName, location, projectID string) (string, error) {
	if g.namespace != "" {
		return g.namespace, nil
	}

	if ns, ok := g.pinnedContextNamespace(); ok {
		g.namespace = ns
		return ns, nil
	}
	ns, err := g.getKubeClient().GetCurrentNamespace(clusterName, location, projectID)
	if err != nil {
		return "", err
	}
	g.namespace = ns
	return ns, nil
}

// pinnedContextNamespace reads the namespace of g.kubeContext from kubeconfig.
func (g *GKEOrchestrator) pinnedContextNamespace() (string, bool) {
	if g.kubeContext == "" {
		return "", false
	}
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return "", false
	}
	kubeCtx := cfg.Contexts[g.kubeContext]
	if kubeCtx == nil {
		return "", false
	}
	if kubeCtx.Namespace == "" {
		return "default", true
	}
	return kubeCtx.Namespace, true
}

func (g *GKEOrchestrator) validateTargetNamespaceExists(clusterName, location, projectID string) error {
	ns, err := g.getCurrentNamespace(clusterName, location, projectID)
	if err != nil {
		return err
	}

	if ns == "" {
		return fmt.Errorf("target namespace cannot be empty for GKE cluster %q. Please pass --gke-namespace, or set a default namespace in your kubeconfig context (e.g., 'kubectl config set-context --current --namespace=<namespace>')", clusterName)
	}

	client, err := g.getDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to initialize Kubernetes client for namespace validation: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err = client.Resource(namespaceGVR).Get(ctx, ns, metav1.GetOptions{})
	switch {
	case err == nil:
		return nil
	case apierrors.IsNotFound(err):
		return fmt.Errorf("target namespace %q does not exist on GKE cluster %q. Please create the namespace first (e.g., 'kubectl create namespace %s')", ns, clusterName, ns)
	case isForbiddenError(err):
		logging.Warn("Insufficient RBAC permissions to verify existence of namespace %q on cluster %q (403 Forbidden). Proceeding with job submission...", ns, clusterName)
		return nil
	default:
		return fmt.Errorf("failed to verify existence of namespace %q on cluster %q: %w", ns, clusterName, err)
	}
}

// Initialize fetches GKE cluster metadata and resolves the cluster location,
// handling regional fallback if necessary.
func (g *GKEOrchestrator) Initialize(clusterName, location, projectID string) (string, error) {
	g.projectID = projectID

	logging.Info("Fetching GKE cluster metadata for '%s'...", clusterName)
	res := g.executor.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", "container", "clusters", "describe", clusterName,
		"--location", location,
		"--project", g.projectID,
		"--format=json")

	timeoutMsg := fmt.Sprintf("timed out after %v while trying to reach GKE cluster '%s' in '%s'. Please check your network connection", shell.DefaultCloudAPITimeout, clusterName, location)
	if err := shell.HandleExecError(res, "gcloud", timeoutMsg); err != nil {
		return "", err
	}

	if res.ExitCode != 0 {
		if strings.Contains(res.Stderr, "403") || strings.Contains(strings.ToLower(res.Stderr), "permission denied") {
			return "", fmt.Errorf("your account lacks the required permission to access cluster '%s' in project '%s'. Please ask your project administrator to grant you the Kubernetes Engine Viewer role (roles/container.viewer)", clusterName, g.projectID)
		}
		// If the user specified a zone (location with 3 components, e.g. us-central1-a), try to fallback to the region
		if len(strings.Split(location, "-")) == 3 {
			region := shell.ExtractRegion(location)
			logging.Info("Failed to find cluster in zone %s. Trying fallback to region %s...", location, region)
			fallbackRes := g.executor.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", "container", "clusters", "describe", clusterName,
				"--location", region,
				"--project", g.projectID,
				"--format=json")

			fallbackTimeoutMsg := fmt.Sprintf("timed out after %v while trying to reach GKE cluster '%s' in fallback region '%s'. Please check your network connection", shell.DefaultCloudAPITimeout, clusterName, region)
			if err := shell.HandleExecError(fallbackRes, "gcloud", fallbackTimeoutMsg); err != nil {
				return "", err
			}

			if fallbackRes.ExitCode == 0 {
				logging.Warn("Cluster '%s' is a regional cluster in '%s'. Found it by falling back from zone '%s'. "+
					"Note: This does NOT restrict your job to '%s'. To run specifically in '%s', "+
					"please use the '--node-constraint topology.kubernetes.io/zone=%s' flag.",
					clusterName, region, location, location, location, location)
				location = region
				res = fallbackRes
			} else {
				return "", fmt.Errorf("failed to describe GKE cluster %s in zone %s and fallback region %s: %s", clusterName, location, region, res.Stderr)
			}
		} else {
			return "", fmt.Errorf("failed to describe GKE cluster %s: %s", clusterName, res.Stderr)
		}
	}

	var clusterDesc gkeCluster
	if err := json.Unmarshal([]byte(res.Stdout), &clusterDesc); err != nil {
		return "", fmt.Errorf("failed to parse GKE cluster description: %w", err)
	}

	g.clusterZones = clusterDesc.Locations
	g.clusterDesc = clusterDesc

	g.napEnabled = clusterDesc.Autoscaling.EnableNodeAutoprovisioning
	g.napLimits = parseNAPLimits(clusterDesc.Autoscaling)

	return location, nil
}

func (g *GKEOrchestrator) configureClusterEnvironment(job *orchestrator.JobDefinition) error {
	ns, err := g.getCurrentNamespace(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		return err
	}

	explicitQueue := job.KueueQueueName != ""
	localQueue, discovered, err := g.resolveKueueQueue(job.KueueQueueName, ns)
	if err != nil {
		if job.DryRunManifest == "" {
			return err
		}
		logging.Info("Warning: Failed to auto-discover Kueue Queue Name: %v. Falling back to %s for dry-run.", err, defaultLocalQueue)
		localQueue = defaultLocalQueue
	}
	job.KueueQueueName = localQueue

	if job.DryRunManifest != "" {
		return nil
	}
	if err := g.ensureLocalQueueExists(localQueue, ns, discovered, explicitQueue); err != nil {
		return err
	}
	if job.IsPathwaysJob {
		return g.ensureClusterQueueCoverage(localQueue, ns)
	}
	return nil
}

// ensureLocalQueueExists verifies that the specified LocalQueue exists in the namespace, prompting to create it if missing.
func (g *GKEOrchestrator) ensureLocalQueueExists(localQueue, ns string, discovered, explicitQueue bool) error {
	if discovered {
		return nil
	}
	if explicitQueue {
		exists, err := g.checkLocalQueueExists(localQueue, ns)
		if err != nil {
			return fmt.Errorf("failed to check if LocalQueue exists: %w", err)
		}
		if exists {
			return nil
		}
	}
	var availableStr string
	if explicitQueue {
		if availableQueues := g.listLocalQueues(ns); len(availableQueues) > 0 {
			availableStr = fmt.Sprintf(" Available LocalQueues in namespace '%s': %s.", ns, strings.Join(availableQueues, ", "))
		}
	}
	promptMsg := fmt.Sprintf("LocalQueue '%s' does not exist in namespace '%s'.%s Do you want gcluster to create default Kueue resources (ClusterQueue and LocalQueue) with calculated cluster capacity?", localQueue, ns, availableStr)
	if !shell.PromptYesNo(promptMsg) {
		return fmt.Errorf("LocalQueue '%s' does not exist in namespace '%s' and user declined to create default queues.%s Please create one manually or specify an existing queue using --queue flag", localQueue, ns, availableStr)
	}
	return g.createDefaultQueues(localQueue, ns)
}

// isForbiddenError returns true if the error indicates a 403 Forbidden RBAC permission error.
func isForbiddenError(err error) bool {
	if err == nil {
		return false
	}
	return apierrors.IsForbidden(err) || strings.Contains(strings.ToLower(err.Error()), "forbidden")
}

func (g *GKEOrchestrator) verifyCheckpointConfigurationCR(job *orchestrator.JobDefinition, docRemediationMsg string) error {
	client, err := g.getDynamicClient()
	if err != nil {
		return fmt.Errorf("failed to initialize dynamic client to verify CheckpointConfiguration: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checkpointConfigList, err := client.Resource(checkpointConfigurationGVR).List(ctx, metav1.ListOptions{Limit: 1})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("the CheckpointConfiguration CustomResourceDefinition (CRD) is not registered on the cluster. %s: %w", docRemediationMsg, err)
		}
		if isForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to verify CheckpointConfiguration resources (403 Forbidden). Assuming CheckpointConfiguration is configured in shared cluster and proceeding with job submission.")
			return nil
		}
		return fmt.Errorf("failed to verify CheckpointConfiguration resource: %w", err)
	}
	if len(checkpointConfigList.Items) == 0 {
		return fmt.Errorf("Multi-Tier Checkpointing (MTC) requires a CheckpointConfiguration resource to be deployed on the cluster. With Cluster Toolkit, set checkpoint_configuration on the kubectl-apply module. %s", docRemediationMsg)
	}

	return g.validateCheckpointConfigurationCompatibility(client, &checkpointConfigList.Items[0], job)
}

// mtcCandidateNodePools returns the node pools pinned with --node-constraint cloud.google.com/gke-nodepool,
// or else the non-system node pools with the job's machine type.
func (g *GKEOrchestrator) mtcCandidateNodePools(nodePoolConstraint, machineType string) []*gkeJobNodePool {
	var pinned []string
	if nodePoolConstraint != "" {
		for _, name := range strings.Split(nodePoolConstraint, "|") {
			pinned = append(pinned, strings.TrimSpace(name))
		}
	}
	var pools []*gkeJobNodePool
	for i := range g.clusterDesc.NodePools {
		np := &g.clusterDesc.NodePools[i]
		match := slices.Contains(pinned, np.Name)
		if len(pinned) == 0 {
			match = machineType != "" && !g.isSystemPool(*np) && strings.EqualFold(np.Config.MachineType, machineType)
		}
		if match {
			pools = append(pools, np)
		}
	}
	return pools
}

// hasAccelerators reports whether the node pool or machine type has accelerators attached.
func (g *GKEOrchestrator) hasAccelerators(np *gkeJobNodePool, machineType, location string) bool {
	if np != nil && len(np.Config.Accelerators) > 0 {
		return true
	}
	if machineType == "" {
		return false
	}
	cap, err := g.FetchMachineCapabilities(machineType, location)
	return err == nil && len(cap.Accelerators) > 0
}

func tolerationMatchesTaint(tol map[string]interface{}, taint gkeTaint) bool {
	effect, _ := tol["effect"].(string)
	if effect != "" && effect != taint.Effect {
		return false
	}
	op, _ := tol["operator"].(string)
	key, _ := tol["key"].(string)
	val, _ := tol["value"].(string)

	switch op {
	case "Exists":
		return key == "" || key == taint.Key
	case "", "Equal":
		return key == taint.Key && val == taint.Value
	default:
		return false
	}
}

func tolerationsCoverTaint(rawTolerations []interface{}, taint gkeTaint) bool {
	for _, item := range rawTolerations {
		if tol, ok := item.(map[string]interface{}); ok && tolerationMatchesTaint(tol, taint) {
			return true
		}
	}
	return false
}

func nodeSelectorMatchesPool(crName string, nodeSelector map[string]string, np *gkeJobNodePool) error {
	if want := nodeSelector[nodePoolLabel]; want != "" && want != np.Name {
		return fmt.Errorf("CheckpointConfiguration %q targets %s=%q in spec.nodeSelector, which does not match the job's node pool %q; the multitier-driver DaemonSet will not run on the job's nodes", crName, nodePoolLabel, want, np.Name)
	}
	for k, v := range nodeSelector {
		if actual, ok := np.Config.Labels[k]; ok && actual != v {
			return fmt.Errorf("CheckpointConfiguration %q targets %s=%q in spec.nodeSelector, which does not match label %s=%q on node pool %q; the multitier-driver DaemonSet will not run on the job's nodes", crName, k, v, k, actual, np.Name)
		}
	}
	return nil
}

// checkpointConfigCoversPool returns an error if the CheckpointConfiguration's nodeSelector or
// tolerations keep the multitier driver off the node pool (np is nil when no pool is known).
func (g *GKEOrchestrator) checkpointConfigCoversPool(crName string, nodeSelector map[string]string, tolerations []interface{}, np *gkeJobNodePool, machineType, location string) error {
	var taints []gkeTaint
	if np != nil {
		machineType = np.Config.MachineType
		if err := nodeSelectorMatchesPool(crName, nodeSelector, np); err != nil {
			return err
		}
		for _, t := range np.Config.Taints {
			if t.Effect == "NoSchedule" || t.Effect == "NoExecute" {
				taints = append(taints, t)
			}
		}
	}
	// GKE taints accelerator nodes without listing the taint in the node pool config;
	// as elsewhere in gcluster, a non-TPU machine with accelerators is a GPU machine.
	if config.IsTPU(machineType) {
		taints = append(taints, gkeTaint{Key: "google.com/tpu", Value: "present", Effect: "NoSchedule"})
	} else if g.hasAccelerators(np, machineType, location) {
		taints = append(taints, gkeTaint{Key: "nvidia.com/gpu", Value: "present", Effect: "NoSchedule"})
	}
	for _, t := range taints {
		if !tolerationsCoverTaint(tolerations, t) {
			return fmt.Errorf("CheckpointConfiguration %q is missing a toleration for taint \"%s=%s:%s\" on the target nodes; the multitier-driver DaemonSet will not schedule on those nodes. Add a matching toleration to your CheckpointConfiguration (e.g. under checkpoint_configuration.tolerations in your blueprint) and redeploy", crName, t.Key, t.Value, t.Effect)
		}
	}
	return nil
}

// coveredCheckpointPools returns the candidate pools the CheckpointConfiguration covers. When it covers
// none, the error is from the pool its nodeSelector targets, so it names the missing toleration or label.
func (g *GKEOrchestrator) coveredCheckpointPools(crName string, nodeSelector map[string]string, tolerations []interface{}, pools []*gkeJobNodePool, location string) ([]*gkeJobNodePool, error) {
	var covered []*gkeJobNodePool
	var bestErr error
	for _, np := range pools {
		err := g.checkpointConfigCoversPool(crName, nodeSelector, tolerations, np, np.Config.MachineType, location)
		if err == nil {
			covered = append(covered, np)
		} else if bestErr == nil || nodeSelectorMatchesPool(crName, nodeSelector, np) == nil {
			bestErr = err
		}
	}
	if len(pools) > 0 && len(covered) == 0 {
		return nil, bestErr
	}
	return covered, nil
}

func (g *GKEOrchestrator) validateCheckpointConfigurationCompatibility(client dynamic.Interface, cr *unstructured.Unstructured, job *orchestrator.JobDefinition) error {
	crName := cr.GetName()
	machineType := job.MachineType
	nodeSelector, _, _ := unstructured.NestedStringMap(cr.Object, "spec", "nodeSelector")
	tolerations, _, _ := unstructured.NestedSlice(cr.Object, "spec", "tolerations")

	if instType := nodeSelector["node.kubernetes.io/instance-type"]; instType != "" && machineType != "" && !strings.EqualFold(instType, machineType) {
		return fmt.Errorf("CheckpointConfiguration %q targets node.kubernetes.io/instance-type=%q in spec.nodeSelector, which does not match the job's machine type %q; the multitier-driver DaemonSet will not run on the job's nodes. Update checkpoint_configuration.node_selector in your blueprint and redeploy", crName, instType, machineType)
	}

	// The job can land on any candidate pool, so one pool the driver covers is enough.
	pools := g.mtcCandidateNodePools(job.NodeConstraint[nodePoolLabel], machineType)
	if len(pools) == 0 {
		if err := g.checkpointConfigCoversPool(crName, nodeSelector, tolerations, nil, machineType, job.ClusterLocation); err != nil {
			return err
		}
	}
	coveredPools, err := g.coveredCheckpointPools(crName, nodeSelector, tolerations, pools, job.ClusterLocation)
	if err != nil {
		return err
	}

	uid := string(cr.GetUID())
	if uid == "" {
		return nil
	}
	canScaleFromZero := g.napEnabled || slices.ContainsFunc(coveredPools, func(np *gkeJobNodePool) bool {
		return np.Autoscaling.Enabled && np.Autoscaling.MinNodeCount == 0
	})
	return waitForMTCDaemonSetReady(client, crName, uid, canScaleFromZero)
}

var (
	mtcDaemonSetPollInterval = 5 * time.Second
	mtcDaemonSetPollTimeout  = 2 * time.Minute
)

// mtcDaemonSetNotReady returns why the multitier driver DaemonSet is not ready, or nil if it is.
func mtcDaemonSetNotReady(ds *unstructured.Unstructured, crName string, canScaleFromZero bool) error {
	gen, _, _ := unstructured.NestedInt64(ds.Object, "metadata", "generation")
	obsGen, _, _ := unstructured.NestedInt64(ds.Object, "status", "observedGeneration")
	desired, _, _ := unstructured.NestedInt64(ds.Object, "status", "desiredNumberScheduled")
	ready, _, _ := unstructured.NestedInt64(ds.Object, "status", "numberReady")
	updated, hasUpdated, _ := unstructured.NestedInt64(ds.Object, "status", "updatedNumberScheduled")
	switch {
	case obsGen < gen: // Also true while the status is not populated yet.
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s is still reconciling generation %d (observedGeneration=%d)", mtcDriverNamespace, ds.GetName(), gen, obsGen)
	case desired == 0 && canScaleFromZero:
		return nil
	case desired == 0:
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s has 0 desired pods (desiredNumberScheduled=0); verify that CheckpointConfiguration %q nodeSelector and tolerations match your target nodes", mtcDriverNamespace, ds.GetName(), crName)
	case hasUpdated && updated < desired:
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s has only %d of %d pods updated to the latest version; wait for the rollout to finish or check 'kubectl describe pods -n %s'", mtcDriverNamespace, ds.GetName(), updated, desired, mtcDriverNamespace)
	case ready < desired:
		return fmt.Errorf("the MTC CSI driver DaemonSet %s/%s has %d of %d pods ready; wait for the driver pods to become Ready or check 'kubectl describe pods -n %s' (common causes: missing bucket IAM permissions or disabled Hierarchical Namespace on the checkpoint bucket)", mtcDriverNamespace, ds.GetName(), ready, desired, mtcDriverNamespace)
	}
	return nil
}

// waitForMTCDaemonSetReady polls until all multitier driver pods are ready. As in the other
// gcluster wait loops, a 403 skips the check and any other error is retried until the deadline.
func waitForMTCDaemonSetReady(client dynamic.Interface, crName, uid string, canScaleFromZero bool) error {
	dsName := "multitier-driver-" + uid
	dsClient := client.Resource(daemonsetGVR).Namespace(mtcDriverNamespace)
	deadline := time.Now().Add(mtcDaemonSetPollTimeout)
	loggedWait := false
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		ds, err := dsClient.Get(ctx, dsName, metav1.GetOptions{})
		cancel()

		var lastErr error
		switch {
		case isForbiddenError(err):
			logging.Warn("Insufficient RBAC permissions to verify MTC driver DaemonSet %s/%s (403 Forbidden). Proceeding with job submission.", mtcDriverNamespace, dsName)
			return nil
		case apierrors.IsNotFound(err):
			lastErr = fmt.Errorf("the MTC CSI driver DaemonSet %q was not found in namespace %q for CheckpointConfiguration %q (uid %s); verify that the HighScaleCheckpointing controller is healthy (`kubectl get daemonset -n %s`)", dsName, mtcDriverNamespace, crName, uid, mtcDriverNamespace)
		case err != nil:
			lastErr = fmt.Errorf("failed to verify MTC driver DaemonSet %s/%s: %w", mtcDriverNamespace, dsName, err)
		default:
			if lastErr = mtcDaemonSetNotReady(ds, crName, canScaleFromZero); lastErr == nil {
				return nil
			}
		}

		if time.Now().Add(mtcDaemonSetPollInterval).After(deadline) {
			return lastErr
		}
		if !loggedWait {
			logging.Info("Waiting up to %s for MTC driver DaemonSet %s/%s to become ready...", mtcDaemonSetPollTimeout, mtcDriverNamespace, dsName)
			loggedWait = true
		}
		time.Sleep(mtcDaemonSetPollInterval)
	}
}

// The GKE-managed MTC driver runs as this Kubernetes service account.
const (
	mtcDriverNamespace = "gke-managed-checkpointing"
	mtcDriverKSA       = "gke-checkpointing-multitier-node"
	// gcpServiceAccountAnnotation makes GKE authenticate a Kubernetes service
	// account as the annotated Google service account.
	gcpServiceAccountAnnotation = "iam.gke.io/gcp-service-account"
)

// legacyMTCDriverGSA returns the GCP service account that older gcluster
// versions annotated on the MTC driver's Kubernetes service account, or "" if
// the annotation is absent or cannot be read.
func (g *GKEOrchestrator) legacyMTCDriverGSA() string {
	client, err := g.getDynamicClient()
	if err != nil {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ksa, err := client.Resource(serviceAccountGVR).Namespace(mtcDriverNamespace).Get(ctx, mtcDriverKSA, metav1.GetOptions{})
	if err != nil {
		return ""
	}
	return ksa.GetAnnotations()[gcpServiceAccountAnnotation]
}

// validateMTCJobFlags checks the MTC flags; SubmitJob runs it before any cluster call.
func (g *GKEOrchestrator) validateMTCJobFlags(job *orchestrator.JobDefinition) error {
	if !job.GKEMTCEnabled {
		if job.GKEMTCRamdiskDirectory != "" {
			return fmt.Errorf("--gke-mtc-ramdisk-dir requires --gke-mtc-enabled")
		}
		return nil
	}
	if strings.TrimSpace(job.GKEMTCRamdiskDirectory) == "" {
		return fmt.Errorf("ramdisk directory path (--gke-mtc-ramdisk-dir) cannot be empty when Multi-Tier Checkpointing (MTC) is enabled (e.g. --gke-mtc-ramdisk-dir=/tmp/mtc_checkpoints)")
	}
	sm := &StorageManager{orchestrator: g}
	return sm.ValidateRamdiskDir(job.GKEMTCRamdiskDirectory, job.RawMounts, job.IsPathwaysJob, job.Pathways.ColocatedPythonSidecarImage != "")
}

func (g *GKEOrchestrator) validateMTCConfig(job *orchestrator.JobDefinition) error {
	if job == nil || !job.GKEMTCEnabled {
		return nil
	}

	mtcDocURL := "https://cloud.google.com/kubernetes-engine/docs/how-to/machine-learning/training/multi-tier-checkpointing"
	docRemediationMsg := fmt.Sprintf("Please follow the official GKE documentation to enable this feature on your cluster: %s", mtcDocURL)

	if g.clusterDesc.AddonsConfig == nil || g.clusterDesc.AddonsConfig.HighScaleCheckpointingConfig == nil || !g.clusterDesc.AddonsConfig.HighScaleCheckpointingConfig.Enabled {
		return fmt.Errorf("Multi-Tier Checkpointing (MTC) requires the HighScaleCheckpointing addon to be enabled on the target GKE cluster. With Cluster Toolkit, set enable_multi_tier_checkpointing: true (and enable_gcsfuse_csi: true) on the gke-cluster module. %s", docRemediationMsg)
	}

	if job.DryRunManifest != "" {
		return nil
	}

	if err := g.verifyCheckpointConfigurationCR(job, docRemediationMsg); err != nil {
		return err
	}
	if gsa := g.legacyMTCDriverGSA(); gsa != "" {
		logging.Warn("The MTC driver service account %s/%s still has the %s=%s annotation added by older gcluster versions. It overrides the direct Workload Identity grant (mtc_service_account_member), so checkpoint bucket mounts may fail with PermissionDenied. See \"Migrating from earlier gcluster versions\" in docs/gke-advanced-features.md.", mtcDriverNamespace, mtcDriverKSA, gcpServiceAccountAnnotation, gsa)
	}
	return nil
}
