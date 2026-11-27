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

package gke

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/validation"
	k8syaml "sigs.k8s.io/yaml"
)

const (
	defaultSliceTeardownTimeout  = 120 * time.Second
	defaultSliceTeardownInterval = 5 * time.Second
	sliceTeardownLogInterval     = 30 * time.Second
	maxTPUSlicePrefixLen         = 35
	tpuCommContainerPort         = 8471
)

func (g *GKEOrchestrator) tpuSliceEnvOverride() (enabled, overridden bool) {
	override, err := k8s.ResolveLabelSchemeOverride(g.getenv())
	if err != nil {
		return false, false
	}
	switch override {
	case k8s.SchemeGeneric, k8s.SchemeGKE:
		return false, true
	case k8s.SchemeTPUSlice:
		return true, true
	default:
		return false, false
	}
}

// hasTPUSliceAPI reports whether the active Kubernetes cluster serves slice.tpu.google.com.
// GKE-mode clusters skip live API probing unless a pre-populated inventory indicates SchemeTPUSlice.
func (g *GKEOrchestrator) hasTPUSliceAPI() bool {
	if g.inventory != nil {
		return g.inventory.TPUSliceAPIPresent || g.inventory.LabelScheme == k8s.SchemeTPUSlice
	}
	if g.mode != orchestrator.TypeK8s || g.executor == nil || strings.TrimSpace(g.kubeContext) == "" {
		return false
	}
	if enabled, overridden := g.tpuSliceEnvOverride(); overridden {
		return enabled
	}
	if g.tpuSliceAPIChecked && g.tpuSliceAPIContext == g.kubeContext {
		return g.tpuSliceAPIPresent
	}
	present, err := g.HasAPIGroupVersion(k8s.TPUSliceAPIGroupVersion)
	if err == nil {
		g.tpuSliceAPIChecked = true
		g.tpuSliceAPIContext = g.kubeContext
		g.tpuSliceAPIPresent = present
	}
	return err == nil && present
}

// shouldEnableTPUCommPort reports whether TPU containers should declare containerPort 8471 (tpu-comm).
func (g *GKEOrchestrator) shouldEnableTPUCommPort(isTPU, isPathways bool) bool {
	return g.mode == orchestrator.TypeK8s && g.labelScheme() == k8s.SchemeTPUSlice && isTPU && !isPathways
}

func hasSliceNameConstraint(nodeConstraint map[string]string) bool {
	return strings.TrimSpace(nodeConstraint[k8s.TPUSliceNameLabel]) != ""
}

func hasSliceNameConstraintKey(nodeConstraint map[string]string) bool {
	_, ok := nodeConstraint[k8s.TPUSliceNameLabel]
	return ok
}

func validateSliceNameConstraintNotEmpty(nodeConstraint map[string]string) error {
	if rawVal, hasKey := nodeConstraint[k8s.TPUSliceNameLabel]; hasKey && strings.TrimSpace(rawVal) == "" {
		return fmt.Errorf("invalid node constraint for key %s: value must not be empty", k8s.TPUSliceNameLabel)
	}
	return nil
}

// isDynamicTPUSliceWorkload reports whether job is a dynamically provisioned TPU JobSet on a
// SchemeTPUSlice cluster (non-Pathways, JobSet enabled, and no static slice-name constraint).
func (g *GKEOrchestrator) isDynamicTPUSliceWorkload(job orchestrator.JobDefinition, isTPU bool) bool {
	return g.mode == orchestrator.TypeK8s &&
		g.labelScheme() == k8s.SchemeTPUSlice &&
		isTPU &&
		!job.IsPathwaysJob &&
		!hasSliceNameConstraint(job.NodeConstraint) &&
		g.jobSetEnabled()
}

// validateDynamicTPUSliceWorkloadName enforces slice-provisioner's 35-character slice-prefix cap
// for dynamically provisioned TPU JobSets before image building or manifest rendering.
func (g *GKEOrchestrator) validateDynamicTPUSliceWorkloadName(job orchestrator.JobDefinition, isTPU bool) error {
	if !g.isDynamicTPUSliceWorkload(job, isTPU) {
		return nil
	}
	prefix := strings.TrimSpace(job.WorkloadName)
	if len(prefix) > maxTPUSlicePrefixLen {
		return fmt.Errorf("workload name %q (%d chars) exceeds the %d-character limit for %s on tpu-slice JobSets; shorten --name",
			prefix, len(prefix), maxTPUSlicePrefixLen, k8s.TPUSlicePrefixLabel)
	}
	return nil
}

// buildTPUSliceWorkloadLabels renders top-level JobSet.metadata.labels required by slice-provisioner
// for dynamically provisioned TPU JobSets (tpu.google.com/slice-prefix, for both single- and multi-slice).
func (g *GKEOrchestrator) buildTPUSliceWorkloadLabels(job orchestrator.JobDefinition, isTPU bool) (string, error) {
	if err := g.validateDynamicTPUSliceWorkloadName(job, isTPU); err != nil {
		return "", err
	}
	if !g.isDynamicTPUSliceWorkload(job, isTPU) {
		return "", nil
	}
	prefix := strings.TrimSpace(job.WorkloadName)
	if prefix == "" {
		return "", nil
	}
	labels := map[string]string{
		k8s.TPUSlicePrefixLabel: prefix,
	}
	b, err := k8syaml.Marshal(labels)
	if err != nil {
		return "", fmt.Errorf("failed to marshal TPU slice workload labels: %w", err)
	}
	return strings.TrimRight(string(b), "\n"), nil
}

// buildTPUSlicePodLabels renders the tpu.google.com/{type,topology} pod-template labels required by
// slice-provisioner / slice-scheduler on SchemeTPUSlice clusters (empty for non-TPU or non-tpu-slice jobs).
// Dynamic JobSets receive tpu.google.com/slice-prefix on JobSet.metadata.labels instead of pod labels;
// static slice reuse relies on nodeSelector/nodeAffinity and never emits slice-name as a pod label.
func (g *GKEOrchestrator) buildTPUSlicePodLabels(job orchestrator.JobDefinition, schedOpts SchedulingOptions, profile JobProfile) (string, error) {
	if g.mode != orchestrator.TypeK8s || g.labelScheme() != k8s.SchemeTPUSlice || !schedOpts.IsTPU || job.IsPathwaysJob {
		return "", nil
	}
	labels := make(map[string]string, 2)
	if tpuType := g.resolveTPUSliceTypeLabel(job, profile); tpuType != "" {
		labels[k8s.TPUSliceTypeLabel] = tpuType
	}
	if topo := strings.TrimSpace(schedOpts.Topology); topo != "" {
		labels[k8s.TPUSliceTopologyLabel] = topo
	}
	if len(labels) == 0 {
		return "", nil
	}
	b, err := k8syaml.Marshal(labels)
	if err != nil {
		return "", fmt.Errorf("failed to marshal TPU slice pod labels: %w", err)
	}
	return strings.TrimRight(string(b), "\n"), nil
}

func resolveWorkloadSchedulerName(explicitScheduler string, dynamicTPUSlice bool) string {
	if s := strings.TrimSpace(explicitScheduler); s != "" {
		return s
	}
	if dynamicTPUSlice {
		return k8s.TPUSliceSchedulerName
	}
	return ""
}

func (g *GKEOrchestrator) resolveTPUSliceTypeLabel(job orchestrator.JobDefinition, profile JobProfile) string {
	if tpuType := g.resolveTPUSliceTypeFromTarget(job, profile); tpuType != "" {
		return tpuType
	}
	rawType := cmp.Or(strings.TrimSpace(job.ComputeType), strings.TrimSpace(job.MachineType))
	if defaultTPUHardwareResolver.IsTPUMachine(job.MachineType) {
		rawType = job.MachineType
	}
	jobType := defaultTPUHardwareResolver.NormalizeTPUType(rawType)
	if invType := g.inventoryTPUType(); invType != "" && g.matchesPrimaryInventoryTPU(jobType, cmp.Or(job.MachineType, job.ComputeType)) {
		return invType
	}
	return jobType
}

func (g *GKEOrchestrator) matchesPrimaryInventoryTPU(jobType, machineType string) bool {
	if g.inventory == nil || g.inventory.TPU == nil {
		return false
	}
	reqMachine := strings.TrimSpace(machineType)
	if jobType == "" && reqMachine == "" {
		return true
	}
	invType := strings.TrimSpace(g.inventory.TPU.TPUType)
	if invType != "" && strings.EqualFold(jobType, invType) {
		return true
	}
	invMachine := strings.TrimSpace(g.inventory.TPU.MachineType)
	return invMachine != "" && strings.EqualFold(cmp.Or(reqMachine, strings.TrimSpace(jobType)), invMachine)
}

func (g *GKEOrchestrator) resolveTPUSliceTypeFromTarget(job orchestrator.JobDefinition, profile JobProfile) string {
	targetSlices, _ := g.findTargetSlices(job.NodeConstraint)
	for _, s := range targetSlices {
		if s.TPUType != "" {
			return s.TPUType
		}
	}
	if profile.K8sPool != nil && profile.K8sPool.TPUType != "" {
		return profile.K8sPool.TPUType
	}
	if pool, ok := g.findK8sPoolForRequest(job.ComputeType, job.MachineType); ok && pool.TPUType != "" {
		return pool.TPUType
	}
	return ""
}

func (g *GKEOrchestrator) inventoryTPUType() string {
	if g.inventory == nil || g.inventory.TPU == nil {
		return ""
	}
	return g.inventory.TPU.TPUType
}

// buildTPUSliceGangAnnotations returns multi-slice gang scheduling annotations for slice-scheduler
// when submitting a dynamically provisioned multi-slice TPU JobSet on a SchemeTPUSlice cluster.
func buildTPUSliceGangAnnotations(workloadName string, numSlices int, dynamicTPUSlice bool) map[string]string {
	workloadName = strings.TrimSpace(workloadName)
	if !dynamicTPUSlice || numSlices <= 1 || workloadName == "" {
		return nil
	}
	return map[string]string{
		k8s.TPUNumSlicesAnnotation:      strconv.Itoa(numSlices),
		k8s.TPUSliceGroupNameAnnotation: workloadName,
	}
}

// fetchWorkloadSlices queries slice.tpu.google.com Slice CRs owned by workloadName (scoped to ns
// when owner-namespace is present), falling back to static slice lookup when the workload targets
// pre-existing slice(s) via nodeSelector or nodeAffinity.
// Standalone batch/v1 Jobs never own dynamically provisioned Slices and resolve static slices only.
// Returns the discovered slices and any static slice names referenced by the workload.
func (g *GKEOrchestrator) fetchWorkloadSlices(ns, workloadName string, isBatchJob bool) ([]k8s.SliceInfo, []string) {
	workloadName = strings.TrimSpace(workloadName)
	if workloadName == "" {
		return nil, nil
	}
	if isBatchJob {
		staticNames := g.resolveWorkloadStaticSliceNames(ns, workloadName, true)
		if len(staticNames) == 0 {
			return nil, nil
		}
		return g.fetchSlicesIndividually(staticNames), staticNames
	}
	var allSlices []k8s.SliceInfo
	listOK := false
	res := g.kubectl("get", k8s.TPUSliceResource, "-o", "json", kubectlReadTimeout)
	if res.ExitCode == 0 {
		if parsed, err := k8s.ParseTPUSliceListJSON(res.Stdout, defaultTPUHardwareResolver); err == nil {
			allSlices = parsed
			listOK = true
			if owned := k8s.FilterOwnedTPUSlices(allSlices, ns, workloadName); len(owned) > 0 {
				return owned, nil
			}
		}
	}

	staticNames := g.resolveWorkloadStaticSliceNames(ns, workloadName, false)
	if len(staticNames) == 0 {
		return nil, nil
	}
	if listOK {
		return matchSlicesByNames(allSlices, staticNames), staticNames
	}
	return g.fetchSlicesIndividually(staticNames), staticNames
}

func matchSlicesByNames(all []k8s.SliceInfo, names []string) []k8s.SliceInfo {
	byName := make(map[string]k8s.SliceInfo, len(all))
	for _, s := range all {
		byName[s.Name] = s
	}
	var matched []k8s.SliceInfo
	for _, name := range names {
		if s, ok := byName[name]; ok {
			matched = append(matched, s)
		}
	}
	return matched
}

func (g *GKEOrchestrator) fetchSlicesIndividually(names []string) []k8s.SliceInfo {
	var matched []k8s.SliceInfo
	for _, name := range names {
		res := g.kubectl("get", k8s.TPUSliceResource, "-o", "json", kubectlReadTimeout, "--", name)
		if res.ExitCode != 0 {
			if rawErr := kubectlOutputError(res); rawErr != nil && k8s.IsForbiddenError(rawErr) {
				return matched
			}
			continue
		}
		if s, err := k8s.ParseTPUSliceJSON(res.Stdout, defaultTPUHardwareResolver); err == nil {
			matched = append(matched, s)
		}
	}
	return matched
}

func (g *GKEOrchestrator) resolveWorkloadStaticSliceNames(ns, workloadName string, isBatchJob bool) []string {
	resource := "jobsets.jobset.x-k8s.io"
	if isBatchJob {
		resource = "job"
	}
	args := []string{"get", resource}
	if ns = strings.TrimSpace(ns); ns != "" {
		args = append(args, "-n", ns)
	}
	args = append(args, "-o", "json", kubectlReadTimeout, "--", workloadName)
	res := g.kubectl(args...)
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return nil
	}
	return extractStaticSliceNamesFromWorkloadJSON(res.Stdout, isBatchJob)
}

func extractStaticSliceNamesFromWorkloadJSON(raw string, isBatchJob bool) []string {
	var podSpecs []*corev1.PodSpec
	if isBatchJob {
		var jobObj batchv1.Job
		if err := json.Unmarshal([]byte(raw), &jobObj); err != nil {
			return nil
		}
		podSpecs = append(podSpecs, &jobObj.Spec.Template.Spec)
	} else {
		var jsObj jobSetValidationSpec
		if err := json.Unmarshal([]byte(raw), &jsObj); err != nil {
			return nil
		}
		for i := range jsObj.Spec.ReplicatedJobs {
			podSpecs = append(podSpecs, &jsObj.Spec.ReplicatedJobs[i].Template.Spec.Template.Spec)
		}
	}
	return collectStaticSliceNames(podSpecs)
}

func collectStaticSliceNames(podSpecs []*corev1.PodSpec) []string {
	seen := make(map[string]bool)
	var names []string
	add := func(raw string) {
		name := strings.TrimSpace(raw)
		if name == "" || seen[name] || len(validation.IsDNS1123Subdomain(name)) > 0 {
			return
		}
		seen[name] = true
		names = append(names, name)
	}
	for _, spec := range podSpecs {
		if spec == nil {
			continue
		}
		add(spec.NodeSelector[k8s.TPUSliceNameLabel])
		addAffinityStaticSliceNames(spec.Affinity, add)
	}
	return names
}

func addAffinityStaticSliceNames(affinity *corev1.Affinity, add func(string)) {
	if affinity == nil || affinity.NodeAffinity == nil || affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return
	}
	for _, term := range affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Key == k8s.TPUSliceNameLabel && expr.Operator == corev1.NodeSelectorOpIn {
				for _, v := range expr.Values {
					add(v)
				}
			}
		}
	}
}

func (g *GKEOrchestrator) fetchSliceEvents(sliceList []k8s.SliceInfo) string {
	wanted := wantedSliceNames(sliceList)
	if len(wanted) == 0 {
		return ""
	}
	res := g.kubectl("get", "events", "-n", "default",
		"--field-selector=involvedObject.kind=Slice",
		"-o", "json",
		kubectlReadTimeout)
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) == "" {
		return ""
	}
	return k8s.FilterSliceEventsJSON(res.Stdout, wanted)
}

func wantedSliceNames(sliceList []k8s.SliceInfo) map[string]bool {
	wanted := make(map[string]bool, len(sliceList))
	for _, s := range sliceList {
		if s.Name != "" {
			wanted[s.Name] = true
		}
	}
	return wanted
}

// formatWorkloadSliceDiagnostics returns formatted Slice readiness and events for a workload,
// or "" if slice.tpu.google.com is absent or no associated Slice CRs exist.
func (g *GKEOrchestrator) formatWorkloadSliceDiagnostics(ns, workloadName string, isBatchJob bool) string {
	return g.formatSliceDiagnostics(ns, workloadName, isBatchJob, false)
}

// formatPendingSliceDiagnostics returns Slice readiness and events only when at least one
// associated Slice CR is not Ready (for example waiting on TPU capacity, gang completion, or node labeling).
func (g *GKEOrchestrator) formatPendingSliceDiagnostics(ns, workloadName string, isBatchJob bool) string {
	return g.formatSliceDiagnostics(ns, workloadName, isBatchJob, true)
}

func (g *GKEOrchestrator) formatSliceDiagnostics(ns, workloadName string, isBatchJob, onlyIfNotReady bool) string {
	if !g.hasTPUSliceAPI() {
		return ""
	}
	sliceList, _ := g.fetchWorkloadSlices(ns, workloadName, isBatchJob)
	if len(sliceList) == 0 {
		return ""
	}
	if onlyIfNotReady && allSlicesReady(sliceList) {
		return ""
	}
	var parts []string
	if summary := k8s.FormatSliceReadiness(sliceList); summary != "" {
		parts = append(parts, summary)
	}
	if events := g.fetchSliceEvents(sliceList); events != "" {
		parts = append(parts, "Slice events:\n"+events)
	}
	return strings.Join(parts, "\n")
}

func allSlicesReady(sliceList []k8s.SliceInfo) bool {
	for _, s := range sliceList {
		if !s.IsSchedulable() {
			return false
		}
	}
	return true
}

func parseExpectedYAMLMap(raw, fieldDesc string) (map[string]string, error) {
	if raw == "" {
		return nil, nil
	}
	var expected map[string]string
	if err := k8syaml.Unmarshal([]byte(raw), &expected); err != nil {
		return nil, fmt.Errorf("failed to parse expected %s: %w", fieldDesc, err)
	}
	return expected, nil
}

func validateTPUSliceManifest(manifestContent string, contract tpuSliceManifestContract, hint string) error {
	if contract.workloadLabelsYAML == "" && contract.podLabelsYAML == "" && !contract.commPortEnabled && !contract.dynamicTPUSlice {
		return nil
	}
	expectedWorkloadLabels, err := parseExpectedYAMLMap(contract.workloadLabelsYAML, "TPU slice workload labels")
	if err != nil {
		return err
	}
	expectedPodLabels, err := parseExpectedYAMLMap(contract.podLabelsYAML, "TPU slice pod labels")
	if err != nil {
		return err
	}
	var expectedAnnots map[string]string
	if contract.dynamicTPUSlice {
		expectedAnnots, err = parseExpectedYAMLMap(contract.topologyAnnotYAML, "TPU slice gang annotations")
		if err != nil {
			return err
		}
	}
	return forEachWorkloadObject(manifestContent, func(obj workloadManifestObject) error {
		prefix := "Job pod template"
		if obj.kind == "JobSet" {
			prefix = fmt.Sprintf("replicatedJob %q", obj.jobName)
		}
		if err := validateWorkloadTPUSliceLabels(obj.kind, hint, obj.objectMeta.Labels, expectedWorkloadLabels, contract.dynamicTPUSlice); err != nil {
			return err
		}
		if contract.dynamicTPUSlice {
			if err := validateDynamicTPUSliceContract(obj, prefix, hint, contract.schedulerName, expectedAnnots); err != nil {
				return err
			}
		}
		return validatePodTPUSliceContract(prefix, hint, expectedPodLabels, contract.commPortEnabled, obj.tmpl)
	})
}

func validateWorkloadTPUSliceLabels(kind, hint string, objectLabels, expected map[string]string, dynamicTPUSlice bool) error {
	for _, k := range slices.Sorted(maps.Keys(expected)) {
		if want := expected[k]; objectLabels[k] != want {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s metadata.labels is missing required TPU slice label %s=%q", kind, k, want), hint))
		}
	}
	for _, forbiddenKey := range []string{k8s.TPUSliceNameLabel, k8s.TPUSliceSchedulerNameLabel} {
		if val := strings.TrimSpace(objectLabels[forbiddenKey]); val != "" {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s metadata.labels must not set %s (%q) on tpu-slice workloads", kind, forbiddenKey, val), hint))
		}
	}
	if !dynamicTPUSlice {
		for _, forbiddenKey := range []string{k8s.TPUSlicePrefixLabel, k8s.TPUSliceSchedulerPrefixLabel} {
			if val := strings.TrimSpace(objectLabels[forbiddenKey]); val != "" {
				return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s metadata.labels must not set %s (%q) when reusing a static TPU slice", kind, forbiddenKey, val), hint))
			}
		}
	}
	return nil
}

func validateDynamicTPUSliceContract(obj workloadManifestObject, prefix, hint, schedulerName string, expectedAnnots map[string]string) error {
	if err := validateNoDynamicExclusiveTopology(obj.kind, prefix, hint, obj.objectMeta.Annotations, obj.jobAnnotations); err != nil {
		return err
	}
	if hasPodSliceNameSelector(obj.tmpl.Spec) {
		return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s must not target %s in nodeSelector or nodeAffinity on dynamically provisioned TPU workloads", prefix, k8s.TPUSliceNameLabel), hint))
	}
	if schedulerName != "" && obj.tmpl.Spec.SchedulerName != schedulerName {
		return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s is missing required schedulerName %q (got %q)", prefix, schedulerName, obj.tmpl.Spec.SchedulerName), hint))
	}
	for _, k := range slices.Sorted(maps.Keys(expectedAnnots)) {
		if want := expectedAnnots[k]; obj.tmpl.Annotations[k] != want {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s is missing required TPU slice gang annotation %s=%q", prefix, k, want), hint))
		}
	}
	return nil
}

func validateNoDynamicExclusiveTopology(kind, prefix, hint string, objectAnnots, jobAnnots map[string]string) error {
	if kind != "JobSet" {
		return nil
	}
	const key = "alpha.jobset.sigs.k8s.io/exclusive-topology"
	if val := strings.TrimSpace(objectAnnots[key]); val != "" {
		return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("JobSet metadata.annotations must not set %s (%q) on dynamically provisioned TPU workloads", key, val), hint))
	}
	if val := strings.TrimSpace(jobAnnots[key]); val != "" {
		return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s template.metadata.annotations must not set %s (%q) on dynamically provisioned TPU workloads", prefix, key, val), hint))
	}
	return nil
}

func hasPodSliceNameSelector(spec corev1.PodSpec) bool {
	if _, ok := spec.NodeSelector[k8s.TPUSliceNameLabel]; ok {
		return true
	}
	if spec.Affinity == nil || spec.Affinity.NodeAffinity == nil || spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		return false
	}
	for _, term := range spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms {
		for _, expr := range term.MatchExpressions {
			if expr.Key == k8s.TPUSliceNameLabel {
				return true
			}
		}
	}
	return false
}

func validatePodTPUSliceContract(prefix, hint string, expectedLabels map[string]string, commPortEnabled bool, tmpl *corev1.PodTemplateSpec) error {
	for _, forbiddenKey := range []string{k8s.TPUSliceNameLabel, k8s.TPUSliceSchedulerNameLabel, k8s.TPUSlicePrefixLabel, k8s.TPUSliceSchedulerPrefixLabel} {
		if val := strings.TrimSpace(tmpl.Labels[forbiddenKey]); val != "" {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s must not set pod label %s (%q) on tpu-slice workloads", prefix, forbiddenKey, val), hint))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(expectedLabels)) {
		if want := expectedLabels[k]; tmpl.Labels[k] != want {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s is missing required TPU slice pod label %s=%q", prefix, k, want), hint))
		}
	}
	if !commPortEnabled {
		return nil
	}
	if !hasContainerPort(tmpl.Spec.Containers, tpuCommContainerPort) {
		return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("%s is missing required TPU communication containerPort %d", prefix, tpuCommContainerPort), hint))
	}
	return nil
}

func hasContainerPort(containers []corev1.Container, port int32) bool {
	for _, c := range containers {
		for _, p := range c.Ports {
			if p.ContainerPort == port {
				return true
			}
		}
	}
	return false
}

// jobSetMayOwnTPUSlices inspects a pre-delete JobSet to determine whether it could own dynamically
// provisioned slice.tpu.google.com Slice CRs. Returning false for CPU, GPU, GKE-labeled, and static-reuse
// JobSets avoids unnecessary cluster-scoped Slice lists and spurious 403 RBAC warnings on non-TPU clusters.
func jobSetMayOwnTPUSlices(js *unstructured.Unstructured) bool {
	if js == nil {
		return true
	}
	if hasSlicePrefixLabel(js.GetLabels()) {
		return true
	}
	for _, f := range js.GetFinalizers() {
		if f == k8s.TPUSliceLegacyFinalizer || f == k8s.TPUSliceSchedulerFinalizer {
			return true
		}
	}
	raw, err := json.Marshal(js.Object)
	if err != nil {
		return true
	}
	var spec jobSetValidationSpec
	if err := json.Unmarshal(raw, &spec); err != nil {
		return true
	}
	for i := range spec.Spec.ReplicatedJobs {
		if podTemplateMayOwnTPUSlices(&spec.Spec.ReplicatedJobs[i].Template.Spec.Template) {
			return true
		}
	}
	return false
}

func podTemplateMayOwnTPUSlices(tmpl *corev1.PodTemplateSpec) bool {
	if tmpl == nil {
		return false
	}
	if hasSlicePrefixLabel(tmpl.Labels) {
		return true
	}
	if len(collectStaticSliceNames([]*corev1.PodSpec{&tmpl.Spec})) > 0 {
		return false
	}
	return tmpl.Spec.SchedulerName == k8s.TPUSliceSchedulerName || strings.TrimSpace(tmpl.Labels[k8s.TPUSliceTypeLabel]) != ""
}

func hasSlicePrefixLabel(labels map[string]string) bool {
	return strings.TrimSpace(labels[k8s.TPUSlicePrefixLabel]) != "" ||
		strings.TrimSpace(labels[k8s.TPUSliceSchedulerPrefixLabel]) != ""
}

func (g *GKEOrchestrator) canPollTPUSliceTeardown(workloadName string) bool {
	if g.kubeClient == nil || strings.TrimSpace(workloadName) == "" {
		return false
	}
	if g.mode == orchestrator.TypeK8s {
		if enabled, overridden := g.tpuSliceEnvOverride(); overridden {
			return enabled
		}
		return true
	}
	return g.inventory != nil && (g.inventory.TPUSliceAPIPresent || g.inventory.LabelScheme == k8s.SchemeTPUSlice)
}

func (g *GKEOrchestrator) sliceTeardownConfig() (time.Duration, time.Duration, func(time.Duration)) {
	timeout := g.sliceTeardownTimeout
	if timeout <= 0 {
		timeout = defaultSliceTeardownTimeout
	}
	interval := g.sliceTeardownInterval
	if interval <= 0 {
		interval = defaultSliceTeardownInterval
	}
	sleepFn := g.sleepFunc
	if sleepFn == nil {
		sleepFn = time.Sleep
	}
	return timeout, interval, sleepFn
}

func (g *GKEOrchestrator) teardownRemaining(deadline time.Time, timeout, simulatedElapsed time.Duration) time.Duration {
	if g.sleepFunc != nil {
		return timeout - simulatedElapsed
	}
	return time.Until(deadline)
}

func (g *GKEOrchestrator) teardownElapsed(start time.Time, simulatedElapsed time.Duration) time.Duration {
	if g.sleepFunc != nil {
		return simulatedElapsed
	}
	return time.Since(start)
}

// waitForOwnedSlicesTeardown waits (bounded, default 120s) after deleting a JobSet for any
// dynamically provisioned slice.tpu.google.com Slice CRs owned by the workload to be reclaimed
// by slice-provisioner's finalizer. Uses g.kubeClient directly (zero subprocesses) and never
// deletes Slice CRs directly.
func (g *GKEOrchestrator) waitForOwnedSlicesTeardown(ns, workloadName string) {
	if !g.canPollTPUSliceTeardown(workloadName) {
		return
	}
	timeout, interval, sleepFn := g.sliceTeardownConfig()
	start := time.Now()
	deadline := start.Add(timeout)
	var simulatedElapsed, lastLogElapsed time.Duration
	var lastNamesKey string
	sawSlices, transientWarned := false, false
	for {
		sliceList, err := k8s.ListTPUSlicesByOwner(g.kubeClient, ns, workloadName, defaultTPUHardwareResolver)
		if err != nil && isFatalSliceTeardownError(workloadName, err) {
			return
		}
		if err == nil && len(sliceList) == 0 {
			if sawSlices {
				logging.Info("TPU Slice(s) for job '%s' released successfully.", workloadName)
			}
			return
		}
		remaining := g.teardownRemaining(deadline, timeout, simulatedElapsed)
		if err != nil {
			if !handleTransientSliceTeardownError(workloadName, timeout, remaining, err, &transientWarned) {
				return
			}
		} else {
			if !logSliceTeardownProgress(sliceList, timeout, remaining, g.teardownElapsed(start, simulatedElapsed), &sawSlices, &lastNamesKey, &lastLogElapsed) {
				return
			}
		}
		step := min(interval, remaining)
		simulatedElapsed += step
		sleepFn(step)
	}
}

func handleTransientSliceTeardownError(workloadName string, timeout, remaining time.Duration, err error, transientWarned *bool) bool {
	if remaining <= 0 {
		logging.Warn("Timed out after %s polling TPU Slice teardown for '%s' (latest error: %v); re-submitting with the same workload name before teardown completes may conflict.", timeout, workloadName, err)
		return false
	}
	if !*transientWarned {
		logging.Warn("Transient error polling TPU Slice teardown for '%s' (%v); retrying until timeout.", workloadName, err)
		*transientWarned = true
	}
	return true
}

func logSliceTeardownProgress(sliceList []k8s.SliceInfo, timeout, remaining, elapsed time.Duration, sawSlices *bool, lastNamesKey *string, lastLogElapsed *time.Duration) bool {
	names := strings.Join(sliceNames(sliceList), ", ")
	if remaining <= 0 {
		logging.Warn("Timed out after %s waiting for TPU Slice(s) (%s) to be deleted; re-submitting with the same workload name before teardown completes may conflict.", timeout, names)
		return false
	}
	if !*sawSlices || names != *lastNamesKey || elapsed-*lastLogElapsed >= sliceTeardownLogInterval {
		logging.Info("Waiting for TPU slice controller to release optical links... (%d slice(s) remaining: %s)", len(sliceList), names)
		*lastNamesKey = names
		*lastLogElapsed = elapsed
	}
	*sawSlices = true
	return true
}

func isFatalSliceTeardownError(workloadName string, err error) bool {
	if k8s.IsNotFoundOrNoMatchError(err) {
		return true
	}
	if k8s.IsForbiddenError(err) {
		logging.Warn("Insufficient RBAC permissions to monitor TPU Slice teardown for '%s' (%v); skipping optical link release wait.", workloadName, err)
		return true
	}
	return false
}

func sliceNames(sliceList []k8s.SliceInfo) []string {
	names := make([]string, 0, len(sliceList))
	for _, s := range sliceList {
		if s.Name != "" {
			names = append(names, s.Name)
		}
	}
	return names
}

func validSliceNames(sliceList []k8s.SliceInfo) []string {
	names := make([]string, 0, len(sliceList))
	for _, s := range sliceList {
		if s.Name != "" && len(validation.IsDNS1123Subdomain(s.Name)) == 0 {
			names = append(names, s.Name)
		}
	}
	return names
}
