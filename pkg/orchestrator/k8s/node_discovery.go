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
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"

	"hpc-toolkit/pkg/logging"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// Label keys recognized across TPU slice, GKE, and standard Kubernetes clusters.
const (
	TPUSliceLabelPrefix          = "tpu.google.com/"
	TPUSliceNameLabel            = "tpu.google.com/slice-name"
	TPUSliceTypeLabel            = "tpu.google.com/type"
	TPUSliceTopologyLabel        = "tpu.google.com/topology"
	TPUSlicePartitionLabelPrefix = "tpu.google.com/partition-id-"

	GKETPUAcceleratorLabel = "cloud.google.com/gke-tpu-accelerator"
	GKETPUTopologyLabel    = "cloud.google.com/gke-tpu-topology"
	GKENodePoolLabel       = "cloud.google.com/gke-nodepool"
	GKEAcceleratorLabel    = "cloud.google.com/gke-accelerator"
	NvidiaGPUProductLabel  = "nvidia.com/gpu.product"

	InstanceTypeLabel     = "node.kubernetes.io/instance-type"
	BetaInstanceTypeLabel = "beta.kubernetes.io/instance-type"
	ControlPlaneRoleLabel = "node-role.kubernetes.io/control-plane"
	MasterRoleLabel       = "node-role.kubernetes.io/master"

	GKESystemTaintKey = "components.gke.io/gke-managed-components"
	gkeDefaultPool    = "default-pool"
)

var errListerRequired = errors.New("Kubernetes resource lister is required for cluster inventory discovery")

// ParseLabelScheme parses a user-supplied label scheme ("tpu-slice", "gke", "generic", or "" for auto).
func ParseLabelScheme(raw string) (LabelScheme, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return SchemeAuto, nil
	case string(SchemeTPUSlice):
		return SchemeTPUSlice, nil
	case string(SchemeGKE):
		return SchemeGKE, nil
	case string(SchemeGeneric):
		return SchemeGeneric, nil
	default:
		return "", fmt.Errorf("invalid label scheme %q: allowed values are tpu-slice, gke, generic", raw)
	}
}

// ResolveLabelSchemeOverride resolves an explicit label scheme override from
// GCLUSTER_LABEL_SCHEME via getenv (or os.Getenv when nil).
func ResolveLabelSchemeOverride(getenv func(string) string) (LabelScheme, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	envVal := getenv(LabelSchemeEnvVar)
	scheme, err := ParseLabelScheme(envVal)
	if err != nil {
		return "", fmt.Errorf("invalid %s value %q: allowed values are tpu-slice, gke, generic", LabelSchemeEnvVar, envVal)
	}
	return scheme, nil
}

func (noopHardwareResolver) IsTPUMachine(string) bool { return false }

func (noopHardwareResolver) NormalizeTPUType(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func (noopHardwareResolver) InferTPUMachine(_, rawMachine string, _ int) string {
	return strings.TrimSpace(rawMachine)
}

func (noopHardwareResolver) ChipsPerNode(string) int { return 0 }

// parseTopologyChipCount parses a 2D or 3D TPU topology string (e.g. "2x2", "2x2x2") into its chip count.
func parseTopologyChipCount(topology string) (int, bool) {
	s := strings.ToLower(strings.TrimSpace(topology))
	if s == "" {
		return 0, false
	}
	parts := strings.Split(s, "x")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	total := 1
	for _, p := range parts {
		if p == "" {
			return 0, false
		}
		v, err := strconv.Atoi(p)
		if err != nil || v <= 0 {
			return 0, false
		}
		if total > math.MaxInt/v {
			return 0, false
		}
		total *= v
	}
	return total, true
}

func isSingleChipTopology(topology string) bool {
	chips, ok := parseTopologyChipCount(topology)
	return ok && chips == 1
}

func calculateNodesFromTopology(topology string, chipsPerNode int) int {
	if chipsPerNode <= 0 {
		return 0
	}
	totalChips, ok := parseTopologyChipCount(topology)
	if !ok || totalChips%chipsPerNode != 0 {
		return 0
	}
	return totalChips / chipsPerNode
}

// IsNoMatchError reports whether err indicates a missing Kubernetes API group, resource, or CRD
// (distinct from an object-level 404 NotFound).
func IsNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	if meta.IsNoMatchError(err) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return false
	}
	var statusErr apierrors.APIStatus
	if errors.As(err, &statusErr) && !apierrors.IsNotFound(err) {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "error from server (") && !strings.Contains(msg, "error from server (notfound)") {
		return false
	}
	return strings.Contains(msg, "could not find the requested resource") ||
		strings.Contains(msg, "doesn't have a resource type") ||
		strings.Contains(msg, "no matches for kind")
}

// IsNotFoundOrNoMatchError reports whether err indicates a missing Kubernetes API resource,
// CRD, or object (typed NotFound/NoMatch or equivalent kubectl stderr text).
func IsNotFoundOrNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) || IsNoMatchError(err) {
		return true
	}
	var statusErr apierrors.APIStatus
	if errors.As(err, &statusErr) {
		return false
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "error from server (notfound)")
}

// DiscoverInventory queries Kubernetes nodes, TPU Slices and PartitionTrees, and the non-privileged
// API group discovery endpoint (/apis/slice.tpu.google.com) without invoking gcloud.
// When listing nodes fails with a Kubernetes RBAC 403 Forbidden error (multi-tenant RBAC), it enters
// explicit mode cleanly and resolves the label scheme via API group discovery or opts.LabelSchemeOverride.
func DiscoverInventory(lister ResourceLister, apiChecker APIGroupChecker, opts DiscoverOptions) (*Inventory, error) {
	override, err := ParseLabelScheme(string(opts.LabelSchemeOverride))
	if err != nil {
		return nil, err
	}
	if lister == nil {
		return nil, errListerRequired
	}

	inv := &Inventory{Pools: make(map[string]PoolCapacity)}
	hw := opts.HardwareResolver
	if hw == nil {
		hw = noopHardwareResolver{}
	}

	signals, err := discoverNodes(lister, inv, hw, override)
	if err != nil {
		return nil, err
	}

	probeErr := probeTPUSliceAPIGroup(apiChecker, inv, signals, override)
	if err := discoverTPUSliceCRDs(lister, apiChecker, probeErr, inv, signals, hw, override); err != nil {
		return nil, err
	}

	inv.LabelScheme = resolveLabelScheme(override, signals.hasTPUSliceLabels || inv.TPUSliceAPIPresent, signals.hasGKELabels)
	inv.TPU, inv.TPUPoolCap = buildTPUInventory(inv, signals, hw)
	return inv, nil
}

// SummaryLines returns deterministic, human-readable lines describing the discovered cluster inventory.
func (inv *Inventory) SummaryLines() []string {
	if inv == nil {
		return nil
	}
	if inv.ExplicitMode {
		return inv.explicitSummaryLines()
	}
	lines := []string{
		fmt.Sprintf("Discovered %d/%d ready Kubernetes %s across %d %s (label scheme: %s).",
			inv.ReadyNodes, inv.TotalNodes, pluralize(inv.TotalNodes, "node", "nodes"),
			len(inv.PoolNames), pluralize(len(inv.PoolNames), "pool", "pools"), inv.LabelScheme),
	}
	if inv.TPU != nil {
		lines = append(lines, formatTPUSummaryLine(inv, inv.TPU, inv.TPUPoolCap))
	}
	lines = append(lines, inv.gpuSummaryLines()...)
	if inv.CPUPoolCap != nil {
		lines = append(lines, formatCPUSummaryLine(inv.CPUPoolCap))
	}
	return lines
}

// LogSummary emits deterministic INFO logs summarizing the discovered cluster inventory.
func (inv *Inventory) LogSummary() {
	for _, line := range inv.SummaryLines() {
		logging.Info("%s", line)
	}
}

func (inv *Inventory) explicitSummaryLines() []string {
	header := fmt.Sprintf("Kubernetes explicit mode active (label scheme: %s).", inv.LabelScheme)
	if inv.TPUSliceAPIPresent {
		header = fmt.Sprintf("Kubernetes explicit mode active (label scheme: %s, TPU slice API detected).", inv.LabelScheme)
	}
	lines := []string{header}
	if inv.TPU != nil {
		lines = append(lines, formatTPUSummaryLine(inv, inv.TPU, inv.TPUPoolCap))
	} else if inv.SliceInventoryUnavailable {
		lines = append(lines, "TPU slice inventory unavailable (could not list Slice resources).")
	}
	return lines
}

func (inv *Inventory) gpuSummaryLines() []string {
	var lines []string
	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if !pool.IsGPU {
			continue
		}
		lines = append(lines, formatGPUSummaryLine(name, pool))
	}
	return lines
}

func formatTPUSummaryLine(inv *Inventory, tpu *TPUInventory, cap *PoolCapacity) string {
	header, headerLabel := tpuSummaryHeader(inv, tpu, cap)
	details := appendTPUIdentityDetails(nil, tpu, cap, headerLabel)
	details = appendTPUSizeDetails(details, tpu, cap)
	if len(details) == 0 {
		return header + "."
	}
	return fmt.Sprintf("%s (%s).", header, strings.Join(details, ", "))
}

func appendTPUIdentityDetails(details []string, tpu *TPUInventory, cap *PoolCapacity, headerLabel string) []string {
	if tpu.TotalTPUNodes > 0 {
		details = append(details, fmt.Sprintf("%d/%d ready %s", tpu.ReadyTPUNodes, tpu.TotalTPUNodes, pluralize(tpu.TotalTPUNodes, "node", "nodes")))
	}
	if tpu.TPUType != "" && tpu.TPUType != headerLabel {
		details = append(details, "type="+tpu.TPUType)
	}
	if tpu.MachineType != "" && tpu.MachineType != headerLabel {
		details = append(details, "machine="+tpu.MachineType)
	}
	if cap != nil && cap.NodePoolName != "" && cap.NodePoolName != tpu.MachineType && cap.NodePoolName != headerLabel {
		details = append(details, "pool="+cap.NodePoolName)
	}
	return details
}

func appendTPUSizeDetails(details []string, tpu *TPUInventory, cap *PoolCapacity) []string {
	if tpu.Topology != "" {
		details = append(details, "topology="+tpu.Topology)
	}
	if tpu.NodesPerSlice > 0 {
		details = append(details, fmt.Sprintf("%d %s/slice", tpu.NodesPerSlice, pluralize(tpu.NodesPerSlice, "node", "nodes")))
	}
	if tpu.ChipsPerNode > 0 {
		details = append(details, fmt.Sprintf("%d TPU %s/node", tpu.ChipsPerNode, pluralize(tpu.ChipsPerNode, "chip", "chips")))
	}
	if cap != nil {
		if res := formatComputeResources(cap.AllocatableCPUMilli, cap.MemMiBPerNode); res != "" {
			details = append(details, res+" per node")
		}
	}
	return details
}

func tpuSummaryHeader(inv *Inventory, tpu *TPUInventory, cap *PoolCapacity) (string, string) {
	if tpu.DefaultSliceName != "" {
		return "Default slice: " + tpu.DefaultSliceName, ""
	}
	if inv.LabelScheme == SchemeTPUSlice {
		if inv.SliceInventoryUnavailable {
			return "TPU pool: slice inventory unavailable", ""
		}
		if slices.ContainsFunc(inv.Slices, func(s SliceInfo) bool { return sliceMatchesPrimaryTPU(s, tpu) }) {
			return "TPU pool: no ready slice", ""
		}
		return "TPU pool: no active slice", ""
	}
	poolName := ""
	if cap != nil {
		poolName = cap.NodePoolName
	}
	if label := firstNonEmpty(tpu.MachineType, poolName, tpu.TPUType); label != "" {
		return "TPU pool: " + label, label
	}
	return "TPU pool", ""
}

func formatGPUSummaryLine(poolKey string, pool PoolCapacity) string {
	label := firstNonEmpty(pool.MachineType, pool.NodePoolName, poolKey)
	details := []string{
		fmt.Sprintf("%d/%d ready %s", pool.ReadyNodes, pool.TotalNodes, pluralize(pool.TotalNodes, "node", "nodes")),
	}
	if pool.NodePoolName != "" && pool.NodePoolName != label {
		details = append(details, "pool="+pool.NodePoolName)
	}
	if pool.GPUType != "" {
		details = append(details, "gpu="+pool.GPUType)
	}
	if pool.GPUsPerNode > 0 {
		details = append(details, fmt.Sprintf("%d %s/node", pool.GPUsPerNode, pluralize(pool.GPUsPerNode, "GPU", "GPUs")))
	}
	if res := formatComputeResources(pool.AllocatableCPUMilli, pool.MemMiBPerNode); res != "" {
		details = append(details, res+" per node")
	}
	return fmt.Sprintf("GPU pool: %s (%s).", label, strings.Join(details, ", "))
}

func formatCPUSummaryLine(c *PoolCapacity) string {
	label := firstNonEmpty(c.MachineType, c.NodePoolName, "cpu")
	details := []string{
		fmt.Sprintf("%d/%d ready %s", c.ReadyNodes, c.TotalNodes, pluralize(c.TotalNodes, "node", "nodes")),
	}
	if c.NodePoolName != "" && c.NodePoolName != label {
		details = append(details, "pool="+c.NodePoolName)
	}
	if res := formatComputeResources(c.AllocatableCPUMilli, c.MemMiBPerNode); res != "" {
		details = append(details, res+" per node")
	}
	return fmt.Sprintf("CPU pool: %s (%s).", label, strings.Join(details, ", "))
}

func formatComputeResources(cpuMilli int64, memMiB int) string {
	var parts []string
	if cpuMilli > 0 {
		if cpuMilli < 1000 {
			parts = append(parts, fmt.Sprintf("%dm vCPU", cpuMilli))
		} else {
			parts = append(parts, fmt.Sprintf("%d vCPU", cpuMilli/1000))
		}
	}
	if memMiB > 0 {
		parts = append(parts, fmt.Sprintf("%d MiB RAM", memMiB))
	}
	return strings.Join(parts, " / ")
}

func pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

func discoverNodes(lister ResourceLister, inv *Inventory, hw TPUHardwareResolver, override LabelScheme) (nodeSignals, error) {
	nodes, err := lister.ListResources(NodeGVR, "", "", "")
	if err != nil {
		if IsForbiddenError(err) {
			inv.ExplicitMode = true
			logging.Info("Lacking permission to list cluster nodes (403 Forbidden); entering explicit mode and skipping node inventory.")
			return nodeSignals{nodeSlices: make(map[string]*nodeSliceAgg)}, nil
		}
		return nodeSignals{}, fmt.Errorf("failed to list cluster nodes: %w", err)
	}
	return accumulateNodes(nodes, inv, hw, override), nil
}

// shouldProbeTPUSliceAPI reports whether auto-detection should probe /apis/slice.tpu.google.com.
// Skipping the probe when readable nodes already prove pure GKE (hasGKELabels && !hasTPUSliceLabels)
// is an intentional fast path to avoid an extra kubectl call on standard GKE clusters (scaled-to-zero
// GKE TPU Slice clusters without active TPU slice nodes can set GCLUSTER_LABEL_SCHEME=tpu-slice).
func shouldProbeTPUSliceAPI(override LabelScheme, inv *Inventory, signals nodeSignals) bool {
	if override != SchemeAuto {
		return false
	}
	if !inv.ExplicitMode && signals.hasGKELabels && !signals.hasTPUSliceLabels {
		return false
	}
	return true
}

func probeTPUSliceAPIGroup(apiChecker APIGroupChecker, inv *Inventory, signals nodeSignals, override LabelScheme) error {
	if apiChecker == nil || !shouldProbeTPUSliceAPI(override, inv, signals) {
		return nil
	}
	present, err := apiChecker.HasAPIGroupVersion(TPUSliceAPIGroupVersion)
	if err != nil {
		return err
	}
	if present {
		inv.TPUSliceAPIPresent = true
	}
	return nil
}

func discoverTPUSliceCRDs(lister ResourceLister, apiChecker APIGroupChecker, probeErr error, inv *Inventory, signals nodeSignals, hw TPUHardwareResolver, override LabelScheme) error {
	if override == SchemeGKE || override == SchemeGeneric {
		return nil
	}

	res, err := queryTPUSliceCRDs(lister, apiChecker, probeErr, inv, signals, hw, override)
	if err != nil {
		return err
	}
	logDeferredDiscoveryWarnings(probeErr, res.sliceErr, res.treeErr, inv)

	if res.sliceFound {
		inv.TPUSliceAPIPresent = true
		inv.Slices = res.slices
	} else if inv.ExplicitMode && (override == SchemeTPUSlice || inv.TPUSliceAPIPresent) {
		inv.SliceInventoryUnavailable = true
	}
	if len(res.trees) > 0 {
		inv.PartitionTrees = res.trees
	}
	mergeNodeDerivedSlices(inv, signals.nodeSlices, hw)
	return nil
}

func queryTPUSliceCRDs(lister ResourceLister, apiChecker APIGroupChecker, probeErr error, inv *Inventory, signals nodeSignals, hw TPUHardwareResolver, override LabelScheme) (crdDiscoveryResult, error) {
	shouldQuery := inv.TPUSliceAPIPresent || signals.hasTPUSliceLabels || override == SchemeTPUSlice
	if !shouldQuery && (apiChecker == nil || probeErr != nil) && !signals.hasGKELabels {
		shouldQuery = true
	}
	if !shouldQuery {
		return crdDiscoveryResult{}, nil
	}
	var out crdDiscoveryResult
	out.slices, out.sliceFound, out.sliceErr = listTPUSlices(lister, hw)
	if err := checkAutoSchemeFailClosed(override, inv, signals, apiChecker, probeErr, out.sliceErr); err != nil {
		return crdDiscoveryResult{}, err
	}
	if inv.TPUSliceAPIPresent || signals.hasTPUSliceLabels || override == SchemeTPUSlice || out.sliceFound {
		out.trees, out.treeErr = listTPUPartitionTrees(lister, hw)
	}
	return out, nil
}

func checkAutoSchemeFailClosed(override LabelScheme, inv *Inventory, signals nodeSignals, apiChecker APIGroupChecker, probeErr, sliceErr error) error {
	if override != SchemeAuto || signals.hasTPUSliceLabels || signals.hasGKELabels || inv.TPUSliceAPIPresent {
		return nil
	}
	if apiChecker != nil && probeErr == nil {
		return nil
	}
	if sliceErr == nil || IsNotFoundOrNoMatchError(sliceErr) {
		return nil
	}
	if probeErr != nil {
		return fmt.Errorf("cannot determine Kubernetes TPU label scheme: API group probe failed (%w) and listing TPU Slice resources failed (%w); set %s to tpu-slice, gke, or generic to override", probeErr, sliceErr, LabelSchemeEnvVar)
	}
	return fmt.Errorf("cannot determine Kubernetes TPU label scheme without an API group probe: listing TPU Slice resources failed (%w); set %s to tpu-slice, gke, or generic to override", sliceErr, LabelSchemeEnvVar)
}

func logDeferredDiscoveryWarnings(probeErr, sliceErr, treeErr error, inv *Inventory) {
	if probeErr != nil {
		logging.Warn("Failed to probe Kubernetes API group %s: %v", TPUSliceAPIGroupVersion, probeErr)
	}
	if sliceErr != nil && !IsNotFoundOrNoMatchError(sliceErr) {
		inv.SliceInventoryUnavailable = true
		if IsForbiddenError(sliceErr) {
			logging.Info("Skipping TPU Slice inventory (lacking permission to list slices).")
		} else {
			logging.Warn("Failed to list TPU Slice resources: %v", sliceErr)
		}
	}
	if treeErr != nil && !IsNotFoundOrNoMatchError(treeErr) {
		if IsForbiddenError(treeErr) {
			logging.Info("Skipping TPU PartitionTree inventory (lacking permission to list partitiontrees).")
		} else {
			logging.Warn("Failed to list TPU PartitionTree resources: %v", treeErr)
		}
	}
}

func resolveLabelScheme(override LabelScheme, tpuSliceDetected, gkeDetected bool) LabelScheme {
	if override != SchemeAuto {
		return override
	}
	if tpuSliceDetected {
		return SchemeTPUSlice
	}
	if gkeDetected {
		return SchemeGKE
	}
	return SchemeGeneric
}

func accumulateNodes(nodes []unstructured.Unstructured, inv *Inventory, hw TPUHardwareResolver, override LabelScheme) nodeSignals {
	signals := nodeSignals{
		poolControlPlane: make(map[string]bool),
		nodeSlices:       make(map[string]*nodeSliceAgg),
	}
	builders := make(map[poolGroupKey]*poolBuilder)

	for i := range nodes {
		snap := extractNodeSnapshot(nodes[i], hw, override)
		inv.TotalNodes++
		if snap.ready {
			inv.ReadyNodes++
		}
		signals.hasTPUSliceLabels = signals.hasTPUSliceLabels || snap.hasTPUSlice
		signals.hasGKELabels = signals.hasGKELabels || snap.hasGKE
		signals.hasPartitionLabels = signals.hasPartitionLabels || snap.hasPartition

		addNodeToPool(builders, snap)
		addNodeToSliceAgg(signals.nodeSlices, snap)
	}

	finalizePools(inv, builders, &signals)
	return signals
}

func extractNodeSnapshot(node unstructured.Unstructured, hw TPUHardwareResolver, override LabelScheme) nodeSnapshot {
	labels := node.GetLabels()
	cpuMilli, memMiB, tpus, gpus, inferredGPUType, gpuResourceName := parseNodeAllocatable(node)
	hasTPUSlice, hasPartition := hasTPUSliceLabels(labels)
	hasGKE := hasGKELabels(labels)

	observedInstanceLabel := strings.TrimSpace(labels[InstanceTypeLabel])
	observedGPUProductLabel := strings.TrimSpace(labels[NvidiaGPUProductLabel])
	observedGKEAccelLabel := strings.TrimSpace(labels[GKEAcceleratorLabel])
	observedGKETPUAccelLabel := strings.TrimSpace(labels[GKETPUAcceleratorLabel])
	rawInstance := firstNonEmpty(observedInstanceLabel, labels[BetaInstanceTypeLabel])
	rawTPUType, topology := pickNodeTPULabels(labels, override)
	sliceName := strings.TrimSpace(labels[TPUSliceNameLabel])
	nodePool := strings.TrimSpace(labels[GKENodePoolLabel])
	if override == SchemeGKE || override == SchemeGeneric {
		sliceName = ""
		hasPartition = false
	}

	isTPU := tpus > 0 || rawTPUType != "" || topology != "" || sliceName != "" || hw.IsTPUMachine(rawInstance)
	tpuType, machineType := resolveNodeMachine(hw, isTPU, rawTPUType, rawInstance, tpus)

	gpuType := firstNonEmpty(labels[GKEAcceleratorLabel], observedGPUProductLabel, inferredGPUType)
	isGPU := !isTPU && (gpus > 0 || gpuType != "")
	poolKey := firstNonEmpty(machineType, nodePool, fallbackClassPoolKey(isTPU, isGPU))

	return nodeSnapshot{
		ready:             isNodeReady(node),
		cpuMilli:          cpuMilli,
		memMiB:            memMiB,
		tpus:              tpus,
		gpus:              gpus,
		isTPU:             isTPU,
		isGPU:             isGPU,
		tpuType:           tpuType,
		gpuType:           gpuType,
		gpuResourceName:   gpuResourceName,
		instanceTypeLabel: observedInstanceLabel,
		gpuProductLabel:   observedGPUProductLabel,
		gkeAccelLabel:     observedGKEAccelLabel,
		gkeTPUAccelLabel:  observedGKETPUAccelLabel,
		machineType:       machineType,
		poolKey:           poolKey,
		nodePool:          nodePool,
		topology:          topology,
		sliceName:         sliceName,
		hasTPUSlice:       hasTPUSlice,
		hasGKE:            hasGKE,
		hasPartition:      hasPartition,
		controlPlane:      isControlPlaneNode(labels),
		systemPool:        isSystemPoolNode(node, nodePool),
	}
}

func pickNodeTPULabels(labels map[string]string, override LabelScheme) (rawType, topology string) {
	if override == SchemeGKE {
		rawType = firstNonEmpty(labels[GKETPUAcceleratorLabel], labels[TPUSliceTypeLabel])
		topology = strings.ToLower(strings.TrimSpace(firstNonEmpty(labels[GKETPUTopologyLabel], labels[TPUSliceTopologyLabel])))
		return rawType, topology
	}
	rawType = firstNonEmpty(labels[TPUSliceTypeLabel], labels[GKETPUAcceleratorLabel])
	topology = strings.ToLower(strings.TrimSpace(firstNonEmpty(labels[TPUSliceTopologyLabel], labels[GKETPUTopologyLabel])))
	return rawType, topology
}

func resolveNodeMachine(hw TPUHardwareResolver, isTPU bool, rawTPUType, rawInstance string, tpus int) (tpuType, machineType string) {
	if !isTPU {
		return "", rawInstance
	}
	tpuType = hw.NormalizeTPUType(rawTPUType)
	if tpuType == "" && hw.IsTPUMachine(rawInstance) {
		tpuType = hw.NormalizeTPUType(rawInstance)
	}
	if rawInstance != "" || tpus > 0 {
		machineType = hw.InferTPUMachine(tpuType, rawInstance, tpus)
	}
	return tpuType, machineType
}

func fallbackClassPoolKey(isTPU, isGPU bool) string {
	if isTPU {
		return "tpu"
	}
	if isGPU {
		return "gpu"
	}
	return "k8s-node"
}

func nodePoolClass(snap nodeSnapshot) string {
	if snap.isTPU {
		return "tpu"
	}
	if snap.isGPU {
		return "gpu"
	}
	return "cpu"
}

func getOrCreatePoolBuilder(builders map[poolGroupKey]*poolBuilder, snap nodeSnapshot) *poolBuilder {
	accelType := ""
	if snap.isTPU {
		accelType = snap.tpuType
	} else if snap.isGPU {
		accelType = strings.ToLower(snap.gpuType)
	}
	key := poolGroupKey{
		poolKey:         snap.poolKey,
		nodePool:        snap.nodePool,
		class:           nodePoolClass(snap),
		accelType:       accelType,
		gpuResourceName: snap.gpuResourceName,
		controlPlane:    snap.controlPlane,
		systemPool:      snap.systemPool,
	}
	if b, ok := builders[key]; ok {
		return b
	}
	b := &poolBuilder{
		key: key,
		cap: PoolCapacity{
			NodePoolName:   snap.nodePool,
			IsControlPlane: snap.controlPlane,
			IsSystemPool:   snap.systemPool,
		},
		topologies: make(map[string]struct{}),
		slices:     make(map[string]struct{}),
	}
	builders[key] = b
	return b
}

func addNodeToPool(builders map[poolGroupKey]*poolBuilder, snap nodeSnapshot) {
	b := getOrCreatePoolBuilder(builders, snap)
	b.cap.TotalNodes++
	b.cap.IsTPU = b.cap.IsTPU || snap.isTPU
	b.cap.IsGPU = b.cap.IsGPU || snap.isGPU

	b.allMachineType = minNonEmpty(b.allMachineType, snap.machineType)
	b.allTPUType = minNonEmpty(b.allTPUType, snap.tpuType)
	b.allGPUType = minNonEmpty(b.allGPUType, snap.gpuType)
	b.allInstanceLabel.observe(snap.instanceTypeLabel)
	b.allGPUProductLabel.observe(snap.gpuProductLabel)
	b.allGKEAccelLabel.observe(snap.gkeAccelLabel)
	b.allGKETPUAccelLabel.observe(snap.gkeTPUAccelLabel)
	b.allCPUMilli = minPositive(b.allCPUMilli, snap.cpuMilli)
	b.allMemMiB = minPositive(b.allMemMiB, snap.memMiB)
	b.allTPUs = minPositive(b.allTPUs, snap.tpus)
	b.allGPUs = minPositive(b.allGPUs, snap.gpus)

	if snap.ready {
		b.cap.ReadyNodes++
		b.readyMachineType = minNonEmpty(b.readyMachineType, snap.machineType)
		b.readyTPUType = minNonEmpty(b.readyTPUType, snap.tpuType)
		b.readyGPUType = minNonEmpty(b.readyGPUType, snap.gpuType)
		b.readyInstanceLabel.observe(snap.instanceTypeLabel)
		b.readyGPUProductLabel.observe(snap.gpuProductLabel)
		b.readyGKEAccelLabel.observe(snap.gkeAccelLabel)
		b.readyGKETPUAccelLabel.observe(snap.gkeTPUAccelLabel)
		b.readyCPUMilli = minPositive(b.readyCPUMilli, snap.cpuMilli)
		b.readyMemMiB = minPositive(b.readyMemMiB, snap.memMiB)
		b.readyTPUs = minPositive(b.readyTPUs, snap.tpus)
		b.readyGPUs = minPositive(b.readyGPUs, snap.gpus)
	}
	if snap.topology != "" {
		b.topologies[snap.topology] = struct{}{}
	}
	if snap.sliceName != "" {
		b.slices[snap.sliceName] = struct{}{}
	}
}

func finalizeBuilderCapacity(b *poolBuilder) PoolCapacity {
	c := b.cap
	c.MachineType = firstNonEmpty(b.readyMachineType, b.allMachineType)
	c.TPUType = firstNonEmpty(b.readyTPUType, b.allTPUType)
	c.GPUType = firstNonEmpty(b.readyGPUType, b.allGPUType)
	c.GPUResourceName = b.key.gpuResourceName
	if b.cap.ReadyNodes > 0 {
		c.InstanceTypeLabel = b.readyInstanceLabel.resolved()
		c.GPUProductLabel = b.readyGPUProductLabel.resolved()
		c.GKEAcceleratorLabel = b.readyGKEAccelLabel.resolved()
		c.GKETPUAcceleratorLabel = b.readyGKETPUAccelLabel.resolved()
	} else {
		c.InstanceTypeLabel = b.allInstanceLabel.resolved()
		c.GPUProductLabel = b.allGPUProductLabel.resolved()
		c.GKEAcceleratorLabel = b.allGKEAccelLabel.resolved()
		c.GKETPUAcceleratorLabel = b.allGKETPUAccelLabel.resolved()
	}
	c.AllocatableCPUMilli = cmp.Or(b.readyCPUMilli, b.allCPUMilli)
	c.MemMiBPerNode = cmp.Or(b.readyMemMiB, b.allMemMiB)
	c.TPUsPerNode = cmp.Or(b.readyTPUs, b.allTPUs)
	c.GPUsPerNode = cmp.Or(b.readyGPUs, b.allGPUs)
	c.Topologies = slices.Sorted(maps.Keys(b.topologies))
	c.SliceNames = slices.Sorted(maps.Keys(b.slices))
	return c
}

func addNodeToSliceAgg(agg map[string]*nodeSliceAgg, snap nodeSnapshot) {
	if snap.sliceName == "" {
		return
	}
	entry, ok := agg[snap.sliceName]
	if !ok {
		entry = &nodeSliceAgg{name: snap.sliceName}
		agg[snap.sliceName] = entry
	}
	entry.totalNodes++
	entry.tpuType = minNonEmpty(entry.tpuType, snap.tpuType)
	entry.machineType = minNonEmpty(entry.machineType, snap.machineType)
	entry.topology = minNonEmpty(entry.topology, snap.topology)
	entry.chips = minPositive(entry.chips, snap.tpus)
	if snap.ready {
		entry.readyNodes++
		entry.readyTPUType = minNonEmpty(entry.readyTPUType, snap.tpuType)
		entry.readyMachineType = minNonEmpty(entry.readyMachineType, snap.machineType)
		entry.readyTopology = minNonEmpty(entry.readyTopology, snap.topology)
		entry.readyChips = minPositive(entry.readyChips, snap.tpus)
	}
}

func comparePoolGroupKeys(a, b poolGroupKey) int {
	return cmp.Or(
		strings.Compare(a.poolKey, b.poolKey),
		strings.Compare(a.nodePool, b.nodePool),
		strings.Compare(a.class, b.class),
		strings.Compare(a.accelType, b.accelType),
		strings.Compare(a.gpuResourceName, b.gpuResourceName),
		compareBool(a.controlPlane, b.controlPlane),
		compareBool(a.systemPool, b.systemPool),
	)
}

func compareBool(a, b bool) int {
	if a == b {
		return 0
	}
	if !a {
		return -1
	}
	return 1
}

func formatDisambiguatedPoolKey(key poolGroupKey) string {
	parts := []string{key.poolKey}
	if key.nodePool != "" && key.nodePool != key.poolKey {
		parts = append(parts, key.nodePool)
	}
	if key.controlPlane {
		if key.class == "cpu" {
			parts = append(parts, "control-plane")
		} else {
			parts = append(parts, "control-plane-"+key.class)
		}
	} else {
		parts = append(parts, key.class)
	}
	if key.accelType != "" {
		parts = append(parts, key.accelType)
	}
	if key.gpuResourceName != "" && !strings.EqualFold(key.gpuResourceName, key.accelType) {
		parts = append(parts, key.gpuResourceName)
	}
	if key.systemPool {
		parts = append(parts, "system")
	}
	return strings.Join(parts, "/")
}

func uniquePoolMapKey(candidate string, used map[string]bool) string {
	if !used[candidate] {
		return candidate
	}
	for i := 2; ; i++ {
		next := fmt.Sprintf("%s#%d", candidate, i)
		if !used[next] {
			return next
		}
	}
}

func finalizePools(inv *Inventory, builders map[poolGroupKey]*poolBuilder, signals *nodeSignals) {
	keys := make([]poolGroupKey, 0, len(builders))
	countByPoolKey := make(map[string]int, len(builders))
	for key := range builders {
		keys = append(keys, key)
		countByPoolKey[key.poolKey]++
	}
	slices.SortFunc(keys, comparePoolGroupKeys)

	used := make(map[string]bool, len(keys))
	byName := make(map[string]*poolBuilder, len(keys))
	names := make([]string, 0, len(keys))
	for _, key := range keys {
		base := key.poolKey
		if countByPoolKey[key.poolKey] > 1 {
			base = formatDisambiguatedPoolKey(key)
		}
		name := uniquePoolMapKey(base, used)
		used[name] = true
		byName[name] = builders[key]
		names = append(names, name)
	}
	slices.Sort(names)
	inv.PoolNames = names

	for _, name := range names {
		b := byName[name]
		inv.Pools[name] = finalizeBuilderCapacity(b)
		signals.poolControlPlane[name] = b.key.controlPlane
	}
	inv.CPUPoolCap = selectCPUPoolCap(inv)
}

func selectCPUPoolCap(inv *Inventory) *PoolCapacity {
	var best *PoolCapacity
	bestName := ""

	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if pool.IsTPU || pool.IsGPU {
			continue
		}
		if best == nil || IsBetterPool(name, pool, bestName, *best) {
			p := pool
			best = &p
			bestName = name
		}
	}
	return best
}

// PoolPreferenceScore ranks discovered pools by scheduling suitability:
// non-control-plane > usable accelerator capacity > non-system > ReadyNodes > 0.
func PoolPreferenceScore(pool PoolCapacity) int {
	score := 0
	if !pool.IsControlPlane {
		score += 8
	}
	if (!pool.IsGPU || pool.GPUsPerNode > 0) && (!pool.IsTPU || pool.TPUsPerNode > 0) {
		score += 4
	}
	if !pool.IsSystemPool {
		score += 2
	}
	if pool.ReadyNodes > 0 {
		score++
	}
	return score
}

// IsBetterPool reports whether cand should be preferred over cur using PoolPreferenceScore,
// breaking ties by AllocatableCPUMilli and then lexicographic pool name.
func IsBetterPool(candName string, cand PoolCapacity, curName string, cur PoolCapacity) bool {
	candScore, curScore := PoolPreferenceScore(cand), PoolPreferenceScore(cur)
	if candScore != curScore {
		return candScore > curScore
	}
	if cand.AllocatableCPUMilli != cur.AllocatableCPUMilli {
		return cand.AllocatableCPUMilli > cur.AllocatableCPUMilli
	}
	return candName < curName
}

// TopPreferredPools returns all pools tied for the highest PoolPreferenceScore in input order.
func TopPreferredPools(candidates []PoolCapacity) []PoolCapacity {
	if len(candidates) == 0 {
		return nil
	}
	bestScore := PoolPreferenceScore(candidates[0])
	top := []PoolCapacity{candidates[0]}
	for _, c := range candidates[1:] {
		s := PoolPreferenceScore(c)
		if s > bestScore {
			bestScore = s
			top = []PoolCapacity{c}
		} else if s == bestScore {
			top = append(top, c)
		}
	}
	return top
}

func nodeAllocatableMap(node unstructured.Unstructured) (map[string]interface{}, bool) {
	rawAlloc, found, err := unstructured.NestedFieldNoCopy(node.Object, "status", "allocatable")
	if err != nil || !found {
		return nil, false
	}
	alloc, ok := rawAlloc.(map[string]interface{})
	return alloc, ok
}

func parseNodeAllocatable(node unstructured.Unstructured) (cpuMilli int64, memMiB, tpus, gpus int, inferredGPU, gpuResourceName string) {
	alloc, ok := nodeAllocatableMap(node)
	if !ok {
		return 0, 0, 0, 0, "", ""
	}
	if q, ok := parseQuantityField(alloc, "cpu"); ok {
		cpuMilli = q.MilliValue()
	}
	if q, ok := parseQuantityField(alloc, "memory"); ok {
		memMiB = int(q.Value() / (1024 * 1024))
	}
	if q, ok := parseQuantityField(alloc, "google.com/tpu"); ok {
		tpus = int(q.Value())
	}
	gpus, inferredGPU, gpuResourceName = parseAllocatableGPUs(alloc)
	return cpuMilli, memMiB, tpus, gpus, inferredGPU, gpuResourceName
}

var gpuResourceRules = []gpuResourceRule{
	{key: "nvidia.com/gpu", inferredType: "nvidia"},
	{key: "nvidia.com/gpu.shared", inferredType: "nvidia"},
	{key: "nvidia.com/mig-", isPrefix: true},
	{key: "amd.com/gpu", inferredType: "amd"},
	{key: "gpu.intel.com/i915"},
	{key: "gpu.intel.com/xe"},
	{key: "gpu.intel.com/", isPrefix: true},
}

func matchGPUResourceRule(k string, rule gpuResourceRule) bool {
	if !rule.isPrefix {
		return k == rule.key
	}
	if !strings.HasPrefix(k, rule.key) || len(k) <= len(rule.key) {
		return false
	}
	return rule.key != "gpu.intel.com/" || !isIntelGPUMetadataKey(k)
}

func parseAllocatableGPUs(alloc map[string]interface{}) (int, string, string) {
	var gpuKeys []string
	for k := range alloc {
		if IsGPUResourceName(k) {
			gpuKeys = append(gpuKeys, k)
		}
	}
	if len(gpuKeys) == 0 {
		return 0, "", ""
	}
	slices.Sort(gpuKeys)
	for _, rule := range gpuResourceRules {
		for _, k := range gpuKeys {
			if !matchGPUResourceRule(k, rule) {
				continue
			}
			if q, ok := parseQuantityField(alloc, k); ok && q.Value() > 0 {
				return int(q.Value()), cmp.Or(rule.inferredType, k), k
			}
		}
	}
	return 0, "", ""
}

// IsGPUResourceName reports whether key is a recognized Kubernetes GPU extended resource name
// (NVIDIA full/time-sliced/MIG, AMD, or Intel non-metadata GPU resource keys).
func IsGPUResourceName(key string) bool {
	k := strings.TrimSpace(key)
	for _, rule := range gpuResourceRules {
		if matchGPUResourceRule(k, rule) {
			return true
		}
	}
	return false
}

func isIntelGPUMetadataKey(k string) bool {
	switch k {
	case "gpu.intel.com/millicores", "gpu.intel.com/memory.max", "gpu.intel.com/tiles":
		return true
	default:
		return strings.HasSuffix(k, "_monitoring")
	}
}

func parseQuantityField(alloc map[string]interface{}, key string) (resource.Quantity, bool) {
	raw, exists := alloc[key]
	if !exists || raw == nil {
		return resource.Quantity{}, false
	}
	var str string
	switch v := raw.(type) {
	case string:
		str = strings.TrimSpace(v)
	case int64, int, float64, json.Number:
		str = fmt.Sprint(v)
	default:
		return resource.Quantity{}, false
	}
	if str == "" {
		return resource.Quantity{}, false
	}
	qty, err := resource.ParseQuantity(str)
	if err != nil || qty.Sign() < 0 {
		return resource.Quantity{}, false
	}
	return qty, true
}

func findReadyCondition(obj map[string]interface{}) (ready bool, reason string, found bool) {
	conditions, _, _ := unstructured.NestedSlice(obj, "status", "conditions")
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cType, _ := cond["type"].(string)
		if cType != "Ready" {
			continue
		}
		cStatus, _ := cond["status"].(string)
		cReason, _ := cond["reason"].(string)
		return cStatus == "True", strings.TrimSpace(cReason), true
	}
	return false, "", false
}

func isNodeReady(node unstructured.Unstructured) bool {
	if node.GetDeletionTimestamp() != nil {
		return false
	}
	if unsched, _, _ := unstructured.NestedBool(node.Object, "spec", "unschedulable"); unsched {
		return false
	}
	ready, _, _ := findReadyCondition(node.Object)
	return ready
}

func hasTPUSliceLabels(labels map[string]string) (hasTPUSlice, hasPartition bool) {
	for k := range labels {
		if strings.HasPrefix(k, TPUSliceLabelPrefix) {
			hasTPUSlice = true
		}
		if strings.HasPrefix(k, TPUSlicePartitionLabelPrefix) {
			hasPartition = true
		}
	}
	return hasTPUSlice, hasPartition
}

func hasGKELabels(labels map[string]string) bool {
	for _, key := range []string{GKENodePoolLabel, GKETPUAcceleratorLabel, GKETPUTopologyLabel, GKEAcceleratorLabel} {
		if strings.TrimSpace(labels[key]) != "" {
			return true
		}
	}
	return false
}

func isControlPlaneNode(labels map[string]string) bool {
	_, hasCtrl := labels[ControlPlaneRoleLabel]
	_, hasMaster := labels[MasterRoleLabel]
	return hasCtrl || hasMaster
}

func isSystemPoolNode(node unstructured.Unstructured, nodePool string) bool {
	if nodePool == gkeDefaultPool {
		return true
	}
	taints, _, _ := unstructured.NestedSlice(node.Object, "spec", "taints")
	for _, raw := range taints {
		taint, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		key, _ := taint["key"].(string)
		val, _ := taint["value"].(string)
		if (key == GKESystemTaintKey || key == "CriticalAddonsOnly") && (val == "" || val == "true") {
			return true
		}
	}
	return false
}

// listTPUSlices queries slice.tpu.google.com/v1alpha1 (spec.{type,topology,partitionIds} and
// status.conditions Ready) and falls back to v1beta1 strictly on NotFound/NoMatch.
func listTPUSlices(lister ResourceLister, hw TPUHardwareResolver) ([]SliceInfo, bool, error) {
	items, err := lister.ListResources(TPUSliceGVR, "", "", "")
	if err != nil && IsNotFoundOrNoMatchError(err) {
		items, err = lister.ListResources(TPUSliceV1Beta1GVR, "", "", "")
	}
	if err != nil {
		return nil, false, err
	}
	result := make([]SliceInfo, 0, len(items))
	for i := range items {
		result = append(result, parseTPUSlice(items[i], hw))
	}
	sortSlices(result)
	return result, true, nil
}

func parseTPUSlice(obj unstructured.Unstructured, hw TPUHardwareResolver) SliceInfo {
	labels := obj.GetLabels()
	rawType := firstNonEmpty(
		nestedString(obj.Object, "spec", "type"),
		labels[TPUSliceTypeLabel],
	)
	topology := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		nestedString(obj.Object, "spec", "topology"),
		labels[TPUSliceTopologyLabel],
	)))
	ready, reason, _ := findReadyCondition(obj.Object)
	if obj.GetDeletionTimestamp() != nil {
		ready = false
		reason = "Terminating"
	}
	partitionIDs, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "partitionIds")

	totalChips, _ := parseTopologyChipCount(topology)
	chipsHint := 0
	if totalChips == 1 {
		chipsHint = 1
	}
	tpuType := hw.NormalizeTPUType(rawType)
	var machineType string
	if !isAmbiguousEightChipSlice(hw, tpuType, totalChips) {
		machineType = hw.InferTPUMachine(tpuType, "", chipsHint)
	}
	chips := hw.ChipsPerNode(machineType)
	if chips == 0 && chipsHint > 0 {
		chips = chipsHint
	}

	return SliceInfo{
		Name:          obj.GetName(),
		Namespace:     obj.GetNamespace(),
		TPUType:       tpuType,
		MachineType:   machineType,
		Topology:      topology,
		State:         reason,
		Ready:         ready,
		NodesPerSlice: calculateNodesFromTopology(topology, chips),
		ChipsPerNode:  chips,
		PartitionIDs:  partitionIDs,
	}
}

func isAmbiguousEightChipSlice(hw TPUHardwareResolver, tpuType string, totalChips int) bool {
	if totalChips != 8 || tpuType == "" {
		return false
	}
	return hw.ChipsPerNode(hw.InferTPUMachine(tpuType, "", 8)) == 8 &&
		hw.ChipsPerNode(hw.InferTPUMachine(tpuType, "", 4)) == 4
}

// listTPUPartitionTrees queries partition.tpu.google.com/v1alpha1 and falls back to v1beta1
// strictly on NotFound/NoMatch.
func listTPUPartitionTrees(lister ResourceLister, hw TPUHardwareResolver) ([]PartitionTreeInfo, error) {
	items, err := lister.ListResources(TPUPartitionTreeGVR, "", "", "")
	if err != nil && IsNotFoundOrNoMatchError(err) {
		items, err = lister.ListResources(TPUPartitionTreeV1Beta1GVR, "", "", "")
	}
	if err != nil {
		return nil, err
	}
	result := make([]PartitionTreeInfo, 0, len(items))
	for i := range items {
		if items[i].GetDeletionTimestamp() != nil {
			continue
		}
		result = append(result, parsePartitionTree(items[i], hw))
	}
	slices.SortFunc(result, comparePartitionTrees)
	return result, nil
}

func comparePartitionTrees(a, b PartitionTreeInfo) int {
	return cmp.Or(
		strings.Compare(a.Name, b.Name),
		strings.Compare(a.TPUType, b.TPUType),
		strings.Compare(a.RootTopology, b.RootTopology),
	)
}

// parsePartitionTree supports both observed partition.tpu.google.com CRD layouts:
//   - v1alpha1: status itself is the root node (status.{partitionId,partitionTypeInfo,childPartitions})
//     and TPU type is carried in metadata.labels["tpu.google.com/type"] or partitionTypeInfo prefix.
//   - v1beta1: status.acceleratorType plus a nested status.root.{topology,partitionId,childPartitions}.
func parsePartitionTree(obj unstructured.Unstructured, hw TPUHardwareResolver) PartitionTreeInfo {
	labels := obj.GetLabels()
	statusMap, _, _ := unstructured.NestedMap(obj.Object, "status")
	rootNode := statusMap
	if nested, found, _ := unstructured.NestedMap(obj.Object, "status", "root"); found && len(nested) > 0 {
		rootNode = nested
	}

	inferredPrefix, _ := splitPartitionTypeInfo(nestedString(rootNode, "partitionTypeInfo"))
	rawType := firstNonEmpty(
		nestedString(obj.Object, "status", "acceleratorType"),
		labels[TPUSliceTypeLabel],
		inferredPrefix,
	)
	rootTopo := extractNodeTopology(rootNode)
	var allTopos []string
	collectPartitionNodeTopologies(rootNode, &allTopos)

	return PartitionTreeInfo{
		Name:                obj.GetName(),
		TPUType:             hw.NormalizeTPUType(rawType),
		RootTopology:        rootTopo,
		SupportedTopologies: normalizeUniqueSorted(allTopos),
	}
}

func collectPartitionNodeTopologies(node map[string]interface{}, out *[]string) {
	if len(node) == 0 {
		return
	}
	if topo := extractNodeTopology(node); topo != "" {
		*out = append(*out, topo)
	}
	children, found, _ := unstructured.NestedSlice(node, "childPartitions")
	if !found {
		return
	}
	for _, raw := range children {
		if child, ok := raw.(map[string]interface{}); ok {
			collectPartitionNodeTopologies(child, out)
		}
	}
}

func extractNodeTopology(node map[string]interface{}) string {
	if topo := strings.ToLower(strings.TrimSpace(nestedString(node, "topology"))); topo != "" {
		return topo
	}
	if _, topo := splitPartitionTypeInfo(nestedString(node, "partitionTypeInfo")); topo != "" {
		return topo
	}
	return extractTopologyFromPartitionID(nestedString(node, "partitionId"))
}

// splitPartitionTypeInfo splits a v1alpha1 partitionTypeInfo string such as
// "IRONWOOD_4_4_4" or "TPU_V6E_2_4" into its accelerator prefix and topology ("4x4x4", "2x4").
func splitPartitionTypeInfo(info string) (prefix, topology string) {
	parts := strings.Split(strings.TrimSpace(info), "_")
	if len(parts) < 3 {
		return "", ""
	}
	dims := make([]string, 0, 3)
	cut := len(parts)
	for i := len(parts) - 1; i >= 1 && len(dims) < 3; i-- {
		v, err := strconv.Atoi(parts[i])
		if err != nil || v <= 0 {
			break
		}
		dims = append(dims, parts[i])
		cut = i
	}
	if len(dims) < 2 || cut == 0 {
		return "", ""
	}
	slices.Reverse(dims)
	return strings.Join(parts[:cut], "_"), strings.Join(dims, "x")
}

// extractTopologyFromPartitionID extracts a "<d1>x<d2>[x<d3>]" token from partition IDs
// such as "pod1-r1-4x4x4-1" or "pod1_r1_4x4x4_1" -> "4x4x4".
func extractTopologyFromPartitionID(id string) string {
	tokens := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(id)), func(r rune) bool {
		return r == '-' || r == '_'
	})
	for _, tok := range tokens {
		if _, ok := parseTopologyChipCount(tok); ok {
			return tok
		}
	}
	return ""
}

func mergeNodeDerivedSlices(inv *Inventory, nodeSlices map[string]*nodeSliceAgg, hw TPUHardwareResolver) {
	if len(nodeSlices) == 0 {
		return
	}
	byName := make(map[string][]int, len(inv.Slices))
	for i := range inv.Slices {
		byName[inv.Slices[i].Name] = append(byName[inv.Slices[i].Name], i)
	}
	for _, name := range slices.Sorted(maps.Keys(nodeSlices)) {
		agg := nodeSlices[name]
		if indices, exists := byName[name]; exists {
			for _, idx := range indices {
				enrichSliceFromNodes(&inv.Slices[idx], agg, hw)
			}
			continue
		}
		inv.Slices = append(inv.Slices, buildNodeDerivedSlice(agg, hw))
	}
	sortSlices(inv.Slices)
}

func preferredSliceAggValues(agg *nodeSliceAgg) (tpuType, machineType, topology string, chips int) {
	tpuType = firstNonEmpty(agg.readyTPUType, agg.tpuType)
	machineType = firstNonEmpty(agg.readyMachineType, agg.machineType)
	topology = firstNonEmpty(agg.readyTopology, agg.topology)
	chips = cmp.Or(agg.readyChips, agg.chips)
	return tpuType, machineType, topology, chips
}

func buildNodeDerivedSlice(agg *nodeSliceAgg, hw TPUHardwareResolver) SliceInfo {
	tpuType, machineType, topology, chips := preferredSliceAggValues(agg)
	if chips == 0 && machineType != "" {
		chips = hw.ChipsPerNode(machineType)
	}
	if chips == 0 && isSingleChipTopology(topology) {
		chips = 1
	}
	if machineType == "" && chips > 0 && tpuType != "" {
		machineType = hw.InferTPUMachine(tpuType, "", chips)
	}
	nodesPerSlice := resolveSliceNodeCount(topology, chips, agg.totalNodes)
	ready := sliceNodesReady(agg.readyNodes, agg.totalNodes, nodesPerSlice)
	state := ""
	if !ready && agg.totalNodes > 0 {
		state = fmt.Sprintf("NodesNotReady (%d/%d)", agg.readyNodes, max(agg.totalNodes, nodesPerSlice))
	}
	return SliceInfo{
		Name:          agg.name,
		TPUType:       tpuType,
		MachineType:   machineType,
		Topology:      topology,
		State:         state,
		Ready:         ready,
		NodesPerSlice: nodesPerSlice,
		ChipsPerNode:  chips,
	}
}

func enrichSliceFromNodes(s *SliceInfo, agg *nodeSliceAgg, hw TPUHardwareResolver) {
	tpuType, machineType, topology, nodeChips := preferredSliceAggValues(agg)
	if s.TPUType == "" {
		s.TPUType = tpuType
	}
	if s.Topology == "" {
		s.Topology = topology
	}
	if machineType != "" {
		s.MachineType = machineType
	}
	updateEnrichedSliceChipsAndMachine(s, machineType, nodeChips, hw)
	s.NodesPerSlice = resolveSliceNodeCount(s.Topology, s.ChipsPerNode, agg.totalNodes)
	if agg.totalNodes > 0 {
		nodesReady := sliceNodesReady(agg.readyNodes, agg.totalNodes, s.NodesPerSlice)
		if s.Ready && !nodesReady {
			s.State = fmt.Sprintf("NodesNotReady (%d/%d)", agg.readyNodes, max(agg.totalNodes, s.NodesPerSlice))
		}
		s.Ready = s.Ready && nodesReady
	}
}

func updateEnrichedSliceChipsAndMachine(s *SliceInfo, nodeMachineType string, nodeChips int, hw TPUHardwareResolver) {
	s.ChipsPerNode = resolveEnrichedSliceChips(s, nodeMachineType, nodeChips, hw)
	if nodeMachineType != "" || s.ChipsPerNode <= 0 || s.TPUType == "" {
		return
	}
	curMachineChips := hw.ChipsPerNode(s.MachineType)
	if s.MachineType != "" && (curMachineChips == 0 || curMachineChips == s.ChipsPerNode) {
		return
	}
	if inferred := hw.InferTPUMachine(s.TPUType, "", s.ChipsPerNode); inferred != "" {
		s.MachineType = inferred
	}
}

func resolveEnrichedSliceChips(s *SliceInfo, nodeMachineType string, nodeChips int, hw TPUHardwareResolver) int {
	if nodeChips > 0 {
		return nodeChips
	}
	if nodeMachineType != "" {
		if c := hw.ChipsPerNode(nodeMachineType); c > 0 {
			return c
		}
		if isSingleChipTopology(s.Topology) {
			return 1
		}
		return 0
	}
	if isSingleChipTopology(s.Topology) {
		return 1
	}
	if s.ChipsPerNode == 0 && s.MachineType != "" {
		return hw.ChipsPerNode(s.MachineType)
	}
	return s.ChipsPerNode
}

func resolveSliceNodeCount(topology string, chipsPerNode, observedNodes int) int {
	if n := calculateNodesFromTopology(topology, chipsPerNode); n > 0 {
		return n
	}
	if observedNodes > 0 {
		return observedNodes
	}
	return 0
}

func sliceNodesReady(readyNodes, totalNodes, nodesPerSlice int) bool {
	if totalNodes <= 0 || readyNodes != totalNodes {
		return false
	}
	if nodesPerSlice > 0 && readyNodes < nodesPerSlice {
		return false
	}
	return true
}

func buildTPUInventory(inv *Inventory, signals nodeSignals, hw TPUHardwareResolver) (*TPUInventory, *PoolCapacity) {
	primaryPool, primaryCtrl := selectPrimaryTPUPool(inv, signals)
	if primaryPool == nil && len(inv.Slices) == 0 && len(inv.PartitionTrees) == 0 {
		return nil, nil
	}

	tpu := &TPUInventory{HasPartitionLabels: signals.hasPartitionLabels}
	if primaryPool != nil {
		tpu.TPUType = primaryPool.TPUType
		tpu.MachineType = primaryPool.MachineType
		tpu.ChipsPerNode = primaryPool.TPUsPerNode
		if tpu.ChipsPerNode == 0 && tpu.MachineType != "" {
			tpu.ChipsPerNode = hw.ChipsPerNode(tpu.MachineType)
		}
	}
	poolTopos := accumulatePrimaryTPUPools(inv, signals, primaryCtrl, tpu, hw)
	applySliceAndTreeMetadata(inv, primaryPool, poolTopos, tpu, hw)
	if tpu.ChipsPerNode == 0 && tpu.MachineType != "" {
		tpu.ChipsPerNode = hw.ChipsPerNode(tpu.MachineType)
	}
	if tpu.NodesPerSlice == 0 {
		tpu.NodesPerSlice = calculateNodesFromTopology(tpu.Topology, tpu.ChipsPerNode)
	}
	return tpu, primaryPool
}

func accumulatePrimaryTPUPools(inv *Inventory, signals nodeSignals, primaryCtrl bool, tpu *TPUInventory, hw TPUHardwareResolver) []string {
	var poolTopos []string
	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if !pool.IsTPU {
			continue
		}
		if !primaryCtrl && signals.poolControlPlane[name] {
			continue
		}
		if !poolMatchesPrimaryTPU(pool, tpu, hw) {
			continue
		}
		tpu.TotalTPUNodes += pool.TotalNodes
		tpu.ReadyTPUNodes += pool.ReadyNodes
		poolTopos = append(poolTopos, pool.Topologies...)
	}
	return poolTopos
}

func poolMatchesPrimaryTPU(pool PoolCapacity, tpu *TPUInventory, hw TPUHardwareResolver) bool {
	if !poolTPUTypeMatches(pool, tpu) {
		return false
	}
	if tpu.MachineType != "" && pool.MachineType != "" && pool.MachineType != tpu.MachineType {
		return false
	}
	poolChips := pool.TPUsPerNode
	if poolChips == 0 {
		poolChips = hw.ChipsPerNode(pool.MachineType)
	}
	return tpu.ChipsPerNode == 0 || poolChips == 0 || poolChips == tpu.ChipsPerNode
}

func poolTPUTypeMatches(pool PoolCapacity, tpu *TPUInventory) bool {
	if tpu.TPUType == "" {
		return true
	}
	if pool.TPUType == "" {
		return tpu.MachineType != "" && pool.MachineType == tpu.MachineType
	}
	return pool.TPUType == tpu.TPUType
}

func applySliceAndTreeMetadata(inv *Inventory, primaryPool *PoolCapacity, poolTopos []string, tpu *TPUInventory, hw TPUHardwareResolver) {
	applyDefaultSlice(inv, tpu)
	backfillTPUFromPartitionTrees(inv, tpu)
	tpu.Topologies = collectAllTopologies(inv, poolTopos, tpu)
	if tpu.DefaultSliceName == "" && tpu.Topology == "" {
		tpu.Topology = pickFallbackTopology(inv, primaryPool, tpu)
	}
	if tpu.MachineType == "" && tpu.TPUType != "" {
		chipsHint := tpu.ChipsPerNode
		if chipsHint == 0 && isSingleChipTopology(tpu.Topology) {
			chipsHint = 1
		}
		tpu.MachineType = hw.InferTPUMachine(tpu.TPUType, "", chipsHint)
	}
}

func backfillTPUFromPartitionTrees(inv *Inventory, tpu *TPUInventory) {
	if tpu.TPUType != "" {
		return
	}
	for _, pt := range inv.PartitionTrees {
		if pt.TPUType != "" {
			tpu.TPUType = pt.TPUType
			return
		}
	}
}

func pickFallbackTopology(inv *Inventory, primaryPool *PoolCapacity, tpu *TPUInventory) string {
	if primaryPool != nil && len(primaryPool.Topologies) > 0 {
		return primaryPool.Topologies[0]
	}
	if topo := firstMatchingPartitionTreeRoot(inv.PartitionTrees, tpu.TPUType, true); topo != "" {
		return topo
	}
	if topo := firstMatchingPartitionTreeRoot(inv.PartitionTrees, tpu.TPUType, false); topo != "" {
		return topo
	}
	if len(tpu.Topologies) > 0 {
		return tpu.Topologies[0]
	}
	return ""
}

func firstMatchingPartitionTreeRoot(trees []PartitionTreeInfo, tpuType string, requireTyped bool) string {
	for _, pt := range trees {
		if pt.RootTopology == "" {
			continue
		}
		if requireTyped {
			if pt.TPUType != "" && tpuTypesCompatible(tpuType, pt.TPUType) {
				return pt.RootTopology
			}
			continue
		}
		if pt.TPUType == "" {
			return pt.RootTopology
		}
	}
	return ""
}

func collectAllTopologies(inv *Inventory, poolTopos []string, tpu *TPUInventory) []string {
	allTopos := append([]string(nil), poolTopos...)
	for _, s := range inv.Slices {
		if s.Topology != "" && sliceMatchesPrimaryTPU(s, tpu) {
			allTopos = append(allTopos, s.Topology)
		}
	}
	for _, pt := range inv.PartitionTrees {
		if tpuTypesCompatible(tpu.TPUType, pt.TPUType) {
			allTopos = append(allTopos, pt.SupportedTopologies...)
		}
	}
	return normalizeUniqueSorted(allTopos)
}

func tpuTypesCompatible(primary, cand string) bool {
	return primary == "" || cand == "" || primary == cand
}

func sliceMatchesPrimaryTPU(s SliceInfo, tpu *TPUInventory) bool {
	if !tpuTypesCompatible(tpu.TPUType, s.TPUType) {
		return false
	}
	if tpu.MachineType != "" && s.MachineType != "" && s.MachineType != tpu.MachineType {
		return false
	}
	if tpu.ChipsPerNode > 0 && s.ChipsPerNode > 0 && s.ChipsPerNode != tpu.ChipsPerNode {
		return false
	}
	return sliceTopologyFitsHostChips(s, tpu)
}

func sliceTopologyFitsHostChips(s SliceInfo, tpu *TPUInventory) bool {
	if tpu.ChipsPerNode <= 0 || s.ChipsPerNode > 0 || s.Topology == "" {
		return true
	}
	totalChips, ok := parseTopologyChipCount(s.Topology)
	return !ok || (totalChips%tpu.ChipsPerNode == 0 && (totalChips == tpu.ChipsPerNode || tpu.ChipsPerNode == 4))
}

func applyDefaultSlice(inv *Inventory, tpu *TPUInventory) {
	defSlice := selectDefaultSlice(inv.Slices, tpu)
	if defSlice == nil {
		return
	}
	tpu.TPUType = firstNonEmpty(tpu.TPUType, defSlice.TPUType)
	tpu.MachineType = firstNonEmpty(tpu.MachineType, defSlice.MachineType)
	if tpu.ChipsPerNode == 0 {
		tpu.ChipsPerNode = defSlice.ChipsPerNode
	}
	if defSlice.Ready {
		tpu.DefaultSliceName = defSlice.Name
		if defSlice.Topology != "" {
			tpu.Topology = defSlice.Topology
		}
		if defSlice.NodesPerSlice > 0 {
			tpu.NodesPerSlice = defSlice.NodesPerSlice
		}
	}
}

func selectPrimaryTPUPool(inv *Inventory, signals nodeSignals) (*PoolCapacity, bool) {
	var best *PoolCapacity
	bestName := ""
	bestCtrl := true
	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if !pool.IsTPU {
			continue
		}
		isCtrl := signals.poolControlPlane[name]
		if isBetterTPUPool(name, pool, isCtrl, bestName, best, bestCtrl) {
			p := pool
			best = &p
			bestName = name
			bestCtrl = isCtrl
		}
	}
	return best, bestCtrl
}

func isBetterTPUPool(candName string, cand PoolCapacity, candCtrl bool, curName string, cur *PoolCapacity, curCtrl bool) bool {
	if cur == nil {
		return true
	}
	if candCtrl != curCtrl {
		return !candCtrl
	}
	if cand.ReadyNodes != cur.ReadyNodes {
		return cand.ReadyNodes > cur.ReadyNodes
	}
	if cand.TotalNodes != cur.TotalNodes {
		return cand.TotalNodes > cur.TotalNodes
	}
	return candName < curName
}

func selectDefaultSlice(sliceList []SliceInfo, tpu *TPUInventory) *SliceInfo {
	if len(sliceList) == 0 {
		return nil
	}
	for _, wantReady := range []bool{true, false} {
		for i := range sliceList {
			s := &sliceList[i]
			if wantReady && !s.Ready {
				continue
			}
			if sliceMatchesPrimaryTPU(*s, tpu) {
				return s
			}
		}
	}
	return nil
}

func sortSlices(sliceList []SliceInfo) {
	slices.SortFunc(sliceList, func(a, b SliceInfo) int {
		if c := strings.Compare(a.Name, b.Name); c != 0 {
			return c
		}
		return strings.Compare(a.Namespace, b.Namespace)
	})
}

func nestedString(obj map[string]interface{}, fields ...string) string {
	val, found, err := unstructured.NestedString(obj, fields...)
	if err != nil || !found {
		return ""
	}
	return strings.TrimSpace(val)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func minNonEmpty(cur, cand string) string {
	cand = strings.TrimSpace(cand)
	if cand == "" {
		return cur
	}
	if cur == "" || cand < cur {
		return cand
	}
	return cur
}

func minPositive[T ~int | ~int64](cur, cand T) T {
	if cand <= 0 {
		return cur
	}
	if cur <= 0 || cand < cur {
		return cand
	}
	return cur
}

func normalizeUniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(v)); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return slices.Sorted(maps.Keys(set))
}
