package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
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
	nodeName := os.Getenv("NODE_NAME")
	if nodeName == "" {
		log.Fatal("NODE_NAME environment variable is required")
	}

	checkInterval := flag.Duration("check-interval", 5*time.Minute, "Interval between health checks")
	flag.Parse()

	log.Println("Starting nv-hostengine...")
	hostEngine := exec.Command("nv-hostengine", "-n")
	if err := hostEngine.Start(); err != nil {
		log.Fatalf("Failed to start nv-hostengine: %v", err)
	}

	go func() {
		err := hostEngine.Wait()
		log.Fatalf("nv-hostengine exited unexpectedly: %v", err)
	}()

	time.Sleep(5 * time.Second)

	log.Println("Setting all health watches via dcgmi...")
	if err := exec.Command("dcgmi", "health", "-s", "a").Run(); err != nil {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Println("Starting Node informer cache...")
	factory.Start(ctx.Done())
	if !cache.WaitForCacheSync(ctx.Done(), informer.HasSynced) {
		log.Fatalf("Timed out waiting for caches to sync")
	}

	ticker := time.NewTicker(*checkInterval)
	defer ticker.Stop()

	log.Printf("Starting health check loop for node %s", nodeName)
	runHealthCheck(clientset, nodeInformer.Lister(), nodeName, true)
	for {
		select {
		case <-ticker.C:
			runHealthCheck(clientset, nodeInformer.Lister(), nodeName, false)
		case <-ctx.Done():
			log.Println("Shutting down...")
			return
		}
	}
}

func getFatalXids() map[int]struct{} {
	fatalXids := make(map[int]struct{})
	data, err := os.ReadFile("/etc/dcgm-healthcheck/fatal-xids")
	if err != nil {
		return fatalXids
	}

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

func runHealthCheck(clientset *kubernetes.Clientset, nodeLister corev1listers.NodeLister, nodeName string, isFirstRun bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	var errorMessages []string
	highestSeverity := SeverityWarning
	updateSeverity := func(sev string) {
		if getSeverity(sev) > getSeverity(highestSeverity) {
			highestSeverity = sev
		}
	}

	fatalXids := getFatalXids()

	// 1. Passive Test: DCGMI Health
	if sev, err := runDcgmiHealth(ctx); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, fmt.Sprintf("dcgmi health failed: %v", err))
	}

	// 2. Passive Test: NIC Heartbeat (ibv_devinfo check port state)
	if sev, err := checkNICHeartbeat(ctx); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, fmt.Sprintf("NIC heartbeat failed: %v", err))
	}

	// 3. Passive Test: HCA FW monitoring
	// if sev, err := checkHCAFW(ctx); err != nil {
	// 	updateSeverity(sev)
	// 	errorMessages = append(errorMessages, fmt.Sprintf("HCA FW check failed: %v", err))
	// }

	// 4. Passive Test: XID / SXID monitoring
	if sev, err := checkKernelLogsForXidSxid(ctx, fatalXids); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, err.Error())
	}

	// 5. Passive Test: ECC Errors
	if sev, err := checkECCErrors(ctx); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, err.Error())
	}

	// 6. Passive Test: PCIe Link Health
	if sev, err := checkPCIe(ctx); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, err.Error())
	}

	// 7. Passive Test: InfiniBand Links
	if sev, err := checkIB(ctx); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, err.Error())
	}

	// 8. Passive Test: GPU Temperature
	if sev, err := checkTemperature(ctx); err != nil {
		updateSeverity(sev)
		errorMessages = append(errorMessages, err.Error())
	}

	// 9. Passive Test: GPU Power and Utilization
	// if sev, err := checkPower(ctx); err != nil {
	// 	updateSeverity(sev)
	// 	errorMessages = append(errorMessages, err.Error())
	// }

	node, err := nodeLister.Get(nodeName)
	if err != nil {
		log.Printf("Failed to get node %s from cache: %v", nodeName, err)
		return
	}

	hasIssue := false
	var currentMessage string
	var currentReason string
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeConditionType(ConditionTypeGPUUnhealthy) {
			if condition.Status == corev1.ConditionTrue {
				hasIssue = true
			}
			currentMessage = condition.Message
			currentReason = condition.Reason
			break
		}
	}

	hasStatus := false
	if node.Labels != nil {
		_, hasStatus = node.Labels[LabelHealthStatus]
	}

	if len(errorMessages) == 0 {
		if currentReason != string(ReasonActiveTestFailed) && (hasIssue || currentMessage != "Node is healthy" || hasStatus) {
			log.Println("Node is now healthy from passive checks. Ensuring condition/labels are cleared.")
			clearNodeHealth(ctx, clientset, nodeName)
		}
	} else {
		desiredMessage := strings.Join(errorMessages, " | ")
		log.Printf("Passive checks failed: %s", desiredMessage)
		currentStatus := ""
		if hasStatus {
			currentStatus = node.Labels[LabelHealthStatus]
		}

		if hasIssue && currentReason == string(ReasonActiveTestFailed) {
			log.Printf("Passive checks failed but node has active test failure. Preserving active failure.")
		} else if !hasIssue || currentMessage != desiredMessage || currentStatus != highestSeverity {
			log.Printf("Node is in warning state. Updating API...")
			updateNodeHealth(ctx, clientset, node, ReasonHealthCheckFailed, highestSeverity, desiredMessage)
		}
	}
}

func extractOverflowMessages(v interface{}) []string {
	var messages []string
	switch val := v.(type) {
	case map[string]interface{}:
		for k, child := range val {
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
	out, err := exec.Command("dmesg").CombinedOutput()
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

func updateNodeHealth(ctx context.Context, clientset *kubernetes.Clientset, node *corev1.Node, reason GPUUnhealthyReason, status, details string) {
	now := time.Now().Format(time.RFC3339)
	nodeName := node.Name

	oldStatus := ""
	oldMsg := ""
	oldReason := ""
	if node.Labels != nil {
		if val, ok := node.Labels[LabelHealthStatus]; ok {
			oldStatus = val
		}
	}
	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeConditionType(ConditionTypeGPUUnhealthy) {
			oldMsg = cond.Message
			oldReason = string(cond.Reason)
			break
		}
	}

	finalStatus := mergeStatus(oldStatus, status)
	finalMsg := details
	if oldReason != "" && oldReason != string(reason) {
		finalMsg = mergeMessage(oldMsg, details)
	}

	// 1. Patch Status Conditions
	statusPatch := fmt.Sprintf(`{"status":{"conditions":[{"type":"%s","status":"True","reason":"%s","message":%q,"lastTransitionTime":"%s"}]}}`,
		ConditionTypeGPUUnhealthy, reason, finalMsg, now)
	_, err := clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, []byte(statusPatch), metav1.PatchOptions{}, "status")
	if err != nil {
		log.Printf("Error patching node status %s: %v", nodeName, err)
	}

	// 2. Patch Metadata Labels
	metadataPatch := fmt.Sprintf(`{"metadata":{"labels":{"%s":"%s"}}}`,
		LabelHealthStatus, finalStatus)
	_, err = clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, []byte(metadataPatch), metav1.PatchOptions{})
	if err != nil {
		log.Printf("Error patching node metadata %s: %v", nodeName, err)
	} else {
		log.Printf("Successfully updated node %s with status %s", nodeName, finalStatus)
	}
}

var xidRegex = regexp.MustCompile(`(?i)(?:xid|sxid)(?:\s*\(.*?\))?:\s*(\d+)`)

func checkKernelLogsForXidSxid(ctx context.Context, fatalXids map[int]struct{}) (string, error) {
	out, err := exec.Command("dmesg").CombinedOutput()
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
		return "", nil // skip if nvidia-smi is unavailable or fails
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
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=pcie.link.gen.current,pcie.link.width.current", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) >= 2 {
			width, _ := strconv.Atoi(strings.TrimSpace(parts[1]))
			if width > 0 && width < 16 {
				return SeverityWarning, fmt.Errorf("PCIe link width degraded on GPU %d: %d", i, width)
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

	outDiag, errDiag := exec.CommandContext(ctx, "ibdiagnet").Output()
	if errDiag == nil && (strings.Contains(string(outDiag), "Error") || strings.Contains(string(outDiag), "Fail")) {
		return SeverityWarning, fmt.Errorf("ibdiagnet reported errors")
	}
	return "", nil
}

func checkTemperature(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=temperature.gpu", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return "", nil
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		temp, err := strconv.Atoi(strings.TrimSpace(line))
		if err == nil && temp > 90 {
			return SeverityWarning, fmt.Errorf("GPU %d temperature too high: %d C", i, temp)
		}
	}
	return "", nil
}

func checkPower(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, nvidiaSmiPath(), "--query-gpu=power.draw", "--format=csv,noheader,nounits").CombinedOutput()
	if err != nil {
		return "", nil
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

func clearNodeHealth(ctx context.Context, clientset *kubernetes.Clientset, nodeName string) {
	// 1. Patch Status Conditions (Delete the condition)
	statusPatch := fmt.Sprintf(`{"status":{"conditions":[{"type":"%s","$patch":"delete"}]}}`, ConditionTypeGPUUnhealthy)
	_, err := clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, []byte(statusPatch), metav1.PatchOptions{}, "status")
	if err != nil {
		log.Printf("Error deleting node status condition %s: %v", nodeName, err)
	}

	// 2. Clear Metadata Labels
	metadataPatch := fmt.Sprintf(`{"metadata":{"labels":{"%s":null}}}`,
		LabelHealthStatus)
	_, err = clientset.CoreV1().Nodes().Patch(ctx, nodeName, types.StrategicMergePatchType, []byte(metadataPatch), metav1.PatchOptions{})
	if err != nil {
		log.Printf("Info: Attempted to clear health labels on node %s: %v", nodeName, err)
	}
}
