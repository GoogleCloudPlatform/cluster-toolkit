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
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"hpc-toolkit/pkg/shell"

	"gopkg.in/yaml.v2"
	"k8s.io/apimachinery/pkg/util/validation"
)

// -----------------------------------------------------------------------------
// 1. Kueue Package Constants
// -----------------------------------------------------------------------------

// defaultKueueVersion is the fallback version of Kueue to install.
// ATTENTION: For cluster creation the corresponding default version
// is in modules/management/kubectl-apply/variables.tf.
const defaultKueueVersion = "v0.15.2"

const (
	defaultLocalQueue    = "default"
	defaultClusterQueue  = "default"
	multisliceLocalQueue = "multislice-queue"

	// kueueAPIVersion is the GVR version used for Kueue resources.
	kueueAPIVersion = "v1beta2"

	clusterQueueManifestFile = "cluster-queue.yaml"
	localQueueManifestFile   = "local-queue.yaml"
	localQueueTemplateFile   = "local_queue.tmpl"

	pathwaysCPUNominalQuota       = "999999"
	pathwaysMemoryNominalQuota    = "999999T"
	kueueControllerRolloutTimeout = "--timeout=600s"
)

var kueuePermissionChecks = []permissionCheck{
	{"create", "namespaces"},
	{"create", "customresourcedefinitions.apiextensions.k8s.io"},
	{"create", "clusterroles.rbac.authorization.k8s.io"},
	{"create", "clusterrolebindings.rbac.authorization.k8s.io"},
}

var kueueCRDs = []string{
	"admissionchecks.kueue.x-k8s.io",
	"clusterqueues.kueue.x-k8s.io",
	"cohorts.kueue.x-k8s.io",
	"localqueues.kueue.x-k8s.io",
	"multikueueclusters.kueue.x-k8s.io",
	"multikueueconfigs.kueue.x-k8s.io",
	"provisioningrequestconfigs.kueue.x-k8s.io",
	"resourceflavors.kueue.x-k8s.io",
	"topologies.kueue.x-k8s.io",
	"workloadpriorityclasses.kueue.x-k8s.io",
	"workloads.kueue.x-k8s.io",
}

// parseKueueQueueName extracts and validates the Kueue LocalQueue name.
// Supports bare names ("my-queue") and resource paths ("namespaces/default/localQueues/my-queue").
func parseKueueQueueName(raw string) (string, error) {
	queueName := strings.TrimSuffix(strings.TrimSpace(raw), "/")
	if strings.Contains(queueName, "/") {
		queueName = extractURIPart(queueName, "localQueues")
		if queueName == "" || strings.Contains(queueName, "/") {
			return "", fmt.Errorf("invalid queue name %q: must not contain slashes", raw)
		}
	}

	if queueName == "" {
		return "", fmt.Errorf("queue name cannot be empty")
	}

	if errs := validation.IsDNS1123Subdomain(queueName); len(errs) > 0 {
		return "", fmt.Errorf("invalid queue name %q: %s", queueName, strings.Join(errs, ", "))
	}

	return queueName, nil
}

// resolveKueueQueue resolves which Kueue LocalQueue to use for job submission.
// The returned bool is true when the queue name was auto-discovered from an
// existing LocalQueue list in the namespace (so callers don't need to re-fetch it).
// Precedence rules:
// 1. If requestedQueueName is specified explicitly by the user, validate and return it directly.
// 2. If 0 LocalQueues are found in the namespace, fall back to defaultLocalQueue ("default").
// 3. If exactly 1 LocalQueue is found, auto-discover and return it.
// 4. If multiple LocalQueues exist, check for standard "default", then "multislice-queue".
// 5. If multiple non-standard LocalQueues exist, return an error asking the user to specify --queue.
func (g *GKEOrchestrator) resolveKueueQueue(requestedQueueName, ns string) (string, bool, error) {
	if requestedQueueName != "" {
		cleanName, err := parseKueueQueueName(requestedQueueName)
		if err != nil {
			return "", false, err
		}
		logging.Info("Using provided Kueue LocalQueue: %s", cleanName)
		return cleanName, false, nil
	}

	res := g.kubectl("get", "localqueue", "-n", ns, "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 {
		return "", false, fmt.Errorf("failed to query LocalQueues in namespace %s: %s", ns, res.Stderr)
	}

	output := strings.TrimSpace(res.Stdout)
	if output == "" {
		logging.Info("No LocalQueues found. Defaulting to '%s'.", defaultLocalQueue)
		return defaultLocalQueue, false, nil
	}

	queues := strings.Fields(output)
	if len(queues) == 1 {
		logging.Info("Auto-discovered Kueue LocalQueue: %s", queues[0])
		return queues[0], true, nil
	}

	if slices.Contains(queues, defaultLocalQueue) {
		logging.Info("Multiple LocalQueues found %v. Defaulting to standard LocalQueue '%s'.", queues, defaultLocalQueue)
		return defaultLocalQueue, true, nil
	}
	if slices.Contains(queues, multisliceLocalQueue) {
		logging.Info("Multiple LocalQueues found %v. Defaulting to '%s'.", queues, multisliceLocalQueue)
		return multisliceLocalQueue, true, nil
	}

	return "", false, fmt.Errorf("multiple LocalQueues found (%v) and no standard '%s' or '%s' present. Please specify which one to use using --queue flag", queues, defaultLocalQueue, multisliceLocalQueue)
}

// listLocalQueues returns a slice of existing LocalQueue names in the given namespace.
func (g *GKEOrchestrator) listLocalQueues(ns string) []string {
	res := g.kubectl("get", "localqueue", "-n", ns, "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return nil
	}
	return strings.Fields(strings.TrimSpace(res.Stdout))
}

// checkLocalQueueExists checks if a LocalQueue with the given name exists in the namespace.
func (g *GKEOrchestrator) checkLocalQueueExists(name, ns string) (bool, error) {
	res := g.kubectl("get", "localqueue", name, "-n", ns)
	if res.ExitCode == 0 {
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		return false, nil
	}
	return false, fmt.Errorf("failed to check localqueue status: %s", res.Stderr)
}

// checkClusterQueueExists checks if a ClusterQueue with the given name exists in the cluster.
func (g *GKEOrchestrator) checkClusterQueueExists(name string) (bool, error) {
	res := g.kubectl("get", "clusterqueue", name)
	if res.ExitCode == 0 {
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		return false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		return false, fmt.Errorf("insufficient RBAC permissions to read ClusterQueue %q (403 Forbidden)", name)
	}
	return false, fmt.Errorf("failed to check clusterqueue status: %s", res.Stderr)
}

// createDefaultQueues creates a LocalQueue in the target namespace and a corresponding ClusterQueue (defaultClusterQueue).
func (g *GKEOrchestrator) createDefaultQueues(localQueueName, ns string) error {
	logging.Info("Creating default ClusterQueue and LocalQueue...")

	if err := g.EnsureResourceFlavors(); err != nil {
		return err
	}

	cqName := defaultClusterQueue
	cqExists, err := g.checkClusterQueueExists(cqName)
	if err != nil {
		if k8s.IsForbiddenError(err) {
			return fmt.Errorf("LocalQueue '%s' does not exist in namespace '%s' and cluster permissions are restricted (403 Forbidden). Please ask your cluster administrator to create a LocalQueue in namespace '%s'", localQueueName, ns, ns)
		}
		return fmt.Errorf("failed to check if ClusterQueue %s exists: %w", cqName, err)
	}

	if !cqExists {
		// Render and apply ClusterQueue only if it does not already exist
		clusterQueueBytes, err := g.renderClusterQueue(cqName)
		if err != nil {
			return fmt.Errorf("failed to render clusterqueue: %w", err)
		}
		if err := g.applyManifests(clusterQueueBytes, clusterQueueManifestFile, ""); err != nil {
			return fmt.Errorf("failed to apply clusterqueue: %w", err)
		}
	} else {
		logging.Info("ClusterQueue '%s' already exists. Skipping ClusterQueue creation.", cqName)
	}

	// Render and apply LocalQueue
	localQueueBytes, err := g.renderLocalQueueInNamespace(localQueueName, cqName, ns)
	if err != nil {
		return err
	}

	if err := g.applyManifests(localQueueBytes, localQueueManifestFile, ""); err != nil {
		return fmt.Errorf("failed to apply localqueue: %w", err)
	}

	logging.Info("Default queues created successfully.")
	return nil
}

// ensureClusterQueueCoverage checks if the parent ClusterQueue covers CPU and Memory quotas, updating it if empty.
func (g *GKEOrchestrator) ensureClusterQueueCoverage(localQueueName, ns string) error {
	cqName, err := g.getClusterQueueName(localQueueName, ns)
	if err != nil {
		return err
	}

	hasCoverage, isEmpty, err := g.checkClusterQueueCoverage(cqName)
	if err != nil {
		return err
	}

	if hasCoverage {
		logging.Info("Kueue ClusterQueue '%s' already covers CPU and Memory.", cqName)
		return nil
	}

	if isEmpty {
		logging.Info("ClusterQueue '%s' is empty. Applying calculated capacity...", cqName)
		clusterQueueBytes, err := g.renderClusterQueue(cqName)
		if err != nil {
			return fmt.Errorf("failed to render clusterqueue with new capacity: %w", err)
		}
		if err := g.applyManifests(clusterQueueBytes, clusterQueueManifestFile, ""); err != nil {
			return fmt.Errorf("failed to apply clusterqueue with new capacity: %w", err)
		}
		return nil
	}

	return fmt.Errorf("ClusterQueue '%s' does not cover required resources (CPU and Memory); please configure it manually to include quotas for 'cpu' and 'memory' resources", cqName)
}

// ensurePathwaysFlavorAlignment ensures that the ClusterQueue bound to localQueueName has a
// non-accelerator ResourceFlavor before any TPU/GPU flavor that matches g.resolvedHeadNodePool.
// When the cluster still has a stale "pathways-flavor" pointing to a nonexistent node pool (for
// example hardcoded "cpu-np"), it updates "pathways-flavor" in place. When both the existing pool
// and g.resolvedHeadNodePool exist on the cluster, it creates a pool-specific flavor without
// clobbering "pathways-flavor" and prepends it to the CPU resourceGroup in the ClusterQueue.
func (g *GKEOrchestrator) ensurePathwaysFlavorAlignment(localQueueName, ns string) error {
	if g.resolvedHeadNodePool == "" {
		return nil
	}

	cqName, err := g.getClusterQueueName(localQueueName, ns)
	if err != nil {
		return err
	}

	cq, cqFound, err := g.fetchKueueClusterQueue(cqName)
	if err != nil || !cqFound {
		return err
	}

	rfLabels, rfReadable, err := g.fetchResourceFlavorLabels()
	if err != nil || !rfReadable {
		return err
	}

	cpuGroups := findCPUResourceGroups(cq, rfLabels)
	if len(cpuGroups) == 0 {
		return nil
	}

	var unalignedGroups []cpuResourceGroupMatch
	for _, group := range cpuGroups {
		if !hasMatchingPreAcceleratorFlavor(group.flavors[:group.firstAccelIdx], rfLabels, g.resolvedHeadNodePool) {
			unalignedGroups = append(unalignedGroups, group)
		}
	}
	if len(unalignedGroups) == 0 {
		return nil
	}

	targetFlavor, err := g.resolveTargetPathwaysFlavor(rfLabels)
	if err != nil {
		return err
	}

	if err := g.applyPathwaysFlavorToClusterQueue(cqName, cq, unalignedGroups, targetFlavor); err != nil {
		return err
	}

	return nil
}

func (g *GKEOrchestrator) fetchKueueClusterQueue(cqName string) (map[string]interface{}, bool, error) {
	res := g.kubectl("get", "clusterqueue", cqName, "-o", "json")
	if res.ExitCode != 0 {
		errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to read ClusterQueue %q (403 Forbidden). Skipping Pathways flavor alignment.", cqName)
			return nil, false, nil
		}
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "notfound") {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get clusterqueue %s: %s", cqName, res.Stderr)
	}

	cq, err := decodeJSONMap(res.Stdout)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse clusterqueue %s JSON: %w", cqName, err)
	}
	return cq, true, nil
}

func (g *GKEOrchestrator) fetchResourceFlavorLabels() (map[string]map[string]string, bool, error) {
	res := g.kubectl("get", "resourceflavor", "-o", "json")
	if res.ExitCode != 0 {
		errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to list ResourceFlavors (403 Forbidden). Skipping Pathways flavor alignment.")
			return nil, false, nil
		}
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "notfound") || strings.Contains(errStr, "no resources found") {
			return map[string]map[string]string{}, true, nil
		}
		return nil, false, fmt.Errorf("failed to list resourceflavors: %s", res.Stderr)
	}

	parsed, err := decodeJSONMap(res.Stdout)
	if err != nil {
		return nil, false, fmt.Errorf("failed to parse resourceflavor list JSON: %w", err)
	}

	labelsByFlavor := make(map[string]map[string]string)
	items, _ := parsed["items"].([]interface{})
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		meta, _ := itemMap["metadata"].(map[string]interface{})
		name, _ := meta["name"].(string)
		if name == "" {
			continue
		}
		spec, _ := itemMap["spec"].(map[string]interface{})
		rawLabels, _ := spec["nodeLabels"].(map[string]interface{})
		labels := make(map[string]string, len(rawLabels))
		for k, v := range rawLabels {
			if vs, ok := v.(string); ok {
				labels[k] = vs
			}
		}
		labelsByFlavor[name] = labels
	}
	return labelsByFlavor, true, nil
}

type cpuResourceGroupMatch struct {
	rg            map[string]interface{}
	flavors       []interface{}
	firstAccelIdx int
}

func findCPUResourceGroups(cq map[string]interface{}, rfLabels map[string]map[string]string) []cpuResourceGroupMatch {
	spec, _ := cq["spec"].(map[string]interface{})
	rgList, _ := spec["resourceGroups"].([]interface{})
	var matches []cpuResourceGroupMatch
	for _, rgItem := range rgList {
		rg, ok := rgItem.(map[string]interface{})
		if !ok || !coversCPUOrMemory(rg) {
			continue
		}
		flavors, _ := rg["flavors"].([]interface{})
		if len(flavors) == 0 {
			continue
		}
		firstAccelIdx := len(flavors)
		for i, fItem := range flavors {
			fMap, ok := fItem.(map[string]interface{})
			if !ok {
				continue
			}
			fname, _ := fMap["name"].(string)
			if isAcceleratorFlavorEntry(fname, fMap, rfLabels[fname]) {
				firstAccelIdx = i
				break
			}
		}
		matches = append(matches, cpuResourceGroupMatch{
			rg:            rg,
			flavors:       flavors,
			firstAccelIdx: firstAccelIdx,
		})
	}
	return matches
}

func coversCPUOrMemory(rg map[string]interface{}) bool {
	covered, _ := rg["coveredResources"].([]interface{})
	for _, r := range covered {
		if rs, ok := r.(string); ok && (rs == "cpu" || rs == "memory") {
			return true
		}
	}
	return false
}

func isAcceleratorFlavorEntry(fname string, fMap map[string]interface{}, nodeLabels map[string]string) bool {
	if isTPUFlavor(fname) || isGPUFlavor(fname) {
		return true
	}
	if _, ok := nodeLabels["cloud.google.com/gke-tpu-accelerator"]; ok {
		return true
	}
	if _, ok := nodeLabels["cloud.google.com/gke-accelerator"]; ok {
		return true
	}
	resources, _ := fMap["resources"].([]interface{})
	for _, rItem := range resources {
		rMap, ok := rItem.(map[string]interface{})
		if !ok {
			continue
		}
		rName, _ := rMap["name"].(string)
		if rName != "google.com/tpu" && rName != "nvidia.com/gpu" {
			continue
		}
		if fmt.Sprint(rMap["nominalQuota"]) != "0" {
			return true
		}
	}
	return false
}

func hasMatchingPreAcceleratorFlavor(preAccelFlavors []interface{}, rfLabels map[string]map[string]string, targetPool string) bool {
	for _, fItem := range preAccelFlavors {
		fMap, ok := fItem.(map[string]interface{})
		if !ok {
			continue
		}
		fname, _ := fMap["name"].(string)
		labels, exists := rfLabels[fname]
		if !exists {
			continue
		}
		if labels["cloud.google.com/gke-nodepool"] == targetPool {
			return true
		}
		if (fname == "flavor-default" || fname == "pathways-flavor") && len(labels) == 0 {
			return true
		}
	}
	return false
}

func (g *GKEOrchestrator) resolveTargetPathwaysFlavor(rfLabels map[string]map[string]string) (string, error) {
	existingLabels, exists := rfLabels["pathways-flavor"]
	if !exists {
		logging.Info("Creating Kueue ResourceFlavor 'pathways-flavor' for node pool %q...", g.resolvedHeadNodePool)
		if err := g.ensureNodePoolResourceFlavor("pathways-flavor", "", g.resolvedHeadNodePool); err != nil {
			return "", err
		}
		return "pathways-flavor", nil
	}

	existingPool := existingLabels["cloud.google.com/gke-nodepool"]
	if existingPool == g.resolvedHeadNodePool {
		return "pathways-flavor", nil
	}

	// Only overwrite 'pathways-flavor' in place when node pool inventory is populated and confirms existingPool is gone.
	if existingPool == "" || (len(g.clusterDesc.NodePools) > 0 && !g.hasClusterNodePool(existingPool)) {
		logging.Info("Updating Kueue ResourceFlavor 'pathways-flavor' from stale node pool %q to %q...", existingPool, g.resolvedHeadNodePool)
		if err := g.ensureNodePoolResourceFlavor("pathways-flavor", existingPool, g.resolvedHeadNodePool); err != nil {
			return "", err
		}
		return "pathways-flavor", nil
	}

	targetFlavor := "pathways-flavor-" + g.resolvedHeadNodePool
	if currentLabels, ok := rfLabels[targetFlavor]; !ok || currentLabels["cloud.google.com/gke-nodepool"] != g.resolvedHeadNodePool {
		logging.Info("Ensuring Kueue ResourceFlavor %q for Pathways head node pool %q (preserving existing 'pathways-flavor' for %q)...", targetFlavor, g.resolvedHeadNodePool, existingPool)
		if err := g.ensureNodePoolResourceFlavor(targetFlavor, existingPool, g.resolvedHeadNodePool); err != nil {
			return "", err
		}
	}
	return targetFlavor, nil
}

func (g *GKEOrchestrator) ensureNodePoolResourceFlavor(flavorName, existingPool, targetPool string) error {
	rfBytes, err := g.renderResourceFlavor(flavorName, map[string]string{
		"cloud.google.com/gke-nodepool": targetPool,
	})
	if err != nil {
		return fmt.Errorf("failed to render ResourceFlavor %s: %w", flavorName, err)
	}
	if err := g.applyManifests(rfBytes, fmt.Sprintf("resource-flavor-%s.yaml", flavorName), ""); err != nil {
		if existingPool != "" {
			return fmt.Errorf("Kueue ResourceFlavor 'pathways-flavor' targets node pool %q, but pathways-head targets %q; failed to update ResourceFlavor %s (pathways-head will stay Pending if Kueue falls through to a TPU flavor): %w", existingPool, targetPool, flavorName, err)
		}
		return fmt.Errorf("failed to apply ResourceFlavor %s for node pool %q: %w", flavorName, targetPool, err)
	}
	return nil
}

func (g *GKEOrchestrator) applyPathwaysFlavorToClusterQueue(cqName string, cq map[string]interface{}, cpuGroups []cpuResourceGroupMatch, targetFlavor string) error {
	updated := false
	for _, group := range cpuGroups {
		if g.prependFlavorToResourceGroup(group, targetFlavor) {
			updated = true
		}
	}
	if !updated {
		return nil
	}

	stripServerManagedCQFields(cq)

	cqBytes, err := yaml.Marshal(cq)
	if err != nil {
		return fmt.Errorf("failed to marshal updated ClusterQueue %s: %w", cqName, err)
	}
	if err := g.applyManifests(cqBytes, clusterQueueManifestFile, ""); err != nil {
		return fmt.Errorf("failed to update ClusterQueue %s with Pathways flavor %q for node pool %q: %w", cqName, targetFlavor, g.resolvedHeadNodePool, err)
	}
	return nil
}

func (g *GKEOrchestrator) prependFlavorToResourceGroup(group cpuResourceGroupMatch, targetFlavor string) bool {
	targetIdx := -1
	for i, fItem := range group.flavors {
		fMap, ok := fItem.(map[string]interface{})
		if ok && fMap["name"] == targetFlavor {
			targetIdx = i
			break
		}
	}

	if targetIdx >= 0 && targetIdx < group.firstAccelIdx {
		return false
	}

	var targetEntry map[string]interface{}
	var updatedFlavors []interface{}
	if targetIdx >= 0 {
		targetEntry = group.flavors[targetIdx].(map[string]interface{})
		for i, fItem := range group.flavors {
			if i != targetIdx {
				updatedFlavors = append(updatedFlavors, fItem)
			}
		}
	} else {
		targetEntry = g.buildPathwaysFlavorEntry(group.rg, group.flavors, targetFlavor)
		updatedFlavors = append(updatedFlavors, group.flavors...)
	}

	// Prepend the Pathways head flavor so CPU-only head pods match it before any TPU/GPU flavor.
	group.rg["flavors"] = append([]interface{}{targetEntry}, updatedFlavors...)
	return true
}

func (g *GKEOrchestrator) buildPathwaysFlavorEntry(rg map[string]interface{}, existingFlavors []interface{}, targetFlavor string) map[string]interface{} {
	existingQuotas := extractFlavorQuotas(existingFlavors, "pathways-flavor")
	fc, hasCapacity := g.capacity.Flavors["pathways-flavor"]
	covered, _ := rg["coveredResources"].([]interface{})
	var resources []map[string]interface{}
	for _, r := range covered {
		resName, ok := r.(string)
		if !ok {
			continue
		}
		resources = append(resources, map[string]interface{}{
			"name":         resName,
			"nominalQuota": g.resolvePathwaysResourceQuota(resName, existingQuotas, fc, hasCapacity),
		})
	}
	return map[string]interface{}{
		"name":      targetFlavor,
		"resources": resources,
	}
}

func extractFlavorQuotas(flavors []interface{}, flavorName string) map[string]interface{} {
	quotas := make(map[string]interface{})
	for _, fItem := range flavors {
		fMap, ok := fItem.(map[string]interface{})
		if !ok || fMap["name"] != flavorName {
			continue
		}
		resources, _ := fMap["resources"].([]interface{})
		for _, rItem := range resources {
			rMap, ok := rItem.(map[string]interface{})
			if !ok {
				continue
			}
			if rName, ok := rMap["name"].(string); ok {
				quotas[rName] = rMap["nominalQuota"]
			}
		}
	}
	return quotas
}

func (g *GKEOrchestrator) resolvePathwaysResourceQuota(resName string, existingQuotas map[string]interface{}, fc FlavorCapacity, hasCapacity bool) interface{} {
	if resName != "cpu" && resName != "memory" {
		return "0"
	}
	if q, ok := existingQuotas[resName]; ok {
		return q
	}
	if hasCapacity {
		return g.getNominalQuota(resName, fc, "pathways-flavor")
	}
	if resName == "cpu" {
		return 480
	}
	return "2000G"
}

func (g *GKEOrchestrator) hasClusterNodePool(poolName string) bool {
	for _, np := range g.clusterDesc.NodePools {
		if np.Name == poolName {
			return true
		}
	}
	return false
}

func decodeJSONMap(raw string) (map[string]interface{}, error) {
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	var parsed map[string]interface{}
	if err := dec.Decode(&parsed); err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, fmt.Errorf("expected JSON object, got null")
	}
	normalized, ok := normalizeJSONValue(parsed).(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("expected JSON object, got %T", normalized)
	}
	return normalized, nil
}

func normalizeJSONValue(v interface{}) interface{} {
	switch val := v.(type) {
	case json.Number:
		if i, err := val.Int64(); err == nil {
			return i
		}
		return val.String()
	case map[string]interface{}:
		for k, item := range val {
			val[k] = normalizeJSONValue(item)
		}
		return val
	case []interface{}:
		for i, item := range val {
			val[i] = normalizeJSONValue(item)
		}
		return val
	default:
		return v
	}
}

func stripServerManagedCQFields(cq map[string]interface{}) {
	delete(cq, "status")
	if meta, ok := cq["metadata"].(map[string]interface{}); ok {
		for _, field := range []string{"uid", "resourceVersion", "generation", "creationTimestamp", "managedFields", "selfLink"} {
			delete(meta, field)
		}
	}
}

// getClusterQueueName inspects a LocalQueue to retrieve the name of its bound ClusterQueue.
func (g *GKEOrchestrator) getClusterQueueName(localQueueName, ns string) (string, error) {
	res := g.kubectl("get", "localqueue", localQueueName, "-n", ns, "-o", "jsonpath={.spec.clusterQueue}")
	if res.ExitCode != 0 {
		return "", fmt.Errorf("failed to find clusterqueue for %s in namespace %s: %s", localQueueName, ns, res.Stderr)
	}
	cqName := strings.TrimSpace(res.Stdout)
	if cqName == "" {
		cqName = localQueueName
	}
	return cqName, nil
}

// checkClusterQueueCoverage inspects a ClusterQueue to verify if resourceGroups cover CPU and Memory.
func (g *GKEOrchestrator) checkClusterQueueCoverage(cqName string) (bool, bool, error) {
	res := g.kubectl("get", "clusterqueue", cqName, "-o", "json")
	if res.ExitCode != 0 {
		errStr := strings.ToLower(res.Stderr)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to read ClusterQueue %q (403 Forbidden). Skipping resource coverage validation.", cqName)
			return true, false, nil
		}
		return false, false, fmt.Errorf("failed to get clusterqueue %s: %s", cqName, res.Stderr)
	}

	var cq map[string]interface{}
	if err := json.Unmarshal([]byte(res.Stdout), &cq); err != nil {
		return false, false, fmt.Errorf("failed to parse clusterqueue %s JSON: %w", cqName, err)
	}

	spec, ok := cq["spec"].(map[string]interface{})
	if !ok {
		return false, true, nil
	}
	rgList, ok := spec["resourceGroups"].([]interface{})
	if !ok || len(rgList) == 0 {
		return false, true, nil
	}

	return g.hasRequiredResources(rgList), false, nil
}

// hasRequiredResources helper checks if resourceGroups list includes both CPU and Memory covered resources.
func (g *GKEOrchestrator) hasRequiredResources(rgList []interface{}) bool {
	hasCPU := false
	hasMem := false
	for _, rgItem := range rgList {
		rg, ok := rgItem.(map[string]interface{})
		if !ok {
			continue
		}
		if covered, ok := rg["coveredResources"].([]interface{}); ok {
			for _, r := range covered {
				if rStr, ok := r.(string); ok {
					switch rStr {
					case "cpu":
						hasCPU = true
					case "memory":
						hasMem = true
					}
				}
			}
		}
	}

	return hasCPU && hasMem
}

// -----------------------------------------------------------------------------
// 3. Kueue Installation & Cluster Lifecycle Management (Cluster-Level)
// -----------------------------------------------------------------------------

// hasFailedDeploymentCondition checks if any of the deployment conditions indicate a failure.
func hasFailedDeploymentCondition(dep *kueueDeployment) bool {
	for _, cond := range dep.Status.Conditions {
		if cond.Type == "Progressing" && (cond.Reason == "ProgressDeadlineExceeded" || cond.Status == "False") {
			return true
		}
		if cond.Type == "Available" && cond.Status == "False" {
			return true
		}
	}
	return false
}

// isKueueDeploymentRolledOut checks if the Kueue deployment has been fully rolled out and is healthy.
func isKueueDeploymentRolledOut(dep *kueueDeployment) bool {
	if dep == nil || dep.Spec.Paused || hasFailedDeploymentCondition(dep) {
		return false
	}
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	return desired > 0 &&
		dep.Metadata.Generation <= dep.Status.ObservedGeneration &&
		dep.Status.UpdatedReplicas >= desired &&
		dep.Status.Replicas <= dep.Status.UpdatedReplicas &&
		dep.Status.AvailableReplicas >= dep.Status.UpdatedReplicas
}

// CheckAndInstallKueue verifies if Kueue is installed on the GKE cluster, installing or updating it if required.
func (g *GKEOrchestrator) CheckAndInstallKueue(version string, clusterName string, clusterLocation string) error {
	kueueCRDInstalled, errCRD := g.isKueueInstalled()
	if errCRD != nil {
		return fmt.Errorf("failed to verify Kueue CRD status: %w", errCRD)
	}
	dep, kueueDeploymentInstalled, errDep := g.getKueueDeployment()
	if errDep != nil {
		return fmt.Errorf("failed to verify Kueue deployment status: %w", errDep)
	}
	var currentVersion string
	if dep != nil {
		logging.Info("Kueue deployment found.")
		var errVer error
		if currentVersion, errVer = g.getKueueVersion(dep); errVer != nil {
			logging.Info("Warning: Unable to determine installed Kueue version: %v", errVer)
		}
	}

	if version == "" {
		version = defaultKueueVersion
	} else if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}

	needReinstall, reinstallReason := g.evaluateKueueReinstall(kueueCRDInstalled, kueueDeploymentInstalled, currentVersion, version)
	if needReinstall {
		if g.checkDynamicSlicingViaGKE() {
			return fmt.Errorf("automatic Kueue installation/modification blocked: cluster %s is set up for Dynamic-Slicing (found 'PROVISION_ONLY' in node pool placementPolicy). Automatic installation or modification of Kueue could conflict with custom topology configurations. Please install Kueue and required CRDs manually", clusterName)
		}

		if err := g.checkKueueInstallPermissions(version); err != nil {
			return err
		}

		if err := g.handleKueueReinstallation(version, reinstallReason); err != nil {
			return err
		}
	} else {
		if err := g.waitForKueueWebhookFast(dep); err != nil {
			return fmt.Errorf("kueue is installed but currently unhealthy. Please check deployment health with 'kubectl get deployment kueue-controller-manager -n kueue-system': %w", err)
		}
	}

	logging.Info("Kueue is already installed.")
	return nil
}

func (g *GKEOrchestrator) evaluateKueueReinstall(crdInstalled, depInstalled bool, currentVer, targetVer string) (bool, string) {
	if currentVer != "" && g.isVersionBelow(currentVer, targetVer) {
		return true, fmt.Sprintf("Current Kueue version %s is below target %s.", currentVer, targetVer)
	}
	if !crdInstalled || !depInstalled {
		return true, "Kueue installation is incomplete (CRD or Deployment missing)."
	}
	return false, ""
}

func (g *GKEOrchestrator) handleKueueReinstallation(targetVersion string, reason string) error {
	promptMsg := fmt.Sprintf("%s\nKueue requires re-installation using %s.\nWARNING: This deletes all queued and suspended workloads in this cluster before proceeding.\nReplying 'no' will cause an immediate exit and you will have to do the re-installation manually. Proceed?", reason, targetVersion)
	if !shell.PromptYesNo(promptMsg) {
		return fmt.Errorf("user declined to re-install Kueue")
	}

	logging.Info("Proceeding with clean re-installation of Kueue...")
	if err := g.DeleteAllKueueResources(); err != nil {
		return fmt.Errorf("failed to delete Kueue resources: %w", err)
	}

	return g.installKueue(targetVersion)
}

func (g *GKEOrchestrator) installKueue(version string) error {
	logging.Info("Installing Kueue version %s...", version)
	kueueManifestsURL := fmt.Sprintf("https://github.com/kubernetes-sigs/kueue/releases/download/%s/manifests.yaml", version)
	manifestBytes, err := g.downloadManifests(kueueManifestsURL)
	if err != nil {
		return err
	}

	cleanedManifests, err := g.cleanAndProcessManifests(manifestBytes, nil)
	if err != nil {
		return err
	}

	if err := g.applyManifests(cleanedManifests, "kueue.yaml", ""); err != nil {
		return err
	}

	logging.Info("Kueue components applied successfully.")

	if err := g.waitForKueueWebhook(); err != nil {
		return err
	}

	return g.installKueueResources(defaultClusterQueue, defaultLocalQueue)
}

func (g *GKEOrchestrator) checkKueueInstallPermissions(version string) error {
	logging.Info("Verifying cluster permissions for Kueue installation...")
	if missing := g.missingPermissions(kueuePermissionChecks); len(missing) > 0 {
		return fmt.Errorf("unable to re-install Kueue to version %s: missing required RBAC permissions: [%s]. Please contact your cluster administrator", version, strings.Join(missing, ", "))
	}
	return nil
}

func (g *GKEOrchestrator) isKueueInstalled() (bool, error) {
	res := g.kubectl("get", "crd", "clusterqueues.kueue.x-k8s.io")
	if res.ExitCode == 0 {
		logging.Info("Kueue CRD found.")
		return true, nil
	}
	errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		logging.Info("Kueue CRD not found.")
		return false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		logging.Warn("Insufficient RBAC permissions to read Kueue CRD (403 Forbidden). Assuming Kueue is installed in shared cluster.")
		return true, nil
	}
	return false, fmt.Errorf("failed to check for Kueue CRD: %s\n%s", res.Stderr, res.Stdout)
}

// getKueueDeployment retrieves the Kueue controller manager deployment from the cluster.
func (g *GKEOrchestrator) getKueueDeployment() (*kueueDeployment, bool, error) {
	res := g.kubectl("get", "deployment", "kueue-controller-manager", "-n", "kueue-system", "-o", "json")
	if res.ExitCode == 0 {
		var dep kueueDeployment
		if err := json.Unmarshal([]byte(res.Stdout), &dep); err != nil {
			return nil, true, fmt.Errorf("failed to unmarshal kueue deployment: %w", err)
		}
		return &dep, true, nil
	}
	errStr := strings.ToLower(res.Stderr + " " + res.Stdout)
	if strings.Contains(errStr, "notfound") || strings.Contains(errStr, "not found") {
		logging.Info("Kueue deployment not found.")
		return nil, false, nil
	}
	if strings.Contains(errStr, "forbidden") {
		logging.Warn("Insufficient RBAC permissions to read Kueue deployment in kueue-system (403 Forbidden). Assuming Kueue deployment is installed in shared cluster.")
		return nil, true, nil
	}
	return nil, false, fmt.Errorf("failed to check for Kueue deployment: %s\n%s", res.Stderr, res.Stdout)
}

// getKueueVersion extracts the deployed version of the Kueue controller from its Deployment object.
func (g *GKEOrchestrator) getKueueVersion(dep *kueueDeployment) (string, error) {
	if dep == nil {
		return "", fmt.Errorf("kueue deployment object is nil")
	}
	containers := dep.Spec.Template.Spec.Containers
	if len(containers) == 0 {
		return "", fmt.Errorf("no containers found in Kueue deployment")
	}
	image := strings.TrimSpace(containers[0].Image)
	for _, c := range containers {
		if c.Name == "manager" {
			image = strings.TrimSpace(c.Image)
			break
		}
		if strings.Contains(c.Image, "kueue") {
			image = strings.TrimSpace(c.Image)
		}
	}
	if idx := strings.Index(image, "@"); idx != -1 {
		image = image[:idx]
	}
	lastColon := strings.LastIndex(image, ":")
	if lastColon == -1 {
		return "", fmt.Errorf("unexpected image format for Kueue: %s", image)
	}
	version := image[lastColon+1:]
	if strings.Contains(version, "/") {
		return "", fmt.Errorf("no tag found in Kueue image: %s", image)
	}
	return version, nil
}

func (g *GKEOrchestrator) isVersionBelow(current, target string) bool {
	curMajor, curMinor, curPatch := parseVersion(current)
	defMajor, defMinor, defPatch := parseVersion(target)

	return curMajor < defMajor || (curMajor == defMajor && curMinor < defMinor) || (curMajor == defMajor && curMinor == defMinor && curPatch < defPatch)
}

func parseVersion(v string) (int, int, int) {
	v = strings.TrimPrefix(v, "v")
	parts := strings.Split(v, ".")
	major, _ := strconv.Atoi(parts[0])
	minor := 0
	patch := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(parts[1])
	}
	if len(parts) > 2 {
		patchStr := parts[2]
		patchStr = strings.Split(patchStr, "-")[0]
		patchStr = strings.Split(patchStr, "+")[0]
		patch, _ = strconv.Atoi(patchStr)
	}
	return major, minor, patch
}

// DeleteKueueDeployment deletes the kueue-controller-manager deployment from the cluster.
func (g *GKEOrchestrator) DeleteKueueDeployment() error {
	logging.Info("Deleting Kueue deployment...")
	res := g.kubectl("delete", "deployment", "kueue-controller-manager", "-n", "kueue-system", "--ignore-not-found")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete Kueue deployment: %s\n%s", res.Stderr, res.Stdout)
	}
	return nil
}

// DeleteAllKueueResources deletes all Kueue custom resources and CRDs from the cluster.
func (g *GKEOrchestrator) DeleteAllKueueResources() error {
	resourceList := strings.Join(kueueCRDs, ",")
	logging.Info("Deleting all Kueue custom resources...")
	res := g.kubectl("delete", resourceList, "--all", "--ignore-not-found", "--timeout=60s")
	if res.ExitCode != 0 {
		logging.Info("Warning: Non-zero exit code while deleting Kueue custom resources: %s", res.Stderr)
	}

	logging.Info("Deleting Kueue CRDs...")
	args := append([]string{"delete", "crd", "--ignore-not-found", "--timeout=60s"}, kueueCRDs...)
	res = g.kubectl(args...)
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete Kueue CRDs: %s\n%s", res.Stderr, res.Stdout)
	}

	return g.DeleteKueueDeployment()
}

func (g *GKEOrchestrator) waitForKueueWebhookFast(dep *kueueDeployment) error {
	if !isKueueDeploymentRolledOut(dep) {
		res := g.kubectl("rollout", "status", "deployment/kueue-controller-manager", "-n", "kueue-system", kueueControllerRolloutTimeout)
		if res.ExitCode != 0 {
			if strings.Contains(strings.ToLower(res.Stderr+" "+res.Stdout), "forbidden") {
				logging.Warn("Insufficient RBAC permissions to check Kueue deployment rollout status in kueue-system (403 Forbidden). Skipping rollout status check.")
				return g.waitForKueueEndpoints(true)
			}
			podDetails := g.getKueuePodDetails()
			return fmt.Errorf("kueue controller manager failed to become ready: %s\n%s%s", res.Stderr, res.Stdout, podDetails)
		}
		if dep == nil {
			var err error
			dep, _, err = g.getKueueDeployment()
			if err != nil {
				logging.Warn("Failed to read Kueue deployment after rollout: %v", err)
			}
		}
	}

	version, err := g.getKueueVersion(dep)
	if err != nil {
		logging.Warn("Failed to get Kueue version, defaulting to Endpoints check: %v", err)
		version = defaultKueueVersion
	}

	major, minor, _ := parseVersion(version)
	useEndpointSlice := major > 0 || (major == 0 && minor > 13)

	return g.waitForKueueEndpoints(useEndpointSlice)
}

func (g *GKEOrchestrator) waitForKueueWebhook() error {
	if err := g.waitForKueueWebhookFast(nil); err != nil {
		return err
	}
	return g.probeKueueWebhookReadiness()
}

func (g *GKEOrchestrator) waitForKueueEndpoints(useEndpointSlice bool) error {
	for i := 0; i < 40; i++ {
		ready, err := g.checkKueueEndpoints(useEndpointSlice)
		if err != nil {
			return err
		}
		if ready {
			logging.Info("Kueue webhook service endpoints are available.")
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("timed out waiting for kueue-webhook-service endpoints to be available")
}

func (g *GKEOrchestrator) probeKueueWebhookReadiness() error {
	logging.Info("Probing Kueue webhook readiness...")
	probeName := fmt.Sprintf("gcluster-webhook-probe-%d", time.Now().UnixNano())
	probeManifest := fmt.Sprintf("apiVersion: kueue.x-k8s.io/%s\nkind: ResourceFlavor\nmetadata:\n  name: %s\n", kueueAPIVersion, probeName)
	f, err := os.CreateTemp("", "gcluster-webhook-probe-*.yaml")
	if err != nil {
		return fmt.Errorf("failed to create probe manifest file: %w", err)
	}
	probeFile := f.Name()
	defer func() {
		_ = f.Close()
		_ = g.kubectl("delete", "-f", probeFile, "--ignore-not-found")
		_ = os.Remove(probeFile)
	}()

	if _, err := f.Write([]byte(probeManifest)); err != nil {
		return fmt.Errorf("failed to write probe manifest: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("failed to close probe manifest file: %w", err)
	}

	for i := 0; i < 20; i++ {
		res := g.kubectl("apply", "-f", probeFile)
		if res.ExitCode == 0 {
			logging.Info("Kueue webhook is fully operational.")
			return nil
		}
		time.Sleep(5 * time.Second)
	}

	return fmt.Errorf("timed out waiting for Kueue webhook to become operational")
}

func (g *GKEOrchestrator) getKueuePodDetails() string {
	podRes := g.kubectl("get", "pods", "-n", "kueue-system", "-l", "control-plane=controller-manager", "-o", "json")
	var podDetails string
	if podRes.ExitCode == 0 {
		var podList struct {
			Items []struct {
				Metadata struct {
					Name string `json:"name"`
				} `json:"metadata"`
				Status struct {
					ContainerStatuses []struct {
						Name  string `json:"name"`
						State struct {
							Waiting struct {
								Reason  string `json:"reason"`
								Message string `json:"message"`
							} `json:"waiting"`
						} `json:"state"`
					} `json:"containerStatuses"`
				} `json:"status"`
			} `json:"items"`
		}
		if err := json.Unmarshal([]byte(podRes.Stdout), &podList); err == nil {
			for _, item := range podList.Items {
				for _, cs := range item.Status.ContainerStatuses {
					if cs.State.Waiting.Reason != "" {
						podDetails += fmt.Sprintf("\n  - Pod %s: %s (%s)", item.Metadata.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message)
					}
				}
			}
		}
	}
	return podDetails
}

func (g *GKEOrchestrator) checkKueueEndpoints(useEndpointSlice bool) (bool, error) {
	if useEndpointSlice {
		ready, _, err := g.checkServiceEndpointsReady("kueue-system", "kueue-webhook-service")
		if errors.Is(err, errEndpointsUnreadable) {
			return false, nil
		}
		return ready, err
	}

	cmdEndpoints := g.kubectl("get", "endpoints", "kueue-webhook-service", "-n", "kueue-system", "-o", "json")
	if cmdEndpoints.ExitCode != 0 {
		errStr := strings.ToLower(cmdEndpoints.Stderr)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to read kueue-webhook-service endpoints in kueue-system (403 Forbidden). Skipping endpoint readiness check.")
			return true, nil
		}
		return false, nil
	}

	var eps struct {
		Subsets []struct {
			Addresses []struct {
				Ip string `json:"ip"`
			} `json:"addresses"`
		} `json:"subsets"`
	}
	if err := json.Unmarshal([]byte(cmdEndpoints.Stdout), &eps); err != nil {
		return false, fmt.Errorf("failed to unmarshal endpoints json: %w", err)
	}
	for _, subset := range eps.Subsets {
		if len(subset.Addresses) > 0 {
			return true, nil
		}
	}
	return false, nil
}

// -----------------------------------------------------------------------------
// 4. Kueue Priority Classes
// -----------------------------------------------------------------------------

func (g *GKEOrchestrator) getClusterPriorityClasses() ([]string, error) {
	res := g.kubectl("get", "priorityclass", "-o", "jsonpath={.items[*].metadata.name}")
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("failed to list priority classes: %s", res.Stderr)
	}
	return strings.Fields(res.Stdout), nil
}

func hasUserPriorityClasses(existing []string) bool {
	for _, name := range existing {
		if strings.HasPrefix(name, "system-") || strings.HasPrefix(name, "gke-") {
			continue
		}
		logging.Info("Pre-existing PriorityClass '%s' found. Skipping default PriorityClass installation.", name)
		return true
	}
	return false
}

func (g *GKEOrchestrator) installPriorityClasses() error {
	logging.Info("Installing Kueue PriorityClasses...")
	tmpl, err := k8s.ParseYAMLTemplate(g.gkeCustomTemplatesPath, "priority_classes.tmpl")
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, nil); err != nil {
		return fmt.Errorf("failed to execute priority_classes template: %w", err)
	}

	logging.Info("Applying Kueue priority classes...")
	if err := g.applyManifests(buf.Bytes(), "kueue_priority_classes.yaml", ""); err != nil {
		return fmt.Errorf("failed to apply Kueue priority classes: %w", err)
	}

	return nil
}

// EnsurePriorityClassesInstalled ensures default priority classes are applied if no custom ones exist.
func (g *GKEOrchestrator) EnsurePriorityClassesInstalled() error {
	existing, err := g.getClusterPriorityClasses()
	if err != nil {
		if k8s.IsForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to list priority classes (403 Forbidden). Skipping default PriorityClass installation.")
			return nil
		}
		return err
	}
	if hasUserPriorityClasses(existing) {
		return nil
	}
	logging.Info("No user-defined PriorityClasses found. Installing defaults...")
	return g.installPriorityClasses()
}

func (g *GKEOrchestrator) ensureAndValidatePriorityClass(requested string) error {
	if requested == "" {
		return nil
	}
	existing, err := g.getClusterPriorityClasses()
	if err != nil {
		if k8s.IsForbiddenError(err) {
			logging.Warn("Insufficient RBAC permissions to list priority classes (403 Forbidden). Skipping PriorityClass install/validation for %q.", requested)
			return nil
		}
		return err
	}
	if !hasUserPriorityClasses(existing) {
		logging.Info("No user-defined PriorityClasses found. Installing defaults...")
		if err := g.installPriorityClasses(); err != nil {
			return err
		}
		if existing, err = g.getClusterPriorityClasses(); err != nil {
			return err
		}
	}
	if !slices.Contains(existing, requested) {
		return fmt.Errorf("priority class %q does not exist in the cluster. Available priority classes are: %v", requested, existing)
	}
	return nil
}

// -----------------------------------------------------------------------------
// 5. Kueue ResourceFlavors & Capacity Calculations
// -----------------------------------------------------------------------------

// EnsureResourceFlavors ensures that Kueue ResourceFlavors exist for all cluster capacity flavors.
func (g *GKEOrchestrator) EnsureResourceFlavors() error {
	logging.Info("Ensuring Kueue ResourceFlavors exist...")
	cmd := g.kubectl("get", "resourceflavor", "-o", "jsonpath={.items[*].metadata.name}")
	existingFlavors := make(map[string]bool)
	if cmd.ExitCode != 0 {
		errStr := strings.ToLower(cmd.Stderr + " " + cmd.Stdout)
		if strings.Contains(errStr, "forbidden") {
			logging.Warn("Insufficient RBAC permissions to list resource flavors (403 Forbidden). Skipping ResourceFlavor verification and creation.")
			return nil
		}
		if strings.Contains(errStr, "not found") || strings.Contains(errStr, "notfound") || strings.Contains(errStr, "no resources found") {
			logging.Info("No ResourceFlavors found.")
		} else {
			return fmt.Errorf("failed to list resource flavors: %s", cmd.Stderr)
		}
	}

	for _, f := range strings.Fields(cmd.Stdout) {
		existingFlavors[f] = true
	}

	names := slices.Sorted(maps.Keys(g.capacity.Flavors))
	for _, name := range names {
		fc := g.capacity.Flavors[name]
		if existingFlavors[name] {
			continue
		}
		logging.Info("Ensuring ResourceFlavor '%s'...", name)
		rfBytes, err := g.renderResourceFlavor(name, fc.NodeLabels)
		if err != nil {
			return fmt.Errorf("failed to render ResourceFlavor %s: %w", name, err)
		}
		if err := g.applyManifests(rfBytes, fmt.Sprintf("resource-flavor-%s.yaml", name), ""); err != nil {
			return fmt.Errorf("failed to apply ResourceFlavor %s: %w", name, err)
		}
	}
	return nil
}

func (g *GKEOrchestrator) renderResourceFlavor(name string, nodeLabels map[string]string) ([]byte, error) {
	rfMap := map[string]interface{}{
		"apiVersion": "kueue.x-k8s.io/" + kueueAPIVersion,
		"kind":       "ResourceFlavor",
		"metadata": map[string]interface{}{
			"name": name,
		},
	}
	if len(nodeLabels) > 0 {
		filteredLabels := make(map[string]string)
		// Omit instance-specific slice and topology labels to keep ResourceFlavor generic across TPU node pools.
		for k, v := range nodeLabels {
			if k != tpuTopologyLabel &&
				!strings.HasPrefix(k, "cloud.google.com/gke-tpu-slice-") &&
				!strings.HasPrefix(k, "cloud.google.com/gke-tpu-partition-") {
				filteredLabels[k] = v
			}
		}
		if len(filteredLabels) > 0 {
			rfMap["spec"] = map[string]interface{}{
				"nodeLabels": filteredLabels,
			}
		}
	}
	return yaml.Marshal(rfMap)
}

func (g *GKEOrchestrator) installKueueResources(cqName string, lqName string) error {
	logging.Info("Installing Kueue resources (ClusterQueue, LocalQueue)...")

	if err := g.EnsureResourceFlavors(); err != nil {
		return err
	}

	if err := g.EnsurePriorityClassesInstalled(); err != nil {
		return err
	}

	// Install ClusterQueue
	clusterQueueBytes, err := g.renderClusterQueue(cqName)
	if err != nil {
		return err
	}
	if err := g.applyManifests(clusterQueueBytes, clusterQueueManifestFile, ""); err != nil {
		return err
	}

	// Install LocalQueue
	localQueueBytes, err := g.renderLocalQueueInNamespace(lqName, cqName, "default")
	if err != nil {
		return err
	}
	if err := g.applyManifests(localQueueBytes, localQueueManifestFile, ""); err != nil {
		return err
	}

	logging.Info("Kueue resources installed successfully.")
	return nil
}

func (g *GKEOrchestrator) renderLocalQueueInNamespace(lqName, cqName, ns string) ([]byte, error) {
	localQueueTmpl, err := k8s.ParseYAMLTemplate(g.gkeCustomTemplatesPath, localQueueTemplateFile)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", localQueueTemplateFile, err)
	}
	var localQueueBuf bytes.Buffer
	if err := localQueueTmpl.Execute(&localQueueBuf, struct {
		Namespace        string
		LocalQueueName   string
		ClusterQueueName string
	}{ns, lqName, cqName}); err != nil {
		return nil, fmt.Errorf("failed to execute %s template: %w", localQueueTemplateFile, err)
	}
	return localQueueBuf.Bytes(), nil
}

func (g *GKEOrchestrator) isResourceActive(staticAmount int, napLimitKey string) bool {
	return staticAmount > 0 || (g.napEnabled && g.napLimits[napLimitKey] > 0)
}

func (g *GKEOrchestrator) isAccelActive(staticAmount int, napLimitKey string, lowerFname string, flavorSub string) bool {
	if staticAmount > 0 {
		return true
	}
	if !g.napEnabled || g.napLimits[napLimitKey] <= 0 {
		return false
	}
	if flavorSub == "tpu" {
		return isTPUFlavor(lowerFname)
	}
	if flavorSub == "nvidia" {
		return isGPUFlavor(lowerFname)
	}
	return strings.Contains(lowerFname, flavorSub)
}

func (g *GKEOrchestrator) calculateCoveredResources() map[string]bool {
	coveredMap := make(map[string]bool)

	fnames := slices.Sorted(maps.Keys(g.capacity.Flavors))
	for _, fname := range fnames {
		fc := g.capacity.Flavors[fname]
		lowerFname := strings.ToLower(fname)
		if g.isResourceActive(fc.CPUs, "cpu") {
			coveredMap["cpu"] = true
		}
		if g.isResourceActive(fc.MemoryGi, "memory") {
			coveredMap["memory"] = true
		}
		if fname != "pathways-flavor" {
			if g.isAccelActive(fc.GPUs, "nvidia.com/gpu", lowerFname, "nvidia") {
				coveredMap["nvidia.com/gpu"] = true
			}
			if g.isAccelActive(fc.TPUs, "google.com/tpu", lowerFname, "tpu") {
				coveredMap["google.com/tpu"] = true
			}
		}
	}
	return coveredMap
}

func isTPUFlavor(fname string) bool {
	cleanName := strings.TrimPrefix(fname, "flavor-")
	if strings.EqualFold(cleanName, "ct") {
		return false
	}
	return config.IsTPU(cleanName) || strings.Contains(strings.ToLower(fname), "tpu")
}

func isGPUFlavor(fname string) bool {
	return strings.Contains(strings.ToLower(fname), "nvidia")
}

func (g *GKEOrchestrator) getNAPNominalQuota(resName string, fname string) (interface{}, bool) {
	if !g.napEnabled {
		return nil, false
	}
	lookupKey := resName
	if resName == "nvidia.com/gpu" || resName == "google.com/tpu" {
		specificKey := strings.TrimPrefix(fname, "flavor-")
		if _, ok := g.napLimits[specificKey]; ok {
			lookupKey = specificKey
		}
	}
	limit, ok := g.napLimits[lookupKey]
	if !ok {
		return nil, false
	}
	if resName == "google.com/tpu" && !isTPUFlavor(fname) {
		return 0, true
	}
	if resName == "nvidia.com/gpu" && !isGPUFlavor(fname) {
		return 0, true
	}
	if resName == "memory" {
		return fmt.Sprintf("%dG", limit), true
	}
	return limit, true
}

func getStaticNominalQuota(resName string, fc FlavorCapacity) interface{} {
	switch resName {
	case "cpu":
		return fc.CPUs
	case "memory":
		return fmt.Sprintf("%dGi", fc.MemoryGi)
	case "nvidia.com/gpu":
		return fc.GPUs
	case "google.com/tpu":
		return fc.TPUs
	default:
		return 0
	}
}

func (g *GKEOrchestrator) getNominalQuota(resName string, fc FlavorCapacity, fname string) interface{} {
	if val, ok := g.getNAPNominalQuota(resName, fname); ok {
		return val
	}
	return getStaticNominalQuota(resName, fc)
}

func compareKueueFlavors(a, b string) int {
	if a == b {
		return 0
	}
	if a == "pathways-flavor" {
		return -1
	}
	if b == "pathways-flavor" {
		return 1
	}
	return strings.Compare(a, b)
}

func (g *GKEOrchestrator) renderClusterQueue(name string) ([]byte, error) {
	coveredResourcesMap := g.calculateCoveredResources()

	var mainFlavors []map[string]interface{}
	// Order pathways-flavor before accelerator flavors so CPU-only Pathways head pods
	// match pathways-flavor first instead of TPU flavors with unconstrained CPU/Memory quotas.
	fnames := slices.SortedFunc(maps.Keys(g.capacity.Flavors), compareKueueFlavors)

	for _, fname := range fnames {
		fc := g.capacity.Flavors[fname]
		resources := g.buildFlavorResources(coveredResourcesMap, fc, fname)
		if len(resources) > 0 {
			mainFlavors = append(mainFlavors, map[string]interface{}{
				"name":      fname,
				"resources": resources,
			})
		}
	}

	var resourceGroups []map[string]interface{}
	if rg := buildResourceGroup(coveredResourcesMap, mainFlavors); rg != nil {
		resourceGroups = append(resourceGroups, rg)
	}

	cqMap := map[string]interface{}{
		"apiVersion": "kueue.x-k8s.io/" + kueueAPIVersion,
		"kind":       "ClusterQueue",
		"metadata": map[string]interface{}{
			"name": name,
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{},
			"queueingStrategy":  "BestEffortFIFO",
			"resourceGroups":    resourceGroups,
		},
	}
	return yaml.Marshal(cqMap)
}

func (g *GKEOrchestrator) buildFlavorResources(coveredMap map[string]bool, fc FlavorCapacity, fname string) []map[string]interface{} {
	resList := slices.Sorted(maps.Keys(coveredMap))

	var resources []map[string]interface{}
	_, hasPathways := g.capacity.Flavors["pathways-flavor"]
	for _, resName := range resList {
		quota := g.getNominalQuota(resName, fc, fname)
		// For Pathways TPU flavors, set CPU and Memory nominal quotas to unconstrained values (999999)
		// so secondary resource limits do not block Pathways helper containers from co-scheduling with TPUs.
		if hasPathways && isTPUFlavor(fname) {
			switch resName {
			case "cpu":
				quota = pathwaysCPUNominalQuota
			case "memory":
				quota = pathwaysMemoryNominalQuota
			}
		}
		resources = append(resources, map[string]interface{}{
			"name":         resName,
			"nominalQuota": quota,
		})
	}
	return resources
}

func buildResourceGroup(coveredMap map[string]bool, flavors []map[string]interface{}) map[string]interface{} {
	if len(flavors) == 0 {
		return nil
	}
	coveredResources := slices.Sorted(maps.Keys(coveredMap))

	return map[string]interface{}{
		"coveredResources": coveredResources,
		"flavors":          flavors,
	}
}
