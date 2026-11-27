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
	"bufio"
	"bytes"
	"cmp"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"io"
	"maps"
	"math"
	"path"
	"slices"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syamlutil "k8s.io/apimachinery/pkg/util/yaml"
	k8syaml "sigs.k8s.io/yaml"
)

const workloadWrapperScript = `echo "GCluster Start: $(date 2>/dev/null || true)" >&2

# Run workload in a separate process group
set -m
/bin/bash -c 'trap : TERM; cmd="$1"; shift; eval "$cmd"' gcluster "$1" &
child=$!

# Forward signals to the workload process group
got_sigterm=0
_fwd() {
  got_sigterm=1
  echo "gcluster: SIGTERM received, running cleanup" >&2
  kill -TERM -- "-$child" 2>/dev/null
}
trap _fwd TERM INT

# Wait for workload to exit
while :; do
  wait "$child"; rc=$?
  is_job=0
  for p in $(jobs -p); do
    [ "$p" = "$child" ] && is_job=1
  done
  [ "$is_job" -eq 1 ] || break
done

# Interrupted workloads should not exit 0 even if cleanup succeeded
if [ "$got_sigterm" -ne 0 ] && [ "$rc" -eq 0 ]; then
  rc=143
fi

echo "GCluster End: $(date 2>/dev/null || true)" >&2
echo "Exit code: $rc" >&2
exit "$rc"
`

// workloadContainerCommand wraps the user's workload command with SIGTERM
// forwarding and lifecycle logging. When the container receives SIGTERM (e.g. on
// job cancellation or Kueue/spot preemption), the signal is forwarded to the
// workload process group so that in-flight commands terminate and any subsequent
// cleanup/upload commands in the user's script can run before the pod grace
// period expires.
func workloadContainerCommand(userCommand string) []string {
	return []string{"/bin/bash", "-c", workloadWrapperScript, "gcluster", userCommand}
}

// GenerateGKEManifest generates the Kubernetes JobSet manifest for a standard workload.
func (g *GKEOrchestrator) GenerateGKEManifest(opts ManifestOptions, profile JobProfile) (string, error) {
	var coordHost string
	if opts.WorkloadName != "" {
		coordHost = fmt.Sprintf("%s-main-job-0-0.%s", opts.WorkloadName, opts.WorkloadName)
	}
	return g.generateWorkloadManifest(opts, profile, k8s.JobSetTemplate, coordHost, "")
}

// GenerateJobManifest generates a headless Service and batch/v1 Indexed Job manifest for single-slice workloads.
func (g *GKEOrchestrator) GenerateJobManifest(opts ManifestOptions, profile JobProfile) (string, error) {
	if opts.NumSlices > 1 {
		return "", fmt.Errorf("multi-slice workloads (numSlices=%d) require JobSet and cannot be rendered with job.tmpl", opts.NumSlices)
	}
	var coordHost string
	if opts.WorkloadName != "" {
		coordHost = fmt.Sprintf("%s-0.%s", opts.WorkloadName, opts.WorkloadName)
	}
	return g.generateWorkloadManifest(opts, profile, k8s.JobTemplate, coordHost, opts.WorkloadName)
}

func (g *GKEOrchestrator) generateWorkloadManifest(opts ManifestOptions, profile JobProfile, tmplName, coordinatorHost, subdomain string) (string, error) {
	opts.IsCPUMachine = opts.IsCPUMachine || profile.IsCPUMachine
	profile.IsCPUMachine = opts.IsCPUMachine
	opts.CPURequest = strings.TrimSpace(opts.CPURequest)
	opts.MemoryRequest = strings.TrimSpace(opts.MemoryRequest)

	resourcesString, isTPU, isGPU, err := g.resolveWorkloadResourcesYAML(&opts, profile, 16)
	if err != nil {
		return "", err
	}

	tmpl, baseBlocks, err := k8s.LoadWorkloadTemplate(g.gkeCustomTemplatesPath, tmplName)
	if err != nil {
		return "", err
	}

	cmdSlice := workloadContainerCommand(opts.CommandToRun)
	opts.Env = withCoordinatorEnv(coordinatorHost, opts.Env)
	data := g.prepareJobSetTemplateData(opts, cmdSlice, resourcesString, isTPU, isGPU)
	data.Subdomain = subdomain
	data.baseBlocks = baseBlocks

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute %s template: %w", tmplName, err)
	}
	manifest := assembleManifest(buf.String(), opts.AdditionalManifests)
	if err := ValidateJobSetManifest(manifest); err != nil {
		return "", err
	}
	customHint := ""
	if g.gkeCustomTemplatesPath != "" {
		customHint = fmt.Sprintf(workloadTemplateExtractHintFmt, tmplName)
	}
	if err := validateWorkloadTelemetryManifest(manifest, opts.TelemetryCollectorImage, opts.WorkloadName, customHint); err != nil {
		return "", err
	}
	return manifest, nil
}

func (g *GKEOrchestrator) resolveWorkloadResourcesYAML(opts *ManifestOptions, profile JobProfile, indent int) (string, bool, bool, error) {
	if opts.CPURequest != "" || opts.MemoryRequest != "" {
		if !opts.IsCPUMachine {
			return "", false, false, fmt.Errorf("--cpu and --memory are only supported for CPU workloads, not accelerator machine type %q", cmp.Or(opts.MachineType, opts.ComputeType))
		}
		opts.ComputeType = ""
		resourcesString, err := buildFractionalCPUResources(opts.CPURequest, opts.MemoryRequest, indent)
		return resourcesString, false, false, err
	}
	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(*opts, profile)
	if err != nil {
		return "", false, false, fmt.Errorf("failed to calculate resource limits: %w", err)
	}
	if opts.ComputeType != "" && gpuLimit == "" && tpuLimit == "" {
		logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.ComputeType)
		opts.ComputeType = ""
	}
	resourcesString, err := buildLimitResources(cpuLimit, memoryLimit, gpuLimit, g.resolveGPUResourceName(*opts, profile), tpuLimit, indent)
	return resourcesString, tpuLimit != "", gpuLimit != "", err
}

func buildFractionalCPUResources(cpuReq, memReq string, indent int) (string, error) {
	requests := corev1.ResourceList{}
	if cpuReq != "" {
		q, err := k8s.ParseCPURequest(cpuReq)
		if err != nil {
			return "", err
		}
		requests[corev1.ResourceCPU] = q
	}
	if memReq != "" {
		q, err := k8s.ParseMemoryRequest(memReq)
		if err != nil {
			return "", err
		}
		requests[corev1.ResourceMemory] = q
	}
	return buildResourceRequirements(requests, nil, indent)
}

func buildLimitResources(cpu, mem, gpu, gpuResourceName, tpu string, indent int) (string, error) {
	limits := corev1.ResourceList{}
	gpuKey := corev1.ResourceName(cmp.Or(strings.TrimSpace(gpuResourceName), "nvidia.com/gpu"))
	for _, entry := range []struct {
		raw   string
		name  corev1.ResourceName
		label string
	}{
		{cpu, corev1.ResourceCPU, "CPU"},
		{mem, corev1.ResourceMemory, "memory"},
		{gpu, gpuKey, "GPU"},
		{tpu, corev1.ResourceName("google.com/tpu"), "TPU"},
	} {
		if entry.raw == "" {
			continue
		}
		q, err := resource.ParseQuantity(entry.raw)
		if err != nil {
			return "", fmt.Errorf("failed to parse %s quantity %q: %w", entry.label, entry.raw, err)
		}
		limits[entry.name] = q
	}
	return buildResourceRequirements(nil, limits, indent)
}

func buildResourceRequirements(requests, limits corev1.ResourceList, indent int) (string, error) {
	if len(requests) == 0 && len(limits) == 0 {
		return "", nil
	}

	resources := corev1.ResourceRequirements{
		Requests: requests,
		Limits:   limits,
	}

	b, err := k8syaml.Marshal(resources)
	if err != nil {
		return "", fmt.Errorf("failed to marshal resources: %w", err)
	}

	resourcesStr := "resources:\n" + indentYaml(string(b), 2)
	return indentYaml(resourcesStr, indent), nil
}

func (g *GKEOrchestrator) resolveK8sGKECPUPinMachine(job orchestrator.JobDefinition, profile JobProfile) string {
	if g.labelScheme() != k8s.SchemeGKE || !profile.IsCPUMachine || (job.CPURequest == "" && job.MemoryRequest == "") {
		return ""
	}
	if profile.K8sPool != nil {
		return strings.TrimSpace(profile.K8sPool.InstanceTypeLabel)
	}
	return strings.TrimSpace(job.MachineType)
}

func (g *GKEOrchestrator) buildSchedulingOptions(job orchestrator.JobDefinition, profile JobProfile, originalAccelType string, isDynamicSlicing, isStaticSlicing bool) SchedulingOptions {
	isK8s := g.mode == orchestrator.TypeK8s
	machineType := job.MachineType
	pinCPU := !isK8s && g.shouldPinCPUMachine(profile.IsCPUMachine, job.CPURequest, job.MemoryRequest, job.GKENAPProvisioning, job.GKENAPReservation, job.MachineType)
	if isK8s {
		if pinTarget := g.resolveK8sGKECPUPinMachine(job, profile); pinTarget != "" {
			pinCPU = true
			machineType = pinTarget
		}
	}
	schedOpts := SchedulingOptions{
		PlacementPolicy:     job.PlacementPolicy,
		NodeAffinityLabels:  job.NodeConstraint,
		Topology:            job.Topology,
		Scheduler:           job.GKEScheduler,
		IsDynamicSlicing:    isDynamicSlicing,
		IsStaticSlicing:     isStaticSlicing,
		MachineType:         machineType,
		IsTPU:               profile.IsTPU || (!isK8s && (config.IsTPU(job.MachineType) || config.IsTPU(originalAccelType))),
		IsNAP:               job.GKENAPProvisioning != "",
		ClusterNAPEnabled:   !isK8s && g.napEnabled,
		PinCPUMachine:       pinCPU,
		ExcludeAccelerators: !isK8s && profile.IsCPUMachine && !job.IsPathwaysJob,
		OmitGKENodePool:     !g.hasGKENodePoolLabel(job.MachineType, profile.K8sPool),
	}
	if !isK8s {
		g.applyCPUHeadPoolAvoidance(&schedOpts, job.GKENAPProvisioning, job.GKENAPReservation)
	}
	return schedOpts
}

// PrepareManifestOptions resolves scheduling, storage, pull-secret, and telemetry options for manifest rendering; on non-GKE programmatic targets (empty ProjectID), MLDiagnosticsEnabled auto-discovers the collector image from TPU nodes when unset.
func (g *GKEOrchestrator) PrepareManifestOptions(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) (ManifestOptions, error) {
	job.CPURequest = strings.TrimSpace(job.CPURequest)
	job.MemoryRequest = strings.TrimSpace(job.MemoryRequest)
	originalAccelType := job.ComputeType

	schedOpts := g.buildSchedulingOptions(job, profile, originalAccelType, isDynamicSlicing, isStaticSlicing)

	pathwaysInstanceType := derivePathwaysInstanceType(g.GenerateGKENodeSelectorLabel(originalAccelType), job.MachineType, originalAccelType, schedOpts.Topology)

	telemetryImg, telemetryPort, err := g.resolveTelemetryCollector(job, schedOpts.IsTPU)
	if err != nil {
		return ManifestOptions{}, err
	}

	if job.ImagePullSecrets, err = g.resolveImagePullSecrets(job, fullImageName, telemetryImg); err != nil {
		return ManifestOptions{}, err
	}

	opts := ManifestOptions{
		IsDynamicSlicing:              isDynamicSlicing,
		IsStaticSlicing:               isStaticSlicing,
		OmitGKENodePool:               schedOpts.OmitGKENodePool,
		K8sPool:                       profile.K8sPool,
		WorkloadName:                  job.WorkloadName,
		FullImageName:                 fullImageName,
		CommandToRun:                  job.CommandToRun,
		ComputeType:                   job.ComputeType,
		MachineType:                   job.MachineType,
		CPURequest:                    job.CPURequest,
		MemoryRequest:                 job.MemoryRequest,
		IsCPUMachine:                  profile.IsCPUMachine,
		PathwaysInstanceType:          pathwaysInstanceType,
		ProjectID:                     job.ProjectID,
		ClusterName:                   job.ClusterName,
		ClusterLocation:               job.ClusterLocation,
		KueueQueueName:                job.KueueQueueName,
		NumSlices:                     job.NumSlices,
		NodesPerSlice:                 job.NodesPerSlice,
		MaxRestarts:                   job.MaxRestarts,
		TtlSecondsAfterFinished:       job.TtlSecondsAfterFinished,
		TerminationGracePeriodSeconds: job.TerminationGracePeriodSeconds,
		ServiceAccountName:            job.ServiceAccountName,
		SchedulerName:                 job.GKEScheduler,
		AwaitJobCompletion:            job.AwaitJobCompletion,
		PriorityClassName:             job.PriorityClassName,
		Topology:                      schedOpts.Topology,
		Verbose:                       job.Verbose,
		Env:                           job.Env,
		IsPathwaysJob:                 job.IsPathwaysJob,
		Pathways:                      job.Pathways,
		MLDiagnosticsEnabled:          job.MLDiagnosticsEnabled,
		TelemetryCollectorImage:       telemetryImg,
		TelemetryCollectorPort:        telemetryPort,
		GKEMTCEnabled:                 job.GKEMTCEnabled,
		GKEMTCRamdiskDirectory:        job.GKEMTCRamdiskDirectory,
	}
	// Single injection point for the telemetry env defaults; the renderers read opts.Env / opts.Pathways.WorkerEnv as is.
	if opts.TelemetryCollectorImage != "" {
		if opts.IsPathwaysJob {
			opts.Pathways.WorkerEnv = k8s.WithTelemetryEnv(opts.Pathways.WorkerEnv)
		} else {
			opts.Env = k8s.WithTelemetryEnv(opts.Env)
		}
	}

	if err := g.fillManifestStrings(&opts, schedOpts, job, isDynamicSlicing, isStaticSlicing, profile); err != nil {
		return ManifestOptions{}, err
	}

	sm := &StorageManager{orchestrator: g}
	mountInfos, manifests, err := sm.ProcessMounts(job.RawMounts, job)
	if err != nil {
		return ManifestOptions{}, err
	}
	opts.AdditionalManifests = manifests

	sm.AddVolumeOptions(&opts, mountInfos)
	if opts.isFractionalCPU() {
		warnFractionalCPUOptions(cmp.Or(opts.MachineType, opts.ComputeType), opts.CPURequest, opts.MemoryRequest, opts.GCSFuseEnabled, profile.CapacityCount, profile.MemoryMb)
	}

	g.applySchedulingGatesAndParallelism(&opts, profile, job)

	podFailurePolicyStr, err := g.generatePodFailurePolicy(job.RestartOnExitCodes, podFailurePolicyContainers(opts)...)
	if err != nil {
		return ManifestOptions{}, err
	}
	opts.PodFailurePolicy = indentYaml(podFailurePolicyStr, 12)

	return opts, nil
}

func (g *GKEOrchestrator) applySchedulingGatesAndParallelism(opts *ManifestOptions, profile JobProfile, job orchestrator.JobDefinition) {
	isTPU := profile.IsTPU || (g.mode != orchestrator.TypeK8s && config.IsTPU(job.MachineType))
	isGPU := !opts.IsCPUMachine && !isTPU
	if isGPU && opts.SchedulerName == "gke.io/topology-aware-auto" {
		opts.SchedulingGates = indentYaml("schedulingGates:\n  - name: \"gke.io/topology-aware-auto-"+opts.WorkloadName+"\"", 14)
		opts.SchedulerName = ""
	}
	opts.ParallelContainers = g.resolveParallelContainerCount(job)
}

func workloadContainerNames(parallelContainers int) []string {
	if parallelContainers > 1 {
		names := make([]string, parallelContainers)
		for i := range parallelContainers {
			names[i] = fmt.Sprintf("workload-container-%d", i+1)
		}
		return names
	}
	return []string{"workload-container"}
}

// podFailurePolicyContainers returns the container names the --restart-on-exit-codes rules are scoped to, so
// that only the workload's exit code decides the outcome: a pod-wide rule would also match the SIGTERM/SIGKILL
// exit codes of native sidecars (ML Diagnostics, GCS FUSE, MTC, custom base.tmpl sidecars, or cluster webhook
// injections such as a service mesh) and turn a retriable workload exit into FailJobSet. Standard workloads are
// always scoped; a headless or SPS Pathways head has no workload container, so its rule stays pod-wide.
func podFailurePolicyContainers(opts ManifestOptions) []string {
	if opts.IsPathwaysJob {
		if !opts.Pathways.Headless && !opts.Pathways.DeploySPS {
			return []string{"workload-container"}
		}
		return nil
	}
	return workloadContainerNames(opts.ParallelContainers)
}

func derivePathwaysInstanceType(gkeLabel, machineType, accelType, topology string) string {
	// Normalize GKE node selector labels to match JAX/Pathways platform keys:
	// 1. Map GKE "v5-lite" (TPU v5e) to JAX standard "v5e" (deriving tpuv5e)
	// 2. Map GKE "v5p" (TPU v5p) to JAX standard "v5" (deriving tpuv5)
	normalizedLabel := gkeLabel
	if strings.Contains(gkeLabel, "v5-lite") {
		normalizedLabel = strings.ReplaceAll(gkeLabel, "v5-lite", "v5e")
	} else if strings.Contains(gkeLabel, "v5p") {
		normalizedLabel = strings.ReplaceAll(gkeLabel, "v5p", "v5")
	}
	pathwaysPlatform := strings.ReplaceAll(normalizedLabel, "-podslice", "")
	pathwaysPlatform = strings.ReplaceAll(pathwaysPlatform, "-slice", "")
	pathwaysPlatform = strings.ReplaceAll(pathwaysPlatform, "-", "")
	// 8-chip single-VM TPUs share their GKE accelerator label with the 4-chip
	// VMs, but Pathways identifies them by a distinct "1t" platform name.
	if (pathwaysPlatform == "tpuv6e" || pathwaysPlatform == "tpuv5e") &&
		(strings.HasSuffix(machineType, "-8t") || strings.HasSuffix(accelType, "-8t")) {
		pathwaysPlatform += "1t"
	}
	return fmt.Sprintf("%s:%s", pathwaysPlatform, topology)
}

func (g *GKEOrchestrator) fillManifestStrings(opts *ManifestOptions, schedOpts SchedulingOptions, job orchestrator.JobDefinition, isDynamicSlicing bool, isStaticSlicing bool, profile JobProfile) error {
	nodeSelectorStr, err := g.buildNodeSelectorWithProfile(schedOpts, job, profile)
	if err != nil {
		return err
	}
	opts.NodeSelector = nodeSelectorStr

	affinityStr, err := g.buildAffinity(schedOpts)
	if err != nil {
		return err
	}
	opts.Affinity = affinityStr

	imagePullSecretsStr := g.generateImagePullSecrets(job.ImagePullSecrets)
	if imagePullSecretsStr != "" {
		opts.ImagePullSecrets = indentYaml(imagePullSecretsStr, 16)
	}

	isSubSlicing := isDynamicSlicing || isStaticSlicing
	opts.TopologyAnnotation = g.buildTopologyAnnotation(schedOpts.Topology, job.MachineType, job.NumSlices, job.NodesPerSlice, isSubSlicing)

	tolerationsStr, err := g.resolveTolerationsForWorkload(job.MachineType, schedOpts.IsTPU, job.GKENAPProvisioning, job.GKENAPReservation, 16)
	if err != nil {
		return err
	}
	opts.Tolerations = tolerationsStr

	return nil
}

func (g *GKEOrchestrator) resolveReservationTolerations(machineType, reservationName string) []corev1.Toleration {
	var tolerations []corev1.Toleration
	res := parseReservationURI(reservationName)
	// Always add the standard GKE reservation toleration to support NAP where the node pool may not exist yet
	tolerations = append(tolerations, corev1.Toleration{
		Key:      "cloud.google.com/reservation-name",
		Operator: corev1.TolerationOpEqual,
		Value:    res.Name,
		Effect:   corev1.TaintEffectNoSchedule,
	})

	for _, np := range g.clusterDesc.NodePools {
		if !strings.EqualFold(np.Config.MachineType, machineType) || !poolMatchesReservation(np.Config, reservationName) {
			continue
		}
		for _, t := range np.Config.Taints {
			tol := corev1.Toleration{
				Key:      t.Key,
				Value:    t.Value,
				Effect:   gkeTaintEffectToCoreV1(t.Effect),
				Operator: corev1.TolerationOpEqual,
			}
			if !slices.Contains(tolerations, tol) {
				tolerations = append(tolerations, tol)
			}
		}
	}
	return tolerations
}

func gkeTaintEffectToCoreV1(effect string) corev1.TaintEffect {
	switch strings.ToUpper(strings.TrimSpace(effect)) {
	case "NO_SCHEDULE", "NOSCHEDULE":
		return corev1.TaintEffectNoSchedule
	case "PREFER_NO_SCHEDULE", "PREFERNOSCHEDULE":
		return corev1.TaintEffectPreferNoSchedule
	case "NO_EXECUTE", "NOEXECUTE":
		return corev1.TaintEffectNoExecute
	default:
		return corev1.TaintEffect(effect)
	}
}

func (g *GKEOrchestrator) resolveTolerationsForWorkload(acceleratorType string, isTPU bool, consumptionModel string, reservationName string, indent int) (string, error) {
	var tolerations []corev1.Toleration
	if g.mode == orchestrator.TypeK8s {
		if isTPU {
			tolerations = append(tolerations, corev1.Toleration{
				Key:      "google.com/tpu",
				Operator: corev1.TolerationOpExists,
				Effect:   corev1.TaintEffectNoSchedule,
			})
		}
	} else {
		// Copy the slice to avoid mutating any shared underlying array returned by GetTolerations
		tolerations = append([]corev1.Toleration(nil), GetTolerations(acceleratorType)...)
	}

	switch consumptionModel {
	case "spot":
		tolerations = append(tolerations, corev1.Toleration{
			Key:      "cloud.google.com/gke-provisioning",
			Operator: corev1.TolerationOpEqual,
			Value:    "spot",
			Effect:   corev1.TaintEffectNoSchedule,
		})
	case "reservation":
		tolerations = append(tolerations, g.resolveReservationTolerations(acceleratorType, reservationName)...)
	}

	if len(tolerations) == 0 {
		return "", nil
	}
	b, err := k8syaml.Marshal(tolerations)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tolerations: %w", err)
	}
	return indentYaml(string(b), indent), nil
}

func assembleManifest(mainManifest string, additionalManifests []string) string {
	var resManifest strings.Builder
	for _, m := range additionalManifests {
		trimmed := strings.TrimSpace(m)
		if trimmed == "" {
			continue
		}
		resManifest.WriteString(trimmed)
		resManifest.WriteString("\n---\n")
	}
	resManifest.WriteString(strings.TrimSpace(mainManifest))
	return resManifest.String()
}

func indentYaml(s string, indent int) string {
	lines := strings.Split(s, "\n")
	padding := strings.Repeat(" ", indent)
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			result = append(result, padding+line)
		}
	}
	return strings.Join(result, "\n")
}

// RenderBlock renders a named block from base.tmpl indented to the requested column width.
func (d jobSetTemplateData) RenderBlock(name string, indent int) (string, error) {
	bt, ok := d.baseBlocks[name]
	if !ok {
		return "", fmt.Errorf("unknown base.tmpl block %q (available blocks: %v); check the block name or extract fresh defaults with '%s' to diff and merge", name, slices.Sorted(maps.Keys(d.baseBlocks)), templateExtractCmd)
	}
	if bt == nil {
		return "", fmt.Errorf("base.tmpl block %q renders itself via .RenderBlock; an override cannot call the block it replaces", name)
	}
	// safetext serializes Execute per template with a non-reentrant lock, so a block that re-enters itself would
	// deadlock. d is a copy: mark the block in flight for this render only, which still allows sequential reuse.
	nextBlocks := maps.Clone(d.baseBlocks)
	nextBlocks[name] = nil
	d.baseBlocks = nextBlocks
	var buf bytes.Buffer
	if err := bt.Execute(&buf, d); err != nil {
		return "", fmt.Errorf("failed to execute base template block %q: %w", name, err)
	}
	return k8s.ReindentYAML(indent, buf.String()), nil
}

func splitYAMLDocuments(content string) ([]string, error) {
	var docs []string
	reader := k8syamlutil.NewYAMLReader(bufio.NewReader(strings.NewReader(content)))
	for {
		doc, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read YAML document stream: %w", err)
		}
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, string(doc))
		}
	}
	return docs, nil
}

func validatePodSpecVolumes(contextPrefix string, podSpec *corev1.PodSpec) error {
	definedVolumes := make(map[string]bool, len(podSpec.Volumes))
	for _, v := range podSpec.Volumes {
		if definedVolumes[v.Name] {
			return fmt.Errorf("%s declares duplicate volume %q", contextPrefix, v.Name)
		}
		definedVolumes[v.Name] = true
	}

	for _, c := range slices.Concat(podSpec.InitContainers, podSpec.Containers) {
		seenMountPaths := make(map[string]bool, len(c.VolumeMounts))
		for _, m := range c.VolumeMounts {
			if !definedVolumes[m.Name] {
				return fmt.Errorf("%s container %q mounts undefined volume %q at %q", contextPrefix, c.Name, m.Name, m.MountPath)
			}
			if m.MountPath != "" {
				cleanPath := path.Clean(m.MountPath)
				if seenMountPaths[cleanPath] {
					return fmt.Errorf("%s container %q has duplicate volumeMount path %q", contextPrefix, c.Name, m.MountPath)
				}
				seenMountPaths[cleanPath] = true
			}
		}
	}
	return nil
}

// validatePodSpecEnv mirrors the JobSet CRD (v0.6.0+) map-list check on env.name (which rejects
// duplicate env var names, including value and valueFrom collisions, at apply time) so
// conflicts fail earlier with a clearer error. Skips empty names for API-server validation.
// Scoped to JobSet because the core batch/v1 Job API accepts duplicate env entries.
func validatePodSpecEnv(contextPrefix string, podSpec *corev1.PodSpec) error {
	for _, c := range slices.Concat(podSpec.InitContainers, podSpec.Containers) {
		seenEnv := make(map[string]bool, len(c.Env))
		for _, e := range c.Env {
			if e.Name == "" {
				continue
			}
			if seenEnv[e.Name] {
				return fmt.Errorf("%s container %q defines duplicate environment variable %q. If you use --gke-custom-templates-path, remove or guard the duplicate in your template (or re-extract fresh defaults with '%s' and merge your customizations)", contextPrefix, c.Name, e.Name, templateExtractCmd)
			}
			seenEnv[e.Name] = true
		}
	}
	return nil
}

func validatePodFailurePolicy(contextPrefix string, pfp *batchv1.PodFailurePolicy, podSpec *corev1.PodSpec) error {
	if pfp == nil {
		return nil
	}
	restartPolicy := string(podSpec.RestartPolicy)
	if restartPolicy != "Never" {
		return fmt.Errorf("%s specifies podFailurePolicy with restartPolicy %q; only \"Never\" is supported when podFailurePolicy is specified", contextPrefix, restartPolicy)
	}
	known := make(map[string]bool, len(podSpec.InitContainers)+len(podSpec.Containers))
	for _, c := range slices.Concat(podSpec.InitContainers, podSpec.Containers) {
		known[c.Name] = true
	}
	for i, rule := range pfp.Rules {
		if err := validatePodFailureExitCodesRule(contextPrefix, i, rule.OnExitCodes, known); err != nil {
			return err
		}
	}
	return nil
}

func validatePodFailureExitCodesRule(contextPrefix string, idx int, req *batchv1.PodFailurePolicyOnExitCodesRequirement, known map[string]bool) error {
	if req == nil {
		return nil
	}
	if req.ContainerName != nil && !known[*req.ContainerName] {
		return fmt.Errorf("%s podFailurePolicy.rules[%d].onExitCodes.containerName %q does not match any container or initContainer in the pod template. --restart-on-exit-codes scopes its rules to the workload container(s), so a custom template must keep the %q name prefix, or drop --restart-on-exit-codes and define podFailurePolicy in the template instead", contextPrefix, idx, *req.ContainerName, "workload-container")
	}
	return nil
}

func forEachWorkloadJobSpec(manifestContent string, visit func(kind, jobName string, pfp *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error) error {
	return forEachWorkloadObject(manifestContent, func(kind, jobName string, _ map[string]string, pfp *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error {
		return visit(kind, jobName, pfp, tmpl)
	})
}

// forEachWorkloadObject is forEachWorkloadJobSpec plus the JobSet/Job object's own metadata.labels, for checks
// that concern the workload object rather than its pod template.
func forEachWorkloadObject(manifestContent string, visit func(kind, jobName string, objectLabels map[string]string, pfp *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error) error {
	docs, err := splitYAMLDocuments(manifestContent)
	if err != nil {
		return err
	}
	for _, doc := range docs {
		var meta metav1.TypeMeta
		if err := k8syaml.Unmarshal([]byte(doc), &meta); err != nil {
			// splitYAMLDocuments does not validate syntax, so this is where a malformed custom template surfaces.
			return fmt.Errorf("failed to parse manifest document: %w", err)
		}
		switch meta.Kind {
		case "JobSet":
			var jsDoc jobSetValidationSpec
			if err := k8syaml.Unmarshal([]byte(doc), &jsDoc); err != nil {
				return fmt.Errorf("failed to parse JobSet manifest spec: %w", err)
			}
			for i := range jsDoc.Spec.ReplicatedJobs {
				rj := &jsDoc.Spec.ReplicatedJobs[i]
				if err := visit("JobSet", rj.Name, jsDoc.Labels, rj.Template.Spec.PodFailurePolicy, &rj.Template.Spec.Template); err != nil {
					return err
				}
			}
		case "Job":
			var jDoc batchv1.Job
			if err := k8syaml.Unmarshal([]byte(doc), &jDoc); err != nil {
				return fmt.Errorf("failed to parse Job manifest spec: %w", err)
			}
			if err := visit("Job", "", jDoc.Labels, jDoc.Spec.PodFailurePolicy, &jDoc.Spec.Template); err != nil {
				return err
			}
		case "Service":
			var svc corev1.Service
			if err := k8syaml.Unmarshal([]byte(doc), &svc); err != nil {
				return fmt.Errorf("failed to parse Service manifest spec: %w", err)
			}
		}
	}
	return nil
}

// ValidateJobSetManifest validates JobSet and Job resources in the manifest.
func ValidateJobSetManifest(manifestContent string) error {
	return forEachWorkloadJobSpec(manifestContent, func(kind, jobName string, pfp *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error {
		prefix := "invalid Job manifest:"
		if kind == "JobSet" {
			prefix = fmt.Sprintf("invalid JobSet manifest: replicatedJob %q", jobName)
		}
		if err := validatePodFailurePolicy(prefix, pfp, &tmpl.Spec); err != nil {
			return err
		}
		if kind == "JobSet" {
			if err := validatePodSpecEnv(prefix, &tmpl.Spec); err != nil {
				return err
			}
		}
		return validatePodSpecVolumes(prefix, &tmpl.Spec)
	})
}

const (
	templateExtractCmd          = "gcluster job gke-template-extract --output-dir=<new-dir>"
	pathwaysTemplateExtractHint = "If you use --gke-custom-templates-path, re-extract pathways_jobset.tmpl and base.tmpl with '" + templateExtractCmd + "' and merge your customizations"
	// workloadTemplateExtractHintFmt takes the workload template name (jobset.tmpl or job.tmpl).
	workloadTemplateExtractHintFmt = "If you use --gke-custom-templates-path, re-extract %s and base.tmpl with '" + templateExtractCmd + "' and merge your customizations"
	// minPathwaysWorkerRestartsPerVM is the per-VM floor of the worker Job backoffLimit; see pathwaysWorkerBackoffLimit.
	minPathwaysWorkerRestartsPerVM = 4
)

// pathwaysWorkerBackoffLimit returns the worker Job backoffLimit for --pathways-max-slice-restarts and the number of
// VMs per slice. Pathways workers exit (code 1) by design whenever their client disconnects, and rely on an in-place
// restart. Under restartPolicy OnFailure the Job fails once container restarts summed over a slice's running worker
// pods (sidecars included, counted over the pods' lifetime) reach backoffLimit, so allow at least 4 per VM: the budget
// XPK (vms_per_slice*4) and the pathways-job controller (NumVMs*4) use for non-elastic workers. backoffLimit is an
// int32, so the result is capped at MaxInt32, the value Kubernetes itself defaults it to when backoffLimitPerIndex is
// set.
func pathwaysWorkerBackoffLimit(maxSliceRestarts, nodesPerSlice int) int {
	if nodesPerSlice <= 0 {
		return 0
	}
	perVM := min(max(maxSliceRestarts, minPathwaysWorkerRestartsPerVM), math.MaxInt32)
	limit := int64(perVM) * int64(min(nodesPerSlice, math.MaxInt32))
	return int(min(limit, math.MaxInt32))
}

func (g *GKEOrchestrator) renderPathwaysWorkerResources(opts ManifestOptions, profile JobProfile) (string, bool, bool, error) {
	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(opts, profile)
	if err != nil {
		logging.Warn("failed to calculate resource limits for Pathways job: %v", err)
		return "", false, false, nil
	}
	resStr, err := buildLimitResources(cpuLimit, memoryLimit, gpuLimit, g.resolveGPUResourceName(opts, profile), tpuLimit, 14)
	return resStr, tpuLimit != "", gpuLimit != "", err
}

// GeneratePathwaysManifest generates the Kubernetes JobSet manifest for a Pathways job.
func (g *GKEOrchestrator) GeneratePathwaysManifest(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) (string, error) {
	job.IsPathwaysJob = true
	var imageNotice string
	var imageNoticeIsWarning bool
	var err error
	job.Pathways, imageNotice, imageNoticeIsWarning, err = resolvePathwaysImages(job.Pathways)
	if err != nil {
		return "", err
	}
	switch {
	case imageNotice == "":
	case imageNoticeIsWarning:
		logging.Warn("%s", imageNotice)
	default:
		logging.Info("%s", imageNotice)
	}

	tmpl, baseBlocks, err := k8s.LoadWorkloadTemplate(g.gkeCustomTemplatesPath, k8s.PathwaysJobSetTemplate)
	if err != nil {
		return "", err
	}

	opts, err := g.PrepareManifestOptions(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		return "", err
	}

	resStr, isTPU, isGPU, err := g.renderPathwaysWorkerResources(opts, profile)
	if err != nil {
		return "", err
	}

	cmdSlice := workloadContainerCommand(opts.CommandToRun)
	data := g.prepareJobSetTemplateData(opts, cmdSlice, resStr, isTPU, isGPU)
	data.baseBlocks = baseBlocks

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute pathways jobset template: %w", err)
	}
	customHint := ""
	if g.gkeCustomTemplatesPath != "" {
		customHint = pathwaysTemplateExtractHint
	}
	if err := validateRenderedPathwaysManifest(buf.String(), opts, customHint); err != nil {
		return "", err
	}
	warnPathwaysHostNetworkManifest(buf.String(), opts, data.HostNetworkEnabled)
	warnPathwaysWorkerRestartPolicy(buf.String())

	manifest := assembleManifest(buf.String(), opts.AdditionalManifests)
	if err := ValidateJobSetManifest(manifest); err != nil {
		return "", err
	}
	return manifest, nil
}

func validateRenderedPathwaysManifest(jobSet string, opts ManifestOptions, customHint string) error {
	if err := validatePathwaysMTCManifest(jobSet, opts); err != nil {
		return err
	}
	if err := validatePathwaysSPSManifest(jobSet, opts); err != nil {
		return err
	}
	return validatePathwaysTelemetryManifest(jobSet, opts.TelemetryCollectorImage, opts.WorkloadName, customHint)
}

func findCSIVolumeName(podSpec *corev1.PodSpec, driver string) string {
	for _, v := range podSpec.Volumes {
		if v.CSI != nil && v.CSI.Driver == driver {
			return v.Name
		}
	}
	return ""
}

func containerMountsVolumeAt(podSpec *corev1.PodSpec, containerName, volumeName, mountPath string) bool {
	for _, c := range slices.Concat(podSpec.InitContainers, podSpec.Containers) {
		if c.Name != containerName {
			continue
		}
		for _, m := range c.VolumeMounts {
			if m.Name == volumeName && path.Clean(m.MountPath) == path.Clean(mountPath) {
				return true
			}
		}
		return false
	}
	return false
}

func extractPathwaysPodTemplates(jobSet string) (workerTmpl, headTmpl *corev1.PodTemplateSpec, err error) {
	err = forEachWorkloadJobSpec(jobSet, func(kind, jobName string, _ *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error {
		if kind == "JobSet" {
			switch jobName {
			case "worker":
				workerTmpl = tmpl
			case "pathways-head":
				headTmpl = tmpl
			}
		}
		return nil
	})
	return workerTmpl, headTmpl, err
}

// validatePathwaysMTCManifest checks that Pathways MTC jobs mount the CSI volume only on worker pods.
func validatePathwaysMTCManifest(jobSet string, opts ManifestOptions) error {
	if !opts.GKEMTCEnabled {
		return nil
	}
	workerTmpl, headTmpl, err := extractPathwaysPodTemplates(jobSet)
	if err != nil {
		return err
	}
	if headTmpl != nil && findCSIVolumeName(&headTmpl.Spec, multitierCheckpointCSIDriver) != "" {
		return fmt.Errorf("--gke-mtc-enabled is set but pathways-head declares a %s volume; the MTC volume must only be declared on worker pods. %s", multitierCheckpointCSIDriver, pathwaysTemplateExtractHint)
	}
	if workerTmpl == nil {
		return fmt.Errorf("--gke-mtc-enabled is set but the Pathways manifest has no worker replicatedJob. %s", pathwaysTemplateExtractHint)
	}
	mtcVolName := findCSIVolumeName(&workerTmpl.Spec, multitierCheckpointCSIDriver)
	if mtcVolName == "" {
		return fmt.Errorf("--gke-mtc-enabled is set but the Pathways manifest has no %s volume on worker pods. %s", multitierCheckpointCSIDriver, pathwaysTemplateExtractHint)
	}
	if !containerMountsVolumeAt(&workerTmpl.Spec, "pathways-worker", mtcVolName, opts.GKEMTCRamdiskDirectory) {
		return fmt.Errorf("--gke-mtc-enabled is set but container \"pathways-worker\" does not mount %s volume %q at %q. %s", multitierCheckpointCSIDriver, mtcVolName, opts.GKEMTCRamdiskDirectory, pathwaysTemplateExtractHint)
	}
	if opts.Pathways.ColocatedPythonSidecarImage != "" && !containerMountsVolumeAt(&workerTmpl.Spec, "colocated-python-sidecar", mtcVolName, opts.GKEMTCRamdiskDirectory) {
		return fmt.Errorf("--gke-mtc-enabled is set with --pathways-colocated-python-sidecar-image, but container \"colocated-python-sidecar\" does not mount %s volume %q at %q. %s", multitierCheckpointCSIDriver, mtcVolName, opts.GKEMTCRamdiskDirectory, pathwaysTemplateExtractHint)
	}
	return nil
}

// validatePathwaysSPSManifest checks that an SPS server's pathways-head runs only pathways-rm,
// which a pathways_jobset.tmpl extracted from an older gcluster version does not do.
func validatePathwaysSPSManifest(jobSet string, opts ManifestOptions) error {
	if !opts.Pathways.DeploySPS {
		return nil
	}
	_, headTmpl, err := extractPathwaysPodTemplates(jobSet)
	if err != nil {
		return err
	}
	if headTmpl == nil {
		return fmt.Errorf("--pathways-deploy-sps is set but the Pathways manifest has no pathways-head replicatedJob. %s", pathwaysTemplateExtractHint)
	}
	for _, c := range slices.Concat(headTmpl.Spec.InitContainers, headTmpl.Spec.Containers) {
		if c.Name == "pathways-proxy" || strings.HasPrefix(c.Name, "workload-container") {
			return fmt.Errorf("--pathways-deploy-sps is set but pathways-head runs container %q; an SPS server runs only pathways-rm. %s", c.Name, pathwaysTemplateExtractHint)
		}
	}
	return nil
}

// warnPathwaysHostNetworkManifest logs a warning when hostNetwork was disabled for
// --service-account, but a custom or stale pathways_jobset.tmpl still renders hostNetwork: true.
func warnPathwaysHostNetworkManifest(jobSet string, opts ManifestOptions, hostNetworkEnabled bool) bool {
	if hostNetworkEnabled {
		return false
	}
	workerTmpl, headTmpl, err := extractPathwaysPodTemplates(jobSet)
	if err != nil {
		return false
	}
	if (headTmpl != nil && headTmpl.Spec.HostNetwork) || (workerTmpl != nil && workerTmpl.Spec.HostNetwork) {
		logging.Warn("--service-account %q disabled hostNetwork for GKE Workload Identity, but the rendered Pathways manifest still sets hostNetwork: true (pods with hostNetwork: true bypass GKE Workload Identity unless GCS FUSE CSI with hostNetworkPodKSA is used). %s", opts.ServiceAccountName, pathwaysTemplateExtractHint)
		return true
	}
	return false
}

// warnPathwaysWorkerRestartPolicy logs a warning when a custom or stale pathways_jobset.tmpl renders worker pods
// with a restartPolicy other than OnFailure (an empty value is defaulted to OnFailure by the JobSet webhook).
func warnPathwaysWorkerRestartPolicy(jobSet string) bool {
	workerTmpl, _, err := extractPathwaysPodTemplates(jobSet)
	if err != nil || workerTmpl == nil {
		return false
	}
	if rp := workerTmpl.Spec.RestartPolicy; rp != "" && rp != corev1.RestartPolicyOnFailure {
		logging.Warn("The rendered Pathways worker pods use restartPolicy %q. Pathways restarts each worker when its client disconnects, so without restartPolicy: OnFailure every disconnect, including the end of a run, fails a worker pod; with --pathways-max-slice-restarts 0 that fails the worker Job, so the JobSet restarts or fails instead of completing. %s", rp, pathwaysTemplateExtractHint)
		return true
	}
	return false
}

func (g *GKEOrchestrator) resolveExclusiveTopologyAnnotation(opts ManifestOptions, isTPU bool) string {
	if opts.IsDynamicSlicing || opts.IsStaticSlicing || opts.isFractionalCPU() {
		return ""
	}
	if opts.NodesPerSlice <= 1 {
		return "alpha.jobset.sigs.k8s.io/exclusive-topology: kubernetes.io/hostname"
	}
	if g.mode == orchestrator.TypeK8s && g.labelScheme() == k8s.SchemeTPUSlice && isTPU {
		return "alpha.jobset.sigs.k8s.io/exclusive-topology: " + k8s.TPUSliceNameLabel
	}
	if !opts.OmitGKENodePool && g.hasGKENodePoolLabel(opts.MachineType, opts.K8sPool) {
		return "alpha.jobset.sigs.k8s.io/exclusive-topology: cloud.google.com/gke-nodepool"
	}
	return ""
}

func (g *GKEOrchestrator) prepareJobSetTemplateData(opts ManifestOptions, command []string, resourcesYAML string, isTPU, isGPU bool) jobSetTemplateData {
	exclusiveTopology := g.resolveExclusiveTopologyAnnotation(opts, isTPU)

	workerBackoffLimit := pathwaysWorkerBackoffLimit(opts.Pathways.MaxSliceRestarts, opts.NodesPerSlice)

	var proxyArgsList []string
	if opts.Pathways.ProxyArgs != "" {
		proxyArgsList = strings.Fields(opts.Pathways.ProxyArgs)
	}
	if opts.Pathways.ColocatedPythonSidecarImage != "" {
		proxyArgsList = withSidecarNameArg(proxyArgsList)
	}
	var serverArgsList []string
	if opts.Pathways.ServerArgs != "" {
		serverArgsList = strings.Fields(opts.Pathways.ServerArgs)
	}
	var workerArgsList []string
	if opts.Pathways.WorkerArgs != "" {
		workerArgsList = strings.Fields(opts.Pathways.WorkerArgs)
	}

	names := workloadContainerNames(opts.ParallelContainers)
	containers := make([]ContainerData, len(names))
	for i, name := range names {
		containers[i] = ContainerData{
			Name:          name,
			ResourcesYAML: resourcesYAML,
		}
	}

	proxyEnv, workloadEnv := resolvePathwaysEnv(opts)
	hostNetworkEnabled := resolveHostNetwork(opts, isTPU, isGPU, g.mode != orchestrator.TypeK8s)

	return jobSetTemplateData{
		WorkloadName:                  opts.WorkloadName,
		Namespace:                     g.namespace,
		ClusterName:                   opts.ClusterName,
		Containers:                    containers,
		ProjectID:                     opts.ProjectID,
		KueueQueueName:                opts.KueueQueueName,
		TtlSecondsAfterFinished:       opts.TtlSecondsAfterFinished,
		TerminationGracePeriodSeconds: opts.TerminationGracePeriodSeconds,
		MaxRestarts:                   opts.MaxRestarts,
		NumSlices:                     opts.NumSlices,
		NodesPerSlice:                 opts.NodesPerSlice,
		WorkerBackoffLimit:            workerBackoffLimit,
		WorkerBackoffLimitPerIndex:    opts.Pathways.MaxSliceRestarts,
		ProxyArgsList:                 proxyArgsList,
		ServerArgsList:                serverArgsList,
		WorkerArgsList:                workerArgsList,
		PathwaysInstanceType:          opts.PathwaysInstanceType,
		CommandToRun:                  opts.CommandToRun,
		ResourcesString:               resourcesYAML,
		FullImageName:                 opts.FullImageName,
		Command:                       command,
		ResourcesYAML:                 resourcesYAML,
		AcceleratorTypeLabel:          g.GenerateGKENodeSelectorLabel(opts.ComputeType),
		NodeSelector:                  opts.NodeSelector,
		Affinity:                      opts.Affinity,
		PodFailurePolicy:              opts.PodFailurePolicy,
		ImagePullSecrets:              opts.ImagePullSecrets,
		ServiceAccountName:            opts.ServiceAccountName,
		TopologyAnnotation:            opts.TopologyAnnotation,
		SchedulerName:                 opts.SchedulerName,
		SchedulingGates:               opts.SchedulingGates,
		Tolerations:                   opts.Tolerations,
		PriorityClassName:             opts.PriorityClassName,
		VolumesYAML:                   opts.VolumesYAML,
		VolumeMountsYAML:              opts.VolumeMountsYAML,
		GCSFuseEnabled:                opts.GCSFuseEnabled,
		HostNetworkEnabled:            hostNetworkEnabled,
		Pathways:                      opts.Pathways,
		PathwaysProxyEnabled:          !opts.Pathways.DeploySPS,
		PathwaysWorkloadEnabled:       !opts.Pathways.Headless && !opts.Pathways.DeploySPS,
		PathwaysRMEnabled:             true,
		PathwaysWorkersEnabled:        true,
		ExclusiveTopologyAnnotation:   exclusiveTopology,
		Verbose:                       opts.Verbose,
		Env:                           sortedEnvVars(workloadEnv),
		PathwaysProxyEnv:              sortedEnvVars(proxyEnv),
		PathwaysServerEnv:             sortedEnvVars(opts.Pathways.ServerEnv),
		PathwaysWorkerEnv:             sortedEnvVars(opts.Pathways.WorkerEnv),
		IsTPU:                         isTPU,
		IsGPU:                         isGPU,
		MLDiagnosticsEnabled:          opts.MLDiagnosticsEnabled,
		TelemetryCollectorImage:       opts.TelemetryCollectorImage,
		TelemetryCollectorPort:        k8s.EffectiveTelemetryPort(opts.TelemetryCollectorImage, opts.TelemetryCollectorPort),
		GKEMTCEnabled:                 opts.GKEMTCEnabled,
		GKEMTCRamdiskDirectory:        opts.GKEMTCRamdiskDirectory,
	}
}

func validateWorkloadTelemetryManifest(manifestContent, collectorImage, workloadName, hint string) error {
	if collectorImage == "" {
		return nil
	}
	return forEachWorkloadObject(manifestContent, func(kind, jobName string, objectLabels map[string]string, _ *batchv1.PodFailurePolicy, tmpl *corev1.PodTemplateSpec) error {
		prefix := "Job pod template"
		if kind == "JobSet" {
			prefix = fmt.Sprintf("replicatedJob %q", jobName)
		}
		if err := k8s.ValidateNoManagedWebhookLabel(kind, hint, objectLabels); err != nil {
			return err
		}
		return k8s.ValidatePodTelemetry(prefix, hint, workloadName, tmpl.Spec, tmpl.Labels)
	})
}

func validatePathwaysTelemetryManifest(jobSet, collectorImage, workloadName, hint string) error {
	if collectorImage == "" {
		return nil
	}
	workerTmpl, headTmpl, err := extractPathwaysPodTemplates(jobSet)
	if err != nil {
		return err
	}
	if err := forEachWorkloadObject(jobSet, func(kind, _ string, objectLabels map[string]string, _ *batchv1.PodFailurePolicy, _ *corev1.PodTemplateSpec) error {
		return k8s.ValidateNoManagedWebhookLabel(kind, hint, objectLabels)
	}); err != nil {
		return err
	}
	if headTmpl != nil {
		hasCollector := func(cs []corev1.Container) bool {
			return slices.ContainsFunc(cs, func(c corev1.Container) bool { return c.Name == k8s.TelemetryCollectorContainerName })
		}
		if hasCollector(headTmpl.Spec.InitContainers) || hasCollector(headTmpl.Spec.Containers) {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("pathways-head must not run %q; telemetry sidecar belongs only on TPU worker pods", k8s.TelemetryCollectorContainerName), hint))
		}
		if _, hasLbl := headTmpl.Labels[k8s.TPUWorkloadNameLabel]; hasLbl {
			return fmt.Errorf("%s", k8s.AppendHint(fmt.Sprintf("pathways-head must not carry label %q; telemetry pod labels belong only on TPU worker pods", k8s.TPUWorkloadNameLabel), hint))
		}
	}
	if workerTmpl == nil {
		return fmt.Errorf("%s", k8s.AppendHint("ML Diagnostics sidecar is enabled but the Pathways manifest has no worker replicatedJob", hint))
	}
	return k8s.ValidatePodTelemetry("Pathways worker pod template", hint, workloadName, workerTmpl.Spec, workerTmpl.Labels)
}

// withCoordinatorEnv sets default MASTER_ADDR and WORKERS_0_HOSTNAME pointing to the rank-0 pod.
func withCoordinatorEnv(coordinatorHost string, userEnv map[string]string) map[string]string {
	if coordinatorHost == "" {
		return userEnv
	}
	merged := make(map[string]string, len(userEnv)+2)
	merged["MASTER_ADDR"] = coordinatorHost
	merged["WORKERS_0_HOSTNAME"] = coordinatorHost
	maps.Copy(merged, userEnv)
	return merged
}

// withSidecarNameArg appends --sidecar_name=external to the pathways-proxy
// args; call it when the colocated Python sidecar is enabled. Without it the
// proxy does not set up the sidecar connection on the workers, so the sidecar
// receives no work. A --sidecar_name the user already passed via
// --pathways-proxy-args is kept as is.
func withSidecarNameArg(proxyArgs []string) []string {
	for _, arg := range proxyArgs {
		// absl accepts both --flag and -flag, with or without "=value".
		name := strings.TrimLeft(arg, "-")
		if name == "sidecar_name" || strings.HasPrefix(name, "sidecar_name=") {
			return proxyArgs
		}
	}
	return append(proxyArgs, "--sidecar_name=external")
}

func resolveHostNetwork(opts ManifestOptions, isTPU, isGPU, isGKE bool) bool {
	if opts.IsPathwaysJob || isTPU {
		if isGKE && opts.ServiceAccountName != "" {
			if opts.NumSlices > 1 {
				logging.Warn("Disabling hostNetwork for --service-account %q (GKE Workload Identity); GKE recommends hostNetwork: true for multi-slice TPU DCN performance.", opts.ServiceAccountName)
			} else {
				logging.Info("Disabling hostNetwork for --service-account %q (GKE Workload Identity).", opts.ServiceAccountName)
			}
			return false
		}
		return true
	}
	if isGKE && isGPU && opts.ServiceAccountName != "" {
		logging.Warn("--service-account %q is set on a GPU workload, which uses hostNetwork: true. Pods with hostNetwork: true bypass GKE Workload Identity and authenticate to Google Cloud APIs as the node pool IAM service account unless GCS FUSE CSI with hostNetworkPodKSA is used.", opts.ServiceAccountName)
	}
	return isGPU
}

func resolvePathwaysEnv(opts ManifestOptions) (map[string]string, map[string]string) {
	proxyEnv := opts.Pathways.ProxyEnv
	workloadEnv := opts.Env
	if !opts.IsPathwaysJob || opts.Pathways.DeploySPS {
		return proxyEnv, workloadEnv
	}
	const ifrtInsecureEnv = "IFRT_PROXY_USE_INSECURE_GRPC_CREDENTIALS"
	proxyVal, hasProxy := proxyEnv[ifrtInsecureEnv]
	workloadVal, hasWorkload := workloadEnv[ifrtInsecureEnv]
	if !opts.Pathways.Headless && hasProxy && hasWorkload && proxyVal != workloadVal {
		logging.Warn("%s is set to %q in --pathways-proxy-env and %q in --env; mismatched gRPC credentials between pathways-proxy and the workload container may cause connection failures.", ifrtInsecureEnv, proxyVal, workloadVal)
	}
	// When --service-account is set, hostNetwork is disabled so pods can
	// authenticate via GKE Workload Identity (gke-metadata-server), which does
	// not expose the GCE VM ALTS handshaker on port 8080. Default IFRT proxy
	// gRPC credentials to insecure unless overridden, and propagate single-sided
	// overrides so client and proxy credentials remain matched.
	defaultVal := "true"
	if hasProxy {
		defaultVal = proxyVal
	} else if !opts.Pathways.Headless && hasWorkload {
		defaultVal = workloadVal
	} else if opts.ServiceAccountName == "" {
		return proxyEnv, workloadEnv
	}
	proxyEnv = withDefaultEnvVar(proxyEnv, ifrtInsecureEnv, defaultVal)
	if !opts.Pathways.Headless {
		workloadEnv = withDefaultEnvVar(workloadEnv, ifrtInsecureEnv, defaultVal)
	}
	return proxyEnv, workloadEnv
}

func withDefaultEnvVar(env map[string]string, key, defaultVal string) map[string]string {
	if _, ok := env[key]; ok {
		return env
	}
	cloned := maps.Clone(env)
	if cloned == nil {
		cloned = make(map[string]string)
	}
	cloned[key] = defaultVal
	return cloned
}

func sortedEnvVars(envMap map[string]string) EnvVars {
	if len(envMap) == 0 {
		return nil
	}
	envKeys := make([]string, 0, len(envMap))
	for k := range envMap {
		envKeys = append(envKeys, k)
	}
	slices.Sort(envKeys)
	res := make(EnvVars, len(envKeys))
	for i, k := range envKeys {
		res[i] = EnvVar{Name: k, Value: envMap[k]}
	}
	return res
}
