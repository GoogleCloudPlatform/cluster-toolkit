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

package k8s

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	minDecimalExponent = -9
	maxDecimalExponent = 30
)

var (
	minCPUQuantity    = resource.NewMilliQuantity(1, resource.DecimalSI)
	minMemoryQuantity = resource.NewQuantity(1, resource.BinarySI)
)

// ParseCPURequest parses and validates a Kubernetes CPU resource request quantity (at least 1m, whole millicores).
func ParseCPURequest(raw string) (resource.Quantity, error) {
	q, err := parsePositiveResource(raw, "--cpu", *minCPUQuantity, "1m", "expected a Kubernetes quantity such as '8', '0.5', or '4500m'")
	if err != nil {
		return resource.Quantity{}, err
	}
	if _, exact := q.AsScale(resource.Milli); !exact {
		return resource.Quantity{}, fmt.Errorf("--cpu must be specified in whole millicores (1m granularity), got %q", raw)
	}
	return q, nil
}

// ValidateCPURequestPreflight validates --cpu and --memory locally before any cluster or registry calls.
func ValidateCPURequestPreflight(computeType, cpuReq, memReq string, isPathways, isAccelerator, requireComputeType bool) error {
	cpuReq = strings.TrimSpace(cpuReq)
	memReq = strings.TrimSpace(memReq)
	if cpuReq == "" && memReq == "" {
		return nil
	}
	if isPathways {
		return fmt.Errorf("--cpu and --memory are only supported for non-Pathways CPU workloads (Pathways head containers omit limits and burst to node capacity; use --pathways-head-np to target a larger CPU node pool or customize pathways_jobset.tmpl via --gke-custom-templates-path)")
	}
	if requireComputeType && strings.TrimSpace(computeType) == "" {
		return fmt.Errorf("--compute-type is required when using --cpu or --memory")
	}
	if isAccelerator {
		return fmt.Errorf("--cpu and --memory are only supported for CPU-only workloads, not accelerator compute type %q", computeType)
	}
	if cpuReq != "" {
		if _, err := ParseCPURequest(cpuReq); err != nil {
			return err
		}
	}
	if memReq != "" {
		if _, err := ParseMemoryRequest(memReq); err != nil {
			return err
		}
	}
	return nil
}

// ParseMemoryRequest parses and validates a Kubernetes memory resource request quantity (at least 1 whole byte).
func ParseMemoryRequest(raw string) (resource.Quantity, error) {
	trimmed := strings.TrimSpace(raw)
	if hasSubUnitMemorySuffix(trimmed) {
		return resource.Quantity{}, fmt.Errorf("--memory cannot use sub-byte suffix in %q (lowercase 'm'/'u'/'n' mean sub-byte units in Kubernetes; 'm' is millibytes; use 'Mi', 'Gi', 'M', or 'G' for memory)", raw)
	}
	q, err := parsePositiveResource(raw, "--memory", *minMemoryQuantity, "1 byte", "expected a Kubernetes quantity such as '64Gi', '512Mi', or '200G'; use 'Gi'/'G' rather than 'GB'")
	if err != nil {
		return resource.Quantity{}, err
	}
	if _, exact := q.AsScale(0); !exact {
		return resource.Quantity{}, fmt.Errorf("--memory must resolve to whole bytes (no fractional bytes), got %q; use whole Mi/Gi units such as '512Mi' or '64Gi'", raw)
	}
	return q, nil
}

func hasSubUnitMemorySuffix(trimmed string) bool {
	if len(trimmed) < 2 {
		return false
	}
	last := trimmed[len(trimmed)-1]
	if last != 'm' && last != 'u' && last != 'n' {
		return false
	}
	prev := trimmed[len(trimmed)-2]
	return (prev >= '0' && prev <= '9') || prev == '.'
}

// validateDecimalExponent bounds e/E exponents before resource.ParseQuantity to prevent int32 overflow or huge allocations.
func validateDecimalExponent(trimmed, flagName, raw string) error {
	idx := strings.IndexAny(trimmed, "eE")
	if idx <= 0 {
		return nil
	}
	if prev := trimmed[idx-1]; (prev < '0' || prev > '9') && prev != '.' {
		return nil
	}
	exp, err := strconv.ParseInt(trimmed[idx+1:], 10, 64)
	if errors.Is(err, strconv.ErrSyntax) {
		return nil // Non-scientific suffix after digit/dot (e.g., "1E", "1Ei", "1e3m"); let ParseQuantity handle.
	}
	if err != nil || exp < minDecimalExponent || exp > maxDecimalExponent {
		return fmt.Errorf("invalid %s quantity %q: scientific exponent out of supported range [%d, %d]", flagName, raw, minDecimalExponent, maxDecimalExponent)
	}
	return nil
}

func parsePositiveResource(raw, flagName string, minQty resource.Quantity, minDesc, formatHint string) (resource.Quantity, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return resource.Quantity{}, fmt.Errorf("%s cannot be empty", flagName)
	}
	if err := validateDecimalExponent(trimmed, flagName, raw); err != nil {
		return resource.Quantity{}, err
	}
	q, err := resource.ParseQuantity(trimmed)
	if err != nil {
		return resource.Quantity{}, fmt.Errorf("invalid %s quantity %q (%s): %w", flagName, raw, formatHint, err)
	}
	if q.Sign() <= 0 {
		return resource.Quantity{}, fmt.Errorf("%s must be a positive Kubernetes quantity, got %q", flagName, raw)
	}
	if q.Cmp(minQty) < 0 {
		return resource.Quantity{}, fmt.Errorf("%s must be at least %s, got %q", flagName, minDesc, raw)
	}
	return q, nil
}
