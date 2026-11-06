// Copyright 2026 Google LLC
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

package k8s

import (
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path"
	"slices"
	"strings"

	"hpc-toolkit/pkg/logging"

	"golang.org/x/mod/semver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	// DefaultTelemetryCollectorPort is the default container port exposed by ai-telemetry-collector.
	DefaultTelemetryCollectorPort = 8889
	// defaultTPURuntimeMetricsPorts lists the libtpu runtime metric ports exposed on TPU hosts.
	defaultTPURuntimeMetricsPorts = "8431,8432,8433,8434,8435,8436,8437,8438"
	// TelemetryCollectorContainerName is the container name of the native ML Diagnostics collector sidecar.
	TelemetryCollectorContainerName = "ai-telemetry-collector"
	// TPUWorkloadNameLabel is the pod label used by cluster PodMonitors to group TPU metrics per workload.
	TPUWorkloadNameLabel = "tpu-workload-name"
	// ManagedMLDiagnosticsLabel asks GKE's managed webhook to inject its own collector; it must not be set
	// on a workload that already carries the native sidecar, or two collectors scrape the same chips.
	ManagedMLDiagnosticsLabel = "managed-mldiagnostics-gke"
	// telemetryCollectorHealthPort is the collector's own health-check listener.
	telemetryCollectorHealthPort = 13133
	// tpuSliceBuilderPort is libtpu's default SliceBuilder/ICI worker gRPC port on hostNetwork.
	tpuSliceBuilderPort = 8471
	// TelemetryImageEnvVar supplies the collector image when --ml-diagnostics-image is omitted.
	TelemetryImageEnvVar = "GCLUSTER_TELEMETRY_IMAGE"

	telemetryImageHint = "specify --ml-diagnostics-image or set " + TelemetryImageEnvVar
)

// ValidateTelemetryPort checks that port is a usable TCP port for the collector's Prometheus endpoint and does
// not collide with listeners the collector itself depends on (libtpu runtime metric ports and its own health
// check) or with libtpu's SliceBuilder worker port (which standard TPU manifests do not declare in ports:).
// Ports declared by other containers in the pod are checked against the rendered manifest by ValidatePodTelemetry.
func ValidateTelemetryPort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid --ml-diagnostics-telemetry-port %d: must be between 1 and 65535", port)
	}
	if strings.Contains(","+defaultTPURuntimeMetricsPorts+",", fmt.Sprintf(",%d,", port)) {
		return fmt.Errorf("invalid --ml-diagnostics-telemetry-port %d: ports %s are the libtpu runtime metrics endpoints the collector scrapes", port, defaultTPURuntimeMetricsPorts)
	}
	if port == tpuSliceBuilderPort {
		return fmt.Errorf("invalid --ml-diagnostics-telemetry-port %d: it is the libtpu SliceBuilder worker port", port)
	}
	if port == telemetryCollectorHealthPort {
		return fmt.Errorf("invalid --ml-diagnostics-telemetry-port %d: it is the collector's own health-check port", port)
	}
	return nil
}

// TelemetryImageFromEnv returns the collector image configured through GCLUSTER_TELEMETRY_IMAGE, or "".
func TelemetryImageFromEnv() string {
	return strings.TrimSpace(os.Getenv(TelemetryImageEnvVar))
}

// DiscoverTelemetryImage returns the newest ai-telemetry-collector (or workload-observability-agent) image
// cached on the cluster's TPU nodes, preferring nodes whose instance type matches machineType.
func DiscoverTelemetryImage(lister ResourceLister, machineType string) (string, error) {
	if lister == nil {
		return "", fmt.Errorf("unable to auto-discover the ML Diagnostics collector image without an active Kubernetes client; %s", telemetryImageHint)
	}
	nodes, err := lister.ListResources(NodeGVR, "", "", "")
	if err != nil {
		if IsForbiddenError(err) {
			return "", fmt.Errorf("lacking permission to list cluster nodes to auto-discover the ML Diagnostics collector image; %s", telemetryImageHint)
		}
		return "", fmt.Errorf("failed to inspect TPU node images for the ML Diagnostics collector (%v); %s", err, telemetryImageHint)
	}
	discovered, tpuNodes := newestCollectorImage(nodes, machineType)
	switch {
	case discovered != "":
		logging.Info("Auto-discovered ML Diagnostics collector image from TPU node cache: %s", discovered)
		return discovered, nil
	case tpuNodes == 0:
		return "", fmt.Errorf("no running TPU nodes found in cluster to auto-discover the ML Diagnostics collector image (node pool may be scaled to zero); %s", telemetryImageHint)
	default:
		return "", fmt.Errorf("no cached ai-telemetry-collector or workload-observability-agent image found across %d TPU node(s); %s", tpuNodes, telemetryImageHint)
	}
}

func newestCollectorImage(nodes []unstructured.Unstructured, machineType string) (string, int) {
	var preferred, all [4][]string
	tpuNodes := 0
	targetMT := strings.TrimSpace(machineType)
	for i := range nodes {
		nodeBuckets, isTPUNode := collectTPUNodeCollectorImages(nodes[i])
		if !isTPUNode {
			continue
		}
		tpuNodes++
		appendBuckets(&all, nodeBuckets)
		if targetMT != "" && strings.EqualFold(nodes[i].GetLabels()["node.kubernetes.io/instance-type"], targetMT) {
			appendBuckets(&preferred, nodeBuckets)
		}
	}
	if picked := selectNewestFromBuckets(preferred); picked != "" {
		return picked, tpuNodes
	}
	return selectNewestFromBuckets(all), tpuNodes
}

func appendBuckets(dst *[4][]string, src [4][]string) {
	for b := range dst {
		dst[b] = append(dst[b], src[b]...)
	}
}

func selectNewestFromBuckets(buckets [4][]string) string {
	for b := range buckets {
		if len(buckets[b]) > 0 {
			return slices.MaxFunc(buckets[b], compareImageRefs)
		}
	}
	return ""
}

// compareImageRefs orders image references by the semantic version of their tag (v1.10.0 > v1.9.0);
// semver-tagged references rank above untagged or non-semver ones, which fall back to a lexical order.
func compareImageRefs(a, b string) int {
	va, vb := semverTag(a), semverTag(b)
	switch {
	case va != "" && vb != "":
		if c := semver.Compare(va, vb); c != 0 {
			return c
		}
	case va != "":
		return 1
	case vb != "":
		return -1
	}
	return strings.Compare(a, b)
}

func semverTag(ref string) string {
	ref, _, _ = strings.Cut(ref, "@")
	_, tag, ok := strings.Cut(ref[strings.LastIndex(ref, "/")+1:], ":")
	if !ok || tag == "" {
		return ""
	}
	if !strings.HasPrefix(tag, "v") {
		if !strings.Contains(tag, ".") {
			return ""
		}
		tag = "v" + tag
	}
	if !semver.IsValid(tag) {
		return ""
	}
	return tag
}

func collectTPUNodeCollectorImages(node unstructured.Unstructured) ([4][]string, bool) {
	var buckets [4][]string
	if !nodeHasTPUAllocatable(node) {
		return buckets, false
	}
	images, _, _ := unstructured.NestedSlice(node.Object, "status", "images")
	for _, item := range images {
		imgMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		names, _, _ := unstructured.NestedSlice(imgMap, "names")
		for _, rawName := range names {
			if ref, ok := rawName.(string); ok {
				if idx := classifyCollectorImageBucket(ref); idx >= 0 {
					buckets[idx] = append(buckets[idx], ref)
				}
			}
		}
	}
	return buckets, true
}

func nodeHasTPUAllocatable(node unstructured.Unstructured) bool {
	rawTPU, found, err := unstructured.NestedFieldNoCopy(node.Object, "status", "allocatable", "google.com/tpu")
	if err != nil || !found || rawTPU == nil {
		return false
	}
	var tpuStr string
	switch v := rawTPU.(type) {
	case string:
		tpuStr = strings.TrimSpace(v)
	case int64, int, float64, json.Number:
		tpuStr = fmt.Sprint(v)
	default:
		return false
	}
	qty, err := resource.ParseQuantity(tpuStr)
	return err == nil && !qty.IsZero()
}

func classifyCollectorImageBucket(ref string) int {
	_, repo, ok := splitImageRef(ref)
	if !ok {
		return -1
	}
	isDigest := strings.Contains(ref, "@")
	switch path.Base(repo) {
	case "ai-telemetry-collector":
		if !isDigest {
			return 0
		}
		return 1
	case "workload-observability-agent":
		if !isDigest {
			return 2
		}
		return 3
	default:
		return -1
	}
}

// EffectiveTelemetryPort returns DefaultTelemetryCollectorPort (8889) when the telemetry sidecar is injected and port is unset.
func EffectiveTelemetryPort(collectorImage string, port int) int {
	if collectorImage != "" && port == 0 {
		return DefaultTelemetryCollectorPort
	}
	return port
}

// WithTelemetryEnv sets default TPU_RUNTIME_METRICS_PORTS unless already provided in userEnv.
func WithTelemetryEnv(userEnv map[string]string) map[string]string {
	merged := make(map[string]string, len(userEnv)+1)
	merged["TPU_RUNTIME_METRICS_PORTS"] = defaultTPURuntimeMetricsPorts
	maps.Copy(merged, userEnv)
	return merged
}

// HasContainerNamed reports whether containers includes a container with the given name.
func HasContainerNamed(containers []corev1.Container, name string) bool {
	return slices.ContainsFunc(containers, func(c corev1.Container) bool { return c.Name == name })
}

// isNativeSidecar reports whether c is a Kubernetes 1.29+ native sidecar: an init container with
// restartPolicy: Always. Without that policy the collector would run as a blocking init container and the
// pod would never leave Init.
func isNativeSidecar(c corev1.Container) bool {
	return c.RestartPolicy != nil && *c.RestartPolicy == corev1.ContainerRestartPolicyAlways
}

// ValidatePodTelemetry verifies that a rendered pod template runs the collector as a native sidecar, carries
// the tpu-workload-name label, and that no other container in the pod declares the collector's metrics port
// (TPU pods use the host network, and the sidecar starts before the workload, so a clash breaks the job).
func ValidatePodTelemetry(contextPrefix, hint, workloadName string, pod corev1.PodSpec, labels map[string]string) error {
	idx := slices.IndexFunc(pod.InitContainers, func(c corev1.Container) bool { return c.Name == TelemetryCollectorContainerName })
	if idx < 0 {
		return fmt.Errorf("%s is missing native sidecar %q in initContainers. %s", contextPrefix, TelemetryCollectorContainerName, hint)
	}
	if !isNativeSidecar(pod.InitContainers[idx]) {
		return fmt.Errorf("%s runs %q as a plain init container; it must set restartPolicy: Always to run as a native sidecar. %s", contextPrefix, TelemetryCollectorContainerName, hint)
	}
	if workloadName != "" && labels[TPUWorkloadNameLabel] != workloadName {
		return fmt.Errorf("%s is missing pod label %s=%q. %s", contextPrefix, TPUWorkloadNameLabel, workloadName, hint)
	}
	for _, p := range pod.InitContainers[idx].Ports {
		if other := containerListeningOn(pod, p.ContainerPort, idx); other != "" {
			return fmt.Errorf("%s: --ml-diagnostics-telemetry-port %d is already declared by container %q, and TPU pods share the host network; choose another port. %s", contextPrefix, p.ContainerPort, other, hint)
		}
	}
	return nil
}

// containerListeningOn returns the name of the first container in pod, other than initContainers[skipInit],
// that declares port, or "".
func containerListeningOn(pod corev1.PodSpec, port int32, skipInit int) string {
	for i, c := range pod.InitContainers {
		if i != skipInit && slices.ContainsFunc(c.Ports, func(cp corev1.ContainerPort) bool { return cp.ContainerPort == port }) {
			return c.Name
		}
	}
	for _, c := range pod.Containers {
		if slices.ContainsFunc(c.Ports, func(cp corev1.ContainerPort) bool { return cp.ContainerPort == port }) {
			return c.Name
		}
	}
	return ""
}

// ValidateNoManagedWebhookLabel rejects the GKE managed-webhook label on a workload object when the native
// sidecar is injected, so a stale custom workload_labels block cannot produce two collectors.
func ValidateNoManagedWebhookLabel(contextPrefix, hint string, objectLabels map[string]string) error {
	if objectLabels[ManagedMLDiagnosticsLabel] == "true" {
		return fmt.Errorf("%s sets label %s=\"true\" while the native %q sidecar is injected; remove the label (GKE's managed webhook would add a second collector). %s", contextPrefix, ManagedMLDiagnosticsLabel, TelemetryCollectorContainerName, hint)
	}
	return nil
}
