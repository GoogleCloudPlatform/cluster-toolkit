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
	"hash/crc32"
	"net/url"
	"reflect"
	"regexp"
	"strings"
)

const (
	// BlueprintLabel is the label that the Cluster Toolkit stamps on every
	// resource with the blueprint name.
	BlueprintLabel = "ghpc_blueprint"

	// DeploymentLabel is the label that the Cluster Toolkit stamps on every
	// resource it creates. It is used to select the instances belonging to a
	// deployment when exporting them to Cluster Director.
	DeploymentLabel = "ghpc_deployment"

	// ImportedFromToolkitLabel is the cluster-level label stamped on every
	// Cluster Director cluster record exported by Cluster Toolkit so the
	// Cluster Director console can filter clusters exported from Cluster Toolkit.
	ImportedFromToolkitLabel = "imported_from_cluster_toolkit"

	// NodesResourceKey is the key used for the label-selected Compute Engine
	// instances exported from the Cluster Toolkit deployment.
	NodesResourceKey = "ctk-nodes"
)

// maxKeyLength is the RFC-1034 limit applied to resource map keys.
const maxKeyLength = 63

// maxClusterIDLength is the Cluster Director API limit on cluster IDs
// ("Start with a lowercase letter. Use only lowercase letters and numbers.
// Limit to 10 characters.").
const maxClusterIDLength = 10

// maxDescriptionLength is the Cluster Director API limit on Cluster.description.
const maxDescriptionLength = 2048

var (
	// clusterIDPattern matches valid Cluster Director cluster IDs: 1-10
	// lower-case alphanumeric characters starting with a letter (no hyphens).
	clusterIDPattern = regexp.MustCompile(`^[a-z][a-z0-9]{0,9}$`)
	// invalidClusterIDChars matches every character not allowed in a cluster ID.
	invalidClusterIDChars = regexp.MustCompile(`[^a-z0-9]+`)
	// invalidKeyChars matches every character that is not allowed in a key.
	invalidKeyChars = regexp.MustCompile(`[^a-z0-9-]+`)
	// computeAPIPrefix matches the various self link prefixes returned by the
	// Compute Engine API, e.g.
	// https://www.googleapis.com/compute/v1/projects/... .
	computeAPIPrefix = regexp.MustCompile(`^(https?://[^/]+)?/?(compute/(v1|beta|alpha)/)?`)
	// serviceAPIPrefix matches the self link prefix of the regional service
	// APIs (Filestore, Managed Lustre), e.g.
	// https://file.googleapis.com/v1/projects/... .
	serviceAPIPrefix = regexp.MustCompile(`^(https?://[^/]+)?/?((v1|v1beta1|v1alpha1)/)?`)
)

// Spec captures the resources of a Cluster Toolkit deployment to export as a
// cluster in Cluster Director.
type Spec struct {
	// ProjectID hosting the deployment, e.g. "my-project".
	ProjectID string
	// Region of the Cluster Director cluster, e.g. "us-central1".
	Region string
	// ClusterName is the Cluster Director cluster ID to create.
	ClusterName string
	// DeploymentName is the Cluster Toolkit deployment name. Instances are
	// selected by the `ghpc_deployment` label carrying this value.
	DeploymentName string

	// Networks holds the (network, subnetwork) pairs discovered from the
	// deployment's Terraform state, as full resource paths.
	Networks []NetworkPair

	// Buckets are Cloud Storage bucket names discovered from Terraform state.
	// A "gs://" prefix is tolerated.
	Buckets []string
	// Filestores are Filestore instance IDs discovered from Terraform state.
	Filestores []string
	// Lustres are Managed Lustre instance IDs discovered from Terraform state.
	Lustres []string

	// MIGs are managed instance group self links or partial URLs discovered
	// from Terraform state. Both zonal and regional MIGs are supported.
	MIGs []string

	// ClusterLabels are labels applied to the Cluster Director cluster.
	ClusterLabels map[string]string
}

// Validate reports whether the spec contains everything needed to build an
// import payload.
func (s Spec) Validate() error {
	var missing []string
	if s.ProjectID == "" {
		missing = append(missing, "project")
	}
	if s.Region == "" {
		missing = append(missing, "region")
	}
	if s.ClusterName == "" {
		missing = append(missing, "cluster name")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required value(s): %s", strings.Join(missing, ", "))
	}
	if !clusterIDPattern.MatchString(s.ClusterName) {
		return fmt.Errorf("cluster name %q must start with a lowercase letter and contain only lowercase letters and digits (at most %d characters, no hyphens)", s.ClusterName, maxClusterIDLength)
	}
	if desc := s.clusterDescription(); len(desc) > maxDescriptionLength {
		return fmt.Errorf("cluster description (%d characters) exceeds maximum length of %d characters", len(desc), maxDescriptionLength)
	}
	if err := s.validateNetworkPairs(); err != nil {
		return err
	}
	if len(s.instanceLabels()) == 0 && len(s.MIGs) == 0 {
		return fmt.Errorf("no compute to export: provide a deployment name or at least one MIG")
	}
	return nil
}

func (s Spec) validateNetworkPairs() error {
	for _, pair := range s.Networks {
		if (pair.Network == "") != (pair.Subnetwork == "") {
			return fmt.Errorf("network and subnetwork must be provided together, got network=%q subnetwork=%q", pair.Network, pair.Subnetwork)
		}
	}
	return nil
}

func (s Spec) clusterDescription() string {
	if s.DeploymentName != "" {
		return fmt.Sprintf("Cluster Toolkit deployment: %s", s.DeploymentName)
	}
	return ""
}

// instanceLabels returns the label selector used to adopt instances.
func (s Spec) instanceLabels() map[string]string {
	if s.DeploymentName == "" {
		return nil
	}
	return map[string]string{DeploymentLabel: s.DeploymentName}
}

// clusterLabels returns the labels applied to the Cluster Director cluster
// record, always stamping `imported_from_cluster_toolkit: "true"` so the
// Cluster Director frontend can identify CTK-imported clusters, and stamping
// `ghpc_deployment: <DeploymentName>` when DeploymentName is set so `gcluster`
// can verify ownership before updating or deleting a cluster record.
func (s Spec) clusterLabels() map[string]string {
	labels := make(map[string]string, len(s.ClusterLabels)+2)
	for k, v := range s.ClusterLabels {
		labels[k] = v
	}
	if s.DeploymentName != "" {
		labels[DeploymentLabel] = s.DeploymentName
	}
	labels[ImportedFromToolkitLabel] = "true"
	return labels
}

// ClusterOwner returns the Cluster Toolkit deployment name that owns c, read
// from the cluster's top-level `ghpc_deployment` label or from its `ctk-nodes`
// ExistingInstances label selector. It returns "" when c does not carry a
// deployment ownership label.
func ClusterOwner(c *Cluster) string {
	if c == nil {
		return ""
	}
	if owner := c.Labels[DeploymentLabel]; owner != "" {
		return owner
	}
	if c.Orchestrator != nil && c.Orchestrator.ComputeEngine != nil {
		if nodes, ok := c.Orchestrator.ComputeEngine.ExistingInstances[NodesResourceKey]; ok {
			if owner := nodes.Labels[DeploymentLabel]; owner != "" {
				return owner
			}
		}
	}
	return ""
}

// Cluster renders the spec as a Cluster Director cluster payload.
func (s Spec) Cluster() (*Cluster, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}

	c := &Cluster{
		Name:             s.ClusterName,
		Description:      s.clusterDescription(),
		Labels:           s.clusterLabels(),
		NetworkResources: s.networkResources(),
	}

	storage, err := s.storageResources()
	if err != nil {
		return nil, err
	}
	if len(storage) > 0 {
		c.StorageResources = storage
	}

	instances, err := s.existingInstances()
	if err != nil {
		return nil, err
	}
	if len(instances) > 0 {
		c.Orchestrator = &Orchestrator{ComputeEngine: &ComputeEngineOrchestrator{ExistingInstances: instances}}
	}

	return c, nil
}

// SameResources reports whether two Cluster payloads describe the same
// exported description, labels, network, storage, and compute resource
// configurations.
func SameResources(a, b *Cluster) bool {
	if a == nil || b == nil {
		return a == b
	}
	return reflect.DeepEqual(canonicalResources(a), canonicalResources(b))
}

func canonicalResources(c *Cluster) Cluster {
	var out Cluster
	out.Description = c.Description
	if len(c.Labels) > 0 {
		out.Labels = c.Labels
	}
	if len(c.NetworkResources) > 0 {
		out.NetworkResources = c.NetworkResources
	}
	if len(c.StorageResources) > 0 {
		out.StorageResources = c.StorageResources
	}
	if c.Orchestrator != nil {
		var orch Orchestrator
		if c.Orchestrator.ComputeEngine != nil && len(c.Orchestrator.ComputeEngine.ExistingInstances) > 0 {
			instances := make(map[string]ExistingInstances, len(c.Orchestrator.ComputeEngine.ExistingInstances))
			for k, v := range c.Orchestrator.ComputeEngine.ExistingInstances {
				if len(v.Labels) == 0 {
					v.Labels = nil
				}
				instances[k] = v
			}
			orch.ComputeEngine = &ComputeEngineOrchestrator{
				ExistingInstances: instances,
			}
		}
		if orch.ComputeEngine != nil {
			out.Orchestrator = &orch
		}
	}
	return out
}

// networkResources renders the VPCs of the deployment, if any. Map keys come
// from each (network, subnetwork) pair (prefixed by host project when the
// network lives outside s.ProjectID), so keys are unique and order-invariant.
func (s Spec) networkResources() map[string]NetworkResource {
	nets := map[string]NetworkResource{}
	for _, p := range s.Networks {
		netLeaf := qualifiedLeaf(p.Network)
		subLeaf := qualifiedLeaf(p.Subnetwork)
		if netLeaf == "" || subLeaf == "" {
			continue
		}
		network := relativePath(p.Network)
		if network == "" {
			network = fmt.Sprintf("projects/%s/global/networks/%s", s.ProjectID, netLeaf)
		}
		subnetwork := relativePath(p.Subnetwork)
		if subnetwork == "" {
			subnetwork = fmt.Sprintf("projects/%s/regions/%s/subnetworks/%s", s.ProjectID, s.Region, subLeaf)
		}
		key := ResourceKey("ctk-net", netKeySuffix(s.ProjectID, network, netLeaf, subLeaf))
		nets[key] = NetworkResource{Config: NetworkResourceConfig{
			ExistingNetwork: &ExistingNetworkConfig{Network: network, Subnetwork: subnetwork},
		}}
	}
	if len(nets) == 0 {
		return nil
	}
	return nets
}

func netKeySuffix(projectID, networkPath, netLeaf, subLeaf string) string {
	if netProj := pathSegment(networkPath, "projects"); netProj != "" && netProj != projectID {
		return netProj + "-" + netLeaf + "-" + subLeaf
	}
	return netLeaf + "-" + subLeaf
}

func scopedLeaf(id string, collections ...string) string {
	leaf := qualifiedLeaf(id)
	for _, col := range collections {
		if loc := pathSegment(id, col); loc != "" {
			return loc + "-" + leaf
		}
	}
	return leaf
}

// storageResources renders the buckets, Filestore and Managed Lustre
// instances of the deployment. Map keys are derived from each resource's
// location and name rather than its slice index, because Cluster Director's
// UpdateCluster endpoint rejects any PATCH where an existing storage key's
// config changes.
func (s Spec) storageResources() (map[string]StorageResource, error) {
	storage := map[string]StorageResource{}

	for _, b := range s.Buckets {
		name, err := normalizeBucket(b)
		if err != nil {
			return nil, err
		}
		storage[ResourceKey("ctk-bucket", name)] = StorageResource{
			Config: StorageResourceConfig{ExistingBucket: &ExistingBucketConfig{Bucket: name}},
		}
	}
	for _, f := range s.Filestores {
		id, err := normalizeInstanceID("filestore", f)
		if err != nil {
			return nil, err
		}
		storage[ResourceKey("ctk-filestore", scopedLeaf(id, "locations", "zones"))] = StorageResource{
			Config: StorageResourceConfig{ExistingFilestore: &ExistingFilestoreConfig{Filestore: id}},
		}
	}
	for _, l := range s.Lustres {
		id, err := normalizeInstanceID("lustre", l)
		if err != nil {
			return nil, err
		}
		storage[ResourceKey("ctk-lustre", scopedLeaf(id, "locations", "zones"))] = StorageResource{
			Config: StorageResourceConfig{ExistingLustre: &ExistingLustreConfig{Lustre: id}},
		}
	}
	return storage, nil
}

// existingInstances renders the compute of the deployment: label selected
// instances (including standalone and reservation-backed VMs) and managed
// instance groups.
func (s Spec) existingInstances() (map[string]ExistingInstances, error) {
	instances := map[string]ExistingInstances{}
	labels := s.instanceLabels()

	if len(labels) > 0 {
		instances[NodesResourceKey] = ExistingInstances{
			Project: fmt.Sprintf("projects/%s", s.ProjectID),
			Labels:  labels,
		}
	}
	for _, m := range s.MIGs {
		mig, regional, err := normalizeMIG(m)
		if err != nil {
			return nil, err
		}
		key := ResourceKey("ctk-mig", scopedLeaf(mig, "zones", "regions"))
		if regional {
			instances[key] = ExistingInstances{RegionInstanceGroupManager: mig}
		} else {
			instances[key] = ExistingInstances{InstanceGroupManager: mig}
		}
	}
	return instances, nil
}

// ResourceKey builds an RFC-1034 compliant map key derived from a prefix and a
// resource's identity (e.g. ResourceKey("ctk-bucket", "my-bucket") ->
// "ctk-bucket-my-bucket"). If the combined key exceeds 63 characters, its tail
// is replaced with a deterministic 4-character CRC32 checksum so long names
// sharing a prefix never collide.
func ResourceKey(prefix, id string) string {
	raw := fmt.Sprintf("%s-%s", prefix, id)
	s := invalidKeyChars.ReplaceAllString(strings.ToLower(raw), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "ctk"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "ctk-" + s
	}
	if len(s) <= maxKeyLength {
		return s
	}
	head := strings.TrimRight(s[:maxKeyLength-5], "-")
	return fmt.Sprintf("%s-%04x", head, crc32.ChecksumIEEE([]byte(raw))&0xffff)
}

// SanitizeClusterID coerces s into a valid Cluster Director cluster ID: 1-10
// lower-case alphanumeric characters starting with a letter (no hyphens).
// If stripping non-alphanumerics yields at most 10 characters, that string is
// used directly (e.g. "ctk-demo" -> "ctkdemo"). Longer names retain their
// 6-character prefix and append a deterministic 4-character hex checksum so
// distinct deployment names do not collide.
func SanitizeClusterID(s string) string {
	clean := invalidClusterIDChars.ReplaceAllString(strings.ToLower(s), "")
	if clean == "" {
		return "ctk"
	}
	if clean[0] >= '0' && clean[0] <= '9' {
		clean = "c" + clean
	}
	if len(clean) <= maxClusterIDLength {
		return clean
	}
	return fmt.Sprintf("%s%04x", clean[:6], crc32.ChecksumIEEE([]byte(s))&0xffff)
}

// ResolveClusterID derives a cluster ID from deploymentName. Cluster Director
// rejects MIGs whose name starts with `<clusterID>-`, which Slurm MIG names
// usually do, so the ID gets a suffix when it would collide with one of migs.
func ResolveClusterID(deploymentName string, migs []string) string {
	id := SanitizeClusterID(deploymentName)
	if !MIGHasClusterIDPrefix(id, migs) {
		return id
	}
	head := id
	if len(head) > maxClusterIDLength-2 {
		head = head[:maxClusterIDLength-2]
	}
	if candidate := head + "cd"; !MIGHasClusterIDPrefix(candidate, migs) {
		return candidate
	}
	prefix := id
	if len(prefix) > 6 {
		prefix = prefix[:6]
	}
	return fmt.Sprintf("%s%04x", prefix, crc32.ChecksumIEEE([]byte(deploymentName+":cd"))&0xffff)
}

// MIGHasClusterIDPrefix reports whether any MIG in migs has a short name
// starting with `<clusterID>-`, which Cluster Director's API rejects.
func MIGHasClusterIDPrefix(clusterID string, migs []string) bool {
	if clusterID == "" {
		return false
	}
	prefix := clusterID + "-"
	for _, m := range migs {
		if strings.HasPrefix(qualifiedLeaf(m), prefix) {
			return true
		}
	}
	return false
}

// ConsoleURL returns the Google Cloud Console URL for viewing an imported
// Cluster Director cluster's overview page.
func ConsoleURL(projectID, region, clusterID string) string {
	return fmt.Sprintf("https://console.cloud.google.com/cluster-director/clusters/details/%s/%s/overview?project=%s",
		url.PathEscape(region), url.PathEscape(clusterID), url.QueryEscape(projectID))
}

// qualifiedLeaf returns the last path segment of a resource name or self
// link, so that both bare names and fully qualified references are accepted.
func qualifiedLeaf(s string) string {
	s = strings.TrimSuffix(strings.TrimSpace(s), "/")
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// normalizeBucket strips the optional gs:// scheme from a bucket name.
func normalizeBucket(b string) (string, error) {
	b = strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(b), "gs://"), "/")
	if b == "" {
		return "", fmt.Errorf("empty bucket name")
	}
	if strings.Contains(b, "/") {
		return "", fmt.Errorf("bucket %q must be a bare bucket name, optionally prefixed with gs://", b)
	}
	return b, nil
}

// normalizeInstanceID converts a Filestore or Managed Lustre reference into
// the `projects/{project}/locations/{location}/instances/{instance}` form
// expected by the API. Self links are accepted and reduced to that form.
func normalizeInstanceID(kind, id string) (string, error) {
	id = strings.Trim(strings.TrimSpace(id), "/")
	if id == "" {
		return "", fmt.Errorf("empty %s instance ID", kind)
	}
	id = serviceAPIPrefix.ReplaceAllString(id, "")
	parts := strings.Split(id, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[2] != "locations" || parts[4] != "instances" {
		return "", fmt.Errorf("%s %q must be of the form projects/{project}/locations/{location}/instances/{instance}", kind, id)
	}
	return id, nil
}

// normalizeMIG converts a managed instance group self link or partial URL
// into a `projects/...` reference and reports whether it is regional.
func normalizeMIG(mig string) (string, bool, error) {
	mig = strings.Trim(strings.TrimSpace(mig), "/")
	if mig == "" {
		return "", false, fmt.Errorf("empty managed instance group")
	}
	mig = computeAPIPrefix.ReplaceAllString(mig, "")
	parts := strings.Split(mig, "/")
	if len(parts) != 6 || parts[0] != "projects" || parts[4] != "instanceGroupManagers" {
		return "", false, fmt.Errorf("managed instance group %q must be of the form projects/{project}/[zones|regions]/{location}/instanceGroupManagers/{name}", mig)
	}
	switch parts[2] {
	case "zones":
		return mig, false, nil
	case "regions":
		return mig, true, nil
	default:
		return "", false, fmt.Errorf("managed instance group %q must be scoped by `zones` or `regions`, got %q", mig, parts[2])
	}
}
