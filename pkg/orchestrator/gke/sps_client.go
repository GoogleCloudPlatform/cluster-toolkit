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

package gke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	corev1 "k8s.io/api/core/v1"
)

const (
	spsProxyStartTimeout = 10 * time.Minute
	spsProxyPort         = 29000
	spsProxyReadyLog     = "IFRT proxy server started"
)

// spsInstance is what an SPS client needs to know about the instance it runs on.
type spsInstance struct {
	Name           string
	Endpoint       string
	KueueQueueName string
	HeadNodePool   string
	ServerImage    string
	ProxyImage     string
	GCSLocation    string
	InstanceType   string
	InstanceCount  int
	HasSidecar     bool
}

type spsJobSet struct {
	Metadata struct {
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		ReplicatedJobs []struct {
			Name     string `json:"name"`
			Replicas int    `json:"replicas"`
			Template struct {
				Spec struct {
					Template struct {
						Spec corev1.PodSpec `json:"spec"`
					} `json:"template"`
				} `json:"spec"`
			} `json:"template"`
		} `json:"replicatedJobs"`
	} `json:"spec"`
}

// spsInstanceRef turns a JobSet name or endpoint into the instance name and the endpoint the proxy connects to.
func spsInstanceRef(ref string) (name, endpoint string) {
	ref = strings.TrimPrefix(strings.TrimSpace(ref), "grpc://")
	if i := strings.Index(ref, "-pathways-head"); i > 0 {
		endpoint = ref
		if !strings.Contains(ref, ":") {
			endpoint += ":29001"
		}
		return ref[:i], endpoint
	}
	return ref, fmt.Sprintf(spsEndpointFormat, ref)
}

// proxyImageFor derives the proxy image that matches an SPS server image, as pathwaysutils does.
func proxyImageFor(serverImage string) string {
	for _, sep := range []string{":", "@"} {
		if strings.Contains(serverImage, "/server"+sep) {
			return strings.Replace(serverImage, "/server"+sep, "/proxy_server"+sep, 1)
		}
	}
	return ""
}

func parseSPSClientTarget(name string, data []byte) (spsInstance, error) {
	var js spsJobSet
	if err := json.Unmarshal(data, &js); err != nil {
		return spsInstance{}, fmt.Errorf("failed to parse SPS instance %q: %w", name, err)
	}
	inst := spsInstance{
		Name:           name,
		KueueQueueName: js.Metadata.Labels["kueue.x-k8s.io/queue-name"],
	}
	for i := range js.Spec.ReplicatedJobs {
		rj := &js.Spec.ReplicatedJobs[i]
		pod := &rj.Template.Spec.Template.Spec
		switch rj.Name {
		case "pathways-head":
			if err := parseSPSClientTargetHead(name, pod, &inst); err != nil {
				return spsInstance{}, err
			}
		case "worker", "pathways-worker":
			parseSPSClientTargetWorker(pod, rj.Replicas, &inst)
		}
	}
	if inst.InstanceType == "" || inst.ServerImage == "" {
		return spsInstance{}, fmt.Errorf("JobSet %q is not an SPS instance: it needs a pathways-head job running pathways-rm and a worker job running pathways-worker", name)
	}
	return inst, nil
}

func parseSPSClientTargetHead(name string, pod *corev1.PodSpec, inst *spsInstance) error {
	inst.HeadNodePool = pod.NodeSelector["cloud.google.com/gke-nodepool"]
	for _, c := range slices.Concat(pod.InitContainers, pod.Containers) {
		switch {
		case c.Name == "pathways-rm":
			if inst.ServerImage == "" {
				inst.ServerImage = c.Image
			}
			parseSPSClientTargetRMArgs(c.Args, inst)
		case c.Name == "pathways-proxy" || strings.HasPrefix(c.Name, "workload-container") || c.Name == "user-workload":
			return fmt.Errorf("JobSet %q is not an SPS instance: its pathways-head runs %s, so it is a regular or headless Pathways job", name, c.Name)
		}
	}
	return nil
}

func parseSPSClientTargetRMArgs(args []string, inst *spsInstance) {
	for i := 0; i < len(args); i++ {
		key, value, ok := strings.Cut(args[i], "=")
		if !ok && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			i++
		} else if !ok {
			continue
		}
		switch strings.TrimLeft(key, "-") {
		case "gcs_scratch_location":
			inst.GCSLocation = value
		case "instance_type":
			inst.InstanceType = value
		case "instance_count":
			inst.InstanceCount, _ = strconv.Atoi(value)
		}
	}
}

func parseSPSClientTargetWorker(pod *corev1.PodSpec, replicas int, inst *spsInstance) {
	if inst.InstanceCount == 0 && replicas > 0 {
		inst.InstanceCount = replicas
	}
	for _, c := range slices.Concat(pod.InitContainers, pod.Containers) {
		switch c.Name {
		case "pathways-worker":
			if c.Image != "" {
				inst.ServerImage = c.Image
			}
		case "colocated-python-sidecar", "external-sidecar":
			inst.HasSidecar = true
		}
	}
}

func spsVirtualSlices(instanceType string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = instanceType
	}
	return strings.Join(parts, ",")
}

func (g *GKEOrchestrator) readSPSInstance(ref, ns string) (spsInstance, error) {
	name, endpoint := spsInstanceRef(ref)
	res := g.kubectl("get", "jobset", name, "-n", ns, "-o", "json")
	if res.ExitCode != 0 {
		return spsInstance{}, fmt.Errorf("failed to read SPS instance %q in namespace %q: %s", name, ns, strings.TrimSpace(res.Stderr))
	}
	inst, err := parseSPSClientTarget(name, []byte(res.Stdout))
	if err != nil {
		return spsInstance{}, err
	}
	inst.Endpoint = endpoint
	inst.ProxyImage = proxyImageFor(inst.ServerImage)

	phases := g.kubectl("get", "pods", "-n", ns, "-l", "jobset.sigs.k8s.io/jobset-name="+name+",jobset.sigs.k8s.io/replicatedjob-name=pathways-head", "-o", "jsonpath={.items[*].status.phase}")
	if phases.ExitCode != 0 {
		return spsInstance{}, fmt.Errorf("failed to check SPS instance pods: %s", strings.TrimSpace(phases.Stderr))
	}
	if !strings.Contains(phases.Stdout, "Running") {
		return spsInstance{}, fmt.Errorf("SPS instance %q has no running pathways-head pod", name)
	}
	return inst, nil
}

// runSPSWorkload runs job.CommandToRun on an existing SPS instance: on this machine when no image is given, in the cluster otherwise.
func (g *GKEOrchestrator) runSPSWorkload(job orchestrator.JobDefinition) error {
	logging.Info("Configuring kubectl for GKE cluster '%s'...", job.ClusterName)
	if err := g.configureKubectl(job.ClusterName, job.ClusterLocation, job.ProjectID); err != nil {
		return err
	}
	ns, err := g.getCurrentNamespace(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		return err
	}
	inst, err := g.readSPSInstance(job.Pathways.RunSPSWorkload, ns)
	if err != nil {
		return err
	}
	if inst.InstanceCount > 0 && job.NumSlices > inst.InstanceCount {
		return fmt.Errorf("--num-slices %d is more than the %d slices of SPS instance %q", job.NumSlices, inst.InstanceCount, inst.Name)
	}
	if err := g.validateJobConflicts(job); err != nil {
		return err
	}
	if job.KueueQueueName == "" {
		job.KueueQueueName = inst.KueueQueueName
	}
	if inst.HeadNodePool == "" {
		inst.HeadNodePool = g.autoDetectCPUNodePool()
	}
	logging.Info("Using SPS instance %s (%s): %d x %s", inst.Name, inst.Endpoint, inst.InstanceCount, inst.InstanceType)

	if job.ImageName == "" && job.BaseImage == "" {
		return g.runSPSWorkloadLocally(job, inst, ns)
	}
	return g.submitSPSClusterWorkload(job, inst)
}

func (g *GKEOrchestrator) submitSPSClusterWorkload(job orchestrator.JobDefinition, inst spsInstance) error {
	sm := &StorageManager{orchestrator: g}
	if err := sm.RunStorageProfilePreflight(job.RawMounts, job.ProjectID, job.ClusterLocation, job.DryRunManifest != ""); err != nil {
		return err
	}
	fullImageName, err := g.BuildContainerImage(job)
	if err != nil {
		return err
	}
	manifest, err := g.generateSPSClientManifest(job, inst, fullImageName)
	if err != nil {
		return err
	}
	if err := g.ApplyManifest(manifest, job.DryRunManifest, job.WorkloadName); err != nil {
		return err
	}
	return g.finishJobSubmission(job)
}

func resolveSPSClientPathways(p orchestrator.PathwaysJobDefinition, inst spsInstance) (orchestrator.PathwaysJobDefinition, error) {
	if p.HeadNodePool == "" {
		p.HeadNodePool = inst.HeadNodePool
	}
	if p.HeadNodePool == "" {
		return p, fmt.Errorf("failed to auto-detect a suitable CPU node pool for the Pathways head job; pass --pathways-head-np")
	}
	if p.ProxyServerImage == "" {
		p.ProxyServerImage = inst.ProxyImage
	}
	if p.ProxyServerImage == "" {
		return p, fmt.Errorf("could not derive the proxy image from SPS server image %q; pass --pathways-proxy-server-image", inst.ServerImage)
	}
	if p.GCSLocation == "" {
		p.GCSLocation = inst.GCSLocation
	}
	if p.GCSLocation == "" {
		return p, fmt.Errorf("could not determine GCS scratch location from SPS instance %q; pass --pathways-gcs-location", inst.Name)
	}
	return p, nil
}

// generateSPSClientManifest renders the client JobSet from pathways_jobset.tmpl: a proxy pointed at the
// instance, plus the workload container when fullImageName is set. No resource manager, no TPU workers.
func (g *GKEOrchestrator) generateSPSClientManifest(job orchestrator.JobDefinition, inst spsInstance, fullImageName string) (string, error) {
	p, err := resolveSPSClientPathways(job.Pathways, inst)
	if err != nil {
		return "", err
	}
	opts := ManifestOptions{
		WorkloadName:                  job.WorkloadName,
		KueueQueueName:                job.KueueQueueName,
		TtlSecondsAfterFinished:       job.TtlSecondsAfterFinished,
		TerminationGracePeriodSeconds: job.TerminationGracePeriodSeconds,
		MaxRestarts:                   job.MaxRestarts,
		NumSlices:                     job.NumSlices,
		NodesPerSlice:                 1,
		FullImageName:                 fullImageName,
		CommandToRun:                  job.CommandToRun,
		Env:                           job.Env,
		PriorityClassName:             job.PriorityClassName,
		ServiceAccountName:            job.ServiceAccountName,
		IsPathwaysJob:                 true,
		Pathways:                      p,
	}
	if fullImageName != "" && len(job.RawMounts) > 0 {
		sm := &StorageManager{orchestrator: g}
		mountInfos, manifests, err := sm.ProcessMounts(job.RawMounts, job)
		if err != nil {
			return "", err
		}
		opts.AdditionalManifests = manifests
		sm.AddVolumeOptions(&opts, mountInfos)
	}
	data := g.prepareJobSetTemplateData(opts, workloadContainerCommand(job.CommandToRun), "", false, false)
	data.PathwaysRMEnabled = false
	data.PathwaysWorkersEnabled = false
	data.PathwaysProxyEnabled = true
	data.PathwaysWorkloadEnabled = fullImageName != ""
	data.PathwaysRMAddress = inst.Endpoint
	data.ProxyArgsList = append(data.ProxyArgsList, "--virtual_slices="+spsVirtualSlices(inst.InstanceType, job.NumSlices))
	if inst.HasSidecar {
		data.ProxyArgsList = withSidecarNameArg(data.ProxyArgsList)
	}

	tmpl, err := g.parseGKETemplate("pathways_jobset.tmpl")
	if err != nil {
		return "", fmt.Errorf("failed to parse pathways jobset template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute pathways jobset template: %w", err)
	}
	return assembleManifest(buf.String(), opts.AdditionalManifests), nil
}

// runSPSWorkloadLocally works like pathwaysutils run_workload: it starts a proxy in the cluster, port-forwards
// to it and runs the command on this machine with the terminal attached, so pdb works. The proxy is deleted on exit.
func (g *GKEOrchestrator) runSPSWorkloadLocally(job orchestrator.JobDefinition, inst spsInstance, ns string) error {
	if len(job.RawMounts) > 0 {
		return fmt.Errorf("--mount requires --image or --base-image when used with --pathways-run-sps-workload")
	}
	manifest, err := g.generateSPSClientManifest(job, inst, "")
	if err != nil {
		return err
	}
	if job.DryRunManifest != "" {
		return g.ApplyManifest(manifest, job.DryRunManifest, job.WorkloadName)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	if err := g.ApplyManifest(manifest, "", job.WorkloadName); err != nil {
		return err
	}
	defer g.deleteSPSClient(job.WorkloadName, ns)

	pod, err := g.waitForSPSProxy(job.WorkloadName, ns, sigCh)
	if err != nil {
		return err
	}
	port, stop, err := g.startPortForward(pod, ns)
	if err != nil {
		return err
	}
	defer stop()

	logging.Info("Proxy %s is ready. Running the command on this machine with JAX_BACKEND_TARGET=grpc://127.0.0.1:%d", pod, port)
	return runLocalCommand(job.CommandToRun, port, job.Env, sigCh)
}

func (g *GKEOrchestrator) waitForSPSProxy(name, ns string, sigCh <-chan os.Signal) (string, error) {
	logging.Info("Waiting for the SPS proxy to start (Kueue admission and CPU node scale-up can take a few minutes)...")
	selector := "jobset.sigs.k8s.io/jobset-name=" + name
	deadline := time.Now().Add(spsProxyStartTimeout)
	for time.Now().Before(deadline) {
		if pod := g.readySPSProxyPod(ns, selector); pod != "" {
			return pod, nil
		}
		select {
		case <-sigCh:
			return "", fmt.Errorf("interrupted while waiting for the SPS proxy")
		case <-time.After(5 * time.Second):
		}
	}
	var warnSuffix string
	if warnEvents := g.checkJobSetWarningEvents(ns, name); warnEvents != "" {
		warnSuffix = fmt.Sprintf("\nJobSet warning events:\n%s", warnEvents)
	}
	return "", fmt.Errorf("timed out after %s waiting for the SPS proxy of %q%s", spsProxyStartTimeout, name, warnSuffix)
}

func (g *GKEOrchestrator) readySPSProxyPod(ns, selector string) string {
	res := g.kubectl("get", "pods", "-n", ns, "-l", selector, "-o", `jsonpath={range .items[*]}{.metadata.name}{" "}{.status.phase}{"\n"}{end}`)
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == "Running" {
			if logs := g.kubectl("logs", "-n", ns, fields[0], "-c", "pathways-proxy"); strings.Contains(logs.Stdout+logs.Stderr, spsProxyReadyLog) {
				return fields[0]
			}
		}
	}
	return ""
}

func (g *GKEOrchestrator) startPortForward(pod, ns string) (int, func(), error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, nil, fmt.Errorf("failed to find a free local port: %w", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	ctx, cancel := context.WithCancel(context.Background())
	args := withKubeContext(g.kubeContext, []string{"port-forward", "-n", ns, "pod/" + pod, fmt.Sprintf("%d:%d", port, spsProxyPort)})
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Start(); err != nil {
		cancel()
		return 0, nil, fmt.Errorf("failed to start kubectl port-forward: %w", err)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	for i := 0; i < 60; i++ {
		if conn, err := net.DialTimeout("tcp", addr, time.Second); err == nil {
			_ = conn.Close()
			done := make(chan struct{})
			go keepPortForwardAlive(ctx, cmd, args, done)
			return port, func() { cancel(); <-done }, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	cancel()
	_ = cmd.Wait()
	return 0, nil, fmt.Errorf("port-forward to %s did not become ready: %s", pod, strings.TrimSpace(out.String()))
}

// keepPortForwardAlive restarts kubectl port-forward if it exits before ctx is cancelled (e.g. on Ctrl-C in pdb).
func keepPortForwardAlive(ctx context.Context, cmd *exec.Cmd, args []string, done chan<- struct{}) {
	defer close(done)
	for {
		_ = cmd.Wait()
		if ctx.Err() != nil {
			return
		}
		cmd = exec.CommandContext(ctx, "kubectl", args...)
		if err := cmd.Start(); err != nil {
			return
		}
	}
}

func runLocalCommand(command string, port int, env map[string]string, sigCh <-chan os.Signal) error {
	cmd := exec.Command("bash", "-c", command)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Env = append(os.Environ(), "JAX_PLATFORMS=proxy", fmt.Sprintf("JAX_BACKEND_TARGET=grpc://127.0.0.1:%d", port))
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %q: %w", command, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("command %q failed: %w", command, err)
			}
			return nil
		case s := <-sigCh:
			// Ctrl-C reaches the command directly (same process group), e.g. inside pdb; forward only SIGTERM.
			if s == syscall.SIGTERM {
				_ = cmd.Process.Signal(s)
			}
		}
	}
}

func (g *GKEOrchestrator) deleteSPSClient(name, ns string) {
	logging.Info("Deleting SPS client %s...", name)
	if res := g.kubectl("delete", "jobset", name, "-n", ns, "--ignore-not-found", "--wait=false"); res.ExitCode != 0 {
		logging.Warn("failed to delete SPS client %s: %s", name, strings.TrimSpace(res.Stderr))
	}
}
