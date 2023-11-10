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
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zclconf/go-cty/cty"
	sub "google.golang.org/api/serviceusage/v1beta1"
	"gopkg.in/yaml.v3"
)

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

func TestValidateBucket(t *testing.T) {
	qm := sub.ConsumerQuotaMetric{Metric: "pony.api/friendship", DisplayName: "apple"}
	ql := sub.ConsumerQuotaLimit{Unit: "1/{road}"}
	b := sub.QuotaBucket{
		EffectiveLimit: 10,
		Dimensions:     map[string]string{"zone": "ponyland"},
	}
	br := bucketRequirements{
		QuotaMetric: &qm,
		QuotaLimit:  &ql,
		Bucket:      &b,
		Requirements: []ResourceRequirement{
			{
				Consumer: "redhat",
				Service:  "pony.api",
				Metric:   "pony.api/friendship",
				Required: 5,
			},
			{
				Consumer: "redhat",
				Service:  "pony.api",
				Metric:   "pony.api/friendship",
				Required: 4,
			},
		},
	}
	up := usageProvider{u: map[usageKey]int64{
		{Metric: "pony.api/friendship", Location: "ponyland"}: 3,
	}}

	errs := validateBucket(br, &up)
	if len(errs) != 1 {
		t.Errorf("got %d errors, want 1", len(errs))
	} else {
		want := QuotaError{
			Metric:         "pony.api/friendship",
			Consumer:       "redhat",
			Service:        "pony.api",
			DisplayName:    "apple",
			Unit:           "1/{road}",
			Dimensions:     map[string]string{"zone": "ponyland"},
			Requested:      5 + 4,
			Usage:          3,
			EffectiveLimit: 10,
		}
		if diff := cmp.Diff(want, errs[0]); diff != "" {
			t.Errorf("diff (-want +got):\n%s", diff)
		}
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
requirements: []`, rrInputs{Requirements: []ResourceRequirement{}}, false},
		{`# complete
ignore_usage: true
requirements:
- metric: pony.api/friendship
  consumer: redhat
  service: zebra.api
  required: 22
  dimensions: {"x": "y", "left": "right"}`, rrInputs{
			IgnoreUsage: true,
			Requirements: []ResourceRequirement{
				{
					Metric:   "pony.api/friendship",
					Consumer: "redhat",
					Service:  "zebra.api",
					Required: 22,
					Dimensions: map[string]string{
						"x":    "y",
						"left": "right",
					},
				},
			},
		}, false},
		{`# fill in
requirements:
- metric: pony.api/friendship
  required: 33`, rrInputs{
			IgnoreUsage: false,
			Requirements: []ResourceRequirement{
				{
					Metric:   "pony.api/friendship",
					Service:  "pony.api",
					Consumer: "projects/apple",
					Required: 33,
					Dimensions: map[string]string{
						"region": "narnia",
						"zone":   "narnia-51",
					},
				},
			},
		}, false},
	}
	for _, tc := range tests {
		t.Run(tc.yml, func(t *testing.T) {
			var in config.Dict
			bp := config.Blueprint{}
			bp.Vars.
				Set("project_id", cty.StringVal("apple")).
				Set("region", cty.StringVal("narnia")).
				Set("zone", cty.StringVal("narnia-51"))
			if err := yaml.Unmarshal([]byte(tc.yml), &in); err != nil {
				t.Fatal("failed to unmarshal yaml")
			}
			rr, err := parseResourceRequirementsInputs(bp, in)
			if (err == nil) == tc.err {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.want, rr); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestQuotaError(t *testing.T) {
	type test struct {
		err  QuotaError
		want string
	}
	tests := []test{
		{QuotaError{
			DisplayName:    "zebra",
			Unit:           "1/{road}",
			Dimensions:     map[string]string{"zone": "zoo"},
			Requested:      10,
			Usage:          5,
			EffectiveLimit: 13,
		}, `not enough quota "zebra" as "1/{road}" in [zone:zoo], limit=13 < requested=10 + usage=5`},
		{QuotaError{
			DisplayName:    "zebra",
			Unit:           "1/{road}",
			Requested:      10,
			Usage:          5,
			EffectiveLimit: 13,
		}, `not enough quota "zebra" as "1/{road}", limit=13 < requested=10 + usage=5`},
		{QuotaError{
			DisplayName:    "zebra",
			Unit:           "1/{road}",
			Requested:      10,
			EffectiveLimit: 13,
		}, `not enough quota "zebra" as "1/{road}", limit=13 < requested=10`},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if diff := cmp.Diff(tc.want, tc.err.Error()); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestGatherBucketsRequirements(t *testing.T) {
	b0 := sub.QuotaBucket{
		EffectiveLimit: 10,
		Dimensions:     map[string]string{"zone": "ponyland"},
	}
	ql := sub.ConsumerQuotaLimit{
		Unit:         "1/{road}",
		QuotaBuckets: []*sub.QuotaBucket{&b0},
	}
	qm := sub.ConsumerQuotaMetric{ConsumerQuotaLimits: []*sub.ConsumerQuotaLimit{&ql}}
	qms := map[string]*sub.ConsumerQuotaMetric{"pony.api/friendship": &qm}
	r0 := ResourceRequirement{Metric: "not_gonna_find_me"}
	r1 := ResourceRequirement{
		Metric:     "pony.api/friendship",
		Dimensions: map[string]string{"zone": "ponyland"},
	}
	rs := []ResourceRequirement{r0, r1}
	br, err := gatherBucketsRequirements(rs, qms)

	brWant := []bucketRequirements{
		{
			QuotaMetric:  &qm,
			QuotaLimit:   &ql,
			Bucket:       &b0,
			Requirements: []ResourceRequirement{r1},
		},
	}
	if diff := cmp.Diff(brWant, br); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
	wantErr := `can't find quota for metric "not_gonna_find_me"`
	if diff := cmp.Diff(wantErr, err.Error()); diff != "" {
		t.Errorf("diff (-want +got):\n%s", diff)
	}
}
