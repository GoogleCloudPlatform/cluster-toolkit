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

package clusterdirector

import (
	"fmt"
	"sort"
	"strings"

	tfjson "github.com/hashicorp/terraform-json"
)

// Terraform resource types that map onto Cluster Director resources. Matching
// is exact: "google_storage_bucket_iam_binding" must not be mistaken for
// "google_storage_bucket".
const (
	tfSubnetwork       = "google_compute_subnetwork"
	tfBucket           = "google_storage_bucket"
	tfFilestore        = "google_filestore_instance"
	tfLustre           = "google_lustre_instance"
	tfMIG              = "google_compute_instance_group_manager"
	tfRegionMIG        = "google_compute_region_instance_group_manager"
	tfContainerCluster = "google_container_cluster"
	tfNodePool         = "google_container_node_pool"
)

// NetworkPair associates a VPC network with one of its subnetworks, both as
// full resource paths: `projects/{project}/global/networks/{network}` and
// `projects/{project}/regions/{region}/subnetworks/{subnetwork}`. The network
// project can differ from the deployment project (Shared VPC).
type NetworkPair struct {
	Network    string
	Subnetwork string
}

// discoveredSubnet is a subnetwork found in Terraform state.
type discoveredSubnet struct {
	region string
	pair   NetworkPair
}

// Discovered holds the importable resources found in a deployment's
// Terraform state.
type Discovered struct {
	// subnets are keyed by subnetwork path.
	subnets map[string]discoveredSubnet

	Buckets    []string
	Filestores []string
	Lustres    []string
	MIGs       []string
	// ContainerClusters are the GKE clusters (`google_container_cluster`)
	// discovered in the deployment's Terraform state. GKE clusters are
	// skipped, because exporting GKE clusters isn't supported yet.
	ContainerClusters []string
}

// DiscoverOptions customizes Terraform state discovery.
type DiscoverOptions struct {
	// IgnoreBucketsFromModules lists top-level blueprint module IDs whose
	// managed google_storage_bucket resources are internal staging/config
	// buckets (such as Slurm controller config or VM startup-script buckets)
	// rather than cluster storage, and should be omitted from Buckets.
	IgnoreBucketsFromModules map[string]bool
}

// Discover walks Terraform states and collects the resources that can be
// exported to Cluster Director.
//
// Managed resources are always considered. Data sources are considered only
// for networks, subnetworks, and GKE clusters (modules/network/pre-existing-vpc
// and modules/scheduler/pre-existing-gke-cluster). Data sources are ignored
// for storage and compute, where a read is far more likely to be incidental to
// a script than a statement that the cluster owns the resource.
func Discover(states []*tfjson.State) Discovered {
	return DiscoverWithOptions(states, DiscoverOptions{})
}

// DiscoverWithOptions behaves like Discover with additional filtering options.
func DiscoverWithOptions(states []*tfjson.State, opts DiscoverOptions) Discovered {
	d := Discovered{subnets: map[string]discoveredSubnet{}}
	for _, st := range states {
		if st == nil || st.Values == nil {
			continue
		}
		d.walk(st.Values.RootModule, "", opts)
	}

	sort.Strings(d.Buckets)
	sort.Strings(d.Filestores)
	sort.Strings(d.Lustres)
	sort.Strings(d.MIGs)
	sort.Strings(d.ContainerClusters)
	return d
}

// walk recurses through a state module and its children. Terraform nests every
// module call, and Cluster Toolkit modules are themselves nested, so the
// resources of interest are never in the root module.
func (d *Discovered) walk(m *tfjson.StateModule, parentAddr string, opts DiscoverOptions) {
	if m == nil {
		return
	}
	addr := m.Address
	if addr == "" {
		addr = parentAddr
	}
	for _, r := range m.Resources {
		d.collect(r, addr, opts)
	}
	for _, c := range m.ChildModules {
		d.walk(c, addr, opts)
	}
}

func (d *Discovered) collect(r *tfjson.StateResource, moduleAddr string, opts DiscoverOptions) {
	if r == nil {
		return
	}
	switch r.Type {
	case tfSubnetwork:
		d.collectSubnetwork(r)
	case tfContainerCluster, tfNodePool:
		d.collectContainerCluster(r)
	default:
		if r.Mode != tfjson.DataResourceMode {
			d.collectManagedResource(r, moduleAddr, opts)
		}
	}
}

func (d *Discovered) collectSubnetwork(r *tfjson.StateResource) {
	project, region, name := attrString(r, "project"), attrString(r, "region"), attrString(r, "name")
	path := relativePath(attrString(r, "self_link"))
	if path == "" && project != "" && region != "" && name != "" {
		path = fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", project, region, name)
	}
	if path == "" {
		return
	}
	if region == "" {
		region = pathSegment(path, "regions")
	}
	network := attrString(r, "network")
	if p := relativePath(network); p != "" {
		network = p
	} else if network != "" {
		network = fmt.Sprintf("projects/%s/global/networks/%s", pathSegment(path, "projects"), network)
	}
	if network == "" {
		return
	}
	if _, seen := d.subnets[path]; !seen {
		d.subnets[path] = discoveredSubnet{region: region, pair: NetworkPair{Network: network, Subnetwork: path}}
	}
}

func (d *Discovered) collectContainerCluster(r *tfjson.StateResource) {
	if r.Type == tfContainerCluster {
		id := attrString(r, "id")
		if id == "" {
			id = attrString(r, "name")
		}
		d.ContainerClusters = appendUnique(d.ContainerClusters, id)
		return
	}
	if r.Mode == tfjson.DataResourceMode {
		return
	}
	if cluster := attrString(r, "cluster"); cluster != "" {
		d.ContainerClusters = appendUnique(d.ContainerClusters, cluster)
	} else if name := attrString(r, "name"); name != "" {
		d.ContainerClusters = appendUnique(d.ContainerClusters, name)
	}
}

func (d *Discovered) collectManagedResource(r *tfjson.StateResource, moduleAddr string, opts DiscoverOptions) {
	switch r.Type {
	case tfBucket:
		if shouldSkipBucket(r, moduleAddr, opts) {
			return
		}
		d.Buckets = appendUnique(d.Buckets, attrString(r, "name"))
	case tfFilestore:
		d.Filestores = appendUnique(d.Filestores, attrString(r, "id"))
	case tfLustre:
		d.Lustres = appendUnique(d.Lustres, attrString(r, "id"))
	case tfMIG, tfRegionMIG:
		d.MIGs = appendUnique(d.MIGs, attrString(r, "self_link"))
	}
}

func shouldSkipBucket(r *tfjson.StateResource, moduleAddr string, opts DiscoverOptions) bool {
	if len(opts.IgnoreBucketsFromModules) == 0 {
		return false
	}
	addr := moduleAddr
	if addr == "" {
		addr = r.Address
	}
	return opts.IgnoreBucketsFromModules[topLevelModuleID(addr)]
}

func topLevelModuleID(addr string) string {
	if !strings.HasPrefix(addr, "module.") {
		return ""
	}
	rest := strings.TrimPrefix(addr, "module.")
	if i := strings.IndexAny(rest, ".["); i >= 0 {
		return rest[:i]
	}
	return rest
}

// NetworkPairs returns the (network, subnetwork) pairs of the subnetworks in
// region, sorted by subnetwork path. Each subnetwork is paired with its own
// VPC, so multi-NIC and multi-VPC deployments are exported accurately.
func (d Discovered) NetworkPairs(region string) []NetworkPair {
	var pairs []NetworkPair
	for _, sn := range d.subnets {
		if sn.region == region {
			pairs = append(pairs, sn.pair)
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Subnetwork < pairs[j].Subnetwork })
	return pairs
}

// OtherRegionSubnetworks returns the paths of the subnetworks outside region,
// sorted. They aren't exported, because a cluster's networks must be in its
// region.
func (d Discovered) OtherRegionSubnetworks(region string) []string {
	var paths []string
	for path, sn := range d.subnets {
		if sn.region != region {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// relativePath turns a Compute Engine self link
// (https://www.googleapis.com/compute/v1/projects/...) into a relative
// resource path (projects/...). It returns "" if s isn't a resource path.
func relativePath(s string) string {
	if i := strings.Index(s, "projects/"); i >= 0 {
		return strings.TrimSuffix(s[i:], "/")
	}
	return ""
}

// pathSegment returns the value after collection in a resource path, for
// example pathSegment("projects/p/regions/r/subnetworks/s", "regions") is "r".
func pathSegment(path, collection string) string {
	parts := strings.Split(path, "/")
	for i := 0; i+1 < len(parts); i++ {
		if parts[i] == collection {
			return parts[i+1]
		}
	}
	return ""
}

// attrString reads a string attribute, tolerating resources that do not carry
// it. A state written by an older provider may omit an attribute entirely.
func attrString(r *tfjson.StateResource, key string) string {
	v, ok := r.AttributeValues[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func appendUnique(xs []string, v string) []string {
	if v == "" {
		return xs
	}
	for _, x := range xs {
		if x == v {
			return xs
		}
	}
	return append(xs, v)
}
