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
	"errors"
	"fmt"
	"maps"
	"math"
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
	TPUSliceManagedByLabel       = "tpu.google.com/managed-by"
	TPUSliceOwnerNameLabel       = "tpu.google.com/owner-name"
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

	gkeSystemTaintKey = "components.gke.io/gke-managed-components"
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
		return "", fmt.Errorf("invalid %s value %q: allowed values are tpu-slice, gke, generic", LabelSchemeEnvVar, raw)
	}
}

// ResolveLabelSchemeOverride resolves an explicit label scheme override from flagValue,
// falling back to GCLUSTER_LABEL_SCHEME via getenv (or os.Getenv when nil).
// A non-empty flagValue takes precedence over the environment variable.
func ResolveLabelSchemeOverride(flagValue string, getenv func(string) string) (LabelScheme, error) {
	if strings.TrimSpace(flagValue) != "" {
		return ParseLabelScheme(flagValue)
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	return ParseLabelScheme(getenv(LabelSchemeEnvVar))
}

func resolveTPUType(hw TPUHardwareResolver, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if hw != nil {
		return hw.NormalizeTPUType(trimmed)
	}
	return strings.ToLower(trimmed)
}

func isTPUMachine(hw TPUHardwareResolver, raw string) bool {
	if hw == nil || strings.TrimSpace(raw) == "" {
		return false
	}
	return hw.IsTPUMachine(raw)
}

func inferTPUMachine(hw TPUHardwareResolver, tpuType, rawMachine string, chipsPerNode int) string {
	if hw != nil {
		return hw.InferTPUMachine(tpuType, rawMachine, chipsPerNode)
	}
	return strings.TrimSpace(rawMachine)
}

func chipsPerNodeForMachine(hw TPUHardwareResolver, machineType string) int {
	if hw == nil || strings.TrimSpace(machineType) == "" {
		return 0
	}
	return hw.ChipsPerNode(machineType)
}

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

func isNotFoundOrNoMatchError(err error) bool {
	if err == nil {
		return false
	}
	if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "could not find the requested resource") ||
		strings.Contains(msg, "doesn't have a resource type") ||
		strings.Contains(msg, "no matches for kind")
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
	lines := []string{
		fmt.Sprintf("Kubernetes explicit mode active (label scheme: %s, TPU slice API present: %t).", inv.LabelScheme, inv.TPUSliceAPIPresent),
	}
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
	header := tpuSummaryHeader(inv, tpu)
	var details []string
	if tpu.TPUType != "" {
		details = append(details, "type="+tpu.TPUType)
	}
	if tpu.MachineType != "" {
		details = append(details, "machine="+tpu.MachineType)
	}
	if cap != nil && cap.NodePoolName != "" && cap.NodePoolName != tpu.MachineType {
		details = append(details, "pool="+cap.NodePoolName)
	}
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
	if len(details) == 0 {
		return header + "."
	}
	return fmt.Sprintf("%s (%s).", header, strings.Join(details, ", "))
}

func tpuSummaryHeader(inv *Inventory, tpu *TPUInventory) string {
	if tpu.DefaultSliceName != "" {
		return "Default slice: " + tpu.DefaultSliceName
	}
	if inv.SliceInventoryUnavailable {
		return "TPU pool: slice inventory unavailable"
	}
	if slices.ContainsFunc(inv.Slices, func(s SliceInfo) bool { return sliceMatchesPrimaryTPU(s, tpu) }) {
		return "TPU pool: no ready slice"
	}
	return "TPU pool: no active slice"
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
	out.trees, out.treeErr = listTPUPartitionTrees(lister, hw)
	return out, nil
}

func checkAutoSchemeFailClosed(override LabelScheme, inv *Inventory, signals nodeSignals, apiChecker APIGroupChecker, probeErr, sliceErr error) error {
	if override != SchemeAuto || signals.hasTPUSliceLabels || signals.hasGKELabels || inv.TPUSliceAPIPresent {
		return nil
	}
	if apiChecker != nil && probeErr == nil {
		return nil
	}
	if sliceErr == nil || isNotFoundOrNoMatchError(sliceErr) {
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
	if sliceErr != nil && !isNotFoundOrNoMatchError(sliceErr) {
		inv.SliceInventoryUnavailable = true
		if IsForbiddenError(sliceErr) {
			logging.Info("Skipping TPU Slice inventory (lacking permission to list slices).")
		} else {
			logging.Warn("Failed to list TPU Slice resources: %v", sliceErr)
		}
	}
	if treeErr != nil && !isNotFoundOrNoMatchError(treeErr) {
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
		poolSystem:       make(map[string]bool),
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
	cpuMilli, memMiB, tpus, gpus, inferredGPUType := parseNodeAllocatable(node)
	hasTPUSlice, hasPartition := hasTPUSliceLabels(labels)
	hasGKE := hasGKELabels(labels)

	rawInstance := firstNonEmpty(labels[InstanceTypeLabel], labels[BetaInstanceTypeLabel])
	rawTPUType, topology := pickNodeTPULabels(labels, override)
	sliceName := strings.TrimSpace(labels[TPUSliceNameLabel])
	nodePool := strings.TrimSpace(labels[GKENodePoolLabel])
	if override == SchemeGKE || override == SchemeGeneric {
		sliceName = ""
		hasPartition = false
	}

	isTPU := tpus > 0 || rawTPUType != "" || topology != "" || sliceName != "" || isTPUMachine(hw, rawInstance)
	tpuType, machineType := resolveNodeMachine(hw, isTPU, rawTPUType, rawInstance, tpus)

	gpuType := firstNonEmpty(labels[GKEAcceleratorLabel], labels[NvidiaGPUProductLabel], inferredGPUType)
	isGPU := !isTPU && (gpus > 0 || gpuType != "")
	poolKey := firstNonEmpty(machineType, nodePool, fallbackClassPoolKey(isTPU, isGPU))

	return nodeSnapshot{
		ready:        isNodeReady(node),
		cpuMilli:     cpuMilli,
		memMiB:       memMiB,
		tpus:         tpus,
		gpus:         gpus,
		isTPU:        isTPU,
		isGPU:        isGPU,
		tpuType:      tpuType,
		gpuType:      gpuType,
		machineType:  machineType,
		poolKey:      poolKey,
		nodePool:     nodePool,
		topology:     topology,
		sliceName:    sliceName,
		hasTPUSlice:  hasTPUSlice,
		hasGKE:       hasGKE,
		hasPartition: hasPartition,
		controlPlane: isControlPlaneNode(labels),
		systemPool:   isSystemPoolNode(node, nodePool),
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
	tpuType = resolveTPUType(hw, rawTPUType)
	if tpuType == "" && isTPUMachine(hw, rawInstance) {
		tpuType = resolveTPUType(hw, rawInstance)
	}
	if rawInstance != "" || tpus > 0 {
		machineType = inferTPUMachine(hw, tpuType, rawInstance, tpus)
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
		poolKey:      snap.poolKey,
		nodePool:     snap.nodePool,
		class:        nodePoolClass(snap),
		accelType:    accelType,
		controlPlane: snap.controlPlane,
		systemPool:   snap.systemPool,
	}
	if b, ok := builders[key]; ok {
		return b
	}
	b := &poolBuilder{
		key: key,
		cap: PoolCapacity{
			NodePoolName: snap.nodePool,
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
	b.allCPUMilli = minPositiveInt64(b.allCPUMilli, snap.cpuMilli)
	b.allMemMiB = minPositive(b.allMemMiB, snap.memMiB)
	b.allTPUs = minPositive(b.allTPUs, snap.tpus)
	b.allGPUs = minPositive(b.allGPUs, snap.gpus)

	if snap.ready {
		b.cap.ReadyNodes++
		b.readyMachineType = minNonEmpty(b.readyMachineType, snap.machineType)
		b.readyTPUType = minNonEmpty(b.readyTPUType, snap.tpuType)
		b.readyGPUType = minNonEmpty(b.readyGPUType, snap.gpuType)
		b.readyCPUMilli = minPositiveInt64(b.readyCPUMilli, snap.cpuMilli)
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
	if c.ReadyNodes > 0 {
		c.MachineType = firstNonEmpty(b.readyMachineType, b.allMachineType)
		c.TPUType = firstNonEmpty(b.readyTPUType, b.allTPUType)
		c.GPUType = firstNonEmpty(b.readyGPUType, b.allGPUType)
		c.AllocatableCPUMilli = preferPositiveInt64(b.readyCPUMilli, b.allCPUMilli)
		c.MemMiBPerNode = preferPositive(b.readyMemMiB, b.allMemMiB)
		c.TPUsPerNode = preferPositive(b.readyTPUs, b.allTPUs)
		c.GPUsPerNode = preferPositive(b.readyGPUs, b.allGPUs)
	} else {
		c.MachineType = b.allMachineType
		c.TPUType = b.allTPUType
		c.GPUType = b.allGPUType
		c.AllocatableCPUMilli = b.allCPUMilli
		c.MemMiBPerNode = b.allMemMiB
		c.TPUsPerNode = b.allTPUs
		c.GPUsPerNode = b.allGPUs
	}
	c.CPUsPerNode = int(c.AllocatableCPUMilli / 1000)
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
	for _, c := range []int{
		strings.Compare(a.poolKey, b.poolKey),
		strings.Compare(a.nodePool, b.nodePool),
		strings.Compare(a.class, b.class),
		strings.Compare(a.accelType, b.accelType),
		compareBool(a.controlPlane, b.controlPlane),
		compareBool(a.systemPool, b.systemPool),
	} {
		if c != 0 {
			return c
		}
	}
	return 0
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
		signals.poolSystem[name] = b.key.systemPool
	}
	inv.CPUPoolCap = selectCPUPoolCap(inv, signals)
}

func selectCPUPoolCap(inv *Inventory, signals *nodeSignals) *PoolCapacity {
	var best *PoolCapacity
	bestName := ""
	bestCtrl := true
	bestSys := true

	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if pool.IsTPU || pool.IsGPU {
			continue
		}
		isCtrl := signals.poolControlPlane[name]
		isSys := signals.poolSystem[name]
		if isBetterCPUPool(name, pool, isCtrl, isSys, bestName, best, bestCtrl, bestSys) {
			p := pool
			best = &p
			bestName = name
			bestCtrl = isCtrl
			bestSys = isSys
		}
	}
	return best
}

func isBetterCPUPool(candName string, cand PoolCapacity, candCtrl, candSys bool, curName string, cur *PoolCapacity, curCtrl, curSys bool) bool {
	if cur == nil {
		return true
	}
	if candCtrl != curCtrl {
		return !candCtrl
	}
	if candSys != curSys {
		return !candSys
	}
	if (cand.ReadyNodes > 0) != (cur.ReadyNodes > 0) {
		return cand.ReadyNodes > 0
	}
	if cand.AllocatableCPUMilli != cur.AllocatableCPUMilli {
		return cand.AllocatableCPUMilli > cur.AllocatableCPUMilli
	}
	return candName < curName
}

func parseNodeAllocatable(node unstructured.Unstructured) (cpuMilli int64, memMiB, tpus, gpus int, inferredGPU string) {
	alloc, found, err := unstructured.NestedMap(node.Object, "status", "allocatable")
	if err != nil || !found {
		return 0, 0, 0, 0, ""
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
	gpus, inferredGPU = parseAllocatableGPUs(alloc)
	return cpuMilli, memMiB, tpus, gpus, inferredGPU
}

func parseAllocatableGPUs(alloc map[string]interface{}) (int, string) {
	if q, ok := parseQuantityField(alloc, "nvidia.com/gpu"); ok && q.Value() > 0 {
		return int(q.Value()), "nvidia"
	}
	if count, profile := parsePrefixGPUAllocatable(alloc, "nvidia.com/mig-"); count > 0 {
		return count, profile
	}
	if q, ok := parseQuantityField(alloc, "amd.com/gpu"); ok && q.Value() > 0 {
		return int(q.Value()), "amd"
	}
	if count, profile := parseIntelGPUAllocatable(alloc); count > 0 {
		return count, profile
	}
	return 0, ""
}

func parseIntelGPUAllocatable(alloc map[string]interface{}) (int, string) {
	for _, key := range []string{"gpu.intel.com/i915", "gpu.intel.com/xe"} {
		if q, ok := parseQuantityField(alloc, key); ok && q.Value() > 0 {
			return int(q.Value()), key
		}
	}
	keys := make([]string, 0)
	for k := range alloc {
		if !strings.HasPrefix(k, "gpu.intel.com/") || k == "gpu.intel.com/millicores" || strings.HasSuffix(k, "_monitoring") {
			continue
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return sumGPUAllocatableKeys(alloc, keys)
}

func parsePrefixGPUAllocatable(alloc map[string]interface{}, prefix string) (int, string) {
	keys := make([]string, 0)
	for k := range alloc {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return sumGPUAllocatableKeys(alloc, keys)
}

func sumGPUAllocatableKeys(alloc map[string]interface{}, keys []string) (int, string) {
	total := 0
	firstProfile := ""
	for _, k := range keys {
		if q, ok := parseQuantityField(alloc, k); ok && q.Value() > 0 {
			total += int(q.Value())
			if firstProfile == "" {
				firstProfile = k
			}
		}
	}
	return total, firstProfile
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
		if (key == gkeSystemTaintKey || key == "CriticalAddonsOnly") && (val == "" || val == "true") {
			return true
		}
	}
	return false
}

// listTPUSlices queries slice.tpu.google.com/v1alpha1 (spec.{type,topology,partitionIds} and
// status.conditions Ready) and falls back to v1beta1 strictly on NotFound/NoMatch.
func listTPUSlices(lister ResourceLister, hw TPUHardwareResolver) ([]SliceInfo, bool, error) {
	items, err := lister.ListResources(TPUSliceGVR, "", "", "")
	if err != nil && isNotFoundOrNoMatchError(err) {
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
	partitionIDs, _, _ := unstructured.NestedStringSlice(obj.Object, "spec", "partitionIds")

	chipsHint := 0
	if isSingleChipTopology(topology) {
		chipsHint = 1
	}
	tpuType := resolveTPUType(hw, rawType)
	machineType := inferTPUMachine(hw, tpuType, "", chipsHint)
	chips := chipsPerNodeForMachine(hw, machineType)
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
		OwnerName:     strings.TrimSpace(labels[TPUSliceOwnerNameLabel]),
		ManagedBy:     strings.TrimSpace(labels[TPUSliceManagedByLabel]),
		PartitionIDs:  partitionIDs,
		FromCRD:       true,
	}
}

// listTPUPartitionTrees queries partition.tpu.google.com/v1alpha1 and falls back to v1beta1
// strictly on NotFound/NoMatch.
func listTPUPartitionTrees(lister ResourceLister, hw TPUHardwareResolver) ([]PartitionTreeInfo, error) {
	items, err := lister.ListResources(TPUPartitionTreeGVR, "", "", "")
	if err != nil && isNotFoundOrNoMatchError(err) {
		items, err = lister.ListResources(TPUPartitionTreeV1Beta1GVR, "", "", "")
	}
	if err != nil {
		return nil, err
	}
	result := make([]PartitionTreeInfo, 0, len(items))
	for i := range items {
		result = append(result, parsePartitionTree(items[i], hw))
	}
	slices.SortFunc(result, comparePartitionTrees)
	return result, nil
}

func comparePartitionTrees(a, b PartitionTreeInfo) int {
	for _, c := range []int{
		strings.Compare(a.Name, b.Name),
		strings.Compare(a.TPUType, b.TPUType),
		strings.Compare(a.RootTopology, b.RootTopology),
	} {
		if c != 0 {
			return c
		}
	}
	return 0
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

	rawType := firstNonEmpty(
		nestedString(obj.Object, "status", "acceleratorType"),
		labels[TPUSliceTypeLabel],
		inferTypeFromPartitionTypeInfo(nestedString(rootNode, "partitionTypeInfo")),
	)
	rootTopo := extractNodeTopology(rootNode)
	var allTopos []string
	collectPartitionNodeTopologies(rootNode, &allTopos)

	return PartitionTreeInfo{
		Name:                obj.GetName(),
		TPUType:             resolveTPUType(hw, rawType),
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
	if topo := extractTopologyFromPartitionTypeInfo(nestedString(node, "partitionTypeInfo")); topo != "" {
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

// extractTopologyFromPartitionTypeInfo parses trailing dimension tokens from v1alpha1
// partitionTypeInfo strings such as "IRONWOOD_4_4_4" -> "4x4x4".
func extractTopologyFromPartitionTypeInfo(info string) string {
	_, topo := splitPartitionTypeInfo(info)
	return topo
}

func inferTypeFromPartitionTypeInfo(info string) string {
	prefix, _ := splitPartitionTypeInfo(info)
	return prefix
}

// extractTopologyFromPartitionID extracts a "<d1>x<d2>[x<d3>]" token from partition IDs
// such as "fin-pod10-r1-4x4x4-1" -> "4x4x4".
func extractTopologyFromPartitionID(id string) string {
	for _, tok := range strings.Split(strings.ToLower(strings.TrimSpace(id)), "-") {
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
	names := make([]string, 0, len(nodeSlices))
	for name := range nodeSlices {
		names = append(names, name)
	}
	slices.Sort(names)

	for _, name := range names {
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
	chips = preferPositive(agg.readyChips, agg.chips)
	return tpuType, machineType, topology, chips
}

func buildNodeDerivedSlice(agg *nodeSliceAgg, hw TPUHardwareResolver) SliceInfo {
	tpuType, machineType, topology, chips := preferredSliceAggValues(agg)
	if chips == 0 && machineType != "" {
		chips = chipsPerNodeForMachine(hw, machineType)
	}
	if chips == 0 && isSingleChipTopology(topology) {
		chips = 1
	}
	if machineType == "" && chips > 0 && tpuType != "" {
		machineType = inferTPUMachine(hw, tpuType, "", chips)
	}
	nodesPerSlice := resolveSliceNodeCount(topology, chips, agg.totalNodes)
	ready := sliceNodesReady(agg.readyNodes, agg.totalNodes, nodesPerSlice)
	return SliceInfo{
		Name:          agg.name,
		TPUType:       tpuType,
		MachineType:   machineType,
		Topology:      topology,
		Ready:         ready,
		NodesPerSlice: nodesPerSlice,
		ChipsPerNode:  chips,
		FromCRD:       false,
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
		s.Ready = s.Ready && sliceNodesReady(agg.readyNodes, agg.totalNodes, s.NodesPerSlice)
	}
}

func updateEnrichedSliceChipsAndMachine(s *SliceInfo, nodeMachineType string, nodeChips int, hw TPUHardwareResolver) {
	s.ChipsPerNode = resolveEnrichedSliceChips(s, nodeMachineType, nodeChips, hw)
	if nodeMachineType != "" || s.ChipsPerNode <= 0 || s.TPUType == "" {
		return
	}
	curMachineChips := chipsPerNodeForMachine(hw, s.MachineType)
	if s.MachineType != "" && (curMachineChips == 0 || curMachineChips == s.ChipsPerNode) {
		return
	}
	if inferred := inferTPUMachine(hw, s.TPUType, "", s.ChipsPerNode); inferred != "" {
		s.MachineType = inferred
	}
}

func resolveEnrichedSliceChips(s *SliceInfo, nodeMachineType string, nodeChips int, hw TPUHardwareResolver) int {
	if nodeChips > 0 {
		return nodeChips
	}
	if nodeMachineType != "" {
		if c := chipsPerNodeForMachine(hw, nodeMachineType); c > 0 {
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
		return chipsPerNodeForMachine(hw, s.MachineType)
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
			tpu.ChipsPerNode = chipsPerNodeForMachine(hw, tpu.MachineType)
		}
	}
	poolTopos := accumulatePrimaryTPUPools(inv, signals, primaryCtrl, tpu, hw)
	applySliceAndTreeMetadata(inv, primaryPool, poolTopos, tpu, hw)
	if tpu.ChipsPerNode == 0 && tpu.MachineType != "" {
		tpu.ChipsPerNode = chipsPerNodeForMachine(hw, tpu.MachineType)
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
		poolChips = chipsPerNodeForMachine(hw, pool.MachineType)
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
	tpu.Topologies = collectAllTopologies(inv, poolTopos, tpu, hw)
	if tpu.DefaultSliceName == "" && tpu.Topology == "" {
		tpu.Topology = pickFallbackTopology(inv, primaryPool, tpu)
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

func collectAllTopologies(inv *Inventory, poolTopos []string, tpu *TPUInventory, hw TPUHardwareResolver) []string {
	allTopos := append([]string(nil), poolTopos...)
	for _, s := range inv.Slices {
		if s.Topology != "" && sliceMatchesPrimaryTPU(s, tpu) {
			allTopos = append(allTopos, s.Topology)
		}
	}
	for _, pt := range inv.PartitionTrees {
		if tpu.TPUType == "" && pt.TPUType != "" {
			tpu.TPUType = pt.TPUType
			if tpu.MachineType == "" {
				tpu.MachineType = inferTPUMachine(hw, tpu.TPUType, "", tpu.ChipsPerNode)
			}
		}
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
	return true
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
	if (cand.ReadyNodes > 0) != (cur.ReadyNodes > 0) {
		return cand.ReadyNodes > 0
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

func minPositive(cur, cand int) int {
	if cand <= 0 {
		return cur
	}
	if cur <= 0 || cand < cur {
		return cand
	}
	return cur
}

func minPositiveInt64(cur, cand int64) int64 {
	if cand <= 0 {
		return cur
	}
	if cur <= 0 || cand < cur {
		return cand
	}
	return cur
}

func preferPositive(primary, fallback int) int {
	if primary > 0 {
		return primary
	}
	return fallback
}

func preferPositiveInt64(primary, fallback int64) int64 {
	if primary > 0 {
		return primary
	}
	return fallback
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
