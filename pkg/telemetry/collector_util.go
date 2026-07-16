// Copyright 2026 "Google LLC"
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

package telemetry

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/modulewriter"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/billing/apiv1/billingpb"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"

	billing "cloud.google.com/go/billing/apiv1"
	resourcemanager "cloud.google.com/go/resourcemanager/apiv3"
	resourcemanagerpb "cloud.google.com/go/resourcemanager/apiv3/resourcemanagerpb"
)

type result struct {
	mi  modulereader.ModuleInfo
	err error
}

// Storage constants
const (
	StoragePrefixGCS       = "gcs"       // Prefix for Google Cloud Storage metrics
	StoragePrefixFilestore = "filestore" // Prefix for Filestore tiers
	StoragePrefixRedis     = "redis"     // Prefix for Redis database usage
	StoragePrefixSpanner   = "spanner"   // Prefix for Spanner database usage
	StorageTypeLocalSSD    = "local-ssd" // Standalone metric base for Local SSDs

	ModuleSourceRedis   = "database/redis"   // Module origin to detect Redis
	ModuleSourceSpanner = "database/spanner" // Module origin to detect Spanner
)

var (
	machineTypeSettings = []string{
		"machine_type",                  // Usual setting for specifying machine type.
		"node_type",                     // For modules that use node_type setting instead of machine_type to set machines.
		"system_node_pool_machine_type", // For gke-cluster system node pools.
	}
	staticNodeCountSettings = []string{
		"static_node_count", // Used in GKE node pool. If set, autoscaling will be disabled. Defaults to 0.
		"node_count_static", // Standalone Slurm V6 CPU and TPU nodesets use 'node_count_static'. Defaults to 0.
		"instance_count",    // VM instances and Batch login nodes use 'instance_count' to define static nodes. Default is 1.
		"target_size",       // Used by HTCondor execute points and MIGs for pool capacity.
	}
	staticNodeCountInlineKeys   = []string{"nodeset", "nodeset_tpu", "partition"} // Combine top-level explicit keys and complex inline object list keys for Slurm V6.
	dynamicMinNodeCountSettings = []string{
		"autoscaling_min_node_count",
		"autoscaling_total_min_nodes",
		"system_node_pool_node_count.total_min_nodes", // Used by gke-cluster system node pools
	}
	dynamicMaxNodeCountSettings = []string{
		"autoscaling_max_node_count",
		"autoscaling_total_max_nodes",
		"node_count_dynamic_max",
		"max_size",                                    // Used by HTCondor execute points for unbounded scaling limit.
		"system_node_pool_node_count.total_max_nodes", // Used by gke-cluster system node pools
	}
	storageTypeSettings = []string{
		"disk_type",
		"system_node_pool_disk_type",
		"filestore_tier",
		"fs_type",
		"storage_type",
		"storage_class",
	}
	localSsdCountSettings = []string{
		"local_ssd_count_ephemeral_storage",
		"local_ssd_count_nvme",
		"local_nvme_ssd_count",
		"local_ssd_count",
	}
	zonalNodeCountSettings = []string{
		"static_node_count",
		"autoscaling_min_node_count",
		"autoscaling_max_node_count",
	}
)

func getBlueprint(cmd *cobra.Command, args []string) config.Blueprint {
	if len(args) == 0 {
		return config.Blueprint{}
	}

	targetPath := resolveBlueprintPath(args[0])

	bp, _, err := config.NewBlueprint(targetPath)
	if err != nil {
		return config.Blueprint{} // Return empty if it fails to parse
	}

	mergeDeploymentFileVars(cmd, &bp)
	mergeCLIVars(cmd, &bp)

	return bp
}

func resolveBlueprintPath(targetPath string) string {
	// If the argument is a directory, it indicates a deployment folder (e.g., used in 'deploy' or 'destroy').
	// We read the expanded blueprint from the artifacts directory instead.
	if info, err := os.Stat(targetPath); err == nil && info.IsDir() {
		return filepath.Join(modulewriter.ArtifactsDir(targetPath), modulewriter.ExpandedBlueprintName)
	}
	return targetPath
}

func mergeDeploymentFileVars(cmd *cobra.Command, bp *config.Blueprint) {
	flag := cmd.Flag("deployment-file")
	if flag == nil || flag.Value.String() == "" {
		return
	}

	ds, _, err := config.NewDeploymentSettings(flag.Value.String())
	if err != nil {
		return
	}

	vars := bp.Vars.Items()
	maps.Copy(vars, ds.Vars.Items())
	bp.Vars = config.NewDict(vars)
}

func mergeCLIVars(cmd *cobra.Command, bp *config.Blueprint) {
	flag := cmd.Flag("vars")
	if flag == nil {
		return
	}

	varsSlice, err := cmd.Flags().GetStringSlice("vars")
	if err != nil {
		return
	}

	for _, cliVar := range varsSlice {
		arr := strings.SplitN(cliVar, "=", 2)
		if len(arr) != 2 {
			continue
		}

		key := arr[0]
		var v config.YamlValue
		// Use YAML unmarshal to support complex types (lists, maps) passed via CLI.
		if err := yaml.Unmarshal([]byte(arr[1]), &v); err == nil {
			bp.Vars = bp.Vars.With(key, v.Unwrap())
		}
	}
}

func getEventMetadataKVPairs(sourceMetadata map[string]string) []map[string]string {
	eventMetadata := make([]map[string]string, 0)
	for k, v := range sourceMetadata {
		eventMetadata = append(eventMetadata, map[string]string{
			"key":   k,
			"value": v,
		})
	}
	return eventMetadata
}

func getBpModulesList(bp config.Blueprint) []string {
	moduleInfos := config.GetAllBpModules(&bp)
	modules := make([]string, len(moduleInfos))
	for i, module := range moduleInfos {
		modules[i] = string(module.Source)
	}
	return modules
}

func ifModulesMatchPatterns(modulesList []string, patterns []string) string {
	for _, m := range modulesList {
		for _, p := range patterns {
			if strings.Contains(m, p) {
				return "true"
			}
		}
	}
	return "false"
}

func getStorageTypesFromModule(m config.Module, bp config.Blueprint) []string {
	var storageTypes []string

	addStorageType := func(t string) {
		t = strings.ToLower(strings.Trim(strings.TrimSpace(t), "\""))
		if t != "" {
			storageTypes = append(storageTypes, t)
		}
	}

	extractExplicitAndDefaultStorageTypes(m, bp, addStorageType)
	extractLocalSsdStorageTypes(m, bp, addStorageType)
	extractControllerStateDiskStorage(m, bp, addStorageType)
	extractNetworkStorage(m, bp, addStorageType)
	extractAdditionalDisks(m, bp, addStorageType)
	extractInlineNodesets(m, bp, addStorageType)
	extractDatabaseStorageTypes(m, bp, addStorageType)

	return storageTypes
}

func extractExplicitAndDefaultStorageTypes(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	formatType := func(key, val string) string {
		val = strings.TrimSpace(val)
		switch key {
		case "storage_class":
			return StoragePrefixGCS + "-" + val
		case "filestore_tier":
			return StoragePrefixFilestore + "-" + val
		}
		return val
	}

	// Explicit string settings
	var remainingKeys []string
	for _, key := range storageTypeSettings {
		if t := extractExplicitStringSetting(key, m, bp); t != "" {
			addStorageType(formatType(key, t))
		}
		if !m.Settings.Has(key) {
			remainingKeys = append(remainingKeys, key)
		}
	}

	// Default string settings
	if len(remainingKeys) > 0 {
		if t, key, found := extractDefaultSetting[string](remainingKeys, m); found && t != "" {
			addStorageType(formatType(key, t))
		}
	}
}

func extractDatabaseStorageTypes(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	src := string(m.Source)

	extractAndAdd := func(moduleType, key string) {
		if val := extractExplicitStringSetting(key, m, bp); val != "" {
			addStorageType(moduleType + "-" + val)
		} else if val, _, found := extractDefaultSetting[string]([]string{key}, m); found && val != "" {
			addStorageType(moduleType + "-" + strings.TrimSpace(val))
		}
	}

	if strings.Contains(src, ModuleSourceRedis) {
		extractAndAdd(StoragePrefixRedis, "tier")
	} else if strings.Contains(src, ModuleSourceSpanner) {
		extractAndAdd(StoragePrefixSpanner, "edition")
	}
}

func extractLocalSsdStorageTypes(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	hasExplicit := false
	for _, key := range localSsdCountSettings {
		if count, ok := extractExplicitIntSetting(key, m, bp); ok {
			hasExplicit = true
			if count > 0 {
				addStorageType(StorageTypeLocalSSD)
				return
			}
		}
	}
	if !hasExplicit {
		if count, _, ok := extractDefaultSetting[int](localSsdCountSettings, m); ok && count > 0 {
			addStorageType(StorageTypeLocalSSD)
		}
	}
}

func extractControllerStateDiskStorage(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	if m.Settings.Has("controller_state_disk") {
		val, err := bp.Eval(m.Settings.Get("controller_state_disk"))
		if err == nil {
			unmarked, _ := val.Unmark()
			if unmarked.IsKnown() && !unmarked.IsNull() && (unmarked.Type().IsObjectType() || unmarked.Type().IsMapType()) {
				if diskType := extractStringFromCtyMap(unmarked, []string{"type"}); diskType != "" {
					addStorageType(diskType)
				}
			}
		}
	}
}

func processCtyList(val cty.Value, processItem func(item cty.Value)) {
	val, _ = val.Unmark()
	if !val.IsKnown() || val.IsNull() || (!val.Type().IsListType() && !val.Type().IsTupleType() && !val.Type().IsSetType()) {
		return
	}
	for _, item := range val.AsValueSlice() {
		itemUnmarked, _ := item.Unmark()
		if itemUnmarked.IsKnown() && !itemUnmarked.IsNull() && (itemUnmarked.Type().IsObjectType() || itemUnmarked.Type().IsMapType()) {
			processItem(itemUnmarked)
		}
	}
}

func extractNetworkStorage(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	for _, key := range []string{"network_storage", "login_network_storage"} {
		if m.Settings.Has(key) {
			if val, err := bp.Eval(m.Settings.Get(key)); err == nil {
				processCtyList(val, func(item cty.Value) {
					if fsType := extractStringFromCtyMap(item, []string{"fs_type"}); fsType != "" {
						addStorageType(fsType)
					}
				})
			}
		}
	}
}

func extractAdditionalDisks(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	if m.Settings.Has("additional_disks") {
		if val, err := bp.Eval(m.Settings.Get("additional_disks")); err == nil {
			processCtyList(val, func(item cty.Value) {
				if diskType := extractStringFromCtyMap(item, []string{"disk_type"}); diskType != "" {
					addStorageType(diskType)
				}
			})
		}
	}
}

func extractInlineNodesets(m config.Module, bp config.Blueprint, addStorageType func(string)) {
	for _, key := range []string{"nodeset", "nodeset_tpu", "partitions", "partition", "login_nodes"} {
		if m.Settings.Has(key) {
			if val, err := bp.Eval(m.Settings.Get(key)); err == nil {
				processCtyList(val, func(itemUnmarked cty.Value) {
					if diskType := extractStringFromCtyMap(itemUnmarked, []string{"disk_type"}); diskType != "" {
						addStorageType(diskType)
					}
					// network_storage in inline item
					for _, nsKey := range []string{"network_storage", "login_network_storage"} {
						if ns, exists := itemUnmarked.AsValueMap()[nsKey]; exists {
							processCtyList(ns, func(nsItemUnmarked cty.Value) {
								if fsType := extractStringFromCtyMap(nsItemUnmarked, []string{"fs_type"}); fsType != "" {
									addStorageType(fsType)
								}
							})
						}
					}
					// additional_disks in inline item
					if ad, exists := itemUnmarked.AsValueMap()["additional_disks"]; exists {
						processCtyList(ad, func(adItemUnmarked cty.Value) {
							if diskType := extractStringFromCtyMap(adItemUnmarked, []string{"disk_type"}); diskType != "" {
								addStorageType(diskType)
							}
						})
					}
				})
			}
		}
	}
}

func getMachineTypeFromModule(m config.Module, bp config.Blueprint) string {
	// 1. Try explicit settings first
	for _, key := range machineTypeSettings {
		if t := extractExplicitStringSetting(key, m, bp); t != "" {
			return strings.Trim(t, "\"")
		}
	}
	// 2. If no explicit setting, try defaults
	if t, _, found := extractDefaultSetting[string](machineTypeSettings, m); found && t != "" {
		return strings.Trim(t, "\"")
	}

	return ""
}

// extractExplicitStringSetting attempts to get the given key string value if explicitly defined in the module's settings.
func extractExplicitStringSetting(key string, m config.Module, bp config.Blueprint) string {
	if !m.Settings.Has(key) {
		return ""
	}

	keyValue := m.Settings.Get(key)
	// Evaluate the value to resolve expressions like $(vars.key)
	evaluatedKey, err := bp.Eval(keyValue)
	if err != nil {
		return ""
	}

	// Some module outputs or references carry cty marks, so we unmark them safely before use.
	unmarkedKey, _ := evaluatedKey.Unmark()
	if unmarkedKey.IsKnown() && !unmarkedKey.IsNull() && unmarkedKey.Type() == cty.String {
		return strings.TrimSpace(unmarkedKey.AsString())
	}
	return ""
}
func getModuleNodeCounts(m config.Module, bp config.Blueprint) map[string]int {
	counts := make(map[string]int)
	topMachineType := getMachineTypeFromModule(m, bp)
	inlineFound := false

	// Process complex inline iterables (Slurm V6)
	for _, key := range staticNodeCountInlineKeys {
		if processInlineKey(m, bp, key, topMachineType, counts, staticNodeCountSettings) {
			inlineFound = true
		}
	}

	if inlineFound {
		return counts
	}

	// Fallback to standard top-level extraction
	if topMachineType != "" {
		if count, found := getTopLevelNodeCount(m, bp, topMachineType, staticNodeCountSettings); found {
			counts[topMachineType] += count
		}
	}
	return counts
}

// processInlineKey evaluates a specific key for iterable structures and processes its items.
func processInlineKey(m config.Module, bp config.Blueprint, key string, topMachineType string, counts map[string]int, targetKeys []string) bool {
	if !m.Settings.Has(key) {
		return false
	}
	val, err := bp.Eval(m.Settings.Get(key))
	if err != nil {
		return false
	}
	unmarked, _ := val.Unmark()
	if !unmarked.IsKnown() || unmarked.IsNull() {
		return false
	}

	ty := unmarked.Type()
	if !ty.IsListType() && !ty.IsTupleType() && !ty.IsSetType() {
		return false
	}

	for _, item := range unmarked.AsValueSlice() {
		processInlineItem(item, topMachineType, counts, targetKeys)
	}
	return true
}

// processInlineItem extracts machine types and counts from an individual map/object in an inline list.
func processInlineItem(item cty.Value, topMachineType string, counts map[string]int, targetKeys []string) {
	item, _ = item.Unmark()
	if !item.IsKnown() || item.IsNull() {
		return
	}
	ty := item.Type()

	if !ty.IsObjectType() && !ty.IsMapType() {
		return
	}

	inlineMType := extractStringFromCtyMap(item, machineTypeSettings)
	if inlineMType == "" {
		inlineMType = topMachineType
	}

	if count, found := extractIntFromCtyMap(item, targetKeys); found && inlineMType != "" {
		counts[inlineMType] += count
	}
}

func extractTargetNodeCount(m config.Module, bp config.Blueprint, targetKeys []string) (int, bool) {
	for _, key := range targetKeys {
		if count, ok := extractExplicitIntSetting(key, m, bp); ok {
			// Apply zonal multipliers only to variables explicitly spanning a single zone natively.
			// Global metrics (e.g. autoscaling_total_max_nodes) span the entire cluster and should not be multiplied.
			if slices.Contains(zonalNodeCountSettings, key) {
				count *= getZonalMultiplier(m, bp)
			}
			return count, true
		}
	}
	return 0, false
}

func getTopLevelNodeCount(m config.Module, bp config.Blueprint, topMachineType string, targetKeys []string) (int, bool) {
	baseCount, found := extractTargetNodeCount(m, bp, targetKeys)

	// Static node count is calculated from the Topology for TPU machines.
	isStatic := len(targetKeys) > 0 && targetKeys[0] == "static_node_count"
	if !found && isStatic {
		if count, ok := getTPUNodeCount(m, bp, topMachineType); ok {
			baseCount = count
			found = true
		}
	}

	if !found {
		if count, key, ok := extractDefaultSetting[int](targetKeys, m); ok {
			baseCount = count
			found = true
			if ifModulesMatchPatterns([]string{string(m.Source)}, isGkeModulePatterns) == "true" {
				// Apply zonal multipliers only to variables explicitly spanning a single zone natively.
				// Global metrics (e.g. autoscaling_total_max_nodes) span the entire cluster and should not be multiplied.
				if slices.Contains(zonalNodeCountSettings, key) {
					baseCount *= getZonalMultiplier(m, bp)
				}
			}
		}
	}

	if !found {
		return 0, false
	}

	pools := getMultiplier("num_node_pools", m, bp)
	slices := getMultiplier("num_slices", m, bp)
	multiplier := max(slices, pools)

	return baseCount * multiplier, true
}

// getZonalMultiplier retrieves the length of the zones list if it exists.
func getZonalMultiplier(m config.Module, bp config.Blueprint) int {
	keys := []string{"zones", "system_node_pool_zones"}
	for _, key := range keys {
		if !m.Settings.Has(key) {
			continue
		}
		evaluated, err := bp.Eval(m.Settings.Get(key))
		if err != nil {
			continue
		}
		unmarked, _ := evaluated.Unmark()
		if unmarked.IsKnown() && !unmarked.IsNull() && (unmarked.Type().IsTupleType() || unmarked.Type().IsListType() || unmarked.Type().IsSetType()) {
			if length := len(unmarked.AsValueSlice()); length > 0 {
				return length
			}
		}
	}
	return 1
}

func getMultiplier(key string, m config.Module, bp config.Blueprint) int {
	if val, ok := extractExplicitIntSetting(key, m, bp); ok {
		if val > 0 {
			return val
		}
		return 1
	}
	if val, _, ok := extractDefaultSetting[int]([]string{key}, m); ok {
		if val > 0 {
			return val
		}
	}
	return 1
}

// Logic taken from pkg/config/hardware.go.
func getTPUNodeCount(m config.Module, bp config.Blueprint, topMachineType string) (int, bool) {
	if !config.IsTPU(topMachineType) {
		return 0, false
	}

	tpuTopologyStr, hasTopology := config.ExtractTopology(bp, &m)
	if !hasTopology {
		return 0, false
	}

	if m.Settings.Has("enable_flex_start") {
		val, err := bp.Eval(m.Settings.Get("enable_flex_start"))
		if err == nil && val.Type() == cty.Bool && !val.IsNull() && val.IsKnown() && val.True() {
			return 0, true
		}
	}

	if nodes, err := config.CalculateAcceleratorNodes(topMachineType, tpuTopologyStr, 0); err == nil {
		return nodes, true
	}

	return 0, false
}

func extractStringFromCtyMap(val cty.Value, targetKeys []string) string {
	valMap := val.AsValueMap()
	for _, key := range targetKeys { // Iterate over slice for deterministic precedence
		if v, exists := valMap[key]; exists {
			v, _ = v.Unmark()
			if v.IsKnown() && !v.IsNull() && v.Type() == cty.String {
				return strings.TrimSpace(strings.Trim(v.AsString(), "\""))
			}
		}
	}
	return ""
}

func extractIntFromCtyMap(val cty.Value, targetKeys []string) (int, bool) {
	valMap := val.AsValueMap()
	for _, key := range targetKeys { // Iterate over slice for deterministic precedence
		if v, exists := valMap[key]; exists {
			v, _ = v.Unmark()
			if v.IsKnown() && !v.IsNull() && v.Type() == cty.Number {
				out, _ := v.AsBigFloat().Int64()
				return int(out), true
			}
			return 0, true
		}
	}
	return 0, false
}

// extractExplicitIntSetting attempts to get the given key int value if explicitly defined.
// It returns the int value, and a boolean indicating if the setting was found.
func extractExplicitIntSetting(key string, m config.Module, bp config.Blueprint) (int, bool) {
	keys := strings.Split(key, ".")
	if !m.Settings.Has(keys[0]) {
		return 0, false
	}
	keyValue := m.Settings.Get(keys[0])

	// Traverse nested object properties if dot notation is used
	for i := 1; i < len(keys); i++ {
		if ev, err := bp.Eval(keyValue); err == nil {
			keyValue = ev
		}
		unmarked, _ := keyValue.Unmark()
		if !unmarked.IsKnown() || unmarked.IsNull() {
			return 0, false
		}
		if !unmarked.Type().IsObjectType() && !unmarked.Type().IsMapType() {
			return 0, false
		}
		asMap := unmarked.AsValueMap()
		if val, exists := asMap[keys[i]]; exists {
			keyValue = val
		} else {
			return 0, false
		}
	}

	evaluatedKey, err := bp.Eval(keyValue)
	if err != nil {
		return 0, false
	}
	unmarkedKey, _ := evaluatedKey.Unmark()
	if unmarkedKey.IsKnown() && !unmarkedKey.IsNull() && unmarkedKey.Type() == cty.Number {
		out, _ := unmarkedKey.AsBigFloat().Int64()
		return int(out), true
	}
	return 0, true // Value exists but is unknown, return safely without fallback
}

// extractDefaultSetting attempts to get a default setting from the module's source variables.
func extractDefaultSetting[T any](keys []string, m config.Module) (T, string, bool) {
	var zero T
	kindStr, valid := isValidModuleKind(m)
	if !valid {
		return zero, "", false
	}

	resCh := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resCh <- result{err: fmt.Errorf("panic in GetModuleInfo: %v", r)}
			}
		}()
		mi, err := modulereader.GetModuleInfo(m.Source, kindStr)
		resCh <- result{mi: mi, err: err}
	}()

	select {
	case res := <-resCh:
		if res.err != nil {
			return zero, "", false
		}
		// Iterate over keys to maintain precedence order
		for _, key := range keys {
			for _, input := range res.mi.Inputs {
				if val, ok := parseDefaultValue[T](input.Name, input.Default, key); ok {
					return val, key, true
				}
			}
		}
	case <-time.After(500 * time.Millisecond):
	}

	return zero, "", false
}

// isValidModuleKind checks if the module has a valid source and returns its kind.
func isValidModuleKind(m config.Module) (string, bool) {
	if m.Source == "" {
		return "", false
	}
	kindStr := m.Kind.String()
	if kindStr == "" {
		kindStr = config.TerraformKind.String()
	}
	if kindStr != config.TerraformKind.String() && kindStr != config.PackerKind.String() {
		return "", false
	}
	return kindStr, true
}

// parseDefaultValue matches the input key and safely casts the default value.
func parseDefaultValue[T any](inputName string, inputDefault any, key string) (T, bool) {
	var zero T
	if inputName == key && inputDefault != nil {
		switch v := inputDefault.(type) {
		case T:
			return v, true
		case int64:
			if _, isInt := any(zero).(int); isInt {
				return any(int(v)).(T), true
			}
		case float64:
			// Safely cast float64 to int if T is int
			if _, isInt := any(zero).(int); isInt {
				return any(int(v)).(T), true
			}
		case float32:
			if _, isInt := any(zero).(int); isInt {
				return any(int(v)).(T), true
			}
		}
	}
	return zero, false
}

// getProjectBillingAccount fetches the billing account associated with a given GCP project in the format "billingAccounts/{billing_account_id}". If billing is disabled for the project, this will return an empty string.
var getProjectBillingAccount = func(ctx context.Context, projectID string) (string, error) {
	client, err := billing.NewCloudBillingClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	req := &billingpb.GetProjectBillingInfoRequest{
		Name: fmt.Sprintf("projects/%s", projectID),
	}

	var info *billingpb.ProjectBillingInfo
	var apiErr error

	// Retry up to 3 times for transient failures (e.g., rate limits or network flakes)
	for attempt := 1; attempt <= 3; attempt++ {
		info, apiErr = client.GetProjectBillingInfo(ctx, req)
		if apiErr == nil {
			return info.GetBillingAccountName(), nil
		}
		// Check for context expiration and avoid sleep on the last iteration to reduce unnecessary latency on failure
		if attempt == 3 || ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) // simple backoff
	}
	return "", apiErr
}

// fetchProjectName retrieves the project name (which contains the project number) for a given project ID.
var fetchProjectName = func(ctx context.Context, projectID string) (string, error) {
	client, err := resourcemanager.NewProjectsClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()
	req := &resourcemanagerpb.GetProjectRequest{Name: fmt.Sprintf("projects/%s", projectID)}

	var project *resourcemanagerpb.Project
	var apiErr error

	// Retry up to 3 times for transient failures (e.g., rate limits or network flakes)
	for attempt := 1; attempt <= 3; attempt++ {
		project, apiErr = client.GetProject(ctx, req)
		if apiErr == nil {
			return project.Name, nil
		}
		// Check for context expiration and avoid sleep on the last iteration to reduce unnecessary latency on failure
		if attempt == 3 || ctx.Err() != nil {
			break
		}
		time.Sleep(time.Duration(attempt) * 500 * time.Millisecond) // simple backoff
	}

	return "", apiErr
}

func evaluateIsGoogler() bool {
	// Check Application Default Credentials (ADC) for Service Accounts. CI pipelines usually inject credentials via this environment variable.
	adcPath := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS")
	if adcPath != "" {
		isInternal, err := checkADCForInternalUser(adcPath)
		if err == nil && isInternal {
			return true
		}
	}

	// Fall back to reading the gcloud active config file directly.
	return checkGcloudConfigForInternalUser()
}

// getGcloudConfigDir resolves the gcloud configuration directory based on environment and OS.
func getGcloudConfigDir() (string, error) {
	// Respect the CLOUDSDK_CONFIG environment variable if set
	if envDir := os.Getenv("CLOUDSDK_CONFIG"); envDir != "" {
		return envDir, nil
	}

	// Fall back to OS-specific default paths
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("APPDATA"), "gcloud"), nil
	}

	return filepath.Join(homeDir, ".config", "gcloud"), nil
}

func checkGcloudConfigForInternalUser() bool {
	configDir, err := getGcloudConfigDir()
	if err != nil {
		return false
	}

	// Find the active configuration name
	activeConfigPath := filepath.Join(configDir, "active_config")
	activeConfigBytes, err := os.ReadFile(activeConfigPath)
	if err != nil {
		return false
	}

	activeConfig := strings.TrimSpace(string(activeConfigBytes))
	if activeConfig == "" {
		return false
	}

	// Read the active configuration file
	configFile := filepath.Join(configDir, "configurations", "config_"+activeConfig)
	configBytes, err := os.ReadFile(configFile)
	if err != nil {
		return false
	}

	// Parse the INI-style file to extract the account under [core]
	email := extractAccountFromConfig(configBytes)
	return isInternalEmail(email)
}

// extractAccountFromConfig parses the INI-style gcloud config bytes to extract the account email.
func extractAccountFromConfig(configBytes []byte) string {
	lines := strings.Split(string(configBytes), "\n")
	inCoreSection := false
	for _, line := range lines {
		// Strip inline comments before doing any processing
		if idx := strings.IndexAny(line, "#;"); idx != -1 {
			line = line[:idx]
		}

		// Trim surrounding whitespaces
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inCoreSection = strings.EqualFold(line, "[core]")
			continue
		}

		if inCoreSection {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 && strings.TrimSpace(parts[0]) == "account" {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// checkADCForInternalUser parses the ADC JSON file to extract the client email.
func checkADCForInternalUser(credentialsPath string) (bool, error) {
	data, err := os.ReadFile(credentialsPath)
	if err != nil {
		return false, err // Fail open (treat as external) if file can't be read
	}

	var key ServiceAccountKey
	if err := json.Unmarshal(data, &key); err != nil {
		return false, err
	}

	return isInternalEmail(key.ClientEmail), nil
}

// isInternalEmail contains the logic to identify Google emails and internal SA domains.
func isInternalEmail(email string) bool {
	if email == "" {
		return false
	}

	// Direct Google employees workstation accounts
	if strings.HasSuffix(email, "@google.com") || strings.HasSuffix(email, ".google.com") {
		return true
	}

	// Allowlist specific internal Cluster Toolkit project IDs that tests use.
	internalProjectNames := []string{
		"hpc-toolkit-dev",
		"hpc-toolkit-demo",
		"hpc-toolkit-gsc",
	}

	for _, projectName := range internalProjectNames {
		pattern := ".*" + projectName + ".*gserviceaccount.com"
		matched, err := regexp.MatchString(pattern, email)

		if err == nil && matched {
			return true
		}
	}

	// Allowlist specific internal Cluster Toolkit project numbers that tests use.
	internalProjectNumbers := []string{
		"508417052821",
		"858831239249",
		"266450182917",
	}
	for _, projectNum := range internalProjectNumbers {
		pattern := ".*" + projectNum + ".*@cloudbuild.gserviceaccount.com"
		matched, err := regexp.MatchString(pattern, email)

		if err == nil && matched {
			return true
		}
	}

	return false
}

// getLinuxVersion parses /etc/os-release to find the pretty name or version ID.
func getLinuxVersion() string {
	// Standard way to identify Linux distribution version
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "Linux (unknown version)"
	}
	defer f.Close()

	var prettyName, versionID string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			prettyName = parseOsReleaseField(line)
		} else if strings.HasPrefix(line, "VERSION_ID=") {
			versionID = parseOsReleaseField(line)
		}
	}

	if prettyName != "" {
		return prettyName
	}
	if versionID != "" {
		return versionID
	}
	return "Linux (unknown version)"
}

// getMacVersion uses sw_vers to get the macOS product version.
func getMacVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, "sw_vers", "-productVersion").Output()
	if err != nil {
		return "Darwin (unknown version)"
	}
	return "Darwin " + strings.TrimSpace(string(out))
}

// getWindowsVersion uses the ver command to get the Windows version.
func getWindowsVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), shortTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "cmd", "/c", "ver")
	out, err := cmd.Output()
	if err != nil {
		return "Windows (unknown version)"
	}
	return strings.TrimSpace(string(out))
}

// parseOsReleaseField helper to clean up quotes from /etc/os-release values
func parseOsReleaseField(line string) string {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.Trim(parts[1], "'\"")
}

func getModuleDynamicNodeCounts(m config.Module, bp config.Blueprint, targetKeys []string) map[string]int {
	if ifModulesMatchPatterns([]string{string(m.Source)}, isGkeModulePatterns) == "true" {
		if m.Settings.Has("static_node_count") {
			if val, err := bp.Eval(m.Settings.Get("static_node_count")); err == nil {
				unmarked, _ := val.Unmark()
				if unmarked.IsKnown() && !unmarked.IsNull() {
					return map[string]int{}
				}
			}
		}
	}
	counts := make(map[string]int)
	topMachineType := getMachineTypeFromModule(m, bp)
	inlineFound := false
	for _, key := range staticNodeCountInlineKeys {
		if processInlineKey(m, bp, key, topMachineType, counts, targetKeys) {
			inlineFound = true
		}
	}
	if inlineFound {
		return counts
	}
	if topMachineType != "" {
		if count, found := getTopLevelNodeCount(m, bp, topMachineType, targetKeys); found {
			counts[topMachineType] += count
		}
	}
	return counts
}
