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

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	corev1listers "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

var lastRxPackets = make(map[string]int64)

type GPUUnhealthyReason string

const (
	ConditionTypeGPUUnhealthy = "GPUUnhealthy"

	ReasonHealthCheckFailed GPUUnhealthyReason = "HealthCheckFailed"
	ReasonActiveTestFailed  GPUUnhealthyReason = "ActiveTestFailed"

	LabelHealthStatus = "cloud.google.com/health-check-status"

	SeverityWarning = "warning"
	SeverityFailure = "failure"
	SeverityFatal   = "fatal"
	SeverityInfo    = "info"
)

func main() {
	startTime := time.Now()
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Fatal("NODE_NAME environment variable is required")
	}

	checkInterval := flag.Duration("check-interval", 5*time.Minute, "Interval between health checks")
	flag.Parse()

	// Wire SIGINT/SIGTERM to ctx so shutdown is cooperative — the informer
	// stops, subprocesses started with CommandContext get killed, and the
	// main select returns cleanly.
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	log.Println("Starting nv-hostengine...")
	hostEngine := exec.CommandContext(ctx, "nv-hostengine", "-n")
	if err := hostEngine.Start(); err != nil {
		log.Fatalf("Failed to start nv-hostengine: %v", err)
	}

	go func() {
		err := hostEngine.Wait()
		// a non-nil ctx.Err() means ctx received a shutdown signal, so nv-hostengine
		// was killed as part of that shutdown instead of errored out. In that case
		// we don't Fatalf
		if ctx.Err() != nil {
			log.Printf("nv-hostengine exited during container shutdown: %v", err)
			return
		}
		log.Fatalf("nv-hostengine exited unexpectedly: %v", err)
	}()

	if err := waitForHostEngine(ctx, 30*time.Second); err != nil {
		log.Fatalf("nv-hostengine not ready: %v", err)
	}

	log.Println("Setting all health watches via dcgmi...")
	setupCtx, setupCancel := context.WithTimeout(ctx, 30*time.Second)
	err := exec.CommandContext(setupCtx, "dcgmi", "health", "-s", "a").Run()
	setupCancel()
	if err != nil {
		log.Fatalf("Failed to set health watches: %v", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("Failed to create in-cluster config: %v", err)
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("Failed to create kubernetes clientset: %v", err)
	}

	tweakListOptions := func(options *metav1.ListOptions) {
		options.FieldSelector = fmt.Sprintf("metadata.name=%s", nodeName)
	}
	factory := informers.NewSharedInformerFactoryWithOptions(clientset, 5*time.Minute, informers.WithTweakListOptions(tweakListOptions))
	nodeInformer := factory.Core().V1().Nodes()
	informer := nodeInformer.Informer()

	log.Println("Starting Node informer cache...")
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Fatalf("Timed out waiting for caches to sync")
	}

	ticker := time.NewTicker(*checkInterval)
	defer ticker.Stop()

	log.Printf("Starting health check loop for node %s", nodeName)
	runHealthCheck(ctx, clientset, nodeInformer.Lister(), nodeName, startTime)
	for {
		select {
		case <-ticker.C:
			runHealthCheck(ctx, clientset, nodeInformer.Lister(), nodeName, startTime)
		case <-ctx.Done():
			log.Println("Shutting down...")
			return
		}
	}
}

// waitForHostEngine polls dcgmi discovery until nv-hostengine responds, or
// timeout elapses. This replaces a blind time.Sleep — slow-boot nodes fail
// on 5s but usually succeed within 10-15s.
func waitForHostEngine(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
		err := exec.CommandContext(probeCtx, "dcgmi", "discovery", "-l").Run()
		probeCancel()
		if err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for nv-hostengine: %v", err)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

func getFatalXids() map[int]struct{} {
	data, err := os.ReadFile("/etc/dcgm-healthcheck/fatal-xids")
	if err != nil {
		return make(map[int]struct{})
	}
	return parseFatalXids(data)
}

func parseFatalXids(data []byte) map[int]struct{} {
	fatalXids := make(map[int]struct{})
	for _, xidStr := range strings.Split(string(data), ",") {
		xidStr = strings.TrimSpace(xidStr)
		if xidStr == "" {
			continue
		}
		if xid, err := strconv.Atoi(xidStr); err == nil {
			fatalXids[xid] = struct{}{}
		}
	}
	return fatalXids
}

func runHealthCheck(mainCtx context.Context, clientset *kubernetes.Clientset, nodeLister corev1listers.NodeLister, nodeName string, startTime time.Time) {
	ctx, cancel := context.WithTimeout(mainCtx, 5*time.Minute)
	defer cancel()

	var errorMessages []string
	// SeverityWarning is the baseline: when no check fails, errorMessages is
	// empty and no NodeCondition/label is written, so this default is never
	// surfaced. It only takes effect after a check returns an error without
	// its own severity — escalating severity can happen but never lowering it.
	highestSeverity := SeverityWarning

	fatalXids := getFatalXids()

	passiveChecks := []struct {
		name string
		fn   func(context.Context) (string, error)
	}{
		{"dcgmi health", runDcgmiHealth},
		// {"NIC heartbeat", checkNICHeartbeat},
		{"XID/SXID monitoring", func(c context.Context) (string, error) { return checkKernelLogsForXidSxid(c, fatalXids, startTime) }},
		{"ECC errors", checkECCErrors},
		{"PCIe link health", checkPCIe},
		{"InfiniBand links", checkIB},
		{"GPU temperature", checkTemperature},
		// {"HCA Firmware", checkHCAFW},
		// {"GPU Power draw", checkPower},
	}
	for _, chk := range passiveChecks {
		sev, err := chk.fn(ctx)
		if err == nil {
			continue
		}
		highestSeverity = mergeStatus(highestSeverity, sev)
		errorMessages = append(errorMessages, fmt.Sprintf("%s: %v", chk.name, err))
	}

	node, err := nodeLister.Get(nodeName)
	if err != nil {
		log.Printf("Failed to get node %s from cache: %v", nodeName, err)
		return
	}

	hasIssue := false
	conditionExists := false
	var currentReason string
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeConditionType(ConditionTypeGPUUnhealthy) {
			conditionExists = true
			if condition.Status == corev1.ConditionTrue {
				hasIssue = true
			}
			currentReason = string(condition.Reason)
			break
		}
	}

	_, hasStatus := node.Labels[LabelHealthStatus]

	if len(errorMessages) == 0 {
		if currentReason != string(ReasonActiveTestFailed) && (conditionExists || hasStatus) {
			log.Println("Node is now healthy from passive checks. Ensuring condition/labels are cleared.")
			clearNodeHealth(ctx, clientset, nodeName, conditionExists, hasStatus)
		}
		return
	}

	desiredMessage := strings.Join(errorMessages, " | ")
	log.Printf("Passive checks failed: %s", desiredMessage)

	// When the node already carries an active-test-failed reason, keep
	// that reason (which has precedence) but still surface the passive
	// severity/message via mergeMessage inside updateNodeHealth.
	reason := ReasonHealthCheckFailed
	if hasIssue && currentReason == string(ReasonActiveTestFailed) {
		reason = ReasonActiveTestFailed
	}

	updateNodeHealth(ctx, clientset, node, reason, highestSeverity, desiredMessage)
}

func extractOverflowMessages(v interface{}) []string {
	var messages []string
	switch val := v.(type) {
	case map[string]interface{}:
		// Sort keys so identical dcgmi output produces an identical joined
		// message across ticks — otherwise randomized map iteration order
		// causes constant Node PATCH churn.
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			child := val[k]
			if k == "overflow" {
				if arr, ok := child.([]interface{}); ok {
					var parts []string
					for _, p := range arr {
						if s, ok := p.(string); ok {
							parts = append(parts, strings.TrimSpace(s))
						}
					}
					messages = append(messages, strings.Join(parts, " "))
				}
			} else {
				messages = append(messages, extractOverflowMessages(child)...)
			}
		}
	case []interface{}:
		for _, child := range val {
			messages = append(messages, extractOverflowMessages(child)...)
		}
	}
	return messages
}

func runDcgmiHealth(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "dcgmi", "health", "-c", "-j").CombinedOutput()
	if err != nil {
		return SeverityWarning, fmt.Errorf("%v: %s", err, string(out))
	}

	var parsed struct {
		Body map[string]interface{} `json:"body"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return SeverityWarning, fmt.Errorf("failed to parse dcgmi health output: %v", err)
	}

	overallHealthVal := ""
	if overall, ok := parsed.Body["Overall Health"].(map[string]interface{}); ok {
		if val, ok := overall["value"].(string); ok {
			overallHealthVal = val
		}
	}

	if overallHealthVal != "Healthy" {
		severity := SeverityWarning
		if strings.ToLower(overallHealthVal) == SeverityFailure {
			severity = SeverityFailure
		}

		detailsStr := ""
		delete(parsed.Body, "Overall Health")

		messages := extractOverflowMessages(parsed.Body)
		if len(messages) > 0 {
			detailsStr = fmt.Sprintf(" | Details: %s", strings.Join(messages, "; "))
		}

		return severity, fmt.Errorf("health issues detected (Overall Health: %s)%s", overallHealthVal, detailsStr)
	}

	return "", nil
}

func checkNICHeartbeat(ctx context.Context) (string, error) {
	sysClassNet := os.Getenv("SYS_CLASS_NET")
	if sysClassNet == "" {
		sysClassNet = "/sys/class/net"
	}
	matches, err := filepath.Glob(filepath.Join(sysClassNet, "gpu*rdma*/statistics/rx_packets"))
	if err != nil || len(matches) == 0 {
		return SeverityWarning, fmt.Errorf("no gpu rdma net interfaces found in %s", sysClassNet)
	}
	allZeroDiff := true
	for _, match := range matches {
		data, err := os.ReadFile(match)
		if err != nil {
			continue
		}
		val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err != nil {
			continue
		}

		if lastVal, ok := lastRxPackets[match]; ok {
			if val-lastVal > 0 {
				allZeroDiff = false
			}
		} else {
			// First check, don't fail immediately
			allZeroDiff = false
		}
		lastRxPackets[match] = val
	}
	if allZeroDiff {
		return SeverityWarning, fmt.Errorf("rx_packets difference is 0 on all GPU RDMA interfaces")
	}
	return "", nil
}

func checkHCAFW(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "dmesg").CombinedOutput()
	if err != nil {
		log.Printf("Cannot run dmesg for HCA FW monitoring: %v", err)
		return "", nil
	}
	lines := strings.Split(string(out), "\n")
	// Only check the last 1000 lines for simplicity
	start := len(lines) - 1000
	if start < 0 {
		start = 0
	}
	for i := start; i < len(lines); i++ {
		if strings.Contains(lines[i], "Health issue observed, firmware internal error") {
			return SeverityWarning, fmt.Errorf("firmware internal error found in dmesg: %s", lines[i])
		}
	}
	return "", nil
}

func getSeverity(s string) int {
	switch strings.ToLower(s) {
	case SeverityFatal:
		return 4
	case SeverityFailure:
		return 3
	case SeverityWarning:
		return 2
	case SeverityInfo:
		return 1
	default:
		return 0
	}
}

func mergeStatus(oldStatus, newStatus string) string {
	if getSeverity(oldStatus) > getSeverity(newStatus) {
		return oldStatus
	}
	return newStatus
}

func mergeMessage(oldMsg, newMsg string) string {
	if oldMsg == "" {
		return newMsg
	}
	if newMsg == "" {
		return oldMsg
	}

	var segments []string
	seen := make(map[string]bool)

	for _, seg := range strings.Split(oldMsg, "|") {
		seg = strings.TrimSpace(seg)
		if seg != "" && !seen[seg] {
			segments = append(segments, seg)
			seen[seg] = true
		}
	}
	for _, seg := range strings.Split(newMsg, "|") {
		seg = strings.TrimSpace(seg)
		if seg != "" && !seen[seg] {
			segments = append(segments, seg)
			seen[seg] = true
		}
	}

	return strings.Join(segments, " | ")
}

// The patch shapes are declared as typed structs and marshaled with
// json.Marshal so that user-derived strings (dmesg lines, dcgmi output) are
// escaped by the standard JSON encoder rather than fmt %q — Go's %q emits
// invalid JSON escapes (\a, \v, \xHH) for control bytes.

type nodeStatusPatch struct {
	Status struct {
		Conditions []corev1.NodeCondition `json:"conditions"`
	} `json:"status"`
}

type nodeLabelsPatch struct {
	Metadata struct {
		Labels map[string]string `json:"labels"`
	} `json:"metadata"`
}

type nodeLabelsClearPatch struct {
	Metadata struct {
		Labels map[string]interface{} `json:"labels"`
	} `json:"metadata"`
}

type nodeConditionDeletePatch struct {
	Status struct {
		Conditions []map[string]interface{} `json:"conditions"`
	} `json:"status"`
}

func updateNodeHealth(ctx context.Context, clientset *kubernetes.Clientset, node *corev1.Node, reason GPUUnhealthyReason, status, details string) {
	nodeName := node.Name

	oldStatus := ""
	oldMsg := ""
	oldReason := ""
	oldCondStatus := corev1.ConditionUnknown
	var oldTransitionTime time.Time
	if val, ok := node.Labels[LabelHealthStatus]; ok {
		oldStatus = val
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeConditionType(ConditionTypeGPUUnhealthy) {
			oldMsg = cond.Message
			oldReason = string(cond.Reason)
			oldCondStatus = cond.Status
			oldTransitionTime = cond.LastTransitionTime.Time
			break
		}
	}

	finalStatus := mergeStatus(oldStatus, status)

	// Replace the message only when the current condition is a
	// same-reason passive re-report (rotating passive failures should
	// not accumulate). Otherwise merge — either the reason is transitioning
	// (preserve prior signal) or we're accumulating active-test context.
	var finalMsg string
	if oldReason != "" && !(oldReason == string(reason) && reason == ReasonHealthCheckFailed) {
		finalMsg = mergeMessage(oldMsg, details)
	} else {
		finalMsg = details
	}

	// No-op if the node already reflects the intended state — otherwise we
	// would rewrite lastTransitionTime every tick and thrash the API server.
	if oldCondStatus == corev1.ConditionTrue &&
		oldReason == string(reason) &&
		oldMsg == finalMsg &&
		oldStatus == finalStatus {
		return
	}

	// Only advance lastTransitionTime when the Status field is actually transitioning
	transitionTime := time.Now()
	if oldCondStatus == corev1.ConditionTrue {
		transitionTime = oldTransitionTime
	}

	var condPatch nodeStatusPatch
	condPatch.Status.Conditions = []corev1.NodeCondition{{
		Type:               corev1.NodeConditionType(ConditionTypeGPUUnhealthy),
		Status:             corev1.ConditionTrue,
		Reason:             string(reason),
		Message:            finalMsg,
		LastTransitionTime: metav1.Time{Time: transitionTime},
	}}

	var labelsPatch nodeLabelsPatch
	labelsPatch.Metadata.Labels = map[string]string{LabelHealthStatus: finalStatus}

	applyPatches(ctx, clientset, nodeName, condPatch, labelsPatch, finalStatus)
}

func applyPatches(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, condPatch nodeStatusPatch, labelsPatch nodeLabelsPatch, finalStatus string) {
	statusPatchBytes, err := json.Marshal(condPatch)
	if err != nil {
		log.Printf("Error building status patch for %s: %v", nodeName, err)
		return
	}
	if _, err := clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, statusPatchBytes, metav1.PatchOptions{}, "status"); err != nil {
		log.Printf("Error patching node status %s: %v", nodeName, err)
	}

	labelsPatchBytes, err := json.Marshal(labelsPatch)
	if err != nil {
		log.Printf("Error building metadata patch for %s: %v", nodeName, err)
		return
	}
	if _, err := clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, labelsPatchBytes, metav1.PatchOptions{}); err != nil {
		log.Printf("Error patching node metadata %s: %v", nodeName, err)
	} else {
		log.Printf("Successfully updated node %s with status %s", nodeName, finalStatus)
	}
}

var xidRegex = regexp.MustCompile(`(?i)(?:xid|sxid)(?:\s*\(.*?\))?:\s*(\d+)`)

func checkKernelLogsForXidSxid(ctx context.Context, fatalXids map[int]struct{}, startTime time.Time) (string, error) {
	timeStr := startTime.Format("2006-01-02 15:04:05")
	out, err := exec.CommandContext(ctx, "dmesg", "--since", timeStr).CombinedOutput()
	if err != nil {
		log.Printf("Cannot run dmesg for kernel log monitoring: %v", err)
		return "", nil
	}
	lines := strings.Split(string(out), "\n")

	latestByCode := make(map[int]string)
	severity := SeverityWarning

	for _, line := range lines {
		lowerLine := strings.ToLower(line)
		if strings.Contains(lowerLine, "xid") || strings.Contains(lowerLine, "sxid") {
			matches := xidRegex.FindStringSubmatch(line)
			if len(matches) > 1 {
				if xid, err := strconv.Atoi(matches[1]); err == nil {
					latestByCode[xid] = line
					if _, ok := fatalXids[xid]; ok {
						severity = SeverityFatal
					}
				}
			}
		}
	}

	var codes []int
	for code := range latestByCode {
		codes = append(codes, code)
	}
	// sort the codes to ensure the generated string is deterministic.
	// Otherwise, random map iteration order will cause constant K8s API patches.
	sort.Ints(codes)

	var foundErrors []string
	for _, code := range codes {
		foundErrors = append(foundErrors, latestByCode[code])
	}

	if len(foundErrors) > 0 {
		return severity, fmt.Errorf("XID/SXID error(s) found in dmesg: %s", strings.Join(foundErrors, " | "))
	}
	return "", nil
}

func nvidiaSmiPath() string {
	if _, err := os.Stat("/usr/bin/nvidia-smi"); err == nil {
		return "/usr/bin/nvidia-smi"
	}
	return "/usr/local/nvidia/bin/nvidia-smi"
}

func checkECCErrors(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=ecc.errors.corrected.volatile.total,ecc.errors.uncorrected.volatile.total", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return SeverityWarning, fmt.Errorf("nvidia-smi ecc query failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			uncorr, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			if uncorr > 0 {
				return SeverityWarning, fmt.Errorf("found uncorrected ECC errors on GPU %d: %d", i, uncorr)
			}
		}
	}
	return "", nil
}

func checkPCIe(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=pcie.link.width.current,pcie.link.width.max", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return SeverityWarning, fmt.Errorf("nvidia-smi pcie query failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			width, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
			maxWidth, errMax := strconv.Atoi(strings.TrimSpace(parts[1]))
			if errMax != nil || maxWidth <= 0 {
				maxWidth = 16 // fallback
			}
			if width > 0 && width < maxWidth {
				return SeverityWarning, fmt.Errorf("PCIe link width degraded on GPU %d: %d (expected %d)", i, width, maxWidth)
			}
		}
	}
	return "", nil
}

func checkIB(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, "ibstat").CombinedOutput()
	if err != nil {
		return "", nil
	}
	if strings.Contains(string(out), "Down") {
		return SeverityWarning, fmt.Errorf("ibstat reported Down")
	}
	return "", nil
}

func checkTemperature(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=temperature.gpu,temperature.gpu.tlimit", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return SeverityWarning, fmt.Errorf("nvidia-smi temperature query failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			tempStr := strings.TrimSpace(parts[0])
			limitStr := strings.TrimSpace(parts[1])

			countdown, err := strconv.Atoi(limitStr)
			if err == nil {
				if countdown <= 0 {
					return SeverityWarning, fmt.Errorf("GPU %d temperature too high: current is %s degree which is %d degrees over target temperature", i, tempStr, -countdown)
				}
			} else {
				// Fallback to absolute temperature check if countdown is not supported
				temp, errTemp := strconv.Atoi(tempStr)
				if errTemp == nil && temp >= 90 {
					return SeverityWarning, fmt.Errorf("GPU %d temperature too high: %d C >= 90 C", i, temp)
				}
			}
		}
	}
	return "", nil
}

func checkPower(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=power.draw", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return SeverityWarning, fmt.Errorf("nvidia-smi power query failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		val, err := strconv.ParseFloat(strings.TrimSpace(line), 64)
		if err == nil && val > 700.0 {
			return SeverityWarning, fmt.Errorf("GPU %d power draw too high: %.1f W", i, val)
		}
	}
	return "", nil
}

func clearNodeHealth(ctx context.Context, clientset *kubernetes.Clientset, nodeName string, clearCondition, clearLabel bool) {
	if clearCondition {
		var condPatch nodeConditionDeletePatch
		condPatch.Status.Conditions = []map[string]interface{}{
			{"type": ConditionTypeGPUUnhealthy, "$patch": "delete"},
		}
		patchBytes, err := json.Marshal(condPatch)
		if err != nil {
			log.Printf("Error building status delete patch for %s: %v", nodeName, err)
		} else if _, err := clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}, "status"); err != nil {
			log.Printf("Error deleting node status condition %s: %v", nodeName, err)
		}
	}

	if clearLabel {
		var labelsPatch nodeLabelsClearPatch
		labelsPatch.Metadata.Labels = map[string]interface{}{LabelHealthStatus: nil}
		patchBytes, err := json.Marshal(labelsPatch)
		if err != nil {
			log.Printf("Error building metadata clear patch for %s: %v", nodeName, err)
		} else if _, err := clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, patchBytes, metav1.PatchOptions{}); err != nil {
			log.Printf("Info: Attempted to clear health labels on node %s: %v", nodeName, err)
		}
	}
}
