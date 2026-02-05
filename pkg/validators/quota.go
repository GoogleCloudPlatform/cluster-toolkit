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

package validators

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"strconv"
	"strings"
	"sync"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"

	"math"
	"time"

	"github.com/zclconf/go-cty/cty"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	testQuotaAvailabilityName = "test_quota_availability"
	localSSDSizeGB            = 375.0
)

var (
	ErrUnknownValue        = errors.New("value is unknown")
	machineTypeFamilyRegex = regexp.MustCompile(`^([a-z][0-9]+[a-z]?)-`)
)

type QuotaClient interface {
	GetRegion(projectID, region string) (*compute.Region, error)
	GetProject(projectID string) (*compute.Project, error)
	GetMachineType(projectID, zone, machineType string) (*compute.MachineType, error)
}

type GCPQuotaClient struct {
	svc          *compute.Service
	regions      sync.Map
	projects     sync.Map
	machineTypes sync.Map
}

func NewGCPQuotaClient(ctx context.Context, projectID string) (*GCPQuotaClient, error) {
	opts := []option.ClientOption{}
	if projectID != "" {
		opts = append(opts, option.WithQuotaProject(projectID))
	}

	svc, err := compute.NewService(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create compute service: %w", err)
	}

	return &GCPQuotaClient{svc: svc}, nil
}

func retryCall[T any](op func() (T, error)) (T, error) {
	var res T
	var err error
	var zero T
	maxRetries := 5
	baseDelay := 500 * time.Millisecond

	for i := 0; i <= maxRetries; i++ {
		res, err = op()
		if err == nil {
			return res, nil
		}

		isRetryable := false
		var gErr *googleapi.Error
		if errors.As(err, &gErr) {
			if gErr.Code == 429 || (gErr.Code >= 500 && gErr.Code < 600) {
				isRetryable = true
			}
		}

		if !isRetryable {
			return zero, err
		}

		if i < maxRetries {
			delay := float64(baseDelay) * math.Pow(2, float64(i))
			time.Sleep(time.Duration(delay))
		}
	}
	return zero, err
}

func (c *GCPQuotaClient) GetRegion(projectID, region string) (*compute.Region, error) {
	key := fmt.Sprintf("%s/%s", projectID, region)
	if v, ok := c.regions.Load(key); ok {
		return v.(*compute.Region), nil
	}

	r, err := retryCall(func() (*compute.Region, error) {
		return c.svc.Regions.Get(projectID, region).Do()
	})
	if err != nil {
		return nil, err
	}
	c.regions.Store(key, r)
	return r, nil
}

func (c *GCPQuotaClient) GetProject(projectID string) (*compute.Project, error) {
	if v, ok := c.projects.Load(projectID); ok {
		return v.(*compute.Project), nil
	}

	p, err := retryCall(func() (*compute.Project, error) {
		return c.svc.Projects.Get(projectID).Do()
	})
	if err != nil {
		return nil, err
	}
	c.projects.Store(projectID, p)
	return p, nil
}

func (c *GCPQuotaClient) GetMachineType(projectID, zone, machineType string) (*compute.MachineType, error) {
	key := fmt.Sprintf("%s/%s/%s", projectID, zone, machineType)
	if v, ok := c.machineTypes.Load(key); ok {
		return v.(*compute.MachineType), nil
	}

	mt, err := retryCall(func() (*compute.MachineType, error) {
		return c.svc.MachineTypes.Get(projectID, zone, machineType).Do()
	})
	if err != nil {
		return nil, err
	}
	c.machineTypes.Store(key, mt)
	return mt, nil
}

type QuotaRequirement struct {
	Metric    string
	Region    string
	Needed    float64
	ProjectID string
}

func testQuotaAvailability(bp config.Blueprint, inputs config.Dict) error {
	projectID := inputs.Get("project_id").AsString()
	defaultRegion := ""
	if inputs.Has("region") {
		defaultRegion = inputs.Get("region").AsString()
	}

	client, err := NewGCPQuotaClient(context.Background(), projectID)
	if err != nil {
		var gErr *googleapi.Error
		if errors.As(err, &gErr) && gErr.Code == 403 {
			logging.Error("WARNING: quota validation skipped due to lack of permissions: %v", err)
			return nil
		}
		return handleClientError(err)
	}

	reqs, err := collectRequirements(bp, client, projectID, defaultRegion)
	if err != nil {
		return err
	}

	return verifyQuotas(client, reqs)
}

func collectRequirements(bp config.Blueprint, client QuotaClient, projectID, defaultRegion string) ([]QuotaRequirement, error) {
	totals := make(map[string]float64)
	var walkErr error

	bp.WalkModulesSafe(func(_ config.ModulePath, m *config.Module) {
		if walkErr != nil {
			return
		}
		settings := m.Settings

		zone, region := getModuleLocation(bp, settings, defaultRegion)
		count := getModuleCount(bp, string(m.ID), settings)

		if count == 0 {
			return
		}

		if !isReservationUsed(bp, settings) {
			addVMQuota(bp, client, settings, projectID, region, zone, count, totals, string(m.ID))
		} else {
			logging.Info("quota: module %s targets a specific reservation, skipping CPU/GPU/LocalSSD quota checks", m.ID)
		}

		addDiskQuota(bp, settings, projectID, region, count, totals)
		addNetworkQuota(bp, m, settings, projectID, totals)
		addFilestoreQuota(bp, settings, projectID, region, count, totals)
		addTPUQuota(bp, settings, projectID, region, count, totals)
	})

	if walkErr != nil {
		return nil, walkErr
	}

	var output []QuotaRequirement
	for key, val := range totals {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) == 3 {
			output = append(output, QuotaRequirement{
				ProjectID: parts[0],
				Region:    parts[1],
				Metric:    parts[2],
				Needed:    val,
			})
		}
	}
	return output, nil
}

func addTotal(totals map[string]float64, projectID, region, metric string, amount float64) {
	key := fmt.Sprintf("%s/%s/%s", projectID, region, metric)
	totals[key] += amount
}

func verifyQuotas(client QuotaClient, reqs []QuotaRequirement) error {
	var errs config.Errors

	for _, req := range reqs {
		if req.Needed <= 0 {
			continue
		}

		var limit float64 = -1
		var usage float64 = 0

		projID := req.ProjectID

		if req.Region == "global" {
			proj, err := client.GetProject(projID)
			if err != nil {
				errs.Add(fmt.Errorf("failed to get project %s: %w", projID, err))
				continue
			}
			for _, q := range proj.Quotas {
				if strings.EqualFold(q.Metric, req.Metric) {
					limit = q.Limit
					usage = q.Usage
					break
				}
			}
		} else {
			reg, err := client.GetRegion(projID, req.Region)
			if err != nil {
				errs.Add(fmt.Errorf("failed to get region %s: %w", req.Region, err))
				continue
			}
			for _, q := range reg.Quotas {
				if strings.EqualFold(q.Metric, req.Metric) {
					limit = q.Limit
					usage = q.Usage
					break
				}
			}
		}

		if limit == -1 {
			if req.Metric == "GPUS_ALL_REGIONS" {
				logging.Info("quota: metric GPUS_ALL_REGIONS not explicitly returned by API (common for new projects), skipping check")
			} else {
				logging.Info("quota: metric %s not found in region %s for project %s (usage check skipped)", req.Metric, req.Region, projID)
			}
			continue
		}

		if usage+req.Needed > limit {
			errs.Add(fmt.Errorf("insufficient quota for %s in %s (project: %s). Requested: %.2f, Used: %.2f, Limit: %.2f",
				req.Metric, req.Region, projID, req.Needed, usage, limit))
		} else {
			logging.Info("quota: %s in %s check passed (Req: %.2f, Used: %.2f, Limit: %.2f)", req.Metric, req.Region, req.Needed, usage, limit)
		}
	}

	return errs.OrNil()
}

func isUnknownError(err error) bool {
	return errors.Is(err, ErrUnknownValue) || strings.Contains(err.Error(), "unknown")
}

func evalToFloat64(bp config.Blueprint, v cty.Value) (float64, error) {
	val, err := bp.Eval(v)
	if err != nil {
		return 0, err
	}
	if !val.IsKnown() {
		return 0, ErrUnknownValue
	}
	return float64FromVal(val)
}

func evalToFloat64OrString(bp config.Blueprint, v cty.Value) (interface{}, error) {
	val, err := bp.Eval(v)
	if err != nil {
		return nil, err
	}
	if !val.IsKnown() {
		return nil, ErrUnknownValue
	}
	if val.Type() == cty.String {
		return val.AsString(), nil
	}
	f, err := float64FromVal(val)
	return f, err
}

func float64FromVal(v cty.Value) (float64, error) {
	if v.Type() == cty.Number {
		f, _ := v.AsBigFloat().Float64()
		return f, nil
	}
	if v.Type() == cty.String {
		f, err := strconv.ParseFloat(v.AsString(), 64)
		if err != nil {
			return 0, err
		}
		return f, nil
	}
	return 0, fmt.Errorf("cannot convert %s to float64", v.Type())
}

func evalBool(bp config.Blueprint, v cty.Value) (bool, error) {
	val, err := bp.Eval(v)
	if err != nil {
		return false, err
	}
	if !val.IsKnown() {
		return false, ErrUnknownValue
	}
	if val.Type() == cty.Bool {
		return val.True(), nil
	}
	if val.Type() == cty.String {
		return strconv.ParseBool(val.AsString())
	}
	return false, fmt.Errorf("not a bool")
}

func evalString(bp config.Blueprint, v cty.Value) (string, error) {
	val, err := bp.Eval(v)
	if err != nil {
		return "", err
	}
	if !val.IsKnown() {
		return "", ErrUnknownValue
	}
	if val.Type() == cty.String {
		return val.AsString(), nil
	}
	return "", fmt.Errorf("not a string")
}

func mapAcceleratorTypeToMetric(accType string) []string {
	original := accType
	lAccType := strings.ToLower(accType)

	var metricMap = map[string]string{
		"h100": "NVIDIA_H100_GPUS",
		"l4":   "NVIDIA_L4_GPUS",
		"t4":   "NVIDIA_T4_GPUS",
		"v100": "NVIDIA_V100_GPUS",
		"p100": "NVIDIA_P100_GPUS",
		"p4":   "NVIDIA_P4_GPUS",
		"k80":  "NVIDIA_K80_GPUS",
	}

	if strings.Contains(lAccType, "nvidia") {
		// Special case for a100
		if strings.Contains(lAccType, "a100") {
			if strings.Contains(lAccType, "80gb") {
				return []string{"NVIDIA_A100_80GB_GPUS"}
			}
			return []string{"NVIDIA_A100_GPUS"}
		}

		for key, metric := range metricMap {
			if strings.Contains(lAccType, key) {
				return []string{metric}
			}
		}
	}

	base := strings.ToUpper(original)
	base = strings.TrimPrefix(base, "NVIDIA-")
	base = strings.TrimPrefix(base, "TESLA-")
	base = strings.ReplaceAll(base, "-", "_")
	base = strings.ReplaceAll(base, "__", "_")
	base = strings.TrimPrefix(base, "NVIDIA_")
	base = strings.TrimPrefix(base, "TESLA_")

	return []string{fmt.Sprintf("NVIDIA_%s_GPUS", base)}
}

func getModuleLocation(bp config.Blueprint, settings config.Dict, defaultRegion string) (string, string) {
	var zone, region string

	if settings.Has("zone") {
		zVal, err := evalString(bp, settings.Get("zone"))
		if err == nil {
			zone = zVal
		}
	}
	if settings.Has("region") {
		rVal, err := evalString(bp, settings.Get("region"))
		if err == nil {
			region = rVal
		}
	}
	if region == "" && zone != "" {
		parts := strings.Split(zone, "-")
		if len(parts) > 2 {
			region = strings.Join(parts[:len(parts)-1], "-")
		}
	}
	if region == "" {
		region = defaultRegion
	}
	return zone, region
}

func getModuleCount(bp config.Blueprint, moduleID string, settings config.Dict) float64 {
	count := 1.0

	addCount := func(key string) {
		if settings.Has(key) {
			val, err := evalToFloat64(bp, settings.Get(key))
			if err == nil {
				count = val
			}
			if err != nil && isUnknownError(err) {
				logging.Error("WARNING: quota validation skipped for module %s: %s is unknown", moduleID, key)
				count = 0
			}
		}
	}

	if settings.Has("node_count_static") || settings.Has("node_count_dynamic_max") {
		return resolveNodeCount(bp, moduleID, settings)
	}

	if settings.Has("node_count") {
		addCount("node_count")
	} else if settings.Has("instance_count") {
		addCount("instance_count")
	} else if settings.Has("vm_count") {
		addCount("vm_count")
	}

	return count
}

func resolveNodeCount(bp config.Blueprint, moduleID string, settings config.Dict) float64 {
	c := 0.0
	found := false
	if settings.Has("node_count_static") {
		v, err := evalToFloat64(bp, settings.Get("node_count_static"))
		if err == nil {
			c += v
			found = true
		} else if isUnknownError(err) {
			logging.Error("WARNING: quota validation skipped for %s: node_count_static is unknown", moduleID)
		}
	}
	if settings.Has("node_count_dynamic_max") {
		v, err := evalToFloat64(bp, settings.Get("node_count_dynamic_max"))
		if err == nil {
			c += v
			found = true
		} else if isUnknownError(err) {
			logging.Error("WARNING: quota validation skipped for %s: node_count_dynamic_max is unknown", moduleID)
		}
	}
	if found {
		return c
	}
	return 1.0
}

func isReservationUsed(bp config.Blueprint, settings config.Dict) bool {
	if settings.Has("reservation_affinity") {
		val := settings.Get("reservation_affinity")
		v, err := bp.Eval(val)
		if err == nil && v.Type().IsObjectType() {
			attrs := v.AsValueMap()
			if t, ok := attrs["consume_reservation_type"]; ok && t.Type() == cty.String {
				if t.AsString() == "SPECIFIC_RESERVATION" {
					return true
				}
			}
		}
	}
	if settings.Has("reservation_name") {
		val := settings.Get("reservation_name")
		v, err := evalString(bp, val)
		if err == nil && v != "" {
			return true
		}
	}
	return false
}

func addVMQuota(bp config.Blueprint, client QuotaClient, settings config.Dict, projectID, region, zone string, count float64, totals map[string]float64, moduleID string) {
	if !settings.Has("machine_type") {
		return
	}

	if region == "" {
		logging.Error("WARNING: Could not determine region for module %s. Regional quota checks (CPUs, GPUs) will be skipped.", moduleID)
		return
	}

	mtStr, err := evalString(bp, settings.Get("machine_type"))
	if err != nil || mtStr == "" {
		return
	}

	lookupZone := resolveVMZone(client, projectID, region, zone, mtStr)
	mt, err := client.GetMachineType(projectID, lookupZone, mtStr)
	if err != nil {
		logging.Error("WARNING: quota: could not look up machine type %s in %s: %v. Usage check skipped.", mtStr, lookupZone, err)
		return
	}

	isSpot := checkSpotSettings(bp, settings)
	prefix := ""
	if isSpot {
		prefix = "PREEMPTIBLE_"
	}

	addCPUMetrics(mtStr, prefix, count, mt.GuestCpus, projectID, region, totals)

	addGPUMetrics(mt.Accelerators, prefix, count, projectID, region, totals)
}

func resolveVMZone(client QuotaClient, projectID, region, zone, mtStr string) string {
	if zone != "" {
		return zone
	}
	regObj, err := client.GetRegion(projectID, region)
	if err == nil && len(regObj.Zones) > 0 {
		zURL := regObj.Zones[0]
		parts := strings.Split(zURL, "/")
		return parts[len(parts)-1]
	}
	lookupZone := region + "-a"
	logging.Info("quota: failed to discover zones for region %s, falling back to %s for %s: %v", region, lookupZone, mtStr, err)
	return lookupZone
}

func checkSpotSettings(bp config.Blueprint, settings config.Dict) bool {
	keys := []string{"enable_spot_vm", "spot", "preemptible"}
	for _, k := range keys {
		if settings.Has(k) {
			v, err := evalBool(bp, settings.Get(k))
			if err == nil && v {
				return true
			}
		}
	}
	if settings.Has("provisioning_model") {
		s, err := evalString(bp, settings.Get("provisioning_model"))
		if err == nil && strings.ToUpper(s) == "SPOT" {
			return true
		}
	}
	return false
}

func addCPUMetrics(mtStr, prefix string, count float64, guestCpus int64, projectID, region string, totals map[string]float64) {
	cpuMetric := "CPUS"
	family := GetMachineTypeFamily(mtStr)
	if family != "" {
		if family != "n1" && family != "e2" && family != "f1" && family != "g1" {
			cpuMetric = strings.ToUpper(family) + "_CPUS"
		}
	}

	if family == "a3" || family == "g2" {
		logging.Info("quota: family %s detected for machine %s, skipping CPU quota check", family, mtStr)
	} else {
		cpuMetric = prefix + cpuMetric
		addTotal(totals, projectID, region, cpuMetric, float64(guestCpus)*count)
	}
}

func addGPUMetrics(accelerators []*compute.MachineTypeAccelerators, prefix string, count float64, projectID, region string, totals map[string]float64) {
	for _, acc := range accelerators {
		metricNames := mapAcceleratorTypeToMetric(acc.GuestAcceleratorType)
		for _, mName := range metricNames {
			addTotal(totals, projectID, region, prefix+mName, float64(acc.GuestAcceleratorCount)*count)
			addTotal(totals, projectID, "global", "GPUS_ALL_REGIONS", float64(acc.GuestAcceleratorCount)*count)
		}
	}
}

func addDiskQuota(bp config.Blueprint, settings config.Dict, projectID, region string, count float64, totals map[string]float64) {
	var diskSizeGB float64 = 0
	if settings.Has("disk_size_gb") {
		v, err := evalToFloat64(bp, settings.Get("disk_size_gb"))
		if err == nil {
			diskSizeGB += v
		}
	}
	if settings.Has("boot_disk_size_gb") {
		v, err := evalToFloat64(bp, settings.Get("boot_disk_size_gb"))
		if err == nil {
			diskSizeGB += v
		}
	}

	if settings.Has("local_ssd_count") {
		lCount, err := evalToFloat64(bp, settings.Get("local_ssd_count"))
		if err == nil {
			addTotal(totals, projectID, region, "LOCAL_SSD_TOTAL_GB", lCount*localSSDSizeGB*count)
		}
	}

	if diskSizeGB <= 0 {
		return
	}

	diskType := "pd-standard"
	if settings.Has("disk_type") {
		s, err := evalString(bp, settings.Get("disk_type"))
		if err == nil {
			diskType = s
		}
	}

	addDetailedDiskMetrics(bp, settings, diskType, diskSizeGB, count, projectID, region, totals)
}

func addDetailedDiskMetrics(bp config.Blueprint, settings config.Dict, diskType string, sizeGB, count float64, projectID, region string, totals map[string]float64) {
	if strings.Contains(diskType, "hyperdisk-balanced") {
		addTotal(totals, projectID, region, "HYPERDISK_BALANCED_TOTAL_GB", sizeGB*count)
		if settings.Has("provisioned_iops") {
			v, err := evalToFloat64(bp, settings.Get("provisioned_iops"))
			if err == nil {
				addTotal(totals, projectID, region, "HYPERDISK_BALANCED_IOPS", v*count)
			}
		}
		if settings.Has("provisioned_throughput") {
			v, err := evalToFloat64(bp, settings.Get("provisioned_throughput"))
			if err == nil {
				addTotal(totals, projectID, region, "HYPERDISK_BALANCED_THROUGHPUT", v*count)
			}
		}
		return
	}

	if strings.Contains(diskType, "pd-extreme") {
		addTotal(totals, projectID, region, "EXTREME_TOTAL_GB", sizeGB*count)
		if settings.Has("provisioned_iops") {
			v, err := evalToFloat64(bp, settings.Get("provisioned_iops"))
			if err == nil {
				addTotal(totals, projectID, region, "PD_EXTREME_TOTAL_PROVISIONED_IOPS", v*count)
			}
		}
		return
	}
	if strings.Contains(diskType, "ssd") || strings.Contains(diskType, "balanced") {
		addTotal(totals, projectID, region, "SSD_TOTAL_GB", sizeGB*count)
		return
	}
	if strings.Contains(diskType, "standard") {
		addTotal(totals, projectID, region, "STANDARD_TOTAL_GB", sizeGB*count)
		return
	}
}

func addNetworkQuota(bp config.Blueprint, m *config.Module, settings config.Dict, projectID string, totals map[string]float64) {
	netProjectID := resolveNetworkProjectID(bp, settings, projectID)

	if strings.Contains(m.Source, "network/vpc") || strings.Contains(m.Source, "gpu-rdma-vpc") {
		addTotal(totals, netProjectID, "global", "NETWORKS", 1)
	}

	if settings.Has("subnetworks") {
		val := settings.Get("subnetworks")
		v, err := bp.Eval(val)
		if err == nil && v.Type().IsListType() {
			iter := v.ElementIterator()
			for iter.Next() {
				_, elem := iter.Element()
				if elem.Type().IsObjectType() {
					addTotal(totals, netProjectID, "global", "SUBNETWORKS", 1)
				}
			}
		}
	}
}

func resolveNetworkProjectID(bp config.Blueprint, settings config.Dict, defaultProjectID string) string {
	if settings.Has("network_project_id") {
		s, err := evalString(bp, settings.Get("network_project_id"))
		if err == nil && s != "" {
			return s
		}
	}
	if settings.Has("project_id") {
		s, err := evalString(bp, settings.Get("project_id"))
		if err == nil && s != "" {
			return s
		}
	}
	return defaultProjectID
}

func addFilestoreQuota(bp config.Blueprint, settings config.Dict, projectID, region string, count float64, totals map[string]float64) {
	if !settings.Has("capacity_gb") {
		return
	}
	tier := "BASIC_HDD"
	if settings.Has("tier") {
		s, err := evalString(bp, settings.Get("tier"))
		if err == nil {
			tier = s
		}
	}

	metricName := "StandardStorageGbPerRegion"
	switch tier {
	case "BASIC_SSD":
		metricName = "PremiumStorageGbPerRegion"
	case "HIGH_SCALE_SSD":
		metricName = "HighScaleSSDStorageGibPerRegion"
	case "ENTERPRISE":
		metricName = "EnterpriseStorageGibPerRegion"
	case "ZONAL":
		metricName = "EnterpriseStorageGibPerRegion"
	}

	v, err := evalToFloat64(bp, settings.Get("capacity_gb"))
	if err == nil {
		addTotal(totals, projectID, region, metricName, v*count)
	}
}

func addTPUQuota(bp config.Blueprint, settings config.Dict, projectID, region string, count float64, totals map[string]float64) {
	if !settings.Has("accelerator_type") {
		return
	}
	accTypeStr, err := evalString(bp, settings.Get("accelerator_type"))
	if err != nil || !strings.HasPrefix(accTypeStr, "v") {
		return
	}

	parts := strings.Split(accTypeStr, "-")
	if len(parts) >= 2 {
		ver := parts[0] // v2, v3
		coresStr := parts[1]
		cores, errC := strconv.ParseFloat(coresStr, 64)
		if errC == nil {
			metric := fmt.Sprintf("%s_TPUS", strings.ToUpper(ver))

			isTpuPreemptible := false
			if settings.Has("preemptible") {
				v, _ := evalBool(bp, settings.Get("preemptible"))
				if v {
					isTpuPreemptible = true
				}
			}

			if isTpuPreemptible {
				metric = "PREEMPTIBLE_" + metric
			}
			addTotal(totals, projectID, region, metric, cores*count)
		}
	}
}

func GetMachineTypeFamily(machineType string) string {
	matches := machineTypeFamilyRegex.FindStringSubmatch(strings.ToLower(machineType))
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}
