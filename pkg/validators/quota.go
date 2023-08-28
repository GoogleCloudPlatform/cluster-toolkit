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
	"hpc-toolkit/pkg/config"
	"strings"
	"time"

	"golang.org/x/exp/maps"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/gocty"
	cm "google.golang.org/api/monitoring/v3"
	sub "google.golang.org/api/serviceusage/v1beta1"
)

// ResourceRequirement represents an amount of desired resource.
type ResourceRequirement struct {
	Consumer   string            `cty:"consumer"` // e.g. "projects/myprojectid""
	Service    string            `cty:"service"`  // e.g. "compute.googleapis.com"
	Metric     string            `cty:"metric"`   // e.g. "compute.googleapis.com/disks_total_storage"
	Required   int64             `cty:"required"`
	Dimensions map[string]string `cty:"dimensions"` // e.g. {"region": "us-central1"}
}

// InBucket returns true if all dimensions specified in the bucket match dimensions the requirement.
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
	DisplayName    string
	Unit           string
	Dimensions     map[string]string
	EffectiveLimit int64
	Usage          int64
	Requested      int64
}

func (e QuotaError) Error() string {
	loc := ""
	if len(e.Dimensions) > 0 {
		prettyMap := fmt.Sprintf("%v", e.Dimensions)[3:]
		loc = fmt.Sprintf(" in %s", prettyMap)
	}
	rhs := fmt.Sprintf("requested=%d", e.Requested)
	if e.Usage > 0 {
		rhs = fmt.Sprintf("requested=%d + usage=%d", e.Requested, e.Usage)
	}
	return fmt.Sprintf("not enough quota %q as %q%s, limit=%d < %s", e.DisplayName, e.Unit, loc, e.EffectiveLimit, rhs)
}

func validateResourceRequirements(rs []ResourceRequirement, up *usageProvider) ([]QuotaError, error) {
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

	// Process all groups in parallel
	type chs struct {
		qe  []QuotaError
		err error
	}
	ch := make(chan chs)
	for k, g := range groups { // Spawn
		go func(k gk, g []ResourceRequirement) {
			qe, err := validateServiceRequirements(k.Consumer, k.Service, g, up)
			ch <- chs{qe, err}
		}(k, g)
	}
	errs := config.Errors{}
	qerrs := []QuotaError{}
	for range groups { // Gather
		s := <-ch
		qerrs = append(qerrs, s.qe...)
		errs.Add(s.err)
	}
	return qerrs, errs.OrNil()
}

// Validate requirements for a single consumer & service pair.
// The `ServiceUsage.ConsumerQuotaMetrics` API call returns following structure:
//
// list[ConsumerQuotaMetric] - one per metric/quota, e.g. compute.googleapis.com/n2_cpus for `N2 CPUs`.
// ├Metric
// └list[ConsumerQuotaLimit] - one per quota scope (e.g. regional or zonal)
// ....|.......................for N2 CPUs there are two: `N2-CPUS-per-project-region` & `N2-CPUS-per-project-zone`
// ....├Unit - e.g. '1/{project}/{region}' for regional "scope"
// ....└list[QuotaBucket] - represents the "slice" of the "scope" with specified limit
// ........|................e.g. for `N2-CPUS-per-project-region` there will be buckets with specific region: `{'region': 'asia-east1'}`,
// ........|................and one bucket, named `N2 CPUs (default)` in the UI, without `dimensions={}` to act as a wildcard and provide default limit.
// ........├EffectiveLimit
// ........└Dimensions - e.g. {"region": "us-central1"}
func validateServiceRequirements(consumer string, service string, rs []ResourceRequirement, up *usageProvider) ([]QuotaError, error) {
	qms, err := queryMetrics(consumer, service)
	if err != nil {
		return nil, err
	}

	errs := config.Errors{}
	reqToBuckets, err := gatherBucketsRequirements(rs, qms)
	errs.Add(err)
	qerrs := []QuotaError{}
	for _, br := range reqToBuckets {
		qerrs = append(qerrs, validateBucket(br, up)...)
	}
	return qerrs, errs.OrNil()
}

// Find a bucket in the ConsumerQuotaLimit that matches the ResourceRequirement.
func findBucket(r ResourceRequirement, ql *sub.ConsumerQuotaLimit) (*sub.QuotaBucket, error) {
	// Iterate buckets in order from most to less specific
	for i := len(ql.QuotaBuckets) - 1; i >= 0; i-- {
		if r.InBucket(ql.QuotaBuckets[i]) {
			return ql.QuotaBuckets[i], nil
		}
	}
	// According to docs the top bucket should be a wildcard `dimensions={}`
	// So we should never end up here, return fake "unlimited" bucket and report error.
	return &sub.QuotaBucket{Dimensions: map[string]string{}, EffectiveLimit: -1},
		fmt.Errorf("unexpected default-less ConsumerQuotaLimit: %q", ql.Name)
}

// Set of requirements that fall into the specific bucket with context attached
type bucketRequirements struct {
	QuotaMetric  *sub.ConsumerQuotaMetric
	QuotaLimit   *sub.ConsumerQuotaLimit
	Bucket       *sub.QuotaBucket
	Requirements []ResourceRequirement
}

// Attribute each requirement to one or more buckets that this requirement should be checked against.
// Organize result by buckets.
func gatherBucketsRequirements(rs []ResourceRequirement, qms map[string]*sub.ConsumerQuotaMetric) ([]bucketRequirements, error) {
	res := map[string]bucketRequirements{}
	errs := config.Errors{}

	for _, r := range rs { // Iterate requirements
		qm, ok := qms[r.Metric]
		if !ok {
			// TODO: add path to ResourceRequirement for better error reporting
			errs.Add(fmt.Errorf("can't find quota for metric %q", r.Metric))
			continue
		}

		// Each ConsumerQuotaMetric can contain multiple ConsumerQuotaLimits,
		// e.g. ConsumerQuotaMetric for "N2 CPUs" has two ConsumerQuotaLimits: regional and zonal.
		for _, ql := range qm.ConsumerQuotaLimits {
			b, err := findBucket(r, ql)
			errs.Add(err)

			k := fmt.Sprintf("%s|%v", ql.Name, b.Dimensions) // unique key to identify bucket across all ConsumerQuotaMetric(s)
			br, ok := res[k]                                 // update stored bucket requirements
			if !ok {
				br = bucketRequirements{qm, ql, b, []ResourceRequirement{}}
			}
			br.Requirements = append(br.Requirements, r)
			res[k] = br
		}

	}
	return maps.Values(res), errs.OrNil()
}

// validateBucket aggregates (sum) all requirements and usage for the given bucket
// and returns QuotaError if the bucket quota limit is not sufficient.
func validateBucket(br bucketRequirements, up *usageProvider) []QuotaError {
	if len(br.Requirements) == 0 {
		return nil
	}

	required := int64(0)
	for _, r := range br.Requirements {
		required += r.Required
	}
	usage := up.Usage(br.QuotaMetric.Metric, br.Bucket.Dimensions["region"], br.Bucket.Dimensions["zone"])

	if !satisfied(required+usage, br.Bucket.EffectiveLimit) {
		r0 := br.Requirements[0] // Take any, they all should have the same metric, service, and consumer
		return []QuotaError{{
			Consumer:       r0.Consumer,
			Service:        r0.Service,
			Metric:         r0.Metric,
			DisplayName:    br.QuotaMetric.DisplayName,
			Unit:           br.QuotaLimit.Unit,
			Dimensions:     br.Bucket.Dimensions,
			EffectiveLimit: br.Bucket.EffectiveLimit,
			Usage:          usage,
			Requested:      required,
		}}
	}
	return nil
}

func satisfied(requested int64, limit int64) bool {
	if limit == -1 {
		return true
	}
	return requested <= limit
}

func queryMetrics(consumer string, service string) (map[string]*sub.ConsumerQuotaMetric, error) {
	ctx := context.Background()
	s, err := sub.NewService(ctx)
	if err != nil {
		return nil, err
	}
	res := map[string]*sub.ConsumerQuotaMetric{}
	parent := fmt.Sprintf("%s/services/%s", consumer, service)
	err = s.Services.ConsumerQuotaMetrics.
		List(parent).
		View("BASIC"). // BASIC reduces the response size & latency
		Pages(ctx, func(page *sub.ListConsumerQuotaMetricsResponse) error {
			for _, m := range page.Metrics {
				res[m.Metric] = m
			}
			return nil
		})
	return res, err
}

type usageKey struct {
	Metric   string
	Location string // either "global", region, or zone
}

// usageProvider provides usage for a given metric and location.
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
		Filter(`metric.type="serviceruntime.googleapis.com/quota/allocation/usage"`).
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

type rrInputs struct {
	Requirements []ResourceRequirement `cty:"requirements"`
	IgnoreUsage  bool                  `cty:"ignore_usage"`
}

func ifNull(v cty.Value, d cty.Value) cty.Value {
	if v.IsNull() {
		return d
	}
	return v
}

func extractServiceName(metric string) (string, error) {
	// metric is in the form of "service.googleapis.com/metric"
	// we want to extract the "service.googleapis.com" part
	parts := strings.Split(metric, "/")
	if len(parts) < 2 {
		return "", fmt.Errorf("can not deduce service from metric %q", metric)
	}
	return parts[0], nil
}

func parseResourceRequirementsInputs(bp config.Blueprint, inputs config.Dict) (rrInputs, error) {
	// sanitize inputs dict by matching with type
	rty := cty.ObjectWithOptionalAttrs(map[string]cty.Type{
		"metric":     cty.String,
		"service":    cty.String,
		"consumer":   cty.String,
		"required":   cty.Number,
		"dimensions": cty.Map(cty.String),
	},
		/*optional=*/ []string{"service", "consumer", "dimensions"})
	ity := cty.ObjectWithOptionalAttrs(map[string]cty.Type{
		"requirements": cty.List(rty),
		"ignore_usage": cty.Bool,
	},
		/*optional=*/ []string{"ignore_usage"})
	clean, err := convert.Convert(inputs.AsObject(), ity)
	if err != nil {
		return rrInputs{}, err
	}

	// fill in default values
	ignoreUsage := ifNull(clean.GetAttr("ignore_usage"), cty.False)
	projectID, err := bp.ProjectID()
	if err != nil {
		return rrInputs{}, err
	}
	reqs := []cty.Value{}
	rit := clean.GetAttr("requirements").ElementIterator()
	for rit.Next() {
		_, r := rit.Element()
		defConsumer := fmt.Sprintf("projects/%s", projectID)
		defService, err := extractServiceName(r.GetAttr("metric").AsString())
		if err != nil {
			return rrInputs{}, err
		}
		defDims := map[string]cty.Value{}
		if bp.Vars.Has("region") {
			defDims["region"] = bp.Vars.Get("region")
		}
		if bp.Vars.Has("zone") {
			defDims["zone"] = bp.Vars.Get("zone")
		}
		defDimsVal := cty.MapValEmpty(cty.String)
		if len(defDims) > 0 {
			defDimsVal = cty.MapVal(defDims)
		}

		reqs = append(reqs, cty.ObjectVal(map[string]cty.Value{
			"metric":     r.GetAttr("metric"),
			"service":    ifNull(r.GetAttr("service"), cty.StringVal(defService)),
			"consumer":   ifNull(r.GetAttr("consumer"), cty.StringVal(defConsumer)),
			"required":   r.GetAttr("required"),
			"dimensions": ifNull(r.GetAttr("dimensions"), defDimsVal),
		}))
	}

	reqsVal := cty.ListValEmpty(rty)
	if len(reqs) > 0 {
		reqsVal = cty.ListVal(reqs)
	}

	full := cty.ObjectVal(map[string]cty.Value{
		"requirements": reqsVal,
		"ignore_usage": ignoreUsage,
	})

	var s rrInputs
	return s, gocty.FromCtyValue(full, &s)
}

func testResourceRequirements(bp config.Blueprint, inputs config.Dict) error {
	in, err := parseResourceRequirementsInputs(bp, inputs)
	if err != nil {
		return err
	}
	errs := config.Errors{}
	up := usageProvider{}
	if !in.IgnoreUsage {
		p, err := bp.ProjectID()
		errs.Add(err)
		if p != "" {
			up, err = newUsageProvider(p)
			errs.Add(err) // don't terminate fallback to ignore usage
		}
	}

	qerrs, err := validateResourceRequirements(in.Requirements, &up)
	for _, qe := range qerrs {
		errs.Add(qe)
	}
	errs.Add(err)
	return errs.OrNil()
}
