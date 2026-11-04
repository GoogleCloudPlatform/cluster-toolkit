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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"
	"hpc-toolkit/pkg/orchestrator/k8s"
	"io"
	"maps"
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
	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(opts, profile)
	if err != nil {
		return "", fmt.Errorf("failed to calculate resource limits: %w", err)
	}

	if opts.ComputeType != "" && gpuLimit == "" && tpuLimit == "" {
		logging.Info("Suppressing nodeSelector label for deduced CPU machine %s", opts.ComputeType)
		opts.ComputeType = ""
	}

	resourcesString, err := g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit, 16)
	if err != nil {
		return "", err
	}

	tmpl, baseBlocks, err := k8s.LoadWorkloadTemplate(g.gkeCustomTemplatesPath, tmplName)
	if err != nil {
		return "", err
	}

	cmdSlice := workloadContainerCommand(opts.CommandToRun)
	isTPU := tpuLimit != ""
	isGPU := gpuLimit != ""
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
	return manifest, nil
}

func (g *GKEOrchestrator) buildResourcesString(cpu, mem, gpu, tpu string, indent int) (string, error) {
	limits := corev1.ResourceList{}
	if cpu != "" {
		q, err := resource.ParseQuantity(cpu)
		if err != nil {
			return "", fmt.Errorf("failed to parse CPU quantity %q: %w", cpu, err)
		}
		limits[corev1.ResourceCPU] = q
	}
	if mem != "" {
		q, err := resource.ParseQuantity(mem)
		if err != nil {
			return "", fmt.Errorf("failed to parse memory quantity %q: %w", mem, err)
		}
		limits[corev1.ResourceMemory] = q
	}
	if gpu != "" {
		q, err := resource.ParseQuantity(gpu)
		if err != nil {
			return "", fmt.Errorf("failed to parse GPU quantity %q: %w", gpu, err)
		}
		limits[corev1.ResourceName("nvidia.com/gpu")] = q
	}
	if tpu != "" {
		q, err := resource.ParseQuantity(tpu)
		if err != nil {
			return "", fmt.Errorf("failed to parse TPU quantity %q: %w", tpu, err)
		}
		limits[corev1.ResourceName("google.com/tpu")] = q
	}

	if len(limits) == 0 {
		return "", nil
	}

	resources := corev1.ResourceRequirements{
		Limits: limits,
	}

	b, err := k8syaml.Marshal(resources)
	if err != nil {
		return "", fmt.Errorf("failed to marshal resources: %w", err)
	}

	resourcesStr := "resources:\n" + indentYaml(string(b), 2)
	return indentYaml(resourcesStr, indent), nil
}

func (g *GKEOrchestrator) PrepareManifestOptions(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) (ManifestOptions, error) {
	var err error
	if job.ImagePullSecrets, err = g.resolveImagePullSecrets(job, fullImageName); err != nil {
		return ManifestOptions{}, err
	}
	originalAccelType := job.ComputeType

	schedOpts := SchedulingOptions{
		PlacementPolicy:    job.PlacementPolicy,
		NodeAffinityLabels: job.NodeConstraint,
		Topology:           job.Topology,
		Scheduler:          job.GKEScheduler,
		IsDynamicSlicing:   isDynamicSlicing,
		IsStaticSlicing:    isStaticSlicing,
		MachineType:        job.MachineType,
		IsTPU:              config.IsTPU(job.MachineType) || config.IsTPU(originalAccelType),
		IsNAP:              job.GKENAPProvisioning != "",
	}

	// Reuse GCluster's existing GKE accelerator label mapping and algorithmically
	// derive the Pathways short platform key to avoid duplicating mapping tables.
	gkeLabel := g.GenerateGKENodeSelectorLabel(originalAccelType)
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
		(strings.HasSuffix(job.MachineType, "-8t") || strings.HasSuffix(originalAccelType, "-8t")) {
		pathwaysPlatform += "1t"
	}

	pathwaysInstanceType := fmt.Sprintf("%s:%s", pathwaysPlatform, schedOpts.Topology)

	opts := ManifestOptions{
		IsDynamicSlicing:              isDynamicSlicing,
		IsStaticSlicing:               isStaticSlicing,
		WorkloadName:                  job.WorkloadName,
		FullImageName:                 fullImageName,
		CommandToRun:                  job.CommandToRun,
		ComputeType:                   job.ComputeType,
		MachineType:                   job.MachineType,
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
		GKEMTCEnabled:                 job.GKEMTCEnabled,
		GKEMTCRamdiskDirectory:        job.GKEMTCRamdiskDirectory,
	}

	if err := g.fillManifestStrings(&opts, schedOpts, job, isDynamicSlicing, isStaticSlicing, profile.IsCPUMachine); err != nil {
		return ManifestOptions{}, err
	}

	sm := &StorageManager{orchestrator: g}
	mountInfos, manifests, err := sm.ProcessMounts(job.RawMounts, job)
	if err != nil {
		return ManifestOptions{}, err
	}
	opts.AdditionalManifests = manifests

	sm.AddVolumeOptions(&opts, mountInfos)

	if _, err = g.resolveResourcesAndGates(&opts, profile.IsCPUMachine, profile.CapacityCount, job); err != nil {
		return ManifestOptions{}, err
	}

	podFailurePolicyStr, err := g.generatePodFailurePolicy(job.RestartOnExitCodes, podFailurePolicyContainers(opts)...)
	if err != nil {
		return ManifestOptions{}, err
	}
	opts.PodFailurePolicy = indentYaml(podFailurePolicyStr, 12)

	return opts, nil
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

func (g *GKEOrchestrator) fillManifestStrings(opts *ManifestOptions, schedOpts SchedulingOptions, job orchestrator.JobDefinition, isDynamicSlicing bool, isStaticSlicing bool, isCPUMachine bool) error {
	nodeSelectorStr, err := g.buildNodeSelector(schedOpts, job, isCPUMachine)
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

	tolerationsStr, err := g.resolveTolerations(job.MachineType, job.GKENAPProvisioning, job.GKENAPReservation, 16)
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

	seenTaints := map[string]bool{
		"cloud.google.com/reservation-name": true,
	}

	shortResName := res.Name
	for _, np := range g.clusterDesc.NodePools {
		lblVal := np.Config.Labels["cloud.google.com/reservation-name"]
		if lblVal == "" {
			continue
		}
		parsedLbl := parseReservationURI(lblVal)
		if strings.EqualFold(np.Config.MachineType, machineType) && parsedLbl.Name == shortResName {
			tolerations[0].Value = parsedLbl.Name
			for _, t := range np.Config.Taints {
				// Avoid duplicate tolerations
				if seenTaints[t.Key] {
					continue
				}
				seenTaints[t.Key] = true
				var effect corev1.TaintEffect
				switch strings.ToUpper(t.Effect) {
				case "NO_SCHEDULE":
					effect = corev1.TaintEffectNoSchedule
				case "PREFER_NO_SCHEDULE":
					effect = corev1.TaintEffectPreferNoSchedule
				case "NO_EXECUTE":
					effect = corev1.TaintEffectNoExecute
				default:
					effect = corev1.TaintEffect(t.Effect)
				}
				tolerations = append(tolerations, corev1.Toleration{
					Key:      t.Key,
					Value:    t.Value,
					Effect:   effect,
					Operator: corev1.TolerationOpEqual,
				})
			}
		}
	}
	return tolerations
}

func (g *GKEOrchestrator) resolveTolerations(acceleratorType string, consumptionModel string, reservationName string, indent int) (string, error) {
	// Copy the slice to avoid mutating any shared underlying array returned by GetTolerations
	tolerations := append([]corev1.Toleration(nil), GetTolerations(acceleratorType)...)

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
		return "", fmt.Errorf("unknown base.tmpl block %q (available blocks: %v); check the block name or extract fresh defaults with 'gcluster job gke-template-extract --output-dir=<new-dir>' to diff and merge", name, slices.Sorted(maps.Keys(d.baseBlocks)))
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
				if err := visit("JobSet", rj.Name, rj.Template.Spec.PodFailurePolicy, &rj.Template.Spec.Template); err != nil {
					return err
				}
			}
		case "Job":
			var jDoc batchv1.Job
			if err := k8syaml.Unmarshal([]byte(doc), &jDoc); err != nil {
				return fmt.Errorf("failed to parse Job manifest spec: %w", err)
			}
			if err := visit("Job", "", jDoc.Spec.PodFailurePolicy, &jDoc.Spec.Template); err != nil {
				return err
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
		return validatePodSpecVolumes(prefix, &tmpl.Spec)
	})
}

const (
	defaultPathwaysProxyImage   = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/proxy_server:latest"
	defaultPathwaysServerImage  = "us-docker.pkg.dev/cloud-tpu-v2-images/pathways/server:latest"
	pathwaysTemplateExtractHint = "If you use --gke-custom-templates-path, re-extract pathways_jobset.tmpl and base.tmpl with 'gcluster job gke-template-extract --output-dir=<new-dir>' and merge your customizations"
)

// GeneratePathwaysManifest generates the Kubernetes JobSet manifest for a Pathways job.
func (g *GKEOrchestrator) GeneratePathwaysManifest(job orchestrator.JobDefinition, fullImageName string, profile JobProfile, isDynamicSlicing bool, isStaticSlicing bool) (string, error) {
	job.IsPathwaysJob = true
	// Set default values for Pathways-specific fields if not provided
	if job.Pathways.ProxyServerImage == "" {
		job.Pathways.ProxyServerImage = defaultPathwaysProxyImage
	}
	if job.Pathways.ServerImage == "" {
		job.Pathways.ServerImage = defaultPathwaysServerImage
	}
	if job.Pathways.WorkerImage == "" {
		// WorkerImage defaults to ServerImage if not explicitly set
		job.Pathways.WorkerImage = job.Pathways.ServerImage
	}

	tmpl, baseBlocks, err := k8s.LoadWorkloadTemplate(g.gkeCustomTemplatesPath, k8s.PathwaysJobSetTemplate)
	if err != nil {
		return "", err
	}

	opts, err := g.PrepareManifestOptions(job, fullImageName, profile, isDynamicSlicing, isStaticSlicing)
	if err != nil {
		return "", err
	}

	cpuLimit, memoryLimit, gpuLimit, tpuLimit, err := g.calculateResourceLimits(opts, profile)
	var resStr string
	if err == nil {
		resStr, err = g.buildResourcesString(cpuLimit, memoryLimit, gpuLimit, tpuLimit, 14)
		if err != nil {
			return "", err
		}
	} else {
		logging.Warn("failed to calculate resource limits for Pathways job: %v", err)
	}

	cmdSlice := workloadContainerCommand(opts.CommandToRun)
	data := g.prepareJobSetTemplateData(opts, cmdSlice, resStr, tpuLimit != "", gpuLimit != "")
	data.baseBlocks = baseBlocks

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute pathways jobset template: %w", err)
	}
	if err := validatePathwaysMTCManifest(buf.String(), opts); err != nil {
		return "", err
	}
	if err := validatePathwaysSPSManifest(buf.String(), opts); err != nil {
		return "", err
	}

	manifest := assembleManifest(buf.String(), opts.AdditionalManifests)
	if err := ValidateJobSetManifest(manifest); err != nil {
		return "", err
	}
	return manifest, nil
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

func (g *GKEOrchestrator) prepareJobSetTemplateData(opts ManifestOptions, command []string, resourcesYAML string, isTPU, isGPU bool) jobSetTemplateData {
	exclusiveTopology := ""
	if !opts.IsDynamicSlicing && !opts.IsStaticSlicing {
		if opts.NodesPerSlice <= 1 {
			exclusiveTopology = "alpha.jobset.sigs.k8s.io/exclusive-topology: kubernetes.io/hostname"
		} else {
			exclusiveTopology = "alpha.jobset.sigs.k8s.io/exclusive-topology: cloud.google.com/gke-nodepool"
		}
	}

	workerBackoffLimit := opts.Pathways.MaxSliceRestarts * opts.NodesPerSlice

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
		HostNetworkEnabled:            isTPU || isGPU,
		Pathways:                      opts.Pathways,
		PathwaysProxyEnabled:          !opts.Pathways.DeploySPS,
		PathwaysWorkloadEnabled:       !opts.Pathways.Headless && !opts.Pathways.DeploySPS,
		ExclusiveTopologyAnnotation:   exclusiveTopology,
		Verbose:                       opts.Verbose,
		Env:                           sortedEnvVars(opts.Env),
		PathwaysProxyEnv:              sortedEnvVars(opts.Pathways.ProxyEnv),
		PathwaysServerEnv:             sortedEnvVars(opts.Pathways.ServerEnv),
		PathwaysWorkerEnv:             sortedEnvVars(opts.Pathways.WorkerEnv),
		IsTPU:                         isTPU,
		IsGPU:                         isGPU,
		MLDiagnosticsEnabled:          opts.MLDiagnosticsEnabled,
		GKEMTCEnabled:                 opts.GKEMTCEnabled,
		GKEMTCRamdiskDirectory:        opts.GKEMTCRamdiskDirectory,
	}
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

func sortedEnvVars(envMap map[string]string) []EnvVar {
	if len(envMap) == 0 {
		return nil
	}
	envKeys := make([]string, 0, len(envMap))
	for k := range envMap {
		envKeys = append(envKeys, k)
	}
	slices.Sort(envKeys)
	res := make([]EnvVar, len(envKeys))
	for i, k := range envKeys {
		res[i] = EnvVar{Name: k, Value: envMap[k]}
	}
	return res
}
