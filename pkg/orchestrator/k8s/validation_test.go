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
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestParseCPURequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "whole cores", in: "8", want: "8"},
		{name: "millicores", in: "4500m", want: "4500m"},
		{name: "decimal cores", in: "0.5", want: "500m"},
		{name: "trimmed whitespace", in: " 2 ", want: "2"},
		{name: "minimum 1m", in: "1m", want: "1m"},
		{name: "valid scientific notation positive", in: "1e3", want: "1000"},
		{name: "valid scientific notation negative", in: "500e-3", want: "500m"},
		{name: "max supported scientific exponent", in: "1e30", want: "1e30"},
		{name: "overflow-scale quantity parses for Cmp checks", in: "10000000000000000", want: "10000000000000000"},
		{name: "empty", in: "", wantErr: "cannot be empty"},
		{name: "whitespace only", in: "   ", wantErr: "cannot be empty"},
		{name: "malformed", in: "abc", wantErr: "invalid --cpu quantity"},
		{name: "non-scientific unit typo prints format hint", in: "8cores", wantErr: "expected a Kubernetes quantity"},
		{name: "leading e rejected without panic", in: "e3", wantErr: "positive Kubernetes quantity"},
		{name: "uppercase scientific notation 1E3", in: "1E3", want: "1000"},
		{name: "positive signed scientific notation 1e+3", in: "1e+3", want: "1000"},
		{name: "zero", in: "0", wantErr: "positive Kubernetes quantity"},
		{name: "zero millicores", in: "0m", wantErr: "positive Kubernetes quantity"},
		{name: "negative", in: "-1", wantErr: "positive Kubernetes quantity"},
		{name: "sub-millicore u", in: "100u", wantErr: "at least 1m"},
		{name: "sub-millicore decimal", in: "0.0001", wantErr: "at least 1m"},
		{name: "sub-millicore scientific at min exponent", in: "1e-9", wantErr: "at least 1m"},
		{name: "fractional millicore 1.5m", in: "1.5m", wantErr: "whole millicores"},
		{name: "fractional millicore 0.0015", in: "0.0015", wantErr: "whole millicores"},
		{name: "fractional millicore 1500u", in: "1500u", wantErr: "whole millicores"},
		{name: "above max scientific exponent", in: "1e31", wantErr: "scientific exponent out of supported range"},
		{name: "uppercase above max scientific exponent", in: "1E31", wantErr: "scientific exponent out of supported range"},
		{name: "signed above max scientific exponent", in: "1e+31", wantErr: "scientific exponent out of supported range"},
		{name: "trailing dot above max scientific exponent", in: "1.e31", wantErr: "scientific exponent out of supported range"},
		{name: "scientific exponent int32 wraparound", in: "1e4294967296", wantErr: "scientific exponent out of supported range"},
		{name: "scientific exponent DoS", in: "1e100000000", wantErr: "scientific exponent out of supported range"},
		{name: "negative scientific exponent out of range", in: "1e-10", wantErr: "scientific exponent out of supported range"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseCPURequest(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseCPURequest(%q) err = %v, want substring %q", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseCPURequest(%q) unexpected err: %v", tc.in, err)
			}
			wantQty := resource.MustParse(tc.want)
			if got.Cmp(wantQty) != 0 {
				t.Errorf("ParseCPURequest(%q) = %s, want %s", tc.in, got.String(), wantQty.String())
			}
		})
	}
}

func TestParseMemoryRequest(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		in      string
		want    string
		wantErr string
	}{
		{name: "binary Gi", in: "64Gi", want: "64Gi"},
		{name: "exact binary decimal 0.5Gi", in: "0.5Gi", want: "512Mi"},
		{name: "exact binary decimal 1.25Gi", in: "1.25Gi", want: "1280Mi"},
		{name: "binary Mi", in: "512Mi", want: "512Mi"},
		{name: "decimal G", in: "200G", want: "200G"},
		{name: "raw bytes", in: "1000000000", want: "1000000000"},
		{name: "trimmed whitespace", in: " 16Gi ", want: "16Gi"},
		{name: "minimum 1 byte", in: "1", want: "1"},
		{name: "decimal exa suffix 1E", in: "1E", want: "1E"},
		{name: "valid scientific notation 1e9", in: "1e9", want: "1G"},
		{name: "overflow-scale quantity parses for Cmp checks", in: "10000000000Ei", want: "10000000000Ei"},
		{name: "empty", in: "", wantErr: "cannot be empty"},
		{name: "whitespace only", in: "  ", wantErr: "cannot be empty"},
		{name: "malformed GB suffix", in: "64GB", wantErr: "use 'Gi'/'G' rather than 'GB'"},
		{name: "non-scientific unit typo with e prints format hint", in: "64GBytes", wantErr: "use 'Gi'/'G' rather than 'GB'"},
		{name: "non-numeric word ending in m is not sub-byte error", in: "ram", wantErr: "invalid --memory quantity"},
		{name: "single char m rejected without panic", in: "m", wantErr: "positive Kubernetes quantity"},
		{name: "zero", in: "0", wantErr: "positive Kubernetes quantity"},
		{name: "negative", in: "-1Gi", wantErr: "positive Kubernetes quantity"},
		{name: "fractional sub-byte decimal 0.5", in: "0.5", wantErr: "at least 1 byte"},
		{name: "fractional byte decimal 1.5", in: "1.5", wantErr: "whole bytes"},
		{name: "fractional byte binary Gi 243.7Gi", in: "243.7Gi", wantErr: "whole bytes"},
		{name: "fractional byte binary Gi 0.1Gi", in: "0.1Gi", wantErr: "whole bytes"},
		{name: "sub-byte scientific 1e-3", in: "1e-3", wantErr: "at least 1 byte"},
		{name: "sub-byte millibytes 500m", in: "500m", wantErr: "millibytes"},
		{name: "milli-suffix typo 64000m", in: "64000m", wantErr: "millibytes"},
		{name: "milli-suffix after dot 1000.m", in: "1000.m", wantErr: "millibytes"},
		{name: "sub-byte micro suffix 1500u", in: "1500u", wantErr: "sub-byte suffix"},
		{name: "sub-byte nano suffix 500n", in: "500n", wantErr: "sub-byte suffix"},
		{name: "sub-byte nano suffix 1000000000n", in: "1000000000n", wantErr: "sub-byte suffix"},
		{name: "uppercase scientific exponent out of range", in: "1E31", wantErr: "scientific exponent out of supported range"},
		{name: "scientific exponent wraparound", in: "1e4294967296", wantErr: "scientific exponent out of supported range"},
		{name: "scientific exponent DoS", in: "1e+100000000", wantErr: "scientific exponent out of supported range"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseMemoryRequest(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ParseMemoryRequest(%q) err = %v, want substring %q", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseMemoryRequest(%q) unexpected err: %v", tc.in, err)
			}
			wantQty := resource.MustParse(tc.want)
			if got.Cmp(wantQty) != 0 {
				t.Errorf("ParseMemoryRequest(%q) = %s, want %s", tc.in, got.String(), wantQty.String())
			}
		})
	}
}

func TestValidateCPURequestPreflight(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name               string
		computeType        string
		cpuReq             string
		memReq             string
		isPathways         bool
		isAccel            bool
		requireComputeType bool
		wantErr            string
	}{
		{name: "no cpu or memory skips", computeType: "v6e-8", isPathways: true, isAccel: true, requireComputeType: true},
		{name: "valid cpu and memory", computeType: "n2-standard-32", cpuReq: "4", memReq: "16Gi", requireComputeType: true},
		{name: "pathways rejected with head-np hint", computeType: "n2-standard-32", cpuReq: "4", isPathways: true, requireComputeType: true, wantErr: "--pathways-head-np"},
		{name: "missing compute-type rejected when required", cpuReq: "4", requireComputeType: true, wantErr: "--compute-type is required"},
		{name: "missing compute-type allowed when optional in k8s mode", cpuReq: "4", memReq: "8Gi", requireComputeType: false},
		{name: "accelerator rejected", computeType: "v6e-8", cpuReq: "4", isAccel: true, requireComputeType: true, wantErr: "not accelerator compute type"},
		{name: "invalid cpu rejected", computeType: "n2-standard-32", cpuReq: "0", requireComputeType: true, wantErr: "positive Kubernetes quantity"},
		{name: "invalid memory rejected", computeType: "n2-standard-32", memReq: "64GB", requireComputeType: true, wantErr: "invalid --memory quantity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateCPURequestPreflight(tc.computeType, tc.cpuReq, tc.memReq, tc.isPathways, tc.isAccel, tc.requireComputeType)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("ValidateCPURequestPreflight() err = %v, want substring %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ValidateCPURequestPreflight() unexpected err: %v", err)
			}
		})
	}
}
