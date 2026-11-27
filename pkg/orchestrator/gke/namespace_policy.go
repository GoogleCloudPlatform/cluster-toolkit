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
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"maps"
	"slices"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var (
	policyResources        = []corev1.ResourceName{corev1.ResourceCPU, corev1.ResourceMemory}
	quotaRequirementChecks = []quotaRequirementCheck{
		{corev1.ResourceLimitsCPU, corev1.ResourceCPU, true},
		{corev1.ResourceLimitsMemory, corev1.ResourceMemory, true},
		{corev1.ResourceRequestsCPU, corev1.ResourceCPU, false},
		{corev1.ResourceCPU, corev1.ResourceCPU, false},
		{corev1.ResourceRequestsMemory, corev1.ResourceMemory, false},
		{corev1.ResourceMemory, corev1.ResourceMemory, false},
	}
)

func (s limitRangeSummary) promotesBestEffort() bool {
	return s.known && (len(s.defaults) > 0 || len(s.defaultReqs) > 0)
}

func (s limitRangeSummary) satisfies(res corev1.ResourceName, requireLimit bool) bool {
	return s.known && (s.defaults[res] || (!requireLimit && s.defaultReqs[res]))
}

func formatPolicyNamespace(ns string) string {
	if ns == "" {
		return "the current namespace"
	}
	return fmt.Sprintf("namespace %q", ns)
}

// checkNamespaceResourcePolicies warns on namespace LimitRange/ResourceQuota conflicts for pods
// with unbounded CPU/memory; warn-only so mutating webhooks or globalDefault PriorityClasses are never blocked.
func (g *GKEOrchestrator) checkNamespaceResourcePolicies(manifestContent string) {
	pods := collectWorkloadPods(manifestContent)
	if !anyPodHasUnboundedCPUMem(pods) {
		return
	}
	ns := strings.TrimSpace(g.namespace)
	nsDesc := formatPolicyNamespace(ns)

	var lrSummary limitRangeSummary
	var warnings []string
	if lrItems, ok := fetchNamespacePolicyList[corev1.LimitRange](g, "limitrange", "LimitRangeList", ns, nsDesc); ok {
		lrSummary, warnings = evaluateLimitRanges(nsDesc, lrItems, pods)
	}
	if rqItems, ok := fetchNamespacePolicyList[corev1.ResourceQuota](g, "resourcequota", "ResourceQuotaList", ns, nsDesc); ok {
		warnings = append(warnings, evaluateResourceQuotas(nsDesc, rqItems, pods, lrSummary)...)
	}
	for _, w := range warnings {
		logging.Warn("%s", w)
	}
}

func collectWorkloadPods(manifestContent string) []*corev1.PodSpec {
	var pods []*corev1.PodSpec
	_ = forEachWorkloadJobSpec(manifestContent, func(_, _ string, _ *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error {
		specCopy := tmpl.Spec
		pods = append(pods, &specCopy)
		return nil
	})
	return pods
}

func hasCPUMemEntry(list corev1.ResourceList) bool {
	if q, ok := list[corev1.ResourceCPU]; ok && !q.IsZero() {
		return true
	}
	if q, ok := list[corev1.ResourceMemory]; ok && !q.IsZero() {
		return true
	}
	return false
}

func podHasResource(pod *corev1.PodSpec, resName corev1.ResourceName, requireLimit bool) bool {
	if pod == nil || pod.Resources == nil {
		return false
	}
	if q, ok := pod.Resources.Limits[resName]; ok && !q.IsZero() {
		return true
	}
	if !requireLimit {
		if q, ok := pod.Resources.Requests[resName]; ok && !q.IsZero() {
			return true
		}
	}
	return false
}

func podIsBestEffort(pod *corev1.PodSpec) bool {
	if pod.Resources != nil && (hasCPUMemEntry(pod.Resources.Requests) || hasCPUMemEntry(pod.Resources.Limits)) {
		return false
	}
	for _, c := range slices.Concat(pod.InitContainers, pod.Containers) {
		if hasCPUMemEntry(c.Resources.Requests) || hasCPUMemEntry(c.Resources.Limits) {
			return false
		}
	}
	return true
}

func podHasMissingCPUMemLimit(pod *corev1.PodSpec) bool {
	podCPULim := podHasResource(pod, corev1.ResourceCPU, true)
	podMemLim := podHasResource(pod, corev1.ResourceMemory, true)
	if podCPULim && podMemLim {
		return false
	}
	for _, c := range slices.Concat(pod.InitContainers, pod.Containers) {
		_, hasCPULim := c.Resources.Limits[corev1.ResourceCPU]
		_, hasMemLim := c.Resources.Limits[corev1.ResourceMemory]
		if (!hasCPULim && !podCPULim) || (!hasMemLim && !podMemLim) {
			return true
		}
	}
	return false
}

func anyPodHasUnboundedCPUMem(pods []*corev1.PodSpec) bool {
	hasNonBestEffort := false
	hasMissingLimit := false
	for _, pod := range pods {
		if !hasNonBestEffort && !podIsBestEffort(pod) {
			hasNonBestEffort = true
		}
		if !hasMissingLimit && podHasMissingCPUMemLimit(pod) {
			hasMissingLimit = true
		}
		if hasNonBestEffort && hasMissingLimit {
			return true
		}
	}
	return false
}

func parseNamespacePolicyList[Item any](stdout, expectedListKind string) ([]Item, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" || trimmed == "null" {
		return nil, fmt.Errorf("unexpected empty/null JSON payload")
	}
	var payload policyListPayload[Item]
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil, err
	}
	if payload.Items == nil || (payload.Kind != "" && payload.Kind != "List" && payload.Kind != expectedListKind) {
		return nil, fmt.Errorf("unexpected JSON payload")
	}
	return payload.Items, nil
}

func fetchNamespacePolicyList[Item any](g *GKEOrchestrator, kind, expectedListKind, ns, nsDesc string) ([]Item, bool) {
	if g == nil || g.executor == nil {
		return nil, false
	}
	args := []string{"get", kind}
	if ns != "" {
		args = append(args, "-n", ns)
	}
	res := g.kubectl(append(args, "-o", "json", kubectlReadTimeout)...)
	if res.ExitCode != 0 || res.Err != nil {
		reason := kubectlFailureDetail(res)
		if rawErr := kubectlOutputError(res); rawErr != nil && k8s.IsForbiddenError(rawErr) {
			reason = "403 Forbidden"
		}
		logging.Warn("Could not read %s in %s (%s); skipping %s preflight check.", kind, nsDesc, reason, kind)
		return nil, false
	}
	items, err := parseNamespacePolicyList[Item](res.Stdout, expectedListKind)
	if err != nil {
		logging.Warn("Could not parse %s list in %s (%v); skipping %s preflight check.", kind, nsDesc, err, kind)
		return nil, false
	}
	return items, true
}

func effectiveLimitRangeDefaults(lr corev1.LimitRange) (defLim, defReq corev1.ResourceList) {
	defLim = make(corev1.ResourceList)
	defReq = make(corev1.ResourceList)
	for _, spec := range lr.Spec.Limits {
		if spec.Type != corev1.LimitTypeContainer {
			continue
		}
		maps.Copy(defLim, spec.Default)
		maps.Copy(defReq, spec.DefaultRequest)
	}
	return defLim, defReq
}

func evaluateLimitRanges(nsDesc string, items []corev1.LimitRange, pods []*corev1.PodSpec) (limitRangeSummary, []string) {
	summary := limitRangeSummary{
		known:       true,
		defaults:    make(map[corev1.ResourceName]bool),
		defaultReqs: make(map[corev1.ResourceName]bool),
	}
	var warnings []string
	allContainers := allPodContainers(pods)
	for _, lr := range items {
		defLim, defReq := effectiveLimitRangeDefaults(lr)
		for _, resName := range policyResources {
			if _, ok := defReq[resName]; ok {
				summary.defaultReqs[resName] = true
			}
			defQty, ok := defLim[resName]
			if !ok {
				continue
			}
			summary.defaults[resName] = true
			warnings = append(warnings, checkLimitRangeDefault(nsDesc, lr.Name, resName, defQty, allContainers)...)
		}
	}
	return summary, warnings
}

func checkLimitRangeDefault(nsDesc, lrName string, resName corev1.ResourceName, defQty resource.Quantity, containers []corev1.Container) []string {
	missing := containersMissingResource(containers, resName, true)
	if len(missing) == 0 {
		return nil
	}
	var exceeding []string
	for _, c := range missing {
		if reqQty, hasReq := c.Resources.Requests[resName]; hasReq && reqQty.Cmp(defQty) > 0 {
			exceeding = append(exceeding, fmt.Sprintf("Container %q requests %s %s, which exceeds LimitRange %q default %s limit (%s) in %s; Pod creation may be rejected unless a mutating admission webhook overrides limits, the LimitRange default is raised/removed, or explicit limits are set via --gke-custom-templates-path (extract defaults with 'gcluster job gke-template-extract --output-dir=<dir>').", c.Name, resName, reqQty.String(), lrName, resName, defQty.String(), nsDesc))
		}
	}
	if len(exceeding) > 0 {
		return exceeding
	}
	names := strings.Join(uniqueContainerNames(missing), ", ")
	return []string{fmt.Sprintf("LimitRange %q in %s sets default %s limit %s; containers without explicit %s limits (%s) will have bursting capped at %s.", lrName, nsDesc, resName, defQty.String(), resName, names, defQty.String())}
}

func podIsBestEffortAfterLimitRange(pod *corev1.PodSpec, lr limitRangeSummary) bool {
	if !podIsBestEffort(pod) {
		return false
	}
	// LimitRanger promotes BestEffort pods to Burstable when injecting any CPU/memory default.
	if lr.promotesBestEffort() && len(pod.InitContainers)+len(pod.Containers) > 0 {
		return false
	}
	return true
}

func scopeMatchesPod(scope corev1.ResourceQuotaScope, pod *corev1.PodSpec, bestEffort bool) (matched, supported bool) {
	switch scope {
	case corev1.ResourceQuotaScopeBestEffort:
		return bestEffort, true
	case corev1.ResourceQuotaScopeNotBestEffort:
		return !bestEffort, true
	case corev1.ResourceQuotaScopeTerminating:
		return pod.ActiveDeadlineSeconds != nil && *pod.ActiveDeadlineSeconds >= 0, true
	case corev1.ResourceQuotaScopeNotTerminating:
		return pod.ActiveDeadlineSeconds == nil, true
	case corev1.ResourceQuotaScopePriorityClass:
		return pod.PriorityClassName != "", true
	default:
		return false, false
	}
}

func priorityClassRequirementMatches(expr corev1.ScopedResourceSelectorRequirement, priorityClass string) bool {
	switch expr.Operator {
	case corev1.ScopeSelectorOpExists:
		return priorityClass != ""
	case corev1.ScopeSelectorOpDoesNotExist:
		return priorityClass == ""
	case corev1.ScopeSelectorOpIn:
		return slices.Contains(expr.Values, priorityClass)
	case corev1.ScopeSelectorOpNotIn:
		return !slices.Contains(expr.Values, priorityClass)
	default:
		return false
	}
}

func scopeRequirementMatchesPod(expr corev1.ScopedResourceSelectorRequirement, pod *corev1.PodSpec, bestEffort bool) bool {
	if expr.ScopeName == corev1.ResourceQuotaScopePriorityClass {
		return priorityClassRequirementMatches(expr, pod.PriorityClassName)
	}
	matched, supported := scopeMatchesPod(expr.ScopeName, pod, bestEffort)
	if !supported {
		return false
	}
	switch expr.Operator {
	case corev1.ScopeSelectorOpExists:
		return matched
	case corev1.ScopeSelectorOpDoesNotExist:
		return !matched
	default:
		return false
	}
}

func quotaAppliesToPod(rq corev1.ResourceQuota, pod *corev1.PodSpec, bestEffort bool) bool {
	for _, scope := range rq.Spec.Scopes {
		matched, supported := scopeMatchesPod(scope, pod, bestEffort)
		if !supported || !matched {
			return false
		}
	}
	if rq.Spec.ScopeSelector != nil {
		for _, expr := range rq.Spec.ScopeSelector.MatchExpressions {
			if !scopeRequirementMatchesPod(expr, pod, bestEffort) {
				return false
			}
		}
	}
	return true
}

func quotaMayExcludeGlobalDefaultPriority(rq corev1.ResourceQuota, pods []*corev1.PodSpec) bool {
	if rq.Spec.ScopeSelector == nil {
		return false
	}
	hasDefaultExclusion := slices.ContainsFunc(rq.Spec.ScopeSelector.MatchExpressions, func(expr corev1.ScopedResourceSelectorRequirement) bool {
		return expr.ScopeName == corev1.ResourceQuotaScopePriorityClass &&
			(expr.Operator == corev1.ScopeSelectorOpDoesNotExist || expr.Operator == corev1.ScopeSelectorOpNotIn)
	})
	if !hasDefaultExclusion {
		return false
	}
	return slices.ContainsFunc(pods, func(pod *corev1.PodSpec) bool {
		return pod.PriorityClassName == ""
	})
}

func formatQuotaWarning(nsDesc, rqName string, chk quotaRequirementCheck, containerNames string, lrKnown, mayExcludeDefaultPriority bool) string {
	if !lrKnown {
		return fmt.Sprintf("ResourceQuota %q in %s requires explicit %s, and container(s) (%s) omit it; LimitRange could not be inspected, so Pod creation may fail unless a namespace LimitRange or mutating webhook sets a default.", rqName, nsDesc, chk.hardKey, containerNames)
	}
	exceptionHint := "a mutating admission webhook injects defaults"
	if mayExcludeDefaultPriority {
		exceptionHint += " or a globalDefault PriorityClass excludes this quota"
	}
	remediation := "Add a namespace LimitRange with defaults or set explicit resources via --gke-custom-templates-path (extract defaults with 'gcluster job gke-template-extract --output-dir=<dir>')."
	if !chk.requireLimit {
		remediation = "Pass both --cpu and --memory (for CPU-only jobs), add a namespace LimitRange with defaults, or set explicit resources via --gke-custom-templates-path (extract defaults with 'gcluster job gke-template-extract --output-dir=<dir>')."
	}
	return fmt.Sprintf("ResourceQuota %q in %s requires explicit %s, but container(s) (%s) omit it and no LimitRange sets a default; Pod creation may be rejected unless %s. %s", rqName, nsDesc, chk.hardKey, containerNames, exceptionHint, remediation)
}

func evaluateResourceQuotas(nsDesc string, items []corev1.ResourceQuota, pods []*corev1.PodSpec, lr limitRangeSummary) []string {
	var warnings []string
	for _, rq := range items {
		matchingPods := podsMatchingQuota(rq, pods, lr)
		if len(matchingPods) == 0 {
			continue
		}
		mayExcludeDefaultPriority := quotaMayExcludeGlobalDefaultPriority(rq, matchingPods)
		for _, chk := range quotaRequirementChecks {
			if _, enforced := rq.Spec.Hard[chk.hardKey]; !enforced || lr.satisfies(chk.resName, chk.requireLimit) {
				continue
			}
			names := uniqueContainerNames(podContainersMissingResource(matchingPods, chk.resName, chk.requireLimit))
			if len(names) == 0 {
				continue
			}
			warnings = append(warnings, formatQuotaWarning(nsDesc, rq.Name, chk, strings.Join(names, ", "), lr.known, mayExcludeDefaultPriority))
		}
	}
	return warnings
}

func allPodContainers(pods []*corev1.PodSpec) []corev1.Container {
	var out []corev1.Container
	for _, pod := range pods {
		out = append(out, pod.InitContainers...)
		out = append(out, pod.Containers...)
	}
	return out
}

func podsMatchingQuota(rq corev1.ResourceQuota, pods []*corev1.PodSpec, lr limitRangeSummary) []*corev1.PodSpec {
	var out []*corev1.PodSpec
	for _, pod := range pods {
		if quotaAppliesToPod(rq, pod, podIsBestEffortAfterLimitRange(pod, lr)) {
			out = append(out, pod)
		}
	}
	return out
}

func podContainersMissingResource(pods []*corev1.PodSpec, resName corev1.ResourceName, requireLimit bool) []corev1.Container {
	var missing []corev1.Container
	for _, pod := range pods {
		if podHasResource(pod, resName, requireLimit) {
			continue
		}
		missing = append(missing, containersMissingResource(slices.Concat(pod.InitContainers, pod.Containers), resName, requireLimit)...)
	}
	return missing
}

func containersMissingResource(containers []corev1.Container, resName corev1.ResourceName, requireLimit bool) []corev1.Container {
	var missing []corev1.Container
	for _, c := range containers {
		_, hasLim := c.Resources.Limits[resName]
		if requireLimit {
			if !hasLim {
				missing = append(missing, c)
			}
			continue
		}
		if _, hasReq := c.Resources.Requests[resName]; !hasReq && !hasLim {
			missing = append(missing, c)
		}
	}
	return missing
}

func uniqueContainerNames(containers []corev1.Container) []string {
	var names []string
	for _, c := range containers {
		if !slices.Contains(names, c.Name) {
			names = append(names, c.Name)
		}
	}
	return names
}
