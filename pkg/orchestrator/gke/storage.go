// Copyright 2026 "Google LLC"
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

package gke

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"text/template"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	filestore "cloud.google.com/go/filestore/apiv1"
	"cloud.google.com/go/filestore/apiv1/filestorepb"
	"google.golang.org/api/iterator"

	"gopkg.in/yaml.v2"
)

// filestoreTmpl is the pre-parsed template for Filestore configuration.
var filestoreTmpl = template.Must(template.ParseFS(templatesFS, "templates/filestore.tmpl"))

// ProcessMounts parses mount strings and generates necessary K8s resources.
func (sm *StorageManager) ProcessMounts(mounts []string, job orchestrator.JobDefinition) ([]MountInfo, []string, error) {
	var mountInfos []MountInfo
	var additionalManifests []string
	for i, vStr := range mounts {
		src, dest, readOnly, err := sm.parseSingleVolume(vStr)
		if err != nil {
			return nil, nil, err
		}

		if strings.HasPrefix(src, "filestore://") {
			info, manifest, err := sm.handleFilestoreMount(src, dest, readOnly, i, job)
			if err != nil {
				return nil, nil, err
			}
			mountInfos = append(mountInfos, info)
			if manifest != "" {
				additionalManifests = append(additionalManifests, manifest)
			}
			continue
		}

		volType := "pvc"
		if strings.HasPrefix(src, "gs://") {
			volType = "gcsfuse"
		} else if strings.HasPrefix(src, "/") {
			volType = "hostPath"
		}

		mountInfos = append(mountInfos, MountInfo{
			Name:      fmt.Sprintf("vol-%d", i),
			Source:    src,
			MountPath: dest,
			Type:      volType,
			ReadOnly:  readOnly,
		})
	}

	return mountInfos, additionalManifests, nil
}

// ValidateMounts checks mounts for duplicate sources/destinations and valid formats.
func (sm *StorageManager) ValidateMounts(mounts []string) error {
	seenSources := make(map[string]bool)
	seenDestinations := make(map[string]bool)

	for _, vStr := range mounts {
		src, dest, _, err := sm.parseSingleVolume(vStr)
		if err != nil {
			return err
		}

		if seenSources[src] {
			return fmt.Errorf("duplicate volume source: %s", src)
		}
		if seenDestinations[dest] {
			return fmt.Errorf("duplicate volume destination: %s", dest)
		}
		seenSources[src] = true
		seenDestinations[dest] = true
	}

	return nil
}

func (sm *StorageManager) parseSingleVolume(vStr string) (src, dest string, readOnly bool, err error) {
	src, dest, readOnly, err = parseSrcDest(vStr)
	if err != nil {
		return "", "", false, err
	}
	if err := validateSrcScheme(src, vStr); err != nil {
		return "", "", false, err
	}
	return src, dest, readOnly, nil
}

func missingDestOrFormatErr(vStr string) error {
	if strings.HasPrefix(vStr, "gs://") || strings.HasPrefix(vStr, "filestore://") {
		return fmt.Errorf("invalid volume format: %s. Missing destination.", vStr)
	}
	return fmt.Errorf("invalid volume format: %s. Expected format: <src>:<dest>[:<mode>]", vStr)
}

func parseVolumeMode(vStr string) (stripped string, readOnly bool, err error) {
	idx := strings.LastIndex(vStr, ":")
	if idx <= 0 || idx == len(vStr)-1 {
		return "", false, missingDestOrFormatErr(vStr)
	}

	lastPart := vStr[idx+1:]
	if lastPart == "ro" || lastPart == "rw" {
		return vStr[:idx], (lastPart == "ro"), nil
	}
	return vStr, true, nil
}

func parseSrcDest(vStr string) (src, dest string, readOnly bool, err error) {
	origVStr := vStr

	vStrWithoutMode, readOnly, err := parseVolumeMode(vStr)
	if err != nil {
		return "", "", false, err
	}

	idx := strings.LastIndex(vStrWithoutMode, ":")
	if idx <= 0 || idx == len(vStrWithoutMode)-1 {
		return "", "", false, missingDestOrFormatErr(origVStr)
	}

	src = vStrWithoutMode[:idx]
	dest = vStrWithoutMode[idx+1:]

	if src == "" || dest == "" || !strings.HasPrefix(dest, "/") {
		return "", "", false, missingDestOrFormatErr(origVStr)
	}

	// Ensure scheme colons are not mistaken for delimiters when destination is missing
	if (strings.HasPrefix(vStrWithoutMode, "gs://") && !strings.HasPrefix(src, "gs://")) ||
		(strings.HasPrefix(vStrWithoutMode, "filestore://") && !strings.HasPrefix(src, "filestore://")) {
		return "", "", false, missingDestOrFormatErr(origVStr)
	}

	return src, dest, readOnly, nil
}

func extractHost(hostStr string) string {
	h, _, err := net.SplitHostPort(hostStr)
	if err == nil {
		return h
	}
	h = hostStr
	h = strings.TrimPrefix(h, "[")
	h = strings.TrimSuffix(h, "]")
	return h
}

func validateSrcScheme(src string, vStr string) error {
	if !strings.Contains(src, ":") {
		return nil
	}
	if strings.HasPrefix(src, "gs://") || strings.HasPrefix(src, "filestore://") {
		idx := strings.Index(src, "://")
		remaining := src[idx+3:]
		// If the source contains a colon after the scheme (e.g., in IPv6 addresses or port),
		// verify that the host part is a valid IP address if it is IPv6.
		if strings.Contains(remaining, ":") {
			host := remaining
			if slashIdx := strings.Index(host, "/"); slashIdx != -1 {
				host = host[:slashIdx]
			}
			actualHost := extractHost(host)
			if strings.Contains(actualHost, ":") {
				if net.ParseIP(actualHost) == nil {
					return fmt.Errorf("invalid volume format: %s", vStr)
				}
			}
		}
		return nil
	}
	return fmt.Errorf("invalid volume format: %s. Unsupported scheme.", vStr)
}

func (sm *StorageManager) handleFilestoreMount(src, dest string, readOnly bool, idx int, job orchestrator.JobDefinition) (MountInfo, string, error) {
	trimmed := strings.TrimPrefix(src, "filestore://")
	trimmed = strings.TrimRight(trimmed, "/")
	parts := strings.SplitN(trimmed, "/", 2)

	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return MountInfo{}, "", fmt.Errorf("invalid filestore mount %q. Expected format: filestore://<instance_or_ip>/<share_name>", src)
	}

	instanceOrIP := parts[0]
	share := strings.TrimLeft(parts[1], "/")
	if share == "" {
		return MountInfo{}, "", fmt.Errorf("invalid filestore mount %q: share name is missing. Expected format: filestore://<instance_or_ip>/<share_name>", src)
	}

	cleanHost := strings.TrimPrefix(strings.TrimRight(instanceOrIP, "]"), "[")
	isIP := net.ParseIP(cleanHost) != nil

	ip, resolvedName, capacityGb, err := sm.resolveFilestoreIP(job.ProjectID, job.ClusterLocation, cleanHost, isIP)
	if err != nil {
		return MountInfo{}, "", err
	}
	capacityStr := fmt.Sprintf("%dGi", capacityGb)

	pvcName := fmt.Sprintf("gcluster-filestore-%s-%s", resolvedName, share)
	pvcName = sanitizePVCName(pvcName)
	// Truncate pvcName to avoid PV name collisions when the namespace is appended.
	// A PV name is derived from <pvc-name>-<namespace>. A namespace can be up to 63
	// characters. By limiting the PVC name to 189, we ensure the combined name does not
	// exceed the 253-character limit and cause truncation that could lead to collisions.
	if len(pvcName) > 189 {
		pvcName = strings.TrimRight(pvcName[:189], "-")
	}

	var ns string
	if sm.orchestrator != nil {
		var err error
		ns, err = sm.orchestrator.getCurrentNamespace()
		if err != nil {
			logging.Warn("failed to get current namespace: %v. Defaulting to 'default' for PV name.", err)
		}
	}
	if ns == "" {
		ns = "default"
	}
	pvName := sanitizePVCName(fmt.Sprintf("%s-%s", pvcName, ns))

	var buf bytes.Buffer
	err = filestoreTmpl.Execute(&buf, map[string]string{
		"PVName":   pvName,
		"PVCName":  pvcName,
		"Share":    share,
		"IP":       ip,
		"Capacity": capacityStr,
	})
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to execute filestore template: %w", err)
	}
	pvYAML := buf.String()

	info := MountInfo{
		Name:      fmt.Sprintf("vol-%d", idx),
		Source:    pvcName,
		MountPath: dest,
		Type:      "pvc",
		ReadOnly:  readOnly,
	}

	return info, pvYAML, nil
}

func sanitizePVCName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return '-'
	}, name)
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	if len(name) > 253 {
		name = name[:253]
	}
	name = strings.Trim(name, "-")
	return name
}

// AddVolumeOptions marshals and indents the volume and volume mount specifications into the manifest options.
func (sm *StorageManager) AddVolumeOptions(opts *ManifestOptions, vols []MountInfo) {
	if len(vols) == 0 {
		return
	}

	var volSpecs []map[string]interface{}
	var mountSpecs []map[string]interface{}
	gcsFuseEnabled := false

	for _, v := range vols {
		mountSpecs = append(mountSpecs, buildVolumeMountSpec(v))
		volSpecs = append(volSpecs, buildVolumeSpec(v))
		if v.Type == "gcsfuse" {
			gcsFuseEnabled = true
		}
	}

	opts.GCSFuseEnabled = gcsFuseEnabled

	if b, err := yaml.Marshal(mountSpecs); err == nil {
		opts.VolumeMountsYAML = indentYaml(string(b), 16)
	}
	if b, err := yaml.Marshal(volSpecs); err == nil {
		opts.VolumesYAML = indentYaml(string(b), 14)
	}
}

func buildVolumeMountSpec(v MountInfo) map[string]interface{} {
	mountSpec := map[string]interface{}{
		"name":      v.Name,
		"mountPath": v.MountPath,
	}
	if v.ReadOnly {
		mountSpec["readOnly"] = true
	}
	return mountSpec
}

func buildVolumeSpec(v MountInfo) map[string]interface{} {
	spec := map[string]interface{}{
		"name": v.Name,
	}
	switch v.Type {
	case "gcsfuse":
		spec["csi"] = map[string]interface{}{
			"driver":   "gcsfuse.csi.storage.gke.io",
			"readOnly": v.ReadOnly,
			"volumeAttributes": map[string]interface{}{
				"bucketName": strings.TrimPrefix(v.Source, "gs://"),
			},
		}
	case "hostPath":
		spec["hostPath"] = map[string]interface{}{
			"path": v.Source,
		}
	case "pvc":
		spec["persistentVolumeClaim"] = map[string]interface{}{
			"claimName": v.Source,
		}
	}
	return spec
}

func (sm *StorageManager) resolveFilestoreIP(projectID, location, nameOrIP string, isIP bool) (string, string, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if sm.getFilestoreIP != nil {
		return sm.getFilestoreIP(ctx, projectID, location, nameOrIP, isIP)
	}

	var instances []*filestorepb.Instance
	if sm.instancesCache != nil {
		instances = sm.instancesCache
	} else {
		var err error
		instances, err = sm.getFilestoreClient().listInstances(ctx, projectID)
		if err != nil {
			if isIP {
				logging.Warn("Filestore API lookup failed for %s: %v. Falling back to a default capacity of 1Ti (1024 GiB) for PV creation.", nameOrIP, err)
				return nameOrIP, strings.ReplaceAll(nameOrIP, ".", "-"), 1024, nil
			}
			return "", "", 0, fmt.Errorf("failed to list Filestore instances: %w", err)
		}
		sm.instancesCache = instances
	}

	isMatch := func(inst *filestorepb.Instance) bool {
		if isIP {
			return hasIPAddress(inst, nameOrIP)
		}
		name, _ := extractInstanceMetadata(inst.GetName())
		return name == nameOrIP
	}

	matches := filterMatchingInstances(instances, isMatch)
	matches = filterInstancesByLocation(matches, location)

	ip, resolvedName, capacity, err := extractInstanceInfo(matches, nameOrIP, isIP, projectID)
	if err != nil && isIP {
		logging.Warn("Filestore API resolution failed for %s: %v. Falling back to a default capacity of 1Ti (1024 GiB) for PV creation.", nameOrIP, err)
		return nameOrIP, strings.ReplaceAll(nameOrIP, ".", "-"), 1024, nil
	}
	return ip, resolvedName, capacity, err
}

func filterMatchingInstances(instances []*filestorepb.Instance, isMatch func(*filestorepb.Instance) bool) []*filestorepb.Instance {
	var matches []*filestorepb.Instance
	for _, inst := range instances {
		if isMatch(inst) {
			matches = append(matches, inst)
		}
	}
	return matches
}

func filterInstancesByLocation(matches []*filestorepb.Instance, location string) []*filestorepb.Instance {
	if len(matches) <= 1 || location == "" {
		return matches
	}
	var filtered []*filestorepb.Instance
	for _, inst := range matches {
		_, loc := extractInstanceMetadata(inst.GetName())
		if loc != "" && (loc == location || strings.HasPrefix(location, loc+"-") || strings.HasPrefix(loc, location+"-")) {
			filtered = append(filtered, inst)
		}
	}
	if len(filtered) > 0 {
		return filtered
	}
	return matches
}

type filestoreClient interface {
	listInstances(ctx context.Context, projectID string) ([]*filestorepb.Instance, error)
}

type gcpFilestoreClient struct{}

func (g *gcpFilestoreClient) listInstances(ctx context.Context, projectID string) ([]*filestorepb.Instance, error) {
	client, err := filestore.NewCloudFilestoreManagerClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create filestore client: %w", err)
	}
	defer client.Close()

	parent := fmt.Sprintf("projects/%s/locations/-", projectID)
	req := &filestorepb.ListInstancesRequest{
		Parent: parent,
	}

	var instances []*filestorepb.Instance
	it := client.ListInstances(ctx, req)
	for {
		resp, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		instances = append(instances, resp)
	}
	return instances, nil
}

func (sm *StorageManager) getFilestoreClient() filestoreClient {
	if sm.filestoreClient != nil {
		return sm.filestoreClient
	}
	return &gcpFilestoreClient{}
}

func hasIPAddress(inst *filestorepb.Instance, ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	for _, netConfig := range inst.GetNetworks() {
		for _, ipAddr := range netConfig.GetIpAddresses() {
			parsedAddr := net.ParseIP(ipAddr)
			if parsedAddr != nil && parsedAddr.Equal(parsedIP) {
				return true
			}
		}
	}
	return false
}

func extractInstanceMetadata(fullName string) (string, string) {
	parts := strings.Split(fullName, "/")
	if len(parts) == 6 {
		return parts[5], parts[3]
	}
	return "", ""
}

func collectLocations(instances []*filestorepb.Instance) []string {
	var locations []string
	for _, inst := range instances {
		if _, loc := extractInstanceMetadata(inst.GetName()); loc != "" {
			locations = append(locations, loc)
		}
	}
	return locations
}

func extractIPAndCapacity(inst *filestorepb.Instance, name string) (string, int64, error) {
	if len(inst.GetNetworks()) == 0 || len(inst.GetNetworks()[0].GetIpAddresses()) == 0 {
		return "", 0, fmt.Errorf("could not find IP address for Filestore instance %s", name)
	}
	if len(inst.GetFileShares()) == 0 {
		return "", 0, fmt.Errorf("Filestore instance %s has no file shares defined", name)
	}
	return inst.GetNetworks()[0].GetIpAddresses()[0], inst.GetFileShares()[0].GetCapacityGb(), nil
}

func extractInstanceInfo(matches []*filestorepb.Instance, nameOrIP string, isIP bool, projectID string) (string, string, int64, error) {
	if len(matches) == 0 {
		if isIP {
			return "", "", 0, fmt.Errorf("Filestore instance with IP %q not found in project %s", nameOrIP, projectID)
		}
		return "", "", 0, fmt.Errorf("Filestore instance %q not found in project %s", nameOrIP, projectID)
	}

	if len(matches) > 1 {
		locations := collectLocations(matches)
		if isIP {
			return "", "", 0, fmt.Errorf("multiple Filestore instances found with IP %q in locations: %v", nameOrIP, locations)
		}
		return "", "", 0, fmt.Errorf("multiple Filestore instances named %q found in locations: %v. Please resolve the ambiguity by specifying the Filestore IP address directly", nameOrIP, locations)
	}

	inst := matches[0]
	resolvedName, _ := extractInstanceMetadata(inst.GetName())
	if inst.GetState() != filestorepb.Instance_READY {
		return "", "", 0, fmt.Errorf("Filestore instance %s not in READY state (current state: %s)", resolvedName, inst.GetState())
	}

	ip, capacity, err := extractIPAndCapacity(inst, resolvedName)
	if err != nil {
		return "", "", 0, err
	}
	return ip, resolvedName, capacity, nil
}
