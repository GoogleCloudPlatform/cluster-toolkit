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

// Kubeconfig and GKE credential handling: endpoint selection, context reuse,
// credential refresh and connectivity probing.

package gke

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	// refreshCredentialsEnv forces get-credentials even when a reusable context exists.
	refreshCredentialsEnv = "GCLUSTER_REFRESH_CREDENTIALS"
	// gkeAuthPluginBinary mints a token per kubectl call, so entries using it never go stale.
	gkeAuthPluginBinary = "gke-gcloud-auth-plugin"
	// probeRequestTimeout keeps an unreachable endpoint from blocking for the full kubectl dial timeout.
	probeRequestTimeout = "5s"
	// gkeVersionProbeTimeout bounds the Layer-1 kubectl version tie-breaker on static-token GKE endpoints.
	gkeVersionProbeTimeout        = "2s"
	gkeVersionProbeProcessTimeout = 3 * time.Second
)

func (m endpointMode) String() string {
	switch m {
	case endpointModeIP:
		return "IP"
	case endpointModeDNS:
		return "DNS"
	default:
		return "unknown"
	}
}

func (m endpointMode) other() endpointMode {
	switch m {
	case endpointModeIP:
		return endpointModeDNS
	case endpointModeDNS:
		return endpointModeIP
	default:
		return endpointModeUnknown
	}
}

// shouldUseDNSEndpoint picks the default mode for a first get-credentials call.
// IP is preferred when a public IP endpoint exists (DNS can hit HTTP 431 behind
// some proxies); DNS is used when it is the only externally reachable option.
func shouldUseDNSEndpoint(cfg *controlPlaneEndpointsConfig) bool {
	if cfg == nil || cfg.DnsEndpointConfig == nil || !cfg.DnsEndpointConfig.AllowExternalTraffic {
		return false
	}
	return cfg.IPEndpointsConfig == nil || !cfg.IPEndpointsConfig.EnablePublicEndpoint
}

// knownEndpoints maps every server URL the cluster can be reached at to its endpoint mode.
func (c gkeCluster) knownEndpoints() map[string]endpointMode {
	out := make(map[string]endpointMode)
	add := func(host string, mode endpointMode) {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			out["https://"+host] = mode
		}
	}
	if cpec := c.ControlPlaneEndpointsConfig; cpec != nil {
		if cpec.DnsEndpointConfig != nil {
			add(cpec.DnsEndpointConfig.Endpoint, endpointModeDNS)
		}
		if cpec.IPEndpointsConfig != nil {
			add(cpec.IPEndpointsConfig.PublicEndpoint, endpointModeIP)
			add(cpec.IPEndpointsConfig.PrivateEndpoint, endpointModeIP)
		}
	}
	add(c.Endpoint, endpointModeIP)
	if c.PrivateClusterConfig != nil {
		add(c.PrivateClusterConfig.PublicEndpoint, endpointModeIP)
		add(c.PrivateClusterConfig.PrivateEndpoint, endpointModeIP)
	}
	return out
}

// dnsEndpointAvailable reports whether `--dns-endpoint` can work for this cluster.
func (c gkeCluster) dnsEndpointAvailable() bool {
	cpec := c.ControlPlaneEndpointsConfig
	return cpec != nil && cpec.DnsEndpointConfig != nil && cpec.DnsEndpointConfig.AllowExternalTraffic
}

// gkeContextName is the context name gcloud writes for a cluster.
func gkeContextName(projectID, location, clusterName string) string {
	return fmt.Sprintf("gke_%s_%s_%s", projectID, location, clusterName)
}

// findKubeContext returns the gcloud-named context (including zonal-to-regional fallback), or a context matching clusterName.
func findKubeContext(cfg *clientcmdapi.Config, clusterName, location, projectID string) (string, *clientcmdapi.Context, bool) {
	if cfg == nil || clusterName == "" {
		return "", nil, false
	}
	if projectID != "" && location != "" {
		expected := gkeContextName(projectID, location, clusterName)
		if kubeCtx, ok := cfg.Contexts[expected]; ok && kubeCtx != nil {
			return expected, kubeCtx, true
		}
		if region := shell.ExtractRegion(location); region != location {
			regExpected := gkeContextName(projectID, region, clusterName)
			if kubeCtx, ok := cfg.Contexts[regExpected]; ok && kubeCtx != nil {
				return regExpected, kubeCtx, true
			}
		}
	}
	return k8s.FindContextByCluster(cfg, clusterName)
}

func resolveGKEContext(cfg *clientcmdapi.Config, clusterName, location, projectID string) (string, *clientcmdapi.Context, string, bool) {
	if strings.TrimSpace(clusterName) != "" {
		name, kubeCtx, ok := findKubeContext(cfg, clusterName, location, projectID)
		if !ok {
			return "", nil, "", false
		}
		return name, kubeCtx, clusterName, true
	}
	return k8s.ResolveCurrentContext(cfg)
}

// detectGKE evaluates GKE kubeconfig credentials, *.gke.goog endpoints, and the bounded 2s serverVersion tie-breaker.
func detectGKE(in orchestrator.ProbeInput) (orchestrator.Detection, bool) {
	ctxName, kubeCtx, resolvedCluster, ok := resolveGKEContext(in.Config, in.ClusterName, in.Location, in.ProjectID)
	if !ok {
		// Only treat --project/--location as GKE bootstrap when no kubeconfig context matched; saved `job config`
		// defaults also populate these fields when targeting an existing non-GKE KUBECONFIG context.
		if strings.TrimSpace(in.ProjectID) != "" || strings.TrimSpace(in.Location) != "" {
			return orchestrator.Detection{
				Type:        orchestrator.TypeGKE,
				KubeContext: "",
				ClusterName: in.ClusterName,
				Reason:      "GKE flags specified with no existing kubeconfig context",
			}, true
		}
		return orchestrator.Detection{}, false
	}
	auth := in.Config.AuthInfos[kubeCtx.AuthInfo]
	cluster := in.Config.Clusters[kubeCtx.Cluster]
	if k8s.ExecPluginBaseName(auth) == gkeAuthPluginBinary {
		return orchestrator.Detection{
			Type:        orchestrator.TypeGKE,
			KubeContext: ctxName,
			ClusterName: resolvedCluster,
			Reason:      fmt.Sprintf("kubeconfig context %q authenticates via %s", ctxName, gkeAuthPluginBinary),
		}, true
	}
	if host := k8s.ClusterServerHostname(cluster); strings.HasSuffix(host, ".gke.goog") {
		return orchestrator.Detection{
			Type:        orchestrator.TypeGKE,
			KubeContext: ctxName,
			ClusterName: resolvedCluster,
			Reason:      fmt.Sprintf("kubeconfig context %q targets GKE endpoint %s", ctxName, host),
		}, true
	}
	if gitVersion, isGKEServer := probeGKEServerVersion(in, ctxName, auth); isGKEServer {
		return orchestrator.Detection{
			Type:        orchestrator.TypeGKE,
			KubeContext: ctxName,
			ClusterName: resolvedCluster,
			Reason:      fmt.Sprintf("API server for context %q reported GKE version %s", ctxName, gitVersion),
		}, true
	}
	return orchestrator.Detection{}, false
}

func probeGKEServerVersion(in orchestrator.ProbeInput, ctxName string, auth *clientcmdapi.AuthInfo) (string, bool) {
	if in.AirGapped || ctxName == "" || in.Runner == nil || k8s.HasClientCertificateAuth(auth) {
		return "", false
	}
	res := in.Runner.ExecuteCommandWithTimeout(
		gkeVersionProbeProcessTimeout,
		"kubectl",
		k8s.WithContext(ctxName, []string{"version", "-o", "json", "--request-timeout=" + gkeVersionProbeTimeout})...,
	)
	if strings.TrimSpace(res.Stdout) == "" {
		return "", false
	}
	var payload struct {
		ServerVersion *struct {
			GitVersion string `json:"gitVersion"`
		} `json:"serverVersion"`
	}
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil || payload.ServerVersion == nil {
		return "", false
	}
	ver := strings.TrimSpace(payload.ServerVersion.GitVersion)
	return ver, strings.Contains(strings.ToLower(ver), "-gke.")
}

func (k kubeContextCheck) reusable() bool { return k.reason == "" }

// evaluateExistingContext checks, offline, that the kubeconfig entry belongs to
// this cluster (server is a known endpoint; CA matches for IP endpoints) and
// authenticates via the GKE auth plugin. Liveness is checked by probeContext.
func evaluateExistingContext(cfg *clientcmdapi.Config, clusterName, location, projectID string, desc gkeCluster) kubeContextCheck {
	name, kubeCtx, ok := findKubeContext(cfg, clusterName, location, projectID)
	if !ok {
		return kubeContextCheck{reason: fmt.Sprintf("no kubeconfig context found for cluster %q", clusterName)}
	}
	check := kubeContextCheck{contextName: name}

	cluster := cfg.Clusters[kubeCtx.Cluster]
	if cluster == nil {
		check.reason = fmt.Sprintf("context %q references unknown cluster entry %q", name, kubeCtx.Cluster)
		return check
	}
	// Tools such as Terraform or Helm may write the default HTTPS port explicitly;
	// knownEndpoints() registers bare https://<host>, so drop it before matching.
	server := strings.ToLower(strings.TrimRight(strings.TrimSpace(cluster.Server), "/"))
	server = strings.TrimSuffix(server, ":443")
	mode, known := desc.knownEndpoints()[server]
	if !known {
		check.reason = fmt.Sprintf("server %q does not match any endpoint of cluster %q", cluster.Server, clusterName)
		return check
	}
	check.mode = mode

	if mode == endpointModeIP {
		if desc.MasterAuth == nil || desc.MasterAuth.ClusterCaCertificate == "" {
			check.reason = "cannot verify certificate authority: cluster description carries no CA"
			return check
		}
		if len(cluster.CertificateAuthorityData) == 0 {
			check.reason = "certificate authority data is missing or file-based in kubeconfig"
			return check
		}
		if base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData) != desc.MasterAuth.ClusterCaCertificate {
			check.reason = "certificate authority mismatch (cluster may have been recreated)"
			return check
		}
	}

	if k8s.ExecPluginBaseName(cfg.AuthInfos[kubeCtx.AuthInfo]) != gkeAuthPluginBinary {
		check.reason = fmt.Sprintf("user %q does not authenticate via %s", kubeCtx.AuthInfo, gkeAuthPluginBinary)
		return check
	}
	return check
}

var (
	probeAuthMarkers = []string{
		"unauthorized",
		"getting credentials",
		gkeAuthPluginBinary,
		"fetch token",
		"could not get token",
		"application default credentials",
		"reauthentication",
	}
	// Only the apiserver's status error counts as RBAC-forbidden; a forward proxy's "403 Forbidden" on CONNECT is a connectivity failure.
	probeForbiddenMarkers    = []string{"error from server (forbidden)"}
	probeConnectivityMarkers = []string{
		"i/o timeout",
		"handshake timeout",
		"client.timeout",
		"timed out",
		"deadline exceeded",
		"connection refused",
		"connection reset",
		"no route to host",
		"network is unreachable",
		"no such host",
		"tls:",
		"tls handshake",
		"x509:",
		"status code 431",
		"request header fields too large",
		// Forward proxies report an unreachable upstream as a gateway error.
		"status code 502",
		"bad gateway",
		"status code 503",
		"service unavailable",
		"status code 504",
		"gateway timeout",
		"dial tcp",
		"unexpected eof",
		"unable to connect to the server",
	}
)

// classifyProbeFailure maps kubectl stderr to a probeOutcome. Auth markers win
// because kubectl prefixes plugin failures with "Unable to connect to the server:".
func classifyProbeFailure(stderr string) probeOutcome {
	s := strings.ToLower(stderr)
	contains := func(markers []string) bool {
		for _, m := range markers {
			if strings.Contains(s, m) {
				return true
			}
		}
		return false
	}
	switch {
	case contains(probeAuthMarkers):
		return probeAuth
	case contains(probeForbiddenMarkers):
		return probeForbidden
	case contains(probeConnectivityMarkers):
		return probeConnectivity
	default:
		return probeUnknown
	}
}

// probeContext runs a bounded read of /version against contextName. Success proves network, TLS and credentials.
func (g *GKEOrchestrator) probeContext(contextName string) (probeOutcome, string) {
	res := g.kubectlIn(contextName, "get", "--raw", "/version", "--request-timeout="+probeRequestTimeout)
	if res.ExitCode == 0 {
		return probeOK, ""
	}
	stderr := strings.TrimSpace(res.Stderr)
	return classifyProbeFailure(stderr), stderr
}

func authFailureError(clusterName, stderr string) error {
	return fmt.Errorf("authentication to GKE cluster %q failed: %s. Re-fetching cluster credentials will not help; run 'gcloud auth application-default login' (or 'gcloud auth login') and ensure %s is installed",
		clusterName, firstLine(stderr), gkeAuthPluginBinary)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// forceCredentialRefresh reports whether the user asked to bypass kubeconfig reuse.
func forceCredentialRefresh() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(refreshCredentialsEnv)))
	return v != "" && v != "0" && v != "false" && v != "no"
}

func (g *GKEOrchestrator) loadKubeconfig() (*clientcmdapi.Config, error) {
	if g.kubeconfigLoader != nil {
		return g.kubeconfigLoader()
	}
	return k8s.DefaultKubeconfigLoader()
}

// kubectlCommandLine renders a kubectl command, pinned like the ones gcluster runs, for users to copy.
// Arguments that contain whitespace, such as Windows paths, are double-quoted.
func kubectlCommandLine(kubeContext string, args ...string) string {
	parts := append([]string{"kubectl"}, k8s.WithContext(kubeContext, args)...)
	for i, p := range parts {
		if strings.ContainsAny(p, " \t") {
			parts[i] = `"` + p + `"`
		}
	}
	return strings.Join(parts, " ")
}

// kubectlIn runs kubectl against an explicit non-empty kubeContext, refusing an empty context so probes never
// silently fall back to current-context.
func (g *GKEOrchestrator) kubectlIn(kubeContext string, args ...string) shell.CommandResult {
	ctx := strings.TrimSpace(kubeContext)
	if ctx == "" {
		err := fmt.Errorf("internal error: refusing to run kubectl without an explicit --context")
		return shell.CommandResult{ExitCode: 1, Stderr: err.Error(), Err: err}
	}
	return g.executor.ExecuteCommand("kubectl", k8s.WithContext(ctx, args)...)
}

// kubectl runs kubectl against the context chosen by configureKubectl.
func (g *GKEOrchestrator) kubectl(args ...string) shell.CommandResult {
	return g.executor.ExecuteCommand("kubectl", k8s.WithContext(g.kubeContext, args)...)
}

// kubectlWithTimeout runs kubectl with a process timeout against the context chosen by configureKubectl.
func (g *GKEOrchestrator) kubectlWithTimeout(timeout time.Duration, args ...string) shell.CommandResult {
	return g.executor.ExecuteCommandWithTimeout(timeout, "kubectl", k8s.WithContext(g.kubeContext, args)...)
}

// kubectlStream streams kubectl output against the context chosen by configureKubectl.
func (g *GKEOrchestrator) kubectlStream(args ...string) error {
	return g.executor.ExecuteCommandStream("kubectl", k8s.WithContext(g.kubeContext, args)...)
}

// inspectExistingContext loads the kubeconfig and statically validates the cluster's entry.
func (g *GKEOrchestrator) inspectExistingContext(clusterName, location, projectID string) kubeContextCheck {
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return kubeContextCheck{reason: fmt.Sprintf("failed to load kubeconfig: %v", err)}
	}
	return evaluateExistingContext(cfg, clusterName, location, projectID, g.clusterDesc)
}

// endpointModeOrder lists the modes to try, preferred first. A mode already
// seen to be unreachable is demoted rather than dropped.
func (g *GKEOrchestrator) endpointModeOrder(avoid endpointMode) []endpointMode {
	preferred := endpointModeIP
	if shouldUseDNSEndpoint(g.clusterDesc.ControlPlaneEndpointsConfig) {
		preferred = endpointModeDNS
	}
	order := []endpointMode{preferred}
	if alt := preferred.other(); alt == endpointModeIP || g.clusterDesc.dnsEndpointAvailable() {
		order = append(order, alt)
	}
	if len(order) > 1 && order[0] == avoid {
		order[0], order[1] = order[1], order[0]
	}
	return order
}

// refreshGKEAuth runs `gcloud container clusters get-credentials`.
func (g *GKEOrchestrator) refreshGKEAuth(clusterName, clusterLocation, projectID string, useDNSEndpoint bool) error {
	args := []string{"container", "clusters", "get-credentials", clusterName, "--location", clusterLocation, "--project", projectID}
	if useDNSEndpoint {
		args = append(args, "--dns-endpoint")
	}
	credsRes := g.executor.ExecuteCommandWithTimeout(shell.DefaultCloudAPITimeout, "gcloud", args...)
	timeoutMsg := fmt.Sprintf("timed out after %v while fetching GKE cluster credentials for '%s' in '%s'. Please check your network connection", shell.DefaultCloudAPITimeout, clusterName, clusterLocation)
	if err := shell.HandleExecError(credsRes, "gcloud", timeoutMsg); err != nil {
		return err
	}
	if credsRes.ExitCode != 0 {
		if strings.Contains(strings.ToLower(credsRes.Stderr), "multiple") || strings.Contains(strings.ToLower(credsRes.Stderr), "ambiguous") {
			return fmt.Errorf("found multiple GKE clusters named %s. Please specify the exact Zone using --location to disambiguate", clusterName)
		}
		return fmt.Errorf("failed to get GKE cluster credentials: %s\n%s", credsRes.Stderr, credsRes.Stdout)
	}
	return nil
}

// restoreNamespaceContext re-applies a non-default namespace that gcloud reset.
func (g *GKEOrchestrator) restoreNamespaceContext(contextName, namespace string) error {
	if namespace == "" || namespace == "default" {
		return nil
	}
	logging.Info("Restoring namespace context to '%s'...", namespace)
	restoreRes := g.executor.ExecuteCommand("kubectl", "config", "set-context", contextName, "--namespace="+namespace)
	if restoreRes.ExitCode != 0 {
		return fmt.Errorf("failed to restore namespace context to %s: %s", namespace, restoreRes.Stderr)
	}
	return nil
}

// refreshCredentialsPreservingNamespace runs get-credentials and keeps the namespace
// already stored in the kubeconfig context (not the --gke-namespace flag value).
func (g *GKEOrchestrator) refreshCredentialsPreservingNamespace(clusterName, clusterLocation, projectID string, mode endpointMode) error {
	originalNamespace := "default"
	if cfg, err := g.loadKubeconfig(); err == nil {
		if _, kubeCtx, ok := findKubeContext(cfg, clusterName, clusterLocation, projectID); ok && kubeCtx.Namespace != "" {
			originalNamespace = kubeCtx.Namespace
		}
	}
	if err := g.refreshGKEAuth(clusterName, clusterLocation, projectID, mode == endpointModeDNS); err != nil {
		return err
	}
	return g.restoreNamespaceContext(gkeContextName(projectID, clusterLocation, clusterName), originalNamespace)
}

// regenerateKubeContext fetches credentials per endpoint mode in order, moving
// to the next mode on a connectivity failure.
func (g *GKEOrchestrator) regenerateKubeContext(clusterName, clusterLocation, projectID string, avoid endpointMode) error {
	contextName := gkeContextName(projectID, clusterLocation, clusterName)
	modes := g.endpointModeOrder(avoid)
	var attempts []string

	for i, mode := range modes {
		if err := g.refreshCredentialsPreservingNamespace(clusterName, clusterLocation, projectID, mode); err != nil {
			return err
		}
		outcome, stderr := g.probeContext(contextName)
		switch outcome {
		case probeOK, probeForbidden:
			if i > 0 {
				logging.Warn("Connected to cluster '%s' via its %s endpoint after the %s endpoint was unreachable. kubeconfig context '%s' now points at the %s endpoint.",
					clusterName, mode, modes[i-1], contextName, mode)
			} else {
				logging.Info("Cluster connectivity verified via %s endpoint.", mode)
			}
			g.kubeContext = contextName
			return nil
		case probeAuth:
			return authFailureError(clusterName, stderr)
		case probeConnectivity:
			attempts = append(attempts, fmt.Sprintf("%s endpoint: %s", mode, firstLine(stderr)))
			if i+1 < len(modes) {
				logging.Warn("Cluster '%s' is unreachable via its %s endpoint (%s). Retrying with the %s endpoint...", clusterName, mode, firstLine(stderr), modes[i+1])
			}
		default:
			logging.Warn("Could not verify connectivity to cluster '%s' via its %s endpoint (%s). Proceeding; subsequent kubectl calls will report the underlying error.", clusterName, mode, firstLine(stderr))
			g.kubeContext = contextName
			return nil
		}
	}

	return fmt.Errorf("failed to connect to GKE cluster %q via any available control-plane endpoint (%s). "+
		"Please verify your IP is allowed in the cluster's authorized networks, that you have network access to the control plane, "+
		"and that the cluster's DNS endpoint allows external traffic if you are outside its authorized networks",
		clusterName, strings.Join(attempts, "; "))
}

// configureKubectl picks the context for this run and pins later kubectl and
// dynamic-client calls to it: reuse a verified context, else fetch credentials.
// Reuse never writes to kubeconfig; only get-credentials changes current-context.
// GCLUSTER_REFRESH_CREDENTIALS=1 skips reuse.
func (g *GKEOrchestrator) configureKubectl(clusterName, clusterLocation, projectID string) error {
	if g.mode == orchestrator.TypeK8s {
		return g.configureK8sKubectl(clusterName)
	}
	g.kubeContext = ""
	avoid := endpointModeUnknown

	if forceCredentialRefresh() {
		logging.Info("%s is set; refreshing GKE credentials for cluster '%s'...", refreshCredentialsEnv, clusterName)
		return g.regenerateKubeContext(clusterName, clusterLocation, projectID, avoid)
	}

	check := g.inspectExistingContext(clusterName, clusterLocation, projectID)
	if !check.reusable() {
		logging.Info("No reusable kubeconfig context for cluster '%s' (%s). Fetching credentials...", clusterName, check.reason)
		return g.regenerateKubeContext(clusterName, clusterLocation, projectID, avoid)
	}

	outcome, stderr := g.probeContext(check.contextName)
	switch outcome {
	case probeOK, probeForbidden:
		g.kubeContext = check.contextName
		logging.Info("Reusing existing kubeconfig context '%s' (%s endpoint); use 'kubectl --context %s' for manual commands. Set %s=1 to force a credential refresh.", check.contextName, check.mode, check.contextName, refreshCredentialsEnv)
		return nil
	case probeAuth:
		return authFailureError(clusterName, stderr)
	case probeConnectivity:
		logging.Warn("Existing kubeconfig context '%s' (%s endpoint) is unreachable from here (%s). Refreshing credentials...", check.contextName, check.mode, firstLine(stderr))
		avoid = check.mode
	default:
		g.kubeContext = check.contextName
		logging.Warn("Could not verify existing kubeconfig context '%s' (%s). Reusing it; use 'kubectl --context %s' for manual commands, or set %s=1 to force a credential refresh.", check.contextName, firstLine(stderr), check.contextName, refreshCredentialsEnv)
		return nil
	}
	return g.regenerateKubeContext(clusterName, clusterLocation, projectID, avoid)
}

// usableKubeClient returns the Kubernetes API client for best-effort discovery, or nil when none can be
// built yet (no pinned context, or an unreadable kubeconfig).
func (g *GKEOrchestrator) usableKubeClient() KubeClient {
	if g.needsDynamicClientInit() {
		if g.dynClient == nil && g.kubeContext == "" {
			return nil
		}
		if _, err := g.getDynamicClient(); err != nil {
			logging.Warn("Kubernetes API client unavailable for cluster discovery (context %q): %v", g.kubeContext, err)
			return nil
		}
	}
	return g.kubeClient
}

// activeWorkloadImages returns the distinct images the rendered pods pull: the workload image, the ML
// Diagnostics telemetry sidecar image when enabled, and, for Pathways, the server, proxy, worker, and
// colocated sidecar images of the selected deployment mode.
func activeWorkloadImages(job orchestrator.JobDefinition, fullImageName, telemetryImage string) []string {
	raw := []string{fullImageName}
	if job.IsPathwaysJob {
		pw := job.Pathways
		if pw.Headless || pw.DeploySPS {
			raw = nil
		}
		if pw.RunSPSWorkload == "" {
			raw = append(raw, pw.ServerImage, pw.WorkerImage, pw.ColocatedPythonSidecarImage)
		}
		if !pw.DeploySPS {
			raw = append(raw, pw.ProxyServerImage)
		}
	}
	raw = append(raw, telemetryImage)
	var images []string
	for _, img := range raw {
		if img != "" && !slices.Contains(images, img) {
			images = append(images, img)
		}
	}
	return images
}

// resolveImagePullSecrets returns the user's --image-pull-secret value verbatim when it was given (an empty value
// attaches none), otherwise the dockerconfigjson secrets in the target namespace matching the workload's images
// (including the ML Diagnostics telemetry sidecar image when sidecar injection is active).
// An unavailable API client or an RBAC denial degrades to no secrets; a namespace that cannot be resolved is an error.
func (g *GKEOrchestrator) resolveImagePullSecrets(job orchestrator.JobDefinition, fullImageName, telemetryImage string) (string, error) {
	if job.ImagePullSecrets != "" || job.ImagePullSecretsSpecified {
		return job.ImagePullSecrets, nil
	}
	kc := g.usableKubeClient()
	if kc == nil {
		return "", nil
	}
	ns, err := g.getCurrentNamespace(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve the target namespace for imagePullSecrets discovery: %w", err)
	}
	return k8s.DiscoverImagePullSecrets(kc, ns, job.ServiceAccountName, activeWorkloadImages(job, fullImageName, telemetryImage)), nil
}
