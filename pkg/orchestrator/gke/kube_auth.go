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
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"hpc-toolkit/pkg/logging"

	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

const (
	// refreshCredentialsEnv forces get-credentials even when a reusable context exists.
	refreshCredentialsEnv = "GCLUSTER_REFRESH_CREDENTIALS"
	// gkeAuthPluginBinary mints a token per kubectl call, so entries using it never go stale.
	gkeAuthPluginBinary = "gke-gcloud-auth-plugin"
	// probeRequestTimeout keeps an unreachable endpoint from blocking for the full kubectl dial timeout.
	probeRequestTimeout = "5s"
)

// kubeconfigLoader returns the merged kubeconfig; injectable for tests.
type kubeconfigLoader func() (*clientcmdapi.Config, error)

func defaultKubeconfigLoader() (*clientcmdapi.Config, error) {
	return clientcmd.NewDefaultClientConfigLoadingRules().Load()
}

// endpointMode identifies which control-plane endpoint a kubeconfig cluster entry targets.
type endpointMode int

const (
	endpointModeUnknown endpointMode = iota
	endpointModeIP                   // public/private IP; subject to authorized networks
	endpointModeDNS                  // *.gke.goog; IAM-gated, bypasses authorized networks
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

// findKubeContext returns the gcloud-named context, or a legacy context renamed to the bare cluster name.
func findKubeContext(cfg *clientcmdapi.Config, clusterName, location, projectID string) (string, *clientcmdapi.Context, bool) {
	if cfg == nil {
		return "", nil, false
	}
	expected := gkeContextName(projectID, location, clusterName)
	if kubeCtx, ok := cfg.Contexts[expected]; ok && kubeCtx != nil {
		return expected, kubeCtx, true
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.Contexts)) {
		kubeCtx := cfg.Contexts[name]
		if kubeCtx != nil && (name == clusterName || kubeCtx.Cluster == clusterName) {
			return name, kubeCtx, true
		}
	}
	return "", nil, false
}

// kubeContextCheck is the static verdict on an existing kubeconfig context.
type kubeContextCheck struct {
	contextName string
	mode        endpointMode
	current     string // current-context at load time
	reason      string // non-empty when the context must not be reused
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
	check := kubeContextCheck{contextName: name, current: cfg.CurrentContext}

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

	auth := cfg.AuthInfos[kubeCtx.AuthInfo]
	var cmd string
	if auth != nil && auth.Exec != nil {
		// Windows kubeconfigs name the plugin gke-gcloud-auth-plugin.exe and may use backslash paths; filepath.Base
		// on Unix (e.g. WSL) does not split on backslashes.
		command := strings.ReplaceAll(auth.Exec.Command, `\`, "/")
		cmd = strings.TrimSuffix(strings.ToLower(filepath.Base(command)), ".exe")
	}
	if cmd != gkeAuthPluginBinary {
		check.reason = fmt.Sprintf("user %q does not authenticate via %s", kubeCtx.AuthInfo, gkeAuthPluginBinary)
		return check
	}
	return check
}

// probeOutcome classifies a connectivity probe result.
type probeOutcome int

const (
	probeOK           probeOutcome = iota
	probeForbidden                 // reachable and authenticated; RBAC denies /version
	probeConnectivity              // this endpoint is unreachable or untrusted from here
	probeAuth                      // credentials are broken; another endpoint will not help
	probeUnknown
)

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

// probeContext runs a bounded read of /version against contextName (or the
// current context when empty). Success proves network, TLS and credentials.
func (g *GKEOrchestrator) probeContext(contextName string) (probeOutcome, string) {
	args := []string{"get", "--raw", "/version", "--request-timeout=" + probeRequestTimeout}
	if contextName != "" {
		args = append([]string{"--context", contextName}, args...)
	}
	res := g.executor.ExecuteCommand("kubectl", args...)
	if res.ExitCode == 0 {
		return probeOK, ""
	}
	stderr := strings.TrimSpace(res.Stderr)
	return classifyProbeFailure(stderr), stderr
}

// checkClusterConnectivity verifies the current kubectl context can reach the
// cluster. Unrecognised probe output is not fatal, matching configureKubectl.
func (g *GKEOrchestrator) checkClusterConnectivity(clusterName string) error {
	logging.Info("Checking cluster connectivity...")
	outcome, stderr := g.probeContext("")
	switch outcome {
	case probeOK, probeForbidden:
		logging.Info("Cluster connectivity verified.")
		return nil
	case probeAuth:
		return authFailureError(clusterName, stderr)
	case probeConnectivity:
		return fmt.Errorf("failed to connect to GKE cluster %q. Please verify your IP is allowed in the cluster's authorized networks or that you have correct network access. Error: %s", clusterName, stderr)
	default:
		logging.Warn("Could not verify connectivity to GKE cluster '%s' (%s). Proceeding; subsequent kubectl calls will report the underlying error.", clusterName, firstLine(stderr))
		return nil
	}
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
	return defaultKubeconfigLoader()
}

// inspectExistingContext loads the kubeconfig and statically validates the cluster's entry.
func (g *GKEOrchestrator) inspectExistingContext(clusterName, location, projectID string) kubeContextCheck {
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return kubeContextCheck{reason: fmt.Sprintf("failed to load kubeconfig: %v", err)}
	}
	return evaluateExistingContext(cfg, clusterName, location, projectID, g.clusterDesc)
}

// ensureCurrentContext points current-context at the reused context.
func (g *GKEOrchestrator) ensureCurrentContext(check kubeContextCheck) error {
	if check.current == check.contextName {
		return nil
	}
	res := g.executor.ExecuteCommand("kubectl", "config", "use-context", check.contextName)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to switch kubectl current-context to %q: %s", check.contextName, res.Stderr)
	}
	return nil
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
	credsRes := g.executor.ExecuteCommand("gcloud", args...)
	if credsRes.ExitCode != 0 {
		if strings.Contains(strings.ToLower(credsRes.Stderr), "multiple") || strings.Contains(strings.ToLower(credsRes.Stderr), "ambiguous") {
			return fmt.Errorf("found multiple GKE clusters named %s. Please specify the exact Zone using --location to disambiguate", clusterName)
		}
		return fmt.Errorf("failed to get GKE cluster credentials: %s\n%s", credsRes.Stderr, credsRes.Stdout)
	}
	return nil
}

// restoreNamespaceContext re-applies a non-default namespace that gcloud reset.
func (g *GKEOrchestrator) restoreNamespaceContext(namespace string) error {
	if namespace == "" || namespace == "default" {
		return nil
	}
	logging.Info("Restoring namespace context to '%s'...", namespace)
	restoreRes := g.executor.ExecuteCommand("kubectl", "config", "set-context", "--current", "--namespace="+namespace)
	if restoreRes.ExitCode != 0 {
		return fmt.Errorf("failed to restore namespace context to %s: %s", namespace, restoreRes.Stderr)
	}
	return nil
}

// refreshCredentialsPreservingNamespace runs get-credentials and keeps the user's namespace.
func (g *GKEOrchestrator) refreshCredentialsPreservingNamespace(clusterName, clusterLocation, projectID string, mode endpointMode) error {
	originalNamespace, err := g.getCurrentNamespace(clusterName, clusterLocation, projectID)
	if err != nil {
		logging.Warn("Could not read current namespace before gcloud (defaulting to 'default'): %v. If you want to target a specific namespace please use the --gke-namespace flag", err)
		originalNamespace = "default"
	}
	if err := g.refreshGKEAuth(clusterName, clusterLocation, projectID, mode == endpointModeDNS); err != nil {
		return err
	}
	return g.restoreNamespaceContext(originalNamespace)
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
			return nil
		}
	}

	return fmt.Errorf("failed to connect to GKE cluster %q via any available control-plane endpoint (%s). "+
		"Please verify your IP is allowed in the cluster's authorized networks, that you have network access to the control plane, "+
		"and that the cluster's DNS endpoint allows external traffic if you are outside its authorized networks",
		clusterName, strings.Join(attempts, "; "))
}

// configureKubectl makes kubectl target the cluster. A verified, reachable
// existing context is reused as-is (get-credentials would rewrite its endpoint);
// otherwise credentials are fetched, falling back across endpoint modes.
// GCLUSTER_REFRESH_CREDENTIALS=1 skips reuse.
func (g *GKEOrchestrator) configureKubectl(clusterName, clusterLocation, projectID string) error {
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
		if err := g.ensureCurrentContext(check); err != nil {
			return err
		}
		logging.Info("Reusing existing kubeconfig context '%s' (%s endpoint). Set %s=1 to force a credential refresh.", check.contextName, check.mode, refreshCredentialsEnv)
		return nil
	case probeAuth:
		return authFailureError(clusterName, stderr)
	case probeConnectivity:
		logging.Warn("Existing kubeconfig context '%s' (%s endpoint) is unreachable from here (%s). Refreshing credentials...", check.contextName, check.mode, firstLine(stderr))
		avoid = check.mode
	default:
		if err := g.ensureCurrentContext(check); err != nil {
			return err
		}
		logging.Warn("Could not verify existing kubeconfig context '%s' (%s). Reusing it; set %s=1 to force a credential refresh.", check.contextName, firstLine(stderr), refreshCredentialsEnv)
		return nil
	}
	return g.regenerateKubeContext(clusterName, clusterLocation, projectID, avoid)
}
