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

	sub "google.golang.org/api/serviceusage/v1beta1"
)

// Quota represents a desired quota.
type Quota struct {
	Consumer   string // e.g. "projects/myprojectid""
	Service    string // e.g. "compute.googleapis.com"
	Metric     string // e.g. "compute.googleapis.com/disks_total_storage"
	Limit      int64
	Dimensions map[string]string // e.g. {"region": "us-central1"}
	// How this Quota should be aggregated with other Quotas in the same bucket.
	Aggregation string
}

// InBucket returns true if the quota is in the QuotaBucket.
func (q Quota) InBucket(b *sub.QuotaBucket) bool {
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

// ValidateQuotas validates the quotas
func ValidateQuotas(quotas []Quota) ([]QuotaError, error) {
	qe := []QuotaError{}
	// Group by Consumer and Service
	type gk struct {
		Consumer string
		Service  string
	}

	groups := map[gk][]Quota{}
	for _, q := range quotas {
		k := gk{q.Consumer, q.Service}
		groups[k] = append(groups[k], q)
	}

	for k, qs := range groups {
		metrics, err := quotaMetrics(k.Consumer, k.Service)
		if err != nil {
			return qe, err
		}
		qse, err := validateServiceMetrics(qs, metrics)
		if err != nil {
			return qe, err
		}
		qe = append(qe, qse...)
	}

	return qe, nil
}

func validateServiceMetrics(quotas []Quota, metrics []*sub.ConsumerQuotaMetric) ([]QuotaError, error) {
	// Group by Metric and Aggregation
	type gk struct {
		Metric      string
		Aggregation string
	}
	groups := map[gk][]Quota{}
	for _, q := range quotas {
		k := gk{q.Metric, q.Aggregation}
		groups[k] = append(groups[k], q)
	}

	qe := []QuotaError{}
	for k, qs := range groups {
		agg, err := aggregation(k.Aggregation)
		if err != nil {
			return qe, err
		}

		limits := []*sub.ConsumerQuotaLimit{}
		for _, m := range metrics {
			if m.Metric == k.Metric {
				limits = append(limits, m.ConsumerQuotaLimits...)
			}
		}
		if len(limits) == 0 {
			return qe, fmt.Errorf("limits for metric %q were not found", k.Metric)
		}

		for _, limit := range limits {
			qle := validateLimitQuotas(qs, limit, agg)
			qe = append(qe, qle...)
		}
	}
	return qe, nil
}

func validateLimitQuotas(quotas []Quota, limit *sub.ConsumerQuotaLimit, agg aggFn) []QuotaError {
	qe := []QuotaError{}
	for _, bucket := range limit.QuotaBuckets {
		ql := []int64{}
		for _, q := range quotas {
			if q.InBucket(bucket) {
				ql = append(ql, q.Limit)
			}
		}
		if len(ql) == 0 {
			continue
		}
		requested := agg(ql)
		for _, r := range requested {
			if !satisfied(r, bucket.EffectiveLimit) {
				q := quotas[0] // all should have the same consumer, service and metric
				qe = append(qe, QuotaError{
					Consumer:       q.Consumer,
					Service:        q.Service,
					Metric:         q.Metric,
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

func quotaMetrics(consumer string, service string) ([]*sub.ConsumerQuotaMetric, error) {
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
