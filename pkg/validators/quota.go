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
	"context"
	"fmt"
	"time"

	cm "google.golang.org/api/monitoring/v3"
	sub "google.golang.org/api/serviceusage/v1beta1"
)

// ResourceRequirement represents an amount of desired resource.
type ResourceRequirement struct {
	Consumer   string // e.g. "projects/myprojectid""
	Service    string // e.g. "compute.googleapis.com"
	Metric     string // e.g. "compute.googleapis.com/disks_total_storage"
	Required   int64
	Dimensions map[string]string // e.g. {"region": "us-central1"}
	// How this requirement should be aggregated with other requirements in the same bucket.
	Aggregation string
}

// InBucket returns true if the quota is in the QuotaBucket.
func (q ResourceRequirement) InBucket(b *sub.QuotaBucket) bool {
	for d, v := range b.Dimensions {
		if q.Dimensions[d] != v {
			return false
		}
	}
	return true
}

// QuotaError represents an event of not having enough quota.
type QuotaError struct {
	Consumer       string
	Service        string
	Metric         string
	Dimensions     map[string]string
	EffectiveLimit int64
	Requested      int64
}

func (e QuotaError) Error() string {
	return fmt.Sprintf("QuotaError: %#v", e)
}

// ValidateQuotas validates the resource requirements.
func ValidateQuotas(rs []ResourceRequirement) ([]QuotaError, error) {
	qe := []QuotaError{}
	// Group by Consumer and Service
	type gk struct {
		Consumer string
		Service  string
	}

	groups := map[gk][]ResourceRequirement{}
	for _, r := range rs {
		k := gk{r.Consumer, r.Service}
		groups[k] = append(groups[k], r)
	}

	for k, g := range groups {
		ls, err := serviceLimits(k.Consumer, k.Service)
		if err != nil {
			return qe, err
		}
		qse, err := validateServiceLimits(g, ls)
		if err != nil {
			return qe, err
		}
		qe = append(qe, qse...)
	}

	return qe, nil
}

func validateServiceLimits(rs []ResourceRequirement, ls []*sub.ConsumerQuotaMetric) ([]QuotaError, error) {
	// Group by Metric and Aggregation
	type gk struct {
		Metric      string
		Aggregation string
	}
	groups := map[gk][]ResourceRequirement{}
	for _, r := range rs {
		k := gk{r.Metric, r.Aggregation}
		groups[k] = append(groups[k], r)
	}

	qe := []QuotaError{}
	for k, g := range groups {
		agg, err := aggregation(k.Aggregation)
		if err != nil {
			return qe, err
		}

		// select limits for the metric
		ml := []*sub.ConsumerQuotaLimit{}
		for _, l := range ls {
			if l.Metric == k.Metric {
				ml = append(ml, l.ConsumerQuotaLimits...)
			}
		}
		if len(ml) == 0 {
			return qe, fmt.Errorf("limits for metric %q were not found", k.Metric)
		}

		for _, limit := range ml {
			qle := validateLimit(g, limit, agg)
			qe = append(qe, qle...)
		}
	}
	return qe, nil
}

func validateLimit(rs []ResourceRequirement, limit *sub.ConsumerQuotaLimit, agg aggFn) []QuotaError {
	qe := []QuotaError{}
	for _, bucket := range limit.QuotaBuckets {
		vs := []int64{}
		for _, r := range rs {
			if r.InBucket(bucket) {
				vs = append(vs, r.Required)
			}
		}
		if len(vs) == 0 {
			continue
		}
		required := agg(vs)
		for _, r := range required {
			if !satisfied(r, bucket.EffectiveLimit) {
				r0 := rs[0] // all should have the same consumer, service and metric
				qe = append(qe, QuotaError{
					Consumer:       r0.Consumer,
					Service:        r0.Service,
					Metric:         r0.Metric,
					Dimensions:     bucket.Dimensions,
					EffectiveLimit: bucket.EffectiveLimit,
					Requested:      r,
				})
			}
		}
	}
	return qe
}

func satisfied(requested int64, limit int64) bool {
	if limit == -1 {
		return true
	}
	if requested == -1 {
		return false
	}
	return requested <= limit
}

type aggFn func([]int64) []int64

func aggregation(agg string) (aggFn, error) {
	switch agg {
	case "MAX":
		return func(l []int64) []int64 {
			max := int64(0)
			for _, v := range l {
				if v == -1 {
					return []int64{-1}
				}
				if v > max {
					max = v
				}
			}
			return []int64{max}
		}, nil
	case "SUM":
		return func(l []int64) []int64 {
			sum := int64(0)
			for _, v := range l {
				if v == -1 {
					return []int64{-1}
				}
				sum += v
			}
			return []int64{sum}
		}, nil
	case "DO_NOT_AGGREGATE":
		return func(l []int64) []int64 { return l }, nil
	default:
		return nil, fmt.Errorf("aggregation %q is not supported", agg)
	}
}

func serviceLimits(consumer string, service string) ([]*sub.ConsumerQuotaMetric, error) {
	ctx := context.Background()
	s, err := sub.NewService(ctx)
	if err != nil {
		return nil, err
	}
	res := []*sub.ConsumerQuotaMetric{}
	parent := fmt.Sprintf("%s/services/%s", consumer, service)
	err = s.Services.ConsumerQuotaMetrics.
		List(parent).
		View("BASIC"). // BASIC reduces the response size & latency
		Pages(ctx, func(page *sub.ListConsumerQuotaMetricsResponse) error {
			res = append(res, page.Metrics...)
			return nil
		})
	return res, err
}

type usageKey struct {
	Metric   string
	Location string // either "global", region, or zone
}

type usageProvider struct {
	u map[usageKey]int64
}

func (up *usageProvider) Usage(metric string, region string, zone string) int64 {
	if up.u == nil {
		return 0
	}
	k := usageKey{metric, "global"}
	if region != "" {
		k.Location = region
	}
	if zone != "" {
		k.Location = zone
	}
	return up.u[k] // 0 if not found
}

func newUsageProvider(projectID string) (usageProvider, error) {
	s, err := cm.NewService(context.Background())
	if err != nil {
		return usageProvider{}, err
	}

	u := map[usageKey]int64{}
	err = s.Projects.TimeSeries.List("projects/"+projectID).
		Filter(`metric.type="serviceruntime.googleapis.com/quota/allocation/usage" resource.type="consumer_quota"`).
		IntervalEndTime(time.Now().Format(time.RFC3339)).
		// Quota usage metrics get duplicated once a day
		IntervalStartTime(time.Now().Add(-24*time.Hour).Format(time.RFC3339)).
		Pages(context.Background(), func(page *cm.ListTimeSeriesResponse) error {
			for _, ts := range page.TimeSeries {
				usage := ts.Points[0].Value.Int64Value // Points[0] is latest
				if *usage == 0 {
					continue
				}
				metric := ts.Metric.Labels["quota_metric"]
				location := ts.Resource.Labels["location"]
				u[usageKey{metric, location}] = *usage
			}
			return nil
		})
	if err != nil {
		return usageProvider{}, err
	}
	return usageProvider{u}, nil
}
