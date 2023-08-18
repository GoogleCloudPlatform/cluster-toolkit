// Copyright 2023 Google LLC
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
	"fmt"
	"hpc-toolkit/pkg/config"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	sub "google.golang.org/api/serviceusage/v1beta1"
	"gopkg.in/yaml.v3"
)

func TestAggregation(t *testing.T) {
	type test struct {
		requested   []int64
		aggregation string
		want        []int64
		err         bool
	}
	tests := []test{
		{[]int64{1, 3, 2}, "SUM", []int64{6}, false},
		{[]int64{1, 3, 2}, "MAX", []int64{3}, false},
		{[]int64{}, "SUM", []int64{0}, false},
		{[]int64{}, "MAX", []int64{0}, false},
		{[]int64{1, -1, 2}, "SUM", []int64{-1}, false},
		{[]int64{1, -1, 2}, "MAX", []int64{-1}, false},
		{[]int64{1, -1, 2}, "DO_NOT_AGGREGATE", []int64{1, -1, 2}, false},
		{[]int64{1, -1, 2}, "KARL_MAX", nil, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s%#v", tc.aggregation, tc.requested), func(t *testing.T) {
			fn, err := aggregation(tc.aggregation)
			if tc.err != (err != nil) {
				t.Errorf("got unexpected error: %s", err)
			}
			if err != nil {
				return
			}
			got := fn(tc.requested)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestSatisfied(t *testing.T) {
	type test struct {
		requested int64
		limit     int64
		want      bool
	}
	tests := []test{
		{1, 1, true},
		{1, 2, true},
		{2, 1, false},
		{1, -1, true},
		{-1, 1, false},
		{-1, -1, true},
	}

	for _, tc := range tests {
		t.Run(fmt.Sprintf("%d::%d", tc.requested, tc.limit), func(t *testing.T) {
			got := satisfied(tc.requested, tc.limit)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestInBucket(t *testing.T) {
	type test struct {
		qDimensions map[string]string
		bDimensions map[string]string
		want        bool
	}
	tests := []test{
		{map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2"}, true},
		{map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "3"}, false},
		{map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1"}, true},
		{map[string]string{"a": "1", "b": "2"}, map[string]string{"a": "1", "b": "2", "c": "3"}, false},
		{map[string]string{}, map[string]string{}, true},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%#v::%#v", tc.qDimensions, tc.bDimensions), func(t *testing.T) {
			q := ResourceRequirement{Dimensions: tc.qDimensions}
			b := sub.QuotaBucket{Dimensions: tc.bDimensions}

			got := q.InBucket(&b)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateServiceLimits(t *testing.T) {
	// Configured quotas:
	// global: 5
	// green_eggs: 3
	// green_sleeve: -1
	//
	// Requested:
	// green_eggs: 4
	// green_sleeve: 7
	//
	// Expected errors:
	// green_eggs: 4 > 3
	// global: 11 > 5
	buckets := []*sub.QuotaBucket{
		{
			EffectiveLimit: int64(5),
		}, {
			EffectiveLimit: int64(3),
			Dimensions:     map[string]string{"green": "eggs"},
		}, {
			EffectiveLimit: int64(-1),
			Dimensions:     map[string]string{"green": "sleeve"},
		},
	}
	quotas := []ResourceRequirement{
		{
			Metric:      "pony",
			Required:    int64(4),
			Dimensions:  map[string]string{"green": "eggs"},
			Aggregation: "SUM",
		}, {
			Metric:      "pony",
			Required:    int64(7),
			Dimensions:  map[string]string{"green": "sleeve"},
			Aggregation: "SUM",
		},
	}
	up := usageProvider{}

	want := []QuotaError{
		{Metric: "pony", Dimensions: map[string]string{"green": "eggs"}, EffectiveLimit: 3, Requested: 4},
		{Metric: "pony", Dimensions: nil, EffectiveLimit: 5, Requested: 11},
	}
	got, err := validateServiceLimits(quotas, []*sub.ConsumerQuotaMetric{
		{
			Metric: "pony",
			ConsumerQuotaLimits: []*sub.ConsumerQuotaLimit{
				{Metric: "pony", QuotaBuckets: buckets}},
		},
	}, &up)

	if err != nil {
		t.Errorf("got unexpected error: %s", err)
		return
	}
	// Sort by error message to make test deterministic
	sort.Slice(got, func(i, j int) bool { return got[i].Error() < got[j].Error() })
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}

func TestUsageProviderGet(t *testing.T) {
	up := usageProvider{u: map[usageKey]int64{
		{Metric: "pony", Location: "global"}:     17,
		{Metric: "pony", Location: "us-west1"}:   13,
		{Metric: "pony", Location: "us-west1-c"}: 11,
		{Metric: "zebra", Location: "us-east1"}:  7,
	}}

	type test struct {
		metric string
		region string
		zone   string
		want   int64
	}
	tests := []test{
		{"pony", "", "", 17},
		{"zebra", "", "", 0},
		{"pony", "us-west1", "", 13},
		{"zebra", "us-east2", "", 0},
		{"pony", "us-west1", "us-west1-c", 11},
		{"zebra", "us-east1", "us-east1-b", 0},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%#v", tc), func(t *testing.T) {
			got := up.Usage(tc.metric, tc.region, tc.zone)
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseResourceRequirementsInputs(t *testing.T) {
	type test struct {
		yml  string
		want rrInputs
		err  bool
	}
	tests := []test{
		{`# empty
ignore_usage: false
requirements: []`, rrInputs{Requirements: []ResourceRequirement{}}, false},
		{`# missing ignore_usage
requirements: []`, rrInputs{}, true},
		{`# complete
ignore_usage: true
requirements:
- metric: pony
  consumer: redhat
  service: friendship
  required: 1
  dimensions: {"x": "y", "left": "right"}
  aggregation: "SUM"`, rrInputs{
			IgnoreUsage: true,
			Requirements: []ResourceRequirement{
				{
					Metric:   "pony",
					Consumer: "redhat",
					Service:  "friendship",
					Required: 1,
					Dimensions: map[string]string{
						"x":    "y",
						"left": "right",
					},
					Aggregation: "SUM",
				},
			},
		}, false},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("%s", tc.yml), func(t *testing.T) {
			var in config.Dict
			if err := yaml.Unmarshal([]byte(tc.yml), &in); err != nil {
				t.Fatal("failed to unmarshal yaml")
			}
			rr, err := parseResourceRequirementsInputs(in)
			if (err == nil) == tc.err {
				t.Errorf("unexpected error %v", err)
			}
			if diff := cmp.Diff(tc.want, rr); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}
