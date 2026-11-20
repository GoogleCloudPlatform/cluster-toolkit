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
	"os"
	"slices"
	"strconv"
	"strings"

	"hpc-toolkit/pkg/config"
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

	InstanceTypeLabel     = "node.kubernetes.io/instance-type"
	BetaInstanceTypeLabel = "beta.kubernetes.io/instance-type"
	ControlPlaneRoleLabel = "node-role.kubernetes.io/control-plane"
	MasterRoleLabel       = "node-role.kubernetes.io/master"
)

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
		return "", fmt.Errorf("invalid --k8s-label-scheme / %s value %q: allowed values are tpu-slice, gke, generic", LabelSchemeEnvVar, raw)
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

var tpuTypeMatchers = []struct {
	canonical string
	tokens    []string
}{
	{canonical: "tpu7x", tokens: []string{"tpu7", "tpu-7x", "v7x", "ironwood"}},
	{canonical: "v6e", tokens: []string{"ct6e", "v6e"}},
	{canonical: "v5p", tokens: []string{"ct5p", "v5p"}},
	{canonical: "v5e", tokens: []string{"ct5lp", "v5e", "v5lite", "v5-lite"}},
	{canonical: "v4", tokens: []string{"ct4p", "tpu-v4"}},
}

// NormalizeTPUType normalizes GCE machine types, GKE accelerator labels, and TPU shorthands
// into canonical short family names ("v5p", "v5e", "v6e", "tpu7x", "v4").
// Unknown non-empty inputs are returned lowercased and trimmed.
func NormalizeTPUType(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	for _, m := range tpuTypeMatchers {
		if containsAny(s, m.tokens) {
			return m.canonical
		}
	}
	if s == "v4" || strings.HasPrefix(s, "v4-") {
		return "v4"
	}
	return s
}

func containsAny(s string, tokens []string) bool {
	for _, tok := range tokens {
		if strings.Contains(s, tok) {
			return true
		}
	}
	return false
}

// InferTPUTypeAndMachine derives a canonical TPU type ("v5p", "v5e", "v6e", "tpu7x", "v4")
// and a resolved machine type (e.g. "ct5p-hightpu-4t") from node/slice labels and per-node chip count.
func InferTPUTypeAndMachine(rawType, rawMachine string, chipsPerNode int) (string, string) {
	trimmedType := strings.TrimSpace(rawType)
	trimmedMachine := strings.TrimSpace(rawMachine)

	tpuType := NormalizeTPUType(trimmedType)
	if tpuType == "" {
		tpuType = NormalizeTPUType(trimmedMachine)
	}

	if trimmedMachine != "" && !isTPUShorthandOrAccelerator(trimmedMachine) {
		return tpuType, trimmedMachine
	}
	if inferred := defaultTPUMachineType(tpuType, chipsPerNode); inferred != "" {
		return tpuType, inferred
	}
	return tpuType, trimmedMachine
}

func isTPUShorthandOrAccelerator(raw string) bool {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch s {
	case "tpu7", "tpu7x", "tpu-7x", "v7x", "ironwood", "v4", "v5e", "v5p", "v6e", "v5litepod":
		return true
	}
	for _, prefix := range []string{"tpu-v4", "tpu-v5", "tpu-v6", "tpu-v7", "tpu-7x", "v4-", "v5e-", "v5p-", "v6e-", "v5litepod-", "v7x-"} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func defaultTPUMachineType(tpuType string, chipsPerNode int) string {
	chips := chipsPerNode
	if chips <= 0 {
		chips = 4
	}
	switch tpuType {
	case "tpu7x":
		return fmt.Sprintf("tpu7x-standard-%dt", normalizeTPUChipCount(chips))
	case "v6e":
		return fmt.Sprintf("ct6e-standard-%dt", normalizeTPUChipCount(chips))
	case "v5p":
		return "ct5p-hightpu-4t"
	case "v5e":
		return fmt.Sprintf("ct5lp-hightpu-%dt", normalizeTPUChipCount(chips))
	case "v4":
		return "ct4p-hightpu-4t"
	default:
		return ""
	}
}

func normalizeTPUChipCount(chips int) int {
	switch chips {
	case 1, 4, 8:
		return chips
	default:
		return 4
	}
}

// IsNotFoundOrNoMatchError reports whether err indicates that a Kubernetes CRD or resource type does not exist.
func IsNotFoundOrNoMatchError(err error) bool {
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
// API group discovery endpoint (/apis/slice.tpu.google.com/v1alpha1) without invoking gcloud.
// When listing nodes fails with 403 Forbidden (multi-tenant RBAC), it enters explicit mode cleanly and
// resolves the label scheme via API group discovery or opts.LabelSchemeOverride.
func DiscoverInventory(lister ResourceLister, apiChecker APIGroupChecker, opts DiscoverOptions) (*Inventory, error) {
	override, err := ParseLabelScheme(string(opts.LabelSchemeOverride))
	if err != nil {
		return nil, err
	}
	if lister == nil {
		return nil, fmt.Errorf("kubernetes resource lister is required for cluster inventory discovery")
	}

	inv := &Inventory{Pools: make(map[string]PoolCapacity)}

	signals, err := discoverNodes(lister, inv)
	if err != nil {
		return nil, err
	}

	probeFailed := probeTPUSliceAPIGroup(apiChecker, inv)
	discoverTPUSliceCRDs(lister, apiChecker, probeFailed, inv, signals, override)

	inv.LabelScheme = resolveLabelScheme(override, signals.hasTPUSliceLabels || inv.TPUSliceAPIPresent, signals.hasGKELabels)
	inv.TPU, inv.TPUPoolCap = buildTPUInventory(inv, signals.hasPartitionLabels)
	return inv, nil
}

// DefaultPoolCap returns the deterministic default node pool capacity for a TPU (isTPU=true) or CPU (isTPU=false) job.
func (inv *Inventory) DefaultPoolCap(isTPU bool) *PoolCapacity {
	if inv == nil {
		return nil
	}
	if isTPU {
		return inv.TPUPoolCap
	}
	return inv.CPUPoolCap
}

// SummaryLines returns deterministic, human-readable lines describing the discovered cluster inventory.
func (inv *Inventory) SummaryLines() []string {
	if inv == nil {
		return nil
	}
	if inv.ExplicitMode {
		lines := []string{
			fmt.Sprintf("Kubernetes explicit mode active (label scheme: %s, TPU slice API present: %t).", inv.LabelScheme, inv.TPUSliceAPIPresent),
		}
		if inv.TPU != nil {
			lines = append(lines, formatTPUSummaryLine(inv.TPU, inv.TPUPoolCap))
		}
		return lines
	}
	lines := []string{
		fmt.Sprintf("Discovered %d/%d ready Kubernetes node(s) across %d pool(s) (label scheme: %s).",
			inv.ReadyNodes, inv.TotalNodes, len(inv.PoolNames), inv.LabelScheme),
	}
	if inv.TPU != nil {
		lines = append(lines, formatTPUSummaryLine(inv.TPU, inv.TPUPoolCap))
	}
	if inv.CPUPoolCap != nil {
		c := inv.CPUPoolCap
		lines = append(lines, fmt.Sprintf("CPU pool: %s (%d/%d ready nodes, %d vCPU / %d MiB RAM per node).",
			c.MachineType, c.ReadyNodes, c.TotalNodes, c.CPUsPerNode, c.MemMiBPerNode))
	}
	return lines
}

// LogSummary emits deterministic INFO logs summarizing the discovered cluster inventory.
func (inv *Inventory) LogSummary() {
	for _, line := range inv.SummaryLines() {
		logging.Info("%s", line)
	}
}

func formatTPUSummaryLine(tpu *TPUInventory, cap *PoolCapacity) string {
	sliceLabel := tpu.DefaultSliceName
	if sliceLabel == "" {
		sliceLabel = "<none>"
	}
	topo := tpu.Topology
	if topo == "" {
		topo = "<unknown>"
	}
	if cap != nil {
		return fmt.Sprintf("Default slice: %s (type=%s, machine=%s, topology=%s, %d nodes/slice, %d TPU chips/node, %d vCPU / %d MiB RAM per node).",
			sliceLabel, tpu.TPUType, tpu.MachineType, topo, tpu.NodesPerSlice, tpu.ChipsPerNode, cap.CPUsPerNode, cap.MemMiBPerNode)
	}
	return fmt.Sprintf("Default slice: %s (type=%s, machine=%s, topology=%s, %d nodes/slice, %d TPU chips/node).",
		sliceLabel, tpu.TPUType, tpu.MachineType, topo, tpu.NodesPerSlice, tpu.ChipsPerNode)
}

func discoverNodes(lister ResourceLister, inv *Inventory) (nodeSignals, error) {
	nodes, err := lister.ListResources(NodeGVR, "", "", "")
	if err != nil {
		if IsForbiddenError(err) {
			inv.ExplicitMode = true
			logging.Info("Lacking permission to list cluster nodes (403 Forbidden); entering explicit mode and skipping node inventory.")
			return nodeSignals{nodeSlices: make(map[string]*nodeSliceAgg)}, nil
		}
		return nodeSignals{}, fmt.Errorf("failed to list cluster nodes: %w", err)
	}
	return accumulateNodes(nodes, inv), nil
}

func probeTPUSliceAPIGroup(apiChecker APIGroupChecker, inv *Inventory) bool {
	if apiChecker == nil {
		return false
	}
	present, err := apiChecker.HasAPIGroupVersion(TPUSliceAPIGroupVersion)
	if err != nil {
		logging.Warn("Failed to probe Kubernetes API group %s: %v", TPUSliceAPIGroupVersion, err)
		return true
	}
	if present {
		inv.TPUSliceAPIPresent = true
	}
	return false
}

func discoverTPUSliceCRDs(lister ResourceLister, apiChecker APIGroupChecker, probeFailed bool, inv *Inventory, signals nodeSignals, override LabelScheme) {
	if override == SchemeGKE || override == SchemeGeneric {
		return
	}
	shouldQuery := inv.TPUSliceAPIPresent || signals.hasTPUSliceLabels || override == SchemeTPUSlice
	if !shouldQuery && (apiChecker == nil || probeFailed) && !signals.hasGKELabels {
		shouldQuery = true
	}
	if shouldQuery {
		crdSlices, foundSlices := listTPUSlices(lister)
		if foundSlices {
			inv.TPUSliceAPIPresent = true
			inv.Slices = crdSlices
		}
		trees, foundTrees := listTPUPartitionTrees(lister)
		if foundTrees {
			inv.TPUSliceAPIPresent = true
			inv.PartitionTrees = trees
		}
	}
	mergeNodeDerivedSlices(inv, signals.nodeSlices)
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

func accumulateNodes(nodes []unstructured.Unstructured, inv *Inventory) nodeSignals {
	signals := nodeSignals{nodeSlices: make(map[string]*nodeSliceAgg)}
	builders := make(map[poolGroupKey]*poolBuilder)

	for i := range nodes {
		snap := extractNodeSnapshot(nodes[i])
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

	finalizePools(inv, builders)
	return signals
}

func extractNodeSnapshot(node unstructured.Unstructured) nodeSnapshot {
	labels := node.GetLabels()
	cpus, memMiB, tpus, gpus := parseNodeAllocatable(node)
	hasTPUSlice, hasPartition := hasTPUSliceLabels(labels)
	hasGKE := hasGKELabels(labels)

	rawInstance := firstNonEmpty(labels[InstanceTypeLabel], labels[BetaInstanceTypeLabel])
	rawTPUType := firstNonEmpty(labels[TPUSliceTypeLabel], labels[GKETPUAcceleratorLabel])
	topology := strings.ToLower(strings.TrimSpace(firstNonEmpty(labels[TPUSliceTopologyLabel], labels[GKETPUTopologyLabel])))
	sliceName := strings.TrimSpace(labels[TPUSliceNameLabel])
	nodePool := strings.TrimSpace(labels[GKENodePoolLabel])

	isTPU := tpus > 0 || rawTPUType != "" || topology != "" || sliceName != "" || config.IsTPU(rawInstance)
	tpuType := ""
	machineType := rawInstance
	if isTPU {
		tpuType, machineType = InferTPUTypeAndMachine(rawTPUType, rawInstance, tpus)
		if machineType == "" {
			machineType = firstNonEmpty(nodePool, "tpu")
		}
	} else if machineType == "" {
		machineType = firstNonEmpty(nodePool, "k8s-node")
	}

	gpuType := strings.TrimSpace(labels[GKEAcceleratorLabel])
	isGPU := !isTPU && (gpus > 0 || gpuType != "")

	return nodeSnapshot{
		labels:       labels,
		ready:        isNodeReady(node),
		cpus:         cpus,
		memMiB:       memMiB,
		tpus:         tpus,
		gpus:         gpus,
		isTPU:        isTPU,
		isGPU:        isGPU,
		tpuType:      tpuType,
		gpuType:      gpuType,
		machineType:  machineType,
		nodePool:     nodePool,
		topology:     topology,
		sliceName:    sliceName,
		hasTPUSlice:  hasTPUSlice,
		hasGKE:       hasGKE,
		hasPartition: hasPartition,
		controlPlane: isControlPlaneNode(labels),
	}
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
	key := poolGroupKey{
		machineType:  snap.machineType,
		class:        nodePoolClass(snap),
		controlPlane: snap.controlPlane,
	}
	if b, ok := builders[key]; ok {
		return b
	}
	b := &poolBuilder{
		key: key,
		cap: PoolCapacity{
			MachineType:  snap.machineType,
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
	if snap.ready {
		b.cap.ReadyNodes++
	}
	b.cap.NodePoolName = minNonEmpty(b.cap.NodePoolName, snap.nodePool)
	b.cap.TPUType = minNonEmpty(b.cap.TPUType, snap.tpuType)
	b.cap.GPUType = minNonEmpty(b.cap.GPUType, snap.gpuType)
	b.cap.IsTPU = b.cap.IsTPU || snap.isTPU
	b.cap.IsGPU = b.cap.IsGPU || snap.isGPU
	b.cap.TPUsPerNode = minPositive(b.cap.TPUsPerNode, snap.tpus)
	b.cap.GPUsPerNode = minPositive(b.cap.GPUsPerNode, snap.gpus)
	b.cap.CPUsPerNode = minPositive(b.cap.CPUsPerNode, snap.cpus)
	b.cap.MemMiBPerNode = minPositive(b.cap.MemMiBPerNode, snap.memMiB)
	if snap.topology != "" {
		b.topologies[snap.topology] = struct{}{}
	}
	if snap.sliceName != "" {
		b.slices[snap.sliceName] = struct{}{}
	}
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
	if snap.ready {
		entry.readyNodes++
	}
	entry.tpuType = minNonEmpty(entry.tpuType, snap.tpuType)
	entry.machineType = minNonEmpty(entry.machineType, snap.machineType)
	entry.topology = minNonEmpty(entry.topology, snap.topology)
	entry.chips = minPositive(entry.chips, snap.tpus)
}

func formatDisambiguatedPoolKey(key poolGroupKey) string {
	if key.controlPlane {
		if key.class == "cpu" {
			return key.machineType + "/control-plane"
		}
		return key.machineType + "/control-plane-" + key.class
	}
	return key.machineType + "/" + key.class
}

func finalizePools(inv *Inventory, builders map[poolGroupKey]*poolBuilder) {
	countByMachine := make(map[string]int, len(builders))
	for key := range builders {
		countByMachine[key.machineType]++
	}

	byName := make(map[string]*poolBuilder, len(builders))
	names := make([]string, 0, len(builders))
	for key, b := range builders {
		poolName := key.machineType
		if countByMachine[key.machineType] > 1 {
			poolName = formatDisambiguatedPoolKey(key)
		}
		byName[poolName] = b
		names = append(names, poolName)
	}
	slices.Sort(names)
	inv.PoolNames = names

	controlPlaneOnly := make(map[string]bool, len(builders))
	for _, name := range names {
		b := byName[name]
		b.cap.Topologies = sortedKeys(b.topologies)
		b.cap.SliceNames = sortedKeys(b.slices)
		inv.Pools[name] = b.cap
		controlPlaneOnly[name] = b.key.controlPlane
	}
	inv.CPUPoolCap = selectCPUPoolCap(inv, controlPlaneOnly)
}

func selectCPUPoolCap(inv *Inventory, controlPlaneOnly map[string]bool) *PoolCapacity {
	var best *PoolCapacity
	bestCtrlOnly := true

	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if pool.IsTPU || pool.IsGPU {
			continue
		}
		isCtrlOnly := controlPlaneOnly[name]
		if isBetterCPUPool(pool, isCtrlOnly, best, bestCtrlOnly) {
			p := pool
			best = &p
			bestCtrlOnly = isCtrlOnly
		}
	}
	return best
}

func isBetterCPUPool(cand PoolCapacity, candCtrlOnly bool, cur *PoolCapacity, curCtrlOnly bool) bool {
	if cur == nil {
		return true
	}
	if candCtrlOnly != curCtrlOnly {
		return !candCtrlOnly
	}
	if (cand.ReadyNodes > 0) != (cur.ReadyNodes > 0) {
		return cand.ReadyNodes > 0
	}
	if cand.CPUsPerNode != cur.CPUsPerNode {
		return cand.CPUsPerNode > cur.CPUsPerNode
	}
	return cand.MachineType < cur.MachineType
}

func parseNodeAllocatable(node unstructured.Unstructured) (cpus, memMiB, tpus, gpus int) {
	alloc, found, err := unstructured.NestedMap(node.Object, "status", "allocatable")
	if err != nil || !found {
		return 0, 0, 0, 0
	}
	if q, ok := parseQuantityField(alloc, "cpu"); ok {
		cpus = int(q.MilliValue() / 1000)
	}
	if q, ok := parseQuantityField(alloc, "memory"); ok {
		memMiB = int(q.Value() / (1024 * 1024))
	}
	if q, ok := parseQuantityField(alloc, "google.com/tpu"); ok {
		tpus = int(q.Value())
	}
	if q, ok := parseQuantityField(alloc, "nvidia.com/gpu"); ok {
		gpus = int(q.Value())
	}
	return cpus, memMiB, tpus, gpus
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

func hasReadyCondition(obj map[string]interface{}) bool {
	conditions, _, _ := unstructured.NestedSlice(obj, "status", "conditions")
	for _, raw := range conditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		cType, _ := cond["type"].(string)
		cStatus, _ := cond["status"].(string)
		if cType == "Ready" && cStatus == "True" {
			return true
		}
	}
	return false
}

func isNodeReady(node unstructured.Unstructured) bool {
	if unsched, _, _ := unstructured.NestedBool(node.Object, "spec", "unschedulable"); unsched {
		return false
	}
	return hasReadyCondition(node.Object)
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

func listTPUSlices(lister ResourceLister) ([]SliceInfo, bool) {
	items, err := lister.ListResources(TPUSliceGVR, "", "", "")
	if err != nil {
		if IsNotFoundOrNoMatchError(err) {
			return nil, false
		}
		if IsForbiddenError(err) {
			logging.Info("Skipping TPU Slice inventory (lacking permission to list slices).")
			return nil, false
		}
		logging.Warn("Failed to list TPU Slice resources: %v", err)
		return nil, false
	}
	result := make([]SliceInfo, 0, len(items))
	for i := range items {
		result = append(result, parseTPUSlice(items[i]))
	}
	sortSlices(result)
	return result, true
}

func parseTPUSlice(obj unstructured.Unstructured) SliceInfo {
	labels := obj.GetLabels()
	rawType := firstNonEmpty(
		nestedString(obj.Object, "spec", "type"),
		nestedString(obj.Object, "spec", "tpuType"),
		labels[TPUSliceTypeLabel],
	)
	topology := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		nestedString(obj.Object, "spec", "topology"),
		labels[TPUSliceTopologyLabel],
	)))
	state := strings.ToUpper(strings.TrimSpace(firstNonEmpty(
		nestedString(obj.Object, "status", "state"),
		nestedString(obj.Object, "status", "phase"),
	)))
	ready := state == "READY" || (state == "" && sliceReadyFromConditions(obj.Object))
	chipsHint := 0
	if topology == "1x1" {
		chipsHint = 1
	}
	tpuType, machineType := InferTPUTypeAndMachine(rawType, "", chipsHint)
	chips := 0
	if machineType != "" {
		chips = config.AcceleratorsPerVM(machineType)
	}
	nodesCount := resolveCRDSliceNodes(machineType, topology, chips, sliceNodeCount(obj.Object))

	return SliceInfo{
		Name:          obj.GetName(),
		Namespace:     obj.GetNamespace(),
		TPUType:       tpuType,
		MachineType:   machineType,
		Topology:      topology,
		State:         state,
		Ready:         ready,
		NodesPerSlice: nodesCount,
		ChipsPerNode:  chips,
		OwnerName:     strings.TrimSpace(labels[TPUSliceOwnerNameLabel]),
		ManagedBy:     strings.TrimSpace(labels[TPUSliceManagedByLabel]),
		PartitionTree: firstNonEmpty(nestedString(obj.Object, "spec", "partitionTree"), nestedString(obj.Object, "spec", "partition")),
		FromCRD:       true,
	}
}

func sliceReadyFromConditions(obj map[string]interface{}) bool {
	return hasReadyCondition(obj)
}

func sliceNodeCount(obj map[string]interface{}) int {
	for _, path := range [][]string{
		{"status", "nodes"},
		{"status", "nodeNames"},
		{"spec", "nodes"},
		{"spec", "nodeNames"},
	} {
		if items, found, _ := unstructured.NestedSlice(obj, path...); found && len(items) > 0 {
			return len(items)
		}
	}
	for _, path := range [][]string{
		{"status", "numNodes"},
		{"spec", "numNodes"},
	} {
		if n, ok := nestedInt(obj, path...); ok && n > 0 {
			return n
		}
	}
	return 0
}

func listTPUPartitionTrees(lister ResourceLister) ([]PartitionTreeInfo, bool) {
	items, err := lister.ListResources(TPUPartitionTreeGVR, "", "", "")
	if err != nil {
		if IsNotFoundOrNoMatchError(err) {
			return nil, false
		}
		if IsForbiddenError(err) {
			logging.Info("Skipping TPU PartitionTree inventory (lacking permission to list partitiontrees).")
			return nil, false
		}
		logging.Warn("Failed to list TPU PartitionTree resources: %v", err)
		return nil, false
	}
	result := make([]PartitionTreeInfo, 0, len(items))
	for i := range items {
		result = append(result, parsePartitionTree(items[i]))
	}
	slices.SortFunc(result, func(a, b PartitionTreeInfo) int {
		return strings.Compare(a.Name, b.Name)
	})
	return result, true
}

func parsePartitionTree(obj unstructured.Unstructured) PartitionTreeInfo {
	labels := obj.GetLabels()
	rawType := firstNonEmpty(
		nestedString(obj.Object, "spec", "type"),
		nestedString(obj.Object, "spec", "tpuType"),
		labels[TPUSliceTypeLabel],
	)
	rootTopo := strings.ToLower(strings.TrimSpace(firstNonEmpty(
		nestedString(obj.Object, "spec", "topology"),
		nestedString(obj.Object, "spec", "rootTopology"),
		labels[TPUSliceTopologyLabel],
	)))
	return PartitionTreeInfo{
		Name:                obj.GetName(),
		TPUType:             NormalizeTPUType(rawType),
		RootTopology:        rootTopo,
		SupportedTopologies: extractPartitionTopologies(obj.Object, rootTopo),
	}
}

func extractPartitionTopologies(obj map[string]interface{}, root string) []string {
	var raw []string
	if root != "" {
		raw = append(raw, root)
	}
	for _, path := range [][]string{
		{"spec", "supportedTopologies"},
		{"spec", "subTopologies"},
		{"status", "topologies"},
		{"status", "availableTopologies"},
	} {
		if list, found, _ := unstructured.NestedStringSlice(obj, path...); found {
			raw = append(raw, list...)
		}
	}
	if parts, found, _ := unstructured.NestedSlice(obj, "spec", "partitions"); found {
		collectNestedPartitionTopologies(parts, &raw)
	}
	return normalizeUniqueSorted(raw)
}

func collectNestedPartitionTopologies(items []interface{}, out *[]string) {
	for _, item := range items {
		switch v := item.(type) {
		case string:
			*out = append(*out, v)
		case map[string]interface{}:
			if topo, _ := v["topology"].(string); topo != "" {
				*out = append(*out, topo)
			}
			if children, found, _ := unstructured.NestedSlice(v, "children"); found {
				collectNestedPartitionTopologies(children, out)
			}
		}
	}
}

func mergeNodeDerivedSlices(inv *Inventory, nodeSlices map[string]*nodeSliceAgg) {
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
				enrichSliceFromNodes(&inv.Slices[idx], agg)
			}
			continue
		}
		chips := agg.chips
		if chips == 0 && agg.machineType != "" {
			chips = config.AcceleratorsPerVM(agg.machineType)
		}
		inv.Slices = append(inv.Slices, SliceInfo{
			Name:          agg.name,
			TPUType:       agg.tpuType,
			MachineType:   agg.machineType,
			Topology:      agg.topology,
			Ready:         agg.readyNodes > 0,
			NodesPerSlice: resolveNodesPerSlice(agg.machineType, agg.topology, chips, agg.totalNodes),
			ChipsPerNode:  chips,
			FromCRD:       false,
		})
	}
	sortSlices(inv.Slices)
}

func enrichSliceFromNodes(s *SliceInfo, agg *nodeSliceAgg) {
	if s.TPUType == "" {
		s.TPUType = agg.tpuType
	}
	if agg.machineType != "" {
		s.MachineType = agg.machineType
	}
	if s.Topology == "" {
		s.Topology = agg.topology
	}
	if agg.chips > 0 {
		s.ChipsPerNode = agg.chips
	}
	if agg.totalNodes > 0 || agg.chips > 0 || s.NodesPerSlice == 0 {
		s.NodesPerSlice = resolveNodesPerSlice(s.MachineType, s.Topology, s.ChipsPerNode, agg.totalNodes)
	}
}

func buildTPUInventory(inv *Inventory, hasPartitionLabels bool) (*TPUInventory, *PoolCapacity) {
	primaryPool := selectPrimaryTPUPool(inv)
	if primaryPool == nil && len(inv.Slices) == 0 && len(inv.PartitionTrees) == 0 {
		return nil, nil
	}

	tpu := &TPUInventory{HasPartitionLabels: hasPartitionLabels}
	if primaryPool != nil {
		tpu.TPUType = primaryPool.TPUType
		tpu.MachineType = primaryPool.MachineType
		tpu.ChipsPerNode = primaryPool.TPUsPerNode
	}
	for _, name := range inv.PoolNames {
		if pool := inv.Pools[name]; pool.IsTPU {
			tpu.TotalTPUNodes += pool.TotalNodes
			tpu.ReadyTPUNodes += pool.ReadyNodes
		}
	}

	applySliceAndTreeMetadata(inv, primaryPool, tpu)
	if tpu.ChipsPerNode == 0 && tpu.MachineType != "" {
		tpu.ChipsPerNode = config.AcceleratorsPerVM(tpu.MachineType)
	}
	if tpu.NodesPerSlice == 0 {
		tpu.NodesPerSlice = resolveNodesPerSlice(tpu.MachineType, tpu.Topology, tpu.ChipsPerNode, 0)
	}
	return tpu, primaryPool
}

func applySliceAndTreeMetadata(inv *Inventory, primaryPool *PoolCapacity, tpu *TPUInventory) {
	applyDefaultSlice(inv, tpu)
	tpu.Topologies = collectAllTopologies(inv, primaryPool, tpu)
	if tpu.Topology == "" && len(tpu.Topologies) > 0 {
		tpu.Topology = tpu.Topologies[0]
	}
}

func collectAllTopologies(inv *Inventory, primaryPool *PoolCapacity, tpu *TPUInventory) []string {
	var allTopos []string
	if primaryPool != nil {
		allTopos = append(allTopos, primaryPool.Topologies...)
	}
	for _, s := range inv.Slices {
		if s.Topology != "" && tpuTypesCompatible(tpu.TPUType, s.TPUType) {
			allTopos = append(allTopos, s.Topology)
		}
	}
	for _, pt := range inv.PartitionTrees {
		if tpu.TPUType == "" && pt.TPUType != "" {
			tpu.TPUType, tpu.MachineType = InferTPUTypeAndMachine(pt.TPUType, tpu.MachineType, tpu.ChipsPerNode)
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

func applyDefaultSlice(inv *Inventory, tpu *TPUInventory) {
	defSlice := selectDefaultSlice(inv.Slices, tpu.TPUType)
	if defSlice == nil {
		return
	}
	if defSlice.Ready {
		tpu.DefaultSliceName = defSlice.Name
	}
	tpu.TPUType = firstNonEmpty(tpu.TPUType, defSlice.TPUType)
	tpu.MachineType = firstNonEmpty(tpu.MachineType, defSlice.MachineType)
	if tpu.ChipsPerNode == 0 {
		tpu.ChipsPerNode = defSlice.ChipsPerNode
	}
	if defSlice.Topology != "" {
		tpu.Topology = defSlice.Topology
	}
	if defSlice.NodesPerSlice > 0 {
		tpu.NodesPerSlice = defSlice.NodesPerSlice
	}
}

func selectPrimaryTPUPool(inv *Inventory) *PoolCapacity {
	var best *PoolCapacity
	for _, name := range inv.PoolNames {
		pool := inv.Pools[name]
		if !pool.IsTPU {
			continue
		}
		if isBetterTPUPool(pool, best) {
			p := pool
			best = &p
		}
	}
	return best
}

func isBetterTPUPool(cand PoolCapacity, cur *PoolCapacity) bool {
	if cur == nil {
		return true
	}
	if (cand.ReadyNodes > 0) != (cur.ReadyNodes > 0) {
		return cand.ReadyNodes > 0
	}
	if cand.TotalNodes != cur.TotalNodes {
		return cand.TotalNodes > cur.TotalNodes
	}
	return cand.MachineType < cur.MachineType
}

func selectDefaultSlice(sliceList []SliceInfo, tpuType string) *SliceInfo {
	if len(sliceList) == 0 {
		return nil
	}
	for _, wantReady := range []bool{true, false} {
		for i := range sliceList {
			s := &sliceList[i]
			if wantReady && !s.Ready {
				continue
			}
			if tpuTypesCompatible(tpuType, s.TPUType) {
				return s
			}
		}
	}
	return nil
}

// resolveCRDSliceNodes computes NodesPerSlice for CRD-only slices (where per-node chip count may be guessed),
// trusting the CRD's reported node count when present before falling back to topology calculation.
func resolveCRDSliceNodes(machineType, topology string, chipsPerNode, crdNodes int) int {
	if crdNodes > 0 {
		return crdNodes
	}
	return resolveNodesPerSlice(machineType, topology, chipsPerNode, 0)
}

// resolveNodesPerSlice computes NodesPerSlice when real node chip counts are available,
// preferring topology math so partially visible or repairing slices do not under-report slice size.
func resolveNodesPerSlice(machineType, topology string, chipsPerNode, observedNodes int) int {
	if topology != "" {
		if n, err := config.CalculateAcceleratorNodes(machineType, topology, chipsPerNode); err == nil && n > 0 {
			return n
		}
	}
	if observedNodes > 0 {
		return observedNodes
	}
	return 0
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

func nestedInt(obj map[string]interface{}, fields ...string) (int, bool) {
	raw, found, err := unstructured.NestedFieldNoCopy(obj, fields...)
	if err != nil || !found || raw == nil {
		return 0, false
	}
	switch v := raw.(type) {
	case int64:
		return int(v), true
	case int:
		return v, true
	case float64:
		return int(v), true
	case json.Number:
		n, err := v.Int64()
		return int(n), err == nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		return n, err == nil
	default:
		return 0, false
	}
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

func sortedKeys(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func normalizeUniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, v := range values {
		if trimmed := strings.ToLower(strings.TrimSpace(v)); trimmed != "" {
			set[trimmed] = struct{}{}
		}
	}
	return sortedKeys(set)
}
