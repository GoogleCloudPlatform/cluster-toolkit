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
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

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
	// gkeVersionProbeTimeout bounds the kubectl version tie-breaker during orchestrator detection on static-token endpoints.
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
func (c gkeCluster) shouldUseDNSEndpoint() bool {
	if !c.dnsEndpointAvailable() {
		return false
	}
	ipCfg := c.ControlPlaneEndpointsConfig.IPEndpointsConfig
	return !c.ipEndpointAvailable() || ipCfg == nil || !ipCfg.EnablePublicEndpoint
}

// ipEndpointAvailable reports whether the cluster exposes any IP control-plane endpoint.
// An omitted Enabled field defaults to true for legacy API responses.
func (c gkeCluster) ipEndpointAvailable() bool {
	cpec := c.ControlPlaneEndpointsConfig
	return cpec == nil || cpec.IPEndpointsConfig == nil || cpec.IPEndpointsConfig.Enabled == nil || *cpec.IPEndpointsConfig.Enabled
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
	cpec := c.ControlPlaneEndpointsConfig
	if c.ipEndpointAvailable() {
		if cpec != nil && cpec.IPEndpointsConfig != nil {
			add(cpec.IPEndpointsConfig.PublicEndpoint, endpointModeIP)
			add(cpec.IPEndpointsConfig.PrivateEndpoint, endpointModeIP)
		}
		add(c.Endpoint, endpointModeIP)
		if c.PrivateClusterConfig != nil {
			add(c.PrivateClusterConfig.PublicEndpoint, endpointModeIP)
			add(c.PrivateClusterConfig.PrivateEndpoint, endpointModeIP)
		}
	}
	// Register DNS after IP so DNS wins when a DNS-only cluster mirrors its DNS host in c.Endpoint.
	if cpec != nil && cpec.DnsEndpointConfig != nil {
		add(cpec.DnsEndpointConfig.Endpoint, endpointModeDNS)
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
	return fmt.Sprintf("gke_%s_%s_%s", strings.TrimSpace(projectID), strings.TrimSpace(location), strings.TrimSpace(clusterName))
}

func gkeNamedContext(cfg *clientcmdapi.Config, clusterName, location, projectID string) (string, *clientcmdapi.Context, bool) {
	clusterName = strings.TrimSpace(clusterName)
	location = strings.TrimSpace(location)
	projectID = strings.TrimSpace(projectID)
	if cfg == nil || clusterName == "" || location == "" || projectID == "" {
		return "", nil, false
	}
	return k8s.FindContextByCluster(cfg, gkeContextName(projectID, location, clusterName))
}

// findKubeContext returns the gcloud-named context for (clusterName, location, projectID),
// or a context matching clusterName. Called after Initialize has resolved location,
// so no zone-to-region fallback is used here.
func findKubeContext(cfg *clientcmdapi.Config, clusterName, location, projectID string) (string, *clientcmdapi.Context, bool) {
	if cfg == nil || strings.TrimSpace(clusterName) == "" {
		return "", nil, false
	}
	if name, kubeCtx, ok := gkeNamedContext(cfg, clusterName, location, projectID); ok {
		return name, kubeCtx, true
	}
	return k8s.FindContextByCluster(cfg, clusterName)
}

// resolveGKEContext resolves a GKE context before or outside Initialize: when clusterName is given it checks
// the exact gcloud context name, then a zone-to-region gcloud context name (for regional clusters passed a zone),
// then k8s.ResolveContext; when clusterName is empty it falls back to current-context.
func resolveGKEContext(cfg *clientcmdapi.Config, clusterName, location, projectID string) (string, *clientcmdapi.Context, string, bool) {
	clusterName = strings.TrimSpace(clusterName)
	location = strings.TrimSpace(location)
	if cfg != nil && clusterName != "" {
		if name, kubeCtx, ok := gkeNamedContext(cfg, clusterName, location, projectID); ok {
			return name, kubeCtx, clusterName, true
		}
		if region := shell.ExtractRegion(location); region != location {
			if name, kubeCtx, ok := gkeNamedContext(cfg, clusterName, region, projectID); ok {
				return name, kubeCtx, clusterName, true
			}
		}
	}
	return k8s.ResolveContext(cfg, clusterName)
}

func hasGKEFlags(projectID, location string) bool {
	return strings.TrimSpace(projectID) != "" || strings.TrimSpace(location) != ""
}

func isMultiCloudOrAttachedGKEName(s string) bool {
	parts := strings.SplitN(strings.TrimSpace(s), "_", 4)
	if len(parts) != 4 || parts[0] != "gke" || parts[2] == "" {
		return false
	}
	switch parts[1] {
	case "aws", "azure", "attached":
		return strings.Contains(parts[3], "_")
	default:
		return false
	}
}

func isCanonicalGKEName(s string) bool {
	parts := strings.SplitN(strings.TrimSpace(s), "_", 4)
	if len(parts) != 4 || parts[0] != "gke" || parts[1] == "" || parts[2] == "" || parts[3] == "" {
		return false
	}
	return !isMultiCloudOrAttachedGKEName(s)
}

func isConnectGatewayHost(host string) bool {
	for _, suffix := range []string{"connectgateway.googleapis.com", "connectgateway.mtls.googleapis.com"} {
		if host == suffix || strings.HasSuffix(host, "-"+suffix) || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

func hasEdgeCloudFlag(args []string) bool {
	edgeCloud := false
	for _, raw := range args {
		name, val, hasVal := strings.Cut(strings.TrimSpace(raw), "=")
		if name != "--use_edge_cloud" && name != "-use_edge_cloud" {
			continue
		}
		if !hasVal {
			edgeCloud = true
		} else if enabled, err := strconv.ParseBool(strings.TrimSpace(val)); err == nil {
			edgeCloud = enabled
		}
	}
	return edgeCloud
}

func isNonStandardGKEExec(auth *clientcmdapi.AuthInfo) bool {
	if auth == nil || auth.Exec == nil {
		return false
	}
	if hasNonGCloudExecPlugin(auth) || hasEdgeCloudFlag(auth.Exec.Args) {
		return true
	}
	if slices.Contains(auth.Exec.Args, "print-access-token") {
		for _, tok := range []string{"aws", "azure", "attached", "edge-cloud"} {
			if slices.Contains(auth.Exec.Args, tok) {
				return true
			}
		}
	}
	return false
}

func isNonStandardGKEContext(ctxName string, kubeCtx *clientcmdapi.Context, auth *clientcmdapi.AuthInfo, cluster *clientcmdapi.Cluster) bool {
	names := []string{ctxName}
	if kubeCtx != nil {
		names = append(names, kubeCtx.Cluster)
	}
	for _, n := range names {
		trimmed := strings.TrimSpace(n)
		if isMultiCloudOrAttachedGKEName(trimmed) ||
			strings.HasPrefix(trimmed, "connectgateway_") ||
			strings.HasPrefix(trimmed, "edgecontainer_") {
			return true
		}
	}
	return isConnectGatewayHost(k8s.ClusterServerHostname(cluster)) || isNonStandardGKEExec(auth)
}

func isIncompleteKubeContext(kubeCtx *clientcmdapi.Context, auth *clientcmdapi.AuthInfo, cluster *clientcmdapi.Cluster) bool {
	if kubeCtx == nil || cluster == nil || strings.TrimSpace(cluster.Server) == "" {
		return true
	}
	return strings.TrimSpace(kubeCtx.AuthInfo) != "" && auth == nil
}

func hasLegacyGCPAuthProvider(auth *clientcmdapi.AuthInfo) bool {
	return auth != nil && auth.AuthProvider != nil && strings.EqualFold(strings.TrimSpace(auth.AuthProvider.Name), "gcp")
}

func hasNonGCloudExecPlugin(auth *clientcmdapi.AuthInfo) bool {
	switch k8s.ExecPluginBaseName(auth) {
	case "", gkeAuthPluginBinary, "gcloud", "gcloud.cmd", "gcloud.bat", "gcloud.ps1":
		return false
	default:
		return true
	}
}

// detectGKE evaluates GKE kubeconfig credentials, *.gke.goog endpoints, and the bounded 2s serverVersion tie-breaker.
func detectGKE(in orchestrator.ProbeInput) (orchestrator.Detection, bool) {
	gkeFlags := hasGKEFlags(in.ProjectID, in.Location)
	ctxName, kubeCtx, resolvedCluster, ok := resolveGKEContext(in.Config, in.ClusterName, in.Location, in.ProjectID)
	var auth *clientcmdapi.AuthInfo
	var cluster *clientcmdapi.Cluster
	if ok {
		if strings.TrimSpace(kubeCtx.AuthInfo) != "" {
			auth = in.Config.AuthInfos[kubeCtx.AuthInfo]
		}
		cluster = in.Config.Clusters[kubeCtx.Cluster]
		if isNonStandardGKEContext(ctxName, kubeCtx, auth, cluster) {
			return orchestrator.Detection{}, false
		}
	}
	if forceCredentialRefreshEnv(in.Getenv) && gkeFlags {
		if !ok {
			resolvedCluster = in.ClusterName
		}
		return orchestrator.Detection{
			Type:        orchestrator.TypeGKE,
			KubeContext: ctxName,
			ClusterName: resolvedCluster,
			Reason:      fmt.Sprintf("credential refresh requested via %s", refreshCredentialsEnv),
		}, true
	}
	if !ok {
		// Bootstrap TypeGKE from project/location when no kubeconfig context matched; when a context does match,
		// detectGKE classifies that context and CLI callers decide how GKE flags interact with non-GKE contexts.
		if gkeFlags {
			return orchestrator.Detection{
				Type:        orchestrator.TypeGKE,
				KubeContext: "",
				ClusterName: in.ClusterName,
				Reason:      "GKE project/location configured with no existing kubeconfig context",
			}, true
		}
		return orchestrator.Detection{}, false
	}
	if reason := detectGKEOfflineReason(ctxName, kubeCtx, auth, cluster, gkeFlags); reason != "" {
		return orchestrator.Detection{
			Type:        orchestrator.TypeGKE,
			KubeContext: ctxName,
			ClusterName: resolvedCluster,
			Reason:      reason,
		}, true
	}
	hasServer := cluster != nil && strings.TrimSpace(cluster.Server) != ""
	if gitVersion, isGKEServer := probeGKEServerVersion(in.Runner, in.AirGapped, hasServer, ctxName, auth); isGKEServer {
		return orchestrator.Detection{
			Type:        orchestrator.TypeGKE,
			KubeContext: ctxName,
			ClusterName: resolvedCluster,
			Reason:      fmt.Sprintf("API server for context %q reported GKE version %s", ctxName, gitVersion),
		}, true
	}
	return orchestrator.Detection{}, false
}

func detectGKEOfflineReason(ctxName string, kubeCtx *clientcmdapi.Context, auth *clientcmdapi.AuthInfo, cluster *clientcmdapi.Cluster, gkeFlags bool) string {
	host := k8s.ClusterServerHostname(cluster)
	switch {
	case k8s.ExecPluginBaseName(auth) == gkeAuthPluginBinary:
		return fmt.Sprintf("kubeconfig context %q authenticates via %s", ctxName, gkeAuthPluginBinary)
	case strings.HasSuffix(host, ".gke.goog"):
		return fmt.Sprintf("kubeconfig context %q targets GKE endpoint %s", ctxName, host)
	case hasLegacyGCPAuthProvider(auth):
		return fmt.Sprintf("kubeconfig context %q uses legacy gcp auth-provider", ctxName)
	case isCanonicalGKEName(ctxName) || isCanonicalGKEName(kubeCtx.Cluster):
		return fmt.Sprintf("kubeconfig context %q matches gke_* naming", ctxName)
	case isIncompleteKubeContext(kubeCtx, auth, cluster) && gkeFlags:
		return fmt.Sprintf("project/location set for incomplete kubeconfig context %q", ctxName)
	default:
		return ""
	}
}

func canProbeGKEServerVersion(runner orchestrator.CommandRunner, airGapped, hasServer bool, ctxName string, auth *clientcmdapi.AuthInfo) bool {
	if airGapped || !hasServer || strings.TrimSpace(ctxName) == "" || runner == nil || auth == nil {
		return false
	}
	return auth.AuthProvider == nil && !k8s.HasClientCertificateAuth(auth) && !hasNonGCloudExecPlugin(auth)
}

func probeGKEServerVersion(runner orchestrator.CommandRunner, airGapped, hasServer bool, ctxName string, auth *clientcmdapi.AuthInfo) (string, bool) {
	if !canProbeGKEServerVersion(runner, airGapped, hasServer, ctxName, auth) {
		return "", false
	}
	res := runner(
		gkeVersionProbeProcessTimeout,
		"kubectl",
		k8s.WithContext(ctxName, []string{"version", "-o", "json", "--request-timeout=" + gkeVersionProbeTimeout})...,
	)
	if strings.TrimSpace(res.Stdout) == "" {
		return "", false
	}
	var payload kubectlVersionOutput
	if err := json.Unmarshal([]byte(res.Stdout), &payload); err != nil || payload.ServerVersion == nil {
		return "", false
	}
	ver := strings.TrimSpace(payload.ServerVersion.GitVersion)
	return ver, strings.Contains(strings.ToLower(ver), "-gke.")
}

func (k kubeContextCheck) reusable() bool {
	return k.reason == "" && strings.TrimSpace(k.contextName) != ""
}

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
	server := strings.ToLower(strings.TrimRight(cluster.Server, "/"))
	server = strings.TrimSuffix(server, ":443")
	mode, known := desc.knownEndpoints()[server]
	if !known {
		check.reason = fmt.Sprintf("server %q does not match any endpoint of cluster %q", cluster.Server, clusterName)
		return check
	}
	check.mode = mode

	if reason := validateClusterTLS(cluster, mode, desc); reason != "" {
		check.reason = reason
		return check
	}
	if reason := validateGKEExecAuth(cfg.AuthInfos[kubeCtx.AuthInfo], kubeCtx.AuthInfo); reason != "" {
		check.reason = reason
		return check
	}
	// Namespace is a targeting concern (validateTargetNamespaceExists reports a bad one) and kubectl
	// does not enforce DNS-1123 on it, so keep it out of the reuse decision.
	usable, ctxCopy := *cfg, *kubeCtx
	ctxCopy.Namespace = ""
	usable.Contexts = map[string]*clientcmdapi.Context{name: &ctxCopy}
	if err := clientcmd.ConfirmUsable(usable, name); err != nil {
		check.reason = fmt.Sprintf("invalid kubeconfig context: %v", err)
		return check
	}
	return check
}

func validateClusterTLS(cluster *clientcmdapi.Cluster, mode endpointMode, desc gkeCluster) string {
	if cluster.InsecureSkipTLSVerify {
		return "cluster has insecure-skip-tls-verify enabled"
	}
	if cluster.CertificateAuthority != "" {
		return "certificate authority data is file-based in kubeconfig"
	}
	if mode == endpointModeDNS {
		if len(cluster.CertificateAuthorityData) > 0 && !x509.NewCertPool().AppendCertsFromPEM(cluster.CertificateAuthorityData) {
			return "certificate authority data is not valid PEM"
		}
		return ""
	}
	if desc.MasterAuth == nil || strings.TrimSpace(desc.MasterAuth.ClusterCaCertificate) == "" {
		return "cannot verify certificate authority: cluster description carries no CA"
	}
	if len(cluster.CertificateAuthorityData) == 0 {
		return "certificate authority data is missing or file-based in kubeconfig"
	}
	if base64.StdEncoding.EncodeToString(cluster.CertificateAuthorityData) != strings.TrimSpace(desc.MasterAuth.ClusterCaCertificate) {
		return "certificate authority mismatch (cluster may have been recreated)"
	}
	return ""
}

func validateGKEExecAuth(auth *clientcmdapi.AuthInfo, userName string) string {
	if k8s.ExecPluginBaseName(auth) != gkeAuthPluginBinary {
		return fmt.Sprintf("user %q does not authenticate via %s", userName, gkeAuthPluginBinary)
	}
	if auth.Exec.Command != strings.TrimSpace(auth.Exec.Command) {
		return fmt.Sprintf("user %q exec command %q contains leading or trailing whitespace", userName, auth.Exec.Command)
	}
	if auth.AuthProvider != nil {
		return fmt.Sprintf("user %q specifies both auth-provider and exec authentication", userName)
	}
	if hasStaticCredentials(auth) {
		return fmt.Sprintf("user %q combines static credentials with exec authentication", userName)
	}
	if !isSupportedExecAPIVersion(auth.Exec.APIVersion) {
		return fmt.Sprintf("user %q uses unsupported exec apiVersion %q (expected v1beta1 or v1)", userName, auth.Exec.APIVersion)
	}
	switch auth.Exec.InteractiveMode {
	case clientcmdapi.NeverExecInteractiveMode, clientcmdapi.IfAvailableExecInteractiveMode:
		return ""
	default:
		return fmt.Sprintf("user %q uses missing or unsupported exec interactiveMode %q", userName, auth.Exec.InteractiveMode)
	}
}

func hasStaticCredentials(auth *clientcmdapi.AuthInfo) bool {
	return auth.Token != "" || auth.TokenFile != "" ||
		auth.Username != "" || auth.Password != "" ||
		auth.ClientCertificate != "" || len(auth.ClientCertificateData) > 0 ||
		auth.ClientKey != "" || len(auth.ClientKeyData) > 0
}

func isSupportedExecAPIVersion(apiVersion string) bool {
	return apiVersion == "client.authentication.k8s.io/v1beta1" || apiVersion == "client.authentication.k8s.io/v1"
}

var (
	probeAuthMarkers = []string{
		"unauthorized",
		"you must be logged in to the server",
		"asked for the client to provide credentials",
		"getting credentials",
		gkeAuthPluginBinary,
		"fetch token",
		"oauth2:",
		"no valid id-token",
		"token response did not contain an id_token",
		"could not persist new tokens",
		"auth plugin has been removed",
		"no auth provider found",
		"exec plugin: invalid apiversion",
		"could not get token",
		"application default credentials",
		"reauthentication",
	}
	probeInvalidMarkers = []string{
		"unable to read certificate-authority",
		"unable to read client-",
		"unable to read token file",
		"error loading config file",
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
		"was refused - did you specify",
		"connection reset",
		"no route to host",
		"network is unreachable",
		"no such host",
		"tls:",
		"tls handshake",
		"x509:",
		"status code 431",
		"request header fields too large",
		// Forward proxies and gateways report unreachable upstreams as HTTP 502/503/504.
		"bad gateway",
		"error from server (serviceunavailable):",
		"service unavailable",
		"error from server (timeout):",
		"gateway timeout",
		"dial tcp",
		"unexpected eof",
		"unable to connect to the server",
	}
)

// classifyProbeFailure maps kubectl stderr to a probeOutcome. Auth and invalid-config
// markers win because kubectl prefixes plugin failures with "Unable to connect to the server:".
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
	case contains(probeInvalidMarkers):
		return probeInvalid
	case contains(probeAuthMarkers):
		return probeAuth
	case k8s.IsDNSGatewayRejectedText(s):
		return probeConnectivity
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
	if errors.Is(res.Err, context.DeadlineExceeded) {
		timeoutPrefix := fmt.Sprintf("timed out after %v", shell.DefaultLocalCommandTimeout)
		if stderr == "" {
			stderr = timeoutPrefix
		} else {
			stderr = fmt.Sprintf("%s: %s", timeoutPrefix, stderr)
		}
	} else if stderr == "" && res.Err != nil {
		stderr = strings.TrimSpace(res.Err.Error())
	}
	if strings.TrimSpace(contextName) == "" {
		return probeInvalid, stderr
	}
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

func forceCredentialRefreshEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	v := strings.ToLower(strings.TrimSpace(getenv(refreshCredentialsEnv)))
	return v != "" && v != "0" && v != "false" && v != "no" && v != "off"
}

// forceCredentialRefresh reports whether the user asked to bypass kubeconfig reuse.
func forceCredentialRefresh() bool {
	return forceCredentialRefreshEnv(os.Getenv)
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

// kubectlIn runs kubectl with DefaultLocalCommandTimeout against an explicit non-blank kubeContext,
// refusing a blank context so probes never silently fall back to current-context.
func (g *GKEOrchestrator) kubectlIn(kubeContext string, args ...string) shell.CommandResult {
	if strings.TrimSpace(kubeContext) == "" {
		err := fmt.Errorf("internal error: refusing to run kubectl without an explicit --context")
		return shell.CommandResult{ExitCode: 1, Stderr: err.Error(), Err: err}
	}
	return g.executor.ExecuteCommandWithTimeout(shell.DefaultLocalCommandTimeout, "kubectl", k8s.WithContext(kubeContext, args)...)
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

const apiGroupProbeTimeout = 15 * time.Second

// HasAPIGroupVersion checks whether the Kubernetes apiserver serves /apis/<groupVersion>
// via the non-privileged API discovery endpoint (implements k8s.APIGroupChecker).
func (g *GKEOrchestrator) HasAPIGroupVersion(groupVersion string) (bool, error) {
	gv := strings.TrimFunc(groupVersion, func(r rune) bool {
		return r == '/' || unicode.IsSpace(r)
	})
	if gv == "" {
		return false, nil
	}
	if strings.TrimSpace(g.kubeContext) == "" {
		return false, fmt.Errorf("internal error: refusing to query Kubernetes API group %q without an explicit --context", gv)
	}
	res := g.kubectlWithTimeout(apiGroupProbeTimeout, "get", "--raw", "/apis/"+gv, kubectlReadTimeout)
	if errors.Is(res.Err, context.DeadlineExceeded) {
		return false, fmt.Errorf("failed to query Kubernetes API group %q: timed out after %v: %w", gv, apiGroupProbeTimeout, res.Err)
	}
	if res.ExitCode == 0 && res.Err == nil {
		return true, nil
	}
	combined := res.Stderr + "\n" + res.Stdout
	if isAPIGroupNotFoundText(combined) {
		return false, nil
	}
	if errLine := firstNonWarningLine(res.Stderr, res.Stdout); errLine != "" {
		return false, fmt.Errorf("failed to query Kubernetes API group %q: %s", gv, errLine)
	}
	if res.Err != nil {
		return false, fmt.Errorf("failed to query Kubernetes API group %q: %w", gv, res.Err)
	}
	if fallback := firstNonWarningFallback(res.Stderr, res.Stdout); fallback != "" {
		return false, fmt.Errorf("failed to query Kubernetes API group %q: %s", gv, fallback)
	}
	return false, fmt.Errorf("failed to query Kubernetes API group %q: exit status %d", gv, res.ExitCode)
}

func firstNonWarningLine(stderr, stdout string) string {
	for _, block := range []string{stderr, stdout} {
		if line := scanNonWarningLine(block); line != "" {
			return line
		}
	}
	return ""
}

func firstNonWarningFallback(stderr, stdout string) string {
	if line := firstLine(strings.TrimSpace(stderr)); line != "" {
		return line
	}
	return firstLine(strings.TrimSpace(stdout))
}

func scanNonWarningLine(s string) string {
	for _, raw := range strings.Split(s, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || isKubectlWarningLine(line) {
			continue
		}
		return line
	}
	return ""
}

func isKubectlWarningLine(line string) bool {
	if strings.HasPrefix(strings.ToUpper(line), "WARNING:") {
		return true
	}
	return len(line) >= 6 && line[0] == 'W' && line[5] == ' ' && isAllDigits(line[1:5])
}

func isAPIGroupNotFoundText(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "error from server (notfound)") ||
		strings.Contains(lower, "the server could not find the requested resource") ||
		strings.Contains(lower, "404 page not found")
}

// inspectExistingContext loads the kubeconfig and statically validates the cluster's entry.
func (g *GKEOrchestrator) inspectExistingContext(clusterName, location, projectID string) kubeContextCheck {
	cfg, err := g.loadKubeconfig()
	if err != nil {
		return kubeContextCheck{reason: fmt.Sprintf("failed to load kubeconfig: %v", err)}
	}
	return evaluateExistingContext(cfg, clusterName, location, projectID, g.clusterDesc)
}

func (c gkeCluster) endpointModeAvailable(mode endpointMode) bool {
	switch mode {
	case endpointModeIP:
		return c.ipEndpointAvailable()
	case endpointModeDNS:
		return c.dnsEndpointAvailable()
	default:
		return false
	}
}

// endpointModeOrder lists the available modes to try, preferred first. A mode already
// seen to be unreachable is demoted rather than dropped.
func (g *GKEOrchestrator) endpointModeOrder(avoid endpointMode) []endpointMode {
	preferred := endpointModeIP
	if g.clusterDesc.shouldUseDNSEndpoint() {
		preferred = endpointModeDNS
	}
	var order []endpointMode
	for _, mode := range []endpointMode{preferred, preferred.other()} {
		if g.clusterDesc.endpointModeAvailable(mode) {
			order = append(order, mode)
		}
	}
	if len(order) > 1 && order[0] == avoid {
		order[0], order[1] = order[1], order[0]
	}
	return order
}

// refreshGKEAuth runs `gcloud container clusters get-credentials`.
func (g *GKEOrchestrator) refreshGKEAuth(clusterName, clusterLocation, projectID string, useDNSEndpoint bool) error {
	clusterName = strings.TrimSpace(clusterName)
	clusterLocation = strings.TrimSpace(clusterLocation)
	projectID = strings.TrimSpace(projectID)
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
	namespace = strings.TrimSpace(namespace)
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
		if _, kubeCtx, ok := gkeNamedContext(cfg, clusterName, clusterLocation, projectID); ok {
			originalNamespace = k8s.ContextNamespace(kubeCtx)
		}
	}
	if err := g.refreshGKEAuth(clusterName, clusterLocation, projectID, mode == endpointModeDNS); err != nil {
		return err
	}
	return g.restoreNamespaceContext(gkeContextName(projectID, clusterLocation, clusterName), originalNamespace)
}

func isMissingExecBinaryStderr(stderr string) bool {
	lower := strings.ToLower(stderr)
	if strings.Contains(lower, "failed with exit code") || !strings.Contains(lower, "getting credentials") {
		return false
	}
	for _, marker := range []string{
		"no such file or directory",
		"not found",
		"cannot find the ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func (g *GKEOrchestrator) hasMissingPathExecBinary(contextName, stderr string) bool {
	if !isMissingExecBinaryStderr(stderr) {
		return false
	}
	cfg, err := g.loadKubeconfig()
	if err != nil || cfg == nil {
		return false
	}
	kubeCtx := cfg.Contexts[contextName]
	if kubeCtx == nil {
		return false
	}
	auth := cfg.AuthInfos[kubeCtx.AuthInfo]
	return auth != nil && auth.Exec != nil && strings.ContainsAny(auth.Exec.Command, `/\`)
}

// regenerateKubeContext fetches credentials per endpoint mode in order, moving
// to the next mode on a connectivity failure.
func (g *GKEOrchestrator) regenerateKubeContext(clusterName, clusterLocation, projectID string, avoid endpointMode) error {
	g.resetManagedDynamicClient()
	contextName := gkeContextName(projectID, clusterLocation, clusterName)
	modes := g.endpointModeOrder(avoid)
	if len(modes) == 0 {
		return fmt.Errorf("GKE cluster %q has no usable control-plane endpoint enabled (both IP endpoints and external DNS access are disabled)", clusterName)
	}
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
				logging.Info("Cluster connectivity verified via %s endpoint (kubeconfig context '%s').", mode, contextName)
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
		case probeUnknown:
			logging.Warn("Could not verify connectivity to cluster '%s' via its %s endpoint (%s). Proceeding; subsequent kubectl calls will report the underlying error.", clusterName, mode, firstLine(stderr))
			g.kubeContext = contextName
			return nil
		default:
			return fmt.Errorf("failed to verify kubeconfig context %q for cluster %q: %s", contextName, clusterName, firstLine(stderr))
		}
	}

	return g.endpointConnectivityError(clusterName, attempts)
}

func (g *GKEOrchestrator) endpointConnectivityError(clusterName string, attempts []string) error {
	if !g.clusterDesc.ipEndpointAvailable() {
		return fmt.Errorf("failed to connect to GKE cluster %q via its DNS control-plane endpoint (%s). "+
			"Please verify network/proxy connectivity to *.gke.goog, that your account has the container.clusters.connect IAM permission "+
			"and is not blocked by VPC Service Controls, and that dnsEndpointConfig.allowExternalTraffic is enabled",
			clusterName, strings.Join(attempts, "; "))
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
	g.resetStorageCaches()
	g.inventory = nil
	if g.mode == orchestrator.TypeK8s {
		return g.configureK8sKubectl(clusterName)
	}
	g.kubeContext = ""
	g.resetManagedDynamicClient()
	defer func() {
		if g.kubeContext == "" {
			g.resetManagedDynamicClient()
		}
	}()
	avoid := endpointModeUnknown
	clusterName = strings.TrimSpace(clusterName)
	clusterLocation = strings.TrimSpace(clusterLocation)
	projectID = strings.TrimSpace(projectID)

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
		if g.hasMissingPathExecBinary(check.contextName, stderr) {
			logging.Warn("Existing kubeconfig context '%s' references a missing %s path (%s). Refreshing credentials...", check.contextName, gkeAuthPluginBinary, firstLine(stderr))
			break
		}
		return authFailureError(clusterName, stderr)
	case probeConnectivity:
		logging.Warn("Existing kubeconfig context '%s' (%s endpoint) is unreachable from here (%s). Refreshing credentials...", check.contextName, check.mode, firstLine(stderr))
		avoid = check.mode
	case probeUnknown:
		g.kubeContext = check.contextName
		logging.Warn("Could not verify existing kubeconfig context '%s' (%s). Reusing it; use 'kubectl --context %s' for manual commands, or set %s=1 to force a credential refresh.", check.contextName, firstLine(stderr), check.contextName, refreshCredentialsEnv)
		return nil
	default:
		return fmt.Errorf("failed to verify kubeconfig context %q for cluster %q: %s", check.contextName, clusterName, firstLine(stderr))
	}
	return g.regenerateKubeContext(clusterName, clusterLocation, projectID, avoid)
}

// usableKubeClient returns the Kubernetes API client for best-effort discovery, or nil when none can be
// built yet (no pinned context, or an unreadable kubeconfig).
func (g *GKEOrchestrator) usableKubeClient() KubeClient {
	kc, err := g.requireKubeClientForDiscovery()
	if err != nil {
		if !errors.Is(err, errKubeClientUninitialized) {
			logging.Warn("Kubernetes API client unavailable for cluster discovery (context %q): %v", g.kubeContext, err)
		}
		return nil
	}
	return kc
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
