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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net"
	"path"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	filestore "cloud.google.com/go/filestore/apiv1"
	"cloud.google.com/go/filestore/apiv1/filestorepb"
	crm "google.golang.org/api/cloudresourcemanager/v1"
	iamapi "google.golang.org/api/iam/v1"
	"google.golang.org/api/iterator"
	gcs "google.golang.org/api/storage/v1"

	"gopkg.in/yaml.v2"
)

const (
	volumeAttributeKeyCharset = `[A-Za-z0-9][A-Za-z0-9._-]*`
	// A gateway PV is named <pvc>-<namespace> and must fit the 253 character DNS
	// subdomain limit, so the PVC budget reserves 63 for the namespace label plus
	// the joining '-'.
	maxGeneratedPVCNameLength = 189

	gcsFuseGatewayPrefix   = "gcluster-gcsfuse"
	filestoreGatewayPrefix = "gcluster-filestore"
	gcsFuseGatewayCapacity = "5Gi" // Ignored by GCSFuse CSI driver, required by Kubernetes.

	gatewayNameDigestLength = 10

	// Gateway cleanup selects on these labels; templates receive them as params.
	managedByLabel       = "gcluster.google.com/managed-by"
	managedByValue       = "cluster-toolkit"
	storageTypeLabel     = "gcluster.google.com/storage-type"
	storageTypeGCSFuse   = "gcsfuse"
	storageTypeFilestore = "filestore"

	servingProfileStorageClass = "gcsfusecsi-serving"

	// minGCSFuseProfileGKEVersion is the first GKE version with storage profile StorageClasses.
	minGCSFuseProfileGKEVersion = "1.35.1-gke.1616000"
	gcsFuseProfileSelector      = "gke-gcsfuse/profile=true"

	gcsFuseProfileDocsURL = "https://cloud.google.com/kubernetes-engine/docs/how-to/persistent-volumes/gcsfuse-profiles"
	gcsFuseProfileIAMURL  = gcsFuseProfileDocsURL + "#configure_permissions"
	anywhereCacheDocsURL  = "https://cloud.google.com/storage/docs/anywhere-cache"

	gkeServiceAgentTemplate = "service-%d@container-engine-robot.iam.gserviceaccount.com"

	anywhereCacheAttributePrefix = "anywhereCache"
	anywhereCacheZonesAttribute  = "anywhereCacheZones"
	// "*" lets GKE pick cache zones; "none" disables Rapid Cache.
	anywhereCacheAllZonesValue = "*"
	anywhereCacheOffValue      = "none"

	bucketRegionLocationType = "region"

	// storageAPITimeout bounds the pre-flight Cloud API calls.
	storageAPITimeout = 30 * time.Second
)

func newMountBuildState() *mountBuildState {
	return &mountBuildState{gatewayVolumeNames: map[string]string{}}
}

func (s *mountBuildState) volumeNameFor(pvName string, idx int) (name string, reused bool) {
	if existing, ok := s.gatewayVolumeNames[pvName]; ok {
		return existing, true
	}
	name = fmt.Sprintf("vol-%d", idx)
	s.gatewayVolumeNames[pvName] = name
	return name, false
}

// ProcessMounts parses mount strings and generates necessary K8s resources.
func (sm *StorageManager) ProcessMounts(mounts []string, job orchestrator.JobDefinition) ([]MountInfo, []string, error) {
	var mountInfos []MountInfo
	var additionalManifests []string
	state := newMountBuildState()

	for i, vStr := range mounts {
		pm, err := sm.parseSingleVolume(vStr)
		if err != nil {
			return nil, nil, err
		}

		info, manifest, err := sm.dispatchMount(pm, i, job, state)
		if err != nil {
			return nil, nil, err
		}
		mountInfos = append(mountInfos, info)
		if manifest != "" {
			additionalManifests = append(additionalManifests, manifest)
		}
	}

	return mountInfos, additionalManifests, nil
}

func (sm *StorageManager) dispatchMount(pm parsedMount, idx int, job orchestrator.JobDefinition, state *mountBuildState) (MountInfo, string, error) {
	switch {
	case strings.HasPrefix(pm.Src, "filestore://"):
		return sm.generateFilestoreResources(pm, idx, job, state)
	case strings.HasPrefix(pm.Src, "gs://") && pm.Profile != "":
		return sm.generateGCSFuseProfileResources(pm, idx, job, state)
	default:
		return buildPodLevelMount(pm, idx), "", nil
	}
}

func buildPodLevelMount(pm parsedMount, idx int) MountInfo {
	volType := "pvc"
	if strings.HasPrefix(pm.Src, "gs://") {
		volType = "gcsfuse"
	} else if strings.HasPrefix(pm.Src, "/") {
		volType = "hostPath"
	}

	info := MountInfo{
		Name:      fmt.Sprintf("vol-%d", idx),
		Source:    pm.Src,
		MountPath: pm.Dest,
		Type:      volType,
		ReadOnly:  pm.ReadOnly,
		Options:   pm.Options,
		SubPath:   pm.SubPath,
	}

	// Warn here rather than in parseSingleVolume so ValidateMounts + ProcessMounts does not warn twice.
	if volType == "gcsfuse" && len(pm.Attributes) > 0 {
		info.Attributes = pm.Attributes
		warnProfileOnlyAttributes(pm.Attributes, pm.Src)
	}

	return info
}

func normalizeMountPath(p string) string {
	return path.Clean(strings.ReplaceAll(p, "\\", "/"))
}

var reservedDirs = map[string]bool{
	"/dev":   true,
	"/proc":  true,
	"/sys":   true,
	"/etc":   true,
	"/bin":   true,
	"/sbin":  true,
	"/usr":   true,
	"/lib":   true,
	"/lib64": true,
}

func checkReservedSystemPath(cleanPath string, targetName string) error {
	if cleanPath == "/" {
		return fmt.Errorf("%s cannot be the root directory '/'", targetName)
	}
	if reservedDirs[cleanPath] {
		return fmt.Errorf("%s cannot be a reserved system directory (%s)", targetName, cleanPath)
	}
	for reserved := range reservedDirs {
		if strings.HasPrefix(cleanPath, reserved+"/") {
			return fmt.Errorf("%s cannot be within a reserved system directory (%s)", targetName, reserved)
		}
	}
	return nil
}

// ValidateMounts checks mounts for duplicate sources/destinations, reserved system paths, and valid formats.
func (sm *StorageManager) ValidateMounts(mounts []string) error {
	seenSources := make(map[string]bool)
	seenDestinations := make(map[string]bool)

	for _, vStr := range mounts {
		pm, err := sm.parseSingleVolume(vStr)
		if err != nil {
			return err
		}

		cleanDest := normalizeMountPath(pm.Dest)
		if err := checkReservedSystemPath(cleanDest, fmt.Sprintf("mount destination %q", pm.Dest)); err != nil {
			return err
		}

		sourceKey := pm.Src
		if pm.SubPath != "" {
			sourceKey += "/" + pm.SubPath
		}
		if pm.Profile != "" {
			sourceKey += ";profile=" + pm.Profile
		}
		// %v prints map keys in sorted order, so the key is independent of attribute order.
		sourceKey += fmt.Sprintf(";options=%s;attributes=%v", pm.Options, pm.Attributes)

		if seenSources[sourceKey] {
			return fmt.Errorf("duplicate volume source: %s", pm.Src)
		}
		if seenDestinations[cleanDest] {
			return fmt.Errorf("duplicate volume destination: %s", pm.Dest)
		}
		seenSources[sourceKey] = true
		seenDestinations[cleanDest] = true
	}
	return nil
}

// ValidateRamdiskDir checks --gke-mtc-ramdisk-dir for valid path format, root '/' prohibition, reserved system directory prohibition, and conflicts with user mounts.
func (sm *StorageManager) ValidateRamdiskDir(ramdiskDir string, rawMounts []string) error {
	if ramdiskDir == "" {
		return nil
	}
	if !strings.HasPrefix(ramdiskDir, "/") {
		return fmt.Errorf("--gke-mtc-ramdisk-dir must be an absolute path (e.g. /tmp/ramdisk), got: %q", ramdiskDir)
	}
	cleanRamdisk := normalizeMountPath(ramdiskDir)
	if err := checkReservedSystemPath(cleanRamdisk, "--gke-mtc-ramdisk-dir"); err != nil {
		return err
	}
	for _, m := range rawMounts {
		pm, err := sm.parseSingleVolume(m)
		if err != nil {
			return err
		}
		if normalizeMountPath(pm.Dest) == cleanRamdisk {
			return fmt.Errorf("--gke-mtc-ramdisk-dir path %q conflicts with duplicate mount destination in --mount flag", ramdiskDir)
		}
	}
	return nil
}

func missingDestOrFormatErr(vStr string) error {
	if strings.HasPrefix(vStr, "gs://") || strings.HasPrefix(vStr, "filestore://") {
		return fmt.Errorf("invalid volume format: %s. Missing destination.", vStr)
	}
	return fmt.Errorf("invalid volume format: %s. Expected format: <src>;<dest>[;<mode>][;profile=<profile>][;options=<options>][;attributes=<k=v,...>]", vStr)
}

var gcsFuseProfileStorageClasses = map[string]string{
	"training":                 "gcsfusecsi-training",
	"checkpointing":            "gcsfusecsi-checkpointing",
	"serving":                  "gcsfusecsi-serving",
	"gcsfusecsi-training":      "gcsfusecsi-training",
	"gcsfusecsi-checkpointing": "gcsfusecsi-checkpointing",
	"gcsfusecsi-serving":       "gcsfusecsi-serving",
}

func supportedGCSFuseProfiles() []string {
	names := make([]string, 0, len(gcsFuseProfileStorageClasses))
	for name := range gcsFuseProfileStorageClasses {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeProfileName(profile string) (string, error) {
	sc, ok := gcsFuseProfileStorageClasses[strings.ToLower(strings.TrimSpace(profile))]
	if !ok {
		return "", fmt.Errorf("unsupported storage profile %q. Supported values are: %s", profile, strings.Join(supportedGCSFuseProfiles(), ", "))
	}
	return sc, nil
}

var volumeAttributeKeyPattern = regexp.MustCompile(`^` + volumeAttributeKeyCharset + `$`)

// profileOnlyVolumeAttributes override gcsfusecsi-* StorageClass parameters and have no effect on inline mounts.
var profileOnlyVolumeAttributes = map[string]bool{
	"anywhereCacheZones":                    true,
	"anywhereCacheTTL":                      true,
	"anywhereCacheAdmissionPolicy":          true,
	"bucketScanTimeout":                     true,
	"bucketScanResyncPeriod":                true,
	"fuseMemoryAllocatableFactor":           true,
	"fuseEphemeralStorageAllocatableFactor": true,
	"fuseFileCacheMediumPriority":           true,
}

func warnProfileOnlyAttributes(attrs map[string]string, src string) {
	var flagged []string
	for key := range attrs {
		if profileOnlyVolumeAttributes[key] {
			flagged = append(flagged, key)
		}
	}
	if len(flagged) == 0 {
		return
	}
	sort.Strings(flagged)
	logging.Warn("mount %q sets volume attribute(s) %s without profile=. These override storage profile StorageClass parameters and are expected to have no effect on an inline mount. Add profile=<training|checkpointing|serving> if you intended them to apply.",
		src, strings.Join(flagged, ", "))
}

// reservedVolumeAttributes are derived by gcluster from other --mount segments, so
// accepting them from attributes= would give the rendered manifest two sources of
// truth. Keys here MUST stay in sync with those written by buildVolumeSpec.
// Matching is case sensitive, as the GCSFuse CSI driver treats attribute keys.
var reservedVolumeAttributes = map[string]string{
	"mountOptions": "use options=<opt1>,<opt2> instead, which gcluster renders into the correct field for both inline and storage-profile mounts",
	"bucketName":   "the bucket is taken from the mount source, src=gs://<bucket>",
}

// volumeAttributeSeparator splits on ',' only when followed by `<key>=`, allowing comma-separated attribute values.
var volumeAttributeSeparator = regexp.MustCompile(`^[,\s]*(?:` + volumeAttributeKeyCharset + `\s*=|$)`)

func splitVolumeAttributes(raw string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != ',' {
			continue
		}
		if !volumeAttributeSeparator.MatchString(raw[i+1:]) {
			continue
		}
		parts = append(parts, raw[start:i])
		start = i + 1
	}
	return append(parts, raw[start:])
}

func parseVolumeAttributes(raw string) (map[string]string, error) {
	attrs := map[string]string{}
	for _, kv := range splitVolumeAttributes(raw) {
		kv = strings.TrimSpace(kv)
		if kv == "" {
			continue
		}
		pair := strings.SplitN(kv, "=", 2)
		if len(pair) != 2 || strings.TrimSpace(pair[0]) == "" {
			return nil, fmt.Errorf("invalid volume attribute %q. Expected format: attributes=<key>=<value>[,<key>=<value>...]", kv)
		}
		key := strings.TrimSpace(pair[0])
		if !volumeAttributeKeyPattern.MatchString(key) {
			return nil, fmt.Errorf("invalid volume attribute key %q. Keys may only contain letters, digits, '.', '_' and '-'", key)
		}
		if hint, reserved := reservedVolumeAttributes[key]; reserved {
			return nil, fmt.Errorf("volume attribute %q is not accepted; %s", key, hint)
		}
		if _, dup := attrs[key]; dup {
			return nil, fmt.Errorf("duplicate volume attribute key %q", key)
		}
		attrs[key] = strings.TrimSpace(pair[1])
	}
	if len(attrs) == 0 {
		return nil, fmt.Errorf("attributes= was specified but no key=value pairs were provided")
	}
	return attrs, nil
}

func repeatedSegmentErr(segment, vStr, joinHint string) error {
	return fmt.Errorf("%s may only be specified once per --mount, but %q specifies it more than once.%s",
		segment, vStr, joinHint)
}

func parseMountSegments(segments []string, vStr string, pm *parsedMount) (mountSegments, error) {
	var out mountSegments
	optionsSet := false
	attributesSet := false

	for _, part := range segments {
		switch {
		case part == "ro":
			pm.ReadOnly = true
		case part == "rw":
			pm.ReadOnly = false
		case strings.HasPrefix(part, "options="):
			if optionsSet {
				return out, repeatedSegmentErr("options=", vStr,
					" Pass every mount option in one comma separated list instead: options=<opt1>,<opt2>")
			}
			optionsSet = true
			pm.Options = strings.TrimPrefix(part, "options=")
		case strings.HasPrefix(part, "profile="):
			if out.ProfileSet {
				return out, repeatedSegmentErr("profile=", vStr,
					" A mount is backed by exactly one storage profile; use a second --mount to read the same bucket through another profile")
			}
			out.RawProfile = strings.TrimPrefix(part, "profile=")
			out.ProfileSet = true
		case strings.HasPrefix(part, "attributes="):
			if attributesSet {
				return out, repeatedSegmentErr("attributes=", vStr,
					" Pass every attribute in one comma separated list instead: attributes=<k1>=<v1>,<k2>=<v2>")
			}
			attributesSet = true
			attrs, err := parseVolumeAttributes(strings.TrimPrefix(part, "attributes="))
			if err != nil {
				return out, err
			}
			pm.Attributes = attrs
		default:
			return out, fmt.Errorf("invalid volume option or mode: %s", part)
		}
	}

	return out, nil
}

func validateGCSOnlySegments(pm parsedMount, profileSet bool) error {
	if strings.HasPrefix(pm.Src, "gs://") {
		return nil
	}

	var unsupportedSegments []string
	if pm.Options != "" {
		unsupportedSegments = append(unsupportedSegments, "options=")
	}
	if profileSet {
		unsupportedSegments = append(unsupportedSegments, "profile=")
	}
	if len(pm.Attributes) > 0 {
		unsupportedSegments = append(unsupportedSegments, "attributes=")
	}
	if len(unsupportedSegments) == 0 {
		return nil
	}

	return fmt.Errorf("volume source %q is not a GCS bucket; %s can only be used with GCS fuse volumes (gs://...)",
		pm.Src, strings.Join(unsupportedSegments, ", "))
}

func (sm *StorageManager) parseSingleVolume(vStr string) (parsedMount, error) {
	parts := strings.Split(vStr, ";")
	if len(parts) < 2 {
		return parsedMount{}, missingDestOrFormatErr(vStr)
	}

	pm := parsedMount{
		Src:      parts[0],
		Dest:     parts[1],
		ReadOnly: true, // default
	}

	segments, err := parseMountSegments(parts[2:], vStr, &pm)
	if err != nil {
		return parsedMount{}, err
	}

	if pm.Src == "" || pm.Dest == "" || !strings.HasPrefix(pm.Dest, "/") {
		return parsedMount{}, missingDestOrFormatErr(vStr)
	}

	if err := validateGCSOnlySegments(pm, segments.ProfileSet); err != nil {
		return parsedMount{}, err
	}

	if strings.HasPrefix(pm.Src, "gs://") {
		bucket, subPath, err := splitGCSSource(pm.Src)
		if err != nil {
			return parsedMount{}, err
		}
		pm.Src = "gs://" + bucket
		pm.SubPath = subPath
	}

	if segments.ProfileSet {
		sc, err := normalizeProfileName(segments.RawProfile)
		if err != nil {
			return parsedMount{}, err
		}
		pm.Profile = sc
	}

	if err := validateSrcScheme(pm.Src, vStr); err != nil {
		return parsedMount{}, err
	}

	return pm, nil
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

// truncatePVCName enforces maxLen without leaving a trailing '-'.
func truncatePVCName(name string, maxLen int) string {
	if len(name) > maxLen {
		name = strings.TrimRight(name[:maxLen], "-")
	}
	return name
}

func (sm *StorageManager) resolveNamespace(job orchestrator.JobDefinition) (string, error) {
	// Test-only scaffolding: all production callers populate sm.orchestrator.
	if sm.orchestrator == nil {
		return "default", nil
	}
	ns, err := sm.orchestrator.getCurrentNamespace(job.ClusterName, job.ClusterLocation, job.ProjectID)
	if err != nil {
		return "", fmt.Errorf("failed to resolve namespace for storage gateway: %w", err)
	}
	if ns == "" {
		return "", fmt.Errorf("resolved an empty namespace for storage gateway. Specify one explicitly with --gke-namespace")
	}
	return ns, nil
}

func (sm *StorageManager) generateFilestoreResources(pm parsedMount, idx int, job orchestrator.JobDefinition, state *mountBuildState) (MountInfo, string, error) {
	src, dest, readOnly := pm.Src, pm.Dest, pm.ReadOnly
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
	pvcName = truncatePVCName(sanitizePVCName(pvcName), maxGeneratedPVCNameLength)

	ns, err := sm.resolveNamespace(job)
	if err != nil {
		return MountInfo{}, "", err
	}
	pvName := sanitizePVCName(pvcName + "-" + ns)

	info := MountInfo{
		Source:    pvcName,
		MountPath: dest,
		Type:      "pvc",
		ReadOnly:  readOnly,
	}

	name, reused := state.volumeNameFor(pvName, idx)
	info.Name = name
	if reused {
		return info, "", nil
	}

	filestoreTmpl, err := sm.orchestrator.parseGKETextTemplate("filestore.tmpl")
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to parse filestore template: %w", err)
	}

	var buf bytes.Buffer
	err = filestoreTmpl.Execute(&buf, map[string]string{
		"PVName":           pvName,
		"PVCName":          pvcName,
		"Share":            share,
		"IP":               ip,
		"Capacity":         capacityStr,
		"ManagedByLabel":   managedByLabel,
		"ManagedByValue":   managedByValue,
		"StorageTypeLabel": storageTypeLabel,
		"StorageType":      storageTypeFilestore,
	})
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to execute filestore template: %w", err)
	}
	pvYAML := buf.String()
	if msg := unmanagedGatewayWarning(pvName, pvYAML); msg != "" {
		logging.Warn("%s", msg)
	}

	return info, pvYAML, nil
}

var gcsBucketNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

func splitGCSSource(src string) (bucket string, subPath string, err error) {
	trimmed := strings.TrimPrefix(src, "gs://")
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return "", "", fmt.Errorf("invalid GCS mount %q: bucket name is missing. Expected format: gs://<bucket>[/<path>]", src)
	}
	parts := strings.SplitN(trimmed, "/", 2)
	bucket = parts[0]
	if !gcsBucketNamePattern.MatchString(bucket) {
		return "", "", fmt.Errorf("invalid GCS bucket name %q in mount %q", bucket, src)
	}
	if len(parts) > 1 {
		subPath = strings.Trim(parts[1], "/")
	}
	return bucket, subPath, nil
}

// gatewaySpecHash digests the PV spec fields checkExistingGatewayPV compares. Naming gateways by it means any spec
// change (options, attributes, template, capacity, new defaults) yields a new gateway instead of clashing with the
// immutable spec of an existing one.
func gatewaySpecHash(renderedYAML string) (string, error) {
	var pv existingGatewayPV
	if err := yaml.Unmarshal([]byte(renderedYAML), &pv); err != nil {
		return "", err
	}
	spec, err := json.Marshal(pv.Spec)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(spec)
	return hex.EncodeToString(sum[:])[:6], nil
}

// gcsFuseGatewayPVCName builds the deterministic PVC name for a (bucket, profile, spec) gateway.
func gcsFuseGatewayPVCName(bucket, profileShortName, specHash string) string {
	suffix := sanitizePVCName(profileShortName) + "-" + specHash

	prefix := gcsFuseGatewayPrefix + "-"
	budget := maxGeneratedPVCNameLength - len(prefix) - len(suffix) - 1

	name := sanitizePVCName(bucket)
	// Append a digest if sanitization collapsed '.' or '_' so distinct buckets never collide.
	lossy := name != bucket
	if budget > 0 && (lossy || len(name) > budget) {
		name = shortenWithDigest(name, bucket, budget)
	}

	return sanitizePVCName(prefix + name + "-" + suffix)
}

func shortenWithDigest(name, identity string, maxLen int) string {
	sum := sha256.Sum256([]byte(identity))
	digest := hex.EncodeToString(sum[:])[:gatewayNameDigestLength]
	keep := maxLen - len(digest) - 1
	if keep < 1 {
		return digest[:min(len(digest), maxLen)]
	}
	if keep > len(name) {
		keep = len(name)
	}
	return strings.TrimRight(name[:keep], "-") + "-" + digest
}

func splitMountOptions(options string) []string {
	var out []string
	for _, opt := range strings.Split(options, ",") {
		if opt = strings.TrimSpace(opt); opt != "" {
			out = append(out, opt)
		}
	}
	return out
}

// generateGCSFuseProfileResources renders the PV/PVC gateway backing a storage-profile mount.
func (sm *StorageManager) generateGCSFuseProfileResources(pm parsedMount, idx int, job orchestrator.JobDefinition, state *mountBuildState) (MountInfo, string, error) {
	bucket := strings.TrimPrefix(pm.Src, "gs://")

	profileShortName := strings.TrimPrefix(pm.Profile, "gcsfusecsi-")
	ns, err := sm.resolveNamespace(job)
	if err != nil {
		return MountInfo{}, "", err
	}

	tmpl, err := sm.orchestrator.parseGKETextTemplate("gcs_fuse_pv_pvc.tmpl")
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to parse GCSFuse PV/PVC template: %w", err)
	}
	params := GCSFusePVPVCTemplateParams{
		Namespace:        ns,
		StorageClassName: pm.Profile,
		Capacity:         gcsFuseGatewayCapacity,
		VolumeHandle:     bucket,
		MountOptions:     splitMountOptions(pm.Options),
		VolumeAttributes: pm.Attributes,
		ManagedByLabel:   managedByLabel,
		ManagedByValue:   managedByValue,
		StorageTypeLabel: storageTypeLabel,
		StorageType:      storageTypeGCSFuse,
	}

	// Render once without names to derive the spec hash the names are built from.
	var specBuf bytes.Buffer
	if err := tmpl.Execute(&specBuf, params); err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to execute GCSFuse PV/PVC template: %w", err)
	}
	specHash, err := gatewaySpecHash(specBuf.String())
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to parse rendered GCSFuse PV: %w", err)
	}
	pvcName := gcsFuseGatewayPVCName(bucket, profileShortName, specHash)
	pvName := sanitizePVCName(pvcName + "-" + ns)

	info := MountInfo{
		Source:              pvcName,
		MountPath:           pm.Dest,
		Type:                "pvc",
		ReadOnly:            pm.ReadOnly,
		SubPath:             pm.SubPath,
		NeedsGCSFuseSidecar: true,
	}

	name, reused := state.volumeNameFor(pvName, idx)
	info.Name = name
	if reused {
		return info, "", nil
	}

	params.PVName, params.PVCName = pvName, pvcName
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to execute GCSFuse PV/PVC template: %w", err)
	}
	if msg := unmanagedGatewayWarning(pvName, buf.String()); msg != "" {
		logging.Warn("%s", msg)
	}

	if err := sm.checkExistingGatewayPV(pvName, pvcName, ns, buf.String(), job.DryRunManifest != ""); err != nil {
		return MountInfo{}, "", err
	}

	return info, buf.String(), nil
}

func (sm *StorageManager) checkExistingGatewayPV(pvName, pvcName, ns, renderedYAML string, dryRun bool) error {
	if dryRun || sm.orchestrator == nil || sm.orchestrator.executor == nil {
		return nil
	}

	res := sm.orchestrator.executor.ExecuteCommand("kubectl", "get", "pv", pvName, "--ignore-not-found", "-o", "json")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to inspect existing gateway PV %q (needs cluster-scoped `get pv`): %s",
			pvName, strings.TrimSpace(res.Stderr))
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return nil
	}

	var existing, rendered existingGatewayPV
	if err := yaml.Unmarshal([]byte(res.Stdout), &existing); err != nil {
		return fmt.Errorf("failed to parse existing gateway PV %q: %w", pvName, err)
	}
	if err := yaml.Unmarshal([]byte(renderedYAML), &rendered); err != nil {
		return fmt.Errorf("failed to parse rendered gateway PV %q: %w", pvName, err)
	}

	if existing.Metadata.DeletionTimestamp != "" {
		return fmt.Errorf(
			"gateway PV %q is being deleted and is waiting for PVC %s/%s to be released. "+
				"Cancel the jobs that mount it (`kubectl describe pvc %s -n %s` lists them under Used By), "+
				"then run `kubectl delete pvc %s -n %s` and resubmit",
			pvName, ns, pvcName, pvcName, ns, pvcName, ns)
	}

	if existing.Status.Phase == "Released" || existing.Status.Phase == "Failed" {
		return sm.recreateStaleGatewayPV(pvName, existing)
	}

	if !reflect.DeepEqual(existing.Spec, rendered.Spec) {
		return fmt.Errorf(
			"gateway PV %q already exists with different settings (created by an older gcluster version or a custom template). "+
				"It is shared by every job in namespace %q that mounts this bucket/profile. To replace it: cancel those jobs "+
				"(`kubectl describe pvc %s -n %s` lists them under Used By), then run "+
				"`kubectl delete pvc %s -n %s && kubectl delete pv %s`, and resubmit. "+
				"Bucket data is not affected (reclaimPolicy: Retain)",
			pvName, ns, pvcName, ns, pvcName, ns, pvName)
	}
	return nil
}

func (sm *StorageManager) recreateStaleGatewayPV(pvName string, existing existingGatewayPV) error {
	phase := existing.Status.Phase
	if existing.Metadata.Labels[managedByLabel] != managedByValue {
		return fmt.Errorf(
			"gateway PV %q already exists in %s state (left behind after its PVC was deleted) and cannot rebind. "+
				"It is not labelled %s=%s, so gcluster will not delete it. Delete it with `kubectl delete pv %s` and resubmit",
			pvName, phase, managedByLabel, managedByValue, pvName)
	}

	logging.Info("Recreating stale gateway PV %q (%s)", pvName, phase)
	res := sm.orchestrator.executor.ExecuteCommand("kubectl", "delete", "pv", pvName,
		"--ignore-not-found", "--wait=true", "--timeout=60s")
	if res.ExitCode != 0 {
		return fmt.Errorf("failed to delete stale gateway PV %q: %s", pvName, strings.TrimSpace(res.Stderr))
	}
	return nil
}

func unmanagedGatewayWarning(pvName, manifest string) string {
	var pv existingGatewayPV
	if yaml.Unmarshal([]byte(manifest), &pv) != nil || pv.Metadata.Labels[managedByLabel] == managedByValue {
		return ""
	}
	return fmt.Sprintf(
		"gateway PV %q is missing label %s=%s, so gcluster cannot identify it for storage cleanup. "+
			"If you override gateway templates with --gke-custom-templates-path, add the labels to the PV and PVC via "+
			"{{ .ManagedByLabel }}: {{ .ManagedByValue }}",
		pvName, managedByLabel, managedByValue)
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
	var volSpecs []map[string]interface{}
	var mountSpecs []map[string]interface{}
	gcsFuseEnabled := false
	seenVolumes := make(map[string]bool)

	for _, v := range vols {
		mountSpecs = append(mountSpecs, buildVolumeMountSpec(v))
		if !seenVolumes[v.Name] {
			seenVolumes[v.Name] = true
			volSpecs = append(volSpecs, buildVolumeSpec(v))
		}
		if v.Type == "gcsfuse" || v.NeedsGCSFuseSidecar {
			gcsFuseEnabled = true
		}
	}

	if opts.GKEMTCEnabled {
		ramdiskDir := opts.GKEMTCRamdiskDirectory
		mountSpecs = append(mountSpecs,
			map[string]interface{}{"name": "cache", "mountPath": ramdiskDir},
		)
		volSpecs = append(volSpecs,
			map[string]interface{}{"name": "cache", "csi": map[string]interface{}{"driver": multitierCheckpointCSIDriver}},
		)
	}

	if opts.IsPathwaysJob && opts.Pathways.ColocatedPythonSidecarImage != "" {
		mountSpecs = append(mountSpecs, map[string]interface{}{"name": "sidecar-shared-memory", "mountPath": "/tmp/sidecar"})
		volSpecs = append(volSpecs, map[string]interface{}{"name": "sidecar-shared-memory", "emptyDir": map[string]interface{}{"medium": "Memory"}})
	}

	if len(volSpecs) == 0 {
		return
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
	if v.SubPath != "" {
		mountSpec["subPath"] = v.SubPath
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
		// Keys derived here are rejected from attributes= by reservedVolumeAttributes;
		// add any new derived key there too.
		volumeAttributes := map[string]interface{}{
			"bucketName": strings.TrimPrefix(v.Source, "gs://"),
		}
		if v.Options != "" {
			volumeAttributes["mountOptions"] = v.Options
		}
		for key, value := range v.Attributes {
			volumeAttributes[key] = value
		}
		spec["csi"] = map[string]interface{}{
			"driver":           "gcsfuse.csi.storage.gke.io",
			"readOnly":         v.ReadOnly,
			"volumeAttributes": volumeAttributes,
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
	ctx, cancel := context.WithTimeout(context.Background(), storageAPITimeout)
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

var gcsFuseProfileBasePermissions = []string{
	"storage.buckets.get",
	"storage.objects.list",
}

var gcsFuseAnywhereCachePermissions = []string{
	"storage.anywhereCaches.create",
	"storage.anywhereCaches.get",
	"storage.anywhereCaches.list",
	"storage.anywhereCaches.update",
}

var gcsFuseProfileAllPermissions = append(append([]string{}, gcsFuseProfileBasePermissions...), gcsFuseAnywhereCachePermissions...)

var gcsFuseProfileKnownRoles = map[string][]string{
	"roles/owner":                      gcsFuseProfileAllPermissions,
	"roles/editor":                     gcsFuseProfileAllPermissions,
	"roles/storage.admin":              gcsFuseProfileAllPermissions,
	"roles/storage.legacyBucketReader": gcsFuseProfileBasePermissions,
}

var multiRegionRegionPrefixes = map[string][]string{
	"US":   {"us-"},
	"EU":   {"europe-"},
	"ASIA": {"asia-"},
}

var (
	gcpZonePattern   = regexp.MustCompile(`^[a-z]+-[a-z]+[0-9]+-[a-z]$`)
	gcpRegionPattern = regexp.MustCompile(`^[a-z]+-[a-z]+[0-9]+$`)
)

// RunStorageProfilePreflight checks that profile= mounts can be satisfied by the cluster and bucket.
func (sm *StorageManager) RunStorageProfilePreflight(job orchestrator.JobDefinition) error {
	mounts, err := sm.collectProfileMounts(job.RawMounts)
	if err != nil || len(mounts) == 0 {
		return err
	}

	if err := sm.validateStorageClassesExist(mounts, job.DryRunManifest != ""); err != nil {
		return err
	}

	return sm.checkProfileMisconfiguration(job, mounts)
}

func (sm *StorageManager) collectProfileMounts(rawMounts []string) ([]profileMount, error) {
	var out []profileMount
	seen := map[string]bool{}

	for _, vStr := range rawMounts {
		pm, err := sm.parseSingleVolume(vStr)
		if err != nil {
			return nil, err
		}
		if pm.Profile == "" {
			continue
		}
		bucket, _, err := splitGCSSource(pm.Src)
		if err != nil {
			return nil, err
		}

		zones := splitAnywhereCacheZones(pm.Attributes[anywhereCacheZonesAttribute])
		cached := usesAnywhereCache(pm)
		key := fmt.Sprintf("%s|%s|%s|%t", bucket, pm.Profile, strings.Join(zones, ","), cached)
		if seen[key] {
			continue
		}
		seen[key] = true

		out = append(out, profileMount{
			Bucket:             bucket,
			Profile:            pm.Profile,
			AnywhereCacheZones: zones,
			UsesAnywhereCache:  cached,
		})
	}
	return out, nil
}

func usesAnywhereCache(pm parsedMount) bool {
	if strings.EqualFold(strings.TrimSpace(pm.Attributes[anywhereCacheZonesAttribute]), anywhereCacheOffValue) {
		return false
	}
	if pm.Profile == servingProfileStorageClass {
		return true
	}
	for key := range pm.Attributes {
		if strings.HasPrefix(key, anywhereCacheAttributePrefix) {
			return true
		}
	}
	return false
}

func splitAnywhereCacheZones(raw string) []string {
	var zones []string
	for _, z := range strings.Split(raw, ",") {
		z = strings.ToLower(strings.TrimSpace(z))
		if z == "" || z == anywhereCacheAllZonesValue || z == anywhereCacheOffValue {
			continue
		}
		zones = append(zones, z)
	}
	return zones
}

func (sm *StorageManager) validateStorageClassesExist(mounts []profileMount, dryRun bool) error {
	if sm.orchestrator == nil || sm.orchestrator.executor == nil {
		return nil
	}

	checked := map[string]bool{}
	for _, m := range mounts {
		if checked[m.Profile] {
			continue
		}
		checked[m.Profile] = true
		if err := sm.validateStorageClassExists(m.Profile, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func (sm *StorageManager) validateStorageClassExists(storageClass string, dryRun bool) error {
	res := sm.orchestrator.executor.ExecuteCommand("kubectl", "get", "storageclass", storageClass, "--ignore-not-found", "-o", "name")
	if res.ExitCode != 0 {
		logging.Warn("Could not verify that StorageClass %q exists on the cluster: %s. Proceeding with job submission.",
			storageClass, strings.TrimSpace(res.Stderr))
		return nil
	}
	if strings.TrimSpace(res.Stdout) != "" {
		return nil
	}

	err := fmt.Errorf("StorageClass %q was not found on cluster, so the generated PersistentVolumeClaim would stay Pending forever. "+
		"GCSFuse storage profiles require a GKE cluster running %s or later with the Cloud Storage FUSE CSI driver enabled. "+
		"Run 'kubectl get storageclass -l %s' to list the profiles available on this cluster, or drop profile= to use an inline GCSFuse mount. See %s",
		storageClass, minGCSFuseProfileGKEVersion, gcsFuseProfileSelector, gcsFuseProfileDocsURL)
	if dryRun {
		logging.Warn("%v. Writing the manifest anyway because this is a dry run.", err)
		return nil
	}
	return err
}

func (sm *StorageManager) checkProfileMisconfiguration(job orchestrator.JobDefinition, mounts []profileMount) error {
	client, err := sm.getPreflightClient(context.Background())
	if err != nil {
		logging.Warn("Skipping Cloud Storage pre-flight checks for storage profile mounts: %v. Proceeding with job submission.", err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), storageAPITimeout)
	defer cancel()

	if agent, err := gkeServiceAgentEmail(ctx, client, job.ProjectID); err != nil {
		logging.Warn("Skipping the GKE Service Agent IAM pre-flight check: could not resolve the project number for %q: %v. "+
			"Verify manually that the GKE Service Agent holds the required permissions on the target bucket(s). See %s",
			job.ProjectID, err, gcsFuseProfileIAMURL)
	} else {
		warnOnMissingIAM(ctx, client, job.ProjectID, agent, mounts)
	}
	return checkBucketLocations(ctx, client, job.ClusterLocation, mounts, job.DryRunManifest != "")
}

func warnOnMissingIAM(ctx context.Context, client storagePreflightClient, projectID, agent string, mounts []profileMount) {
	resolve := newRoleResolver(client)
	projectGranted := lazyProjectGrants(ctx, client, resolve, projectID, agent)

	buckets, needsCache := bucketCacheUse(mounts)
	for _, bucket := range buckets {
		required := requiredProfilePermissions(needsCache[bucket])
		if msg := bucketIAMWarning(ctx, client, resolve, agent, bucket, required, projectGranted); msg != "" {
			logging.Warn("%s", msg)
		}
	}
}

func bucketCacheUse(mounts []profileMount) ([]string, map[string]bool) {
	var order []string
	needsCache := map[string]bool{}
	for _, m := range mounts {
		if _, seen := needsCache[m.Bucket]; !seen {
			order = append(order, m.Bucket)
		}
		needsCache[m.Bucket] = needsCache[m.Bucket] || m.UsesAnywhereCache
	}
	return order, needsCache
}

func checkBucketLocations(ctx context.Context, client storagePreflightClient, clusterLocation string, mounts []profileMount, dryRun bool) error {
	checked := map[string]bool{}
	for _, m := range mounts {
		key := fmt.Sprintf("%s|%s|%t|%t", m.Bucket, strings.Join(m.AnywhereCacheZones, ","), m.UsesAnywhereCache, colocationRequired(m))
		if checked[key] {
			continue
		}
		checked[key] = true
		if err := checkBucketLocation(ctx, client, clusterLocation, m, dryRun); err != nil {
			return err
		}
	}
	return nil
}

func gkeServiceAgentEmail(ctx context.Context, client storagePreflightClient, projectID string) (string, error) {
	number, err := client.projectNumber(ctx, projectID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf(gkeServiceAgentTemplate, number), nil
}

func requiredProfilePermissions(usesAnywhereCache bool) []string {
	perms := append([]string{}, gcsFuseProfileBasePermissions...)
	if usesAnywhereCache {
		perms = append(perms, gcsFuseAnywhereCachePermissions...)
	}
	sort.Strings(perms)
	return perms
}

// lazyProjectGrants reads project-level grants only when a bucket check needs them.
func lazyProjectGrants(ctx context.Context, client storagePreflightClient, resolve *roleResolver, projectID, agent string) func() map[string]bool {
	return sync.OnceValue(func() map[string]bool {
		bindings, err := client.projectIAMBindings(ctx, projectID)
		if err != nil {
			return nil
		}
		return grantedAgentPermissions(ctx, resolve, bindings, agent)
	})
}

func bucketIAMWarning(ctx context.Context, client storagePreflightClient, resolve *roleResolver, agent, bucket string, required []string, projectGranted func() map[string]bool) string {
	missing, err := bucketIAMShortfall(ctx, client, resolve, agent, bucket, required, projectGranted)
	if len(missing) == 0 {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("Could not read the IAM policy of bucket gs://%s: %v. Verify manually that %s holds %s on the bucket. See %s",
			bucket, err, agent, strings.Join(required, ", "), gcsFuseProfileIAMURL)
	}

	msg := fmt.Sprintf("Storage profile pre-flight: the GKE Service Agent %s does not appear to hold %s on bucket gs://%s. "+
		"Mounting it through a GCSFuse storage profile may fail or its Anywhere Cache may not be provisioned. This check reads "+
		"bucket-level and project-level bindings only, so it cannot see permissions inherited from a folder or organization, "+
		"or granted through a group. Proceeding with job submission. To grant them explicitly see %s",
		agent, strings.Join(missing, ", "), bucket, gcsFuseProfileIAMURL)
	if unreadable := resolve.unreadableRoles(); len(unreadable) > 0 {
		msg += fmt.Sprintf(" The definitions of these roles held by the agent could not be read, so any of the permissions above that they grant were not counted: %s.",
			strings.Join(unreadable, ", "))
	}
	return msg
}

func bucketIAMShortfall(ctx context.Context, client storagePreflightClient, resolve *roleResolver, agent, bucket string, required []string, projectGranted func() map[string]bool) ([]string, error) {
	bindings, err := client.bucketIAMBindings(ctx, bucket)
	if err != nil {
		return missingPermissions(required, projectGranted()), err
	}

	granted := grantedAgentPermissions(ctx, resolve, bindings, agent)
	if len(missingPermissions(required, granted)) == 0 {
		return nil, nil
	}
	maps.Copy(granted, projectGranted())
	return missingPermissions(required, granted), nil
}

func newRoleResolver(client storagePreflightClient) *roleResolver {
	return &roleResolver{client: client, cache: map[string][]string{}, unreadable: map[string]bool{}}
}

// permissions expands a role into its permissions, memoizing successes and failures.
func (r *roleResolver) permissions(ctx context.Context, role string) []string {
	if perms, ok := r.cache[role]; ok {
		return perms
	}
	perms, known := gcsFuseProfileKnownRoles[role]
	if !known {
		var err error
		if perms, err = r.client.rolePermissions(ctx, role); err != nil {
			r.unreadable[role] = true
			perms = nil
		}
	}
	r.cache[role] = perms
	return perms
}

func (r *roleResolver) unreadableRoles() []string {
	return slices.Sorted(maps.Keys(r.unreadable))
}

func grantedAgentPermissions(ctx context.Context, resolve *roleResolver, bindings []iamBinding, agent string) map[string]bool {
	granted := map[string]bool{}
	member := "serviceaccount:" + strings.ToLower(agent)

	for _, b := range bindings {
		if !slices.ContainsFunc(b.Members, func(m string) bool { return strings.ToLower(strings.TrimSpace(m)) == member }) {
			continue
		}
		for _, p := range resolve.permissions(ctx, b.Role) {
			granted[p] = true
		}
	}
	return granted
}

func missingPermissions(required []string, granted map[string]bool) []string {
	var missing []string
	for _, p := range required {
		if !granted[p] {
			missing = append(missing, p)
		}
	}
	return missing
}

// colocationRequired: GKE mandates co-location for serving and whenever Rapid Cache is used.
func colocationRequired(m profileMount) bool {
	return m.Profile == servingProfileStorageClass || m.UsesAnywhereCache
}

// checkBucketLocation blocks on a mismatch when co-location is required (warns on dry run), else warns.
func checkBucketLocation(ctx context.Context, client storagePreflightClient, clusterLocation string, m profileMount, dryRun bool) error {
	loc, err := client.bucketLocation(ctx, m.Bucket)
	if err != nil {
		logging.Warn("Could not read the location of bucket gs://%s: %v. Verify manually that it is in the same region as the cluster. See %s",
			m.Bucket, err, gcsFuseProfileDocsURL)
		return nil
	}
	var zones []string
	if m.UsesAnywhereCache {
		zones = m.AnywhereCacheZones
	}
	for _, region := range cacheRegions(clusterLocation, zones) {
		if regionWithinBucketLocation(region, loc) {
			continue
		}
		if !colocationRequired(m) {
			logging.Warn("Storage profile pre-flight: bucket gs://%s is located in %q but the cluster is in region %q. "+
				"GKE recommends a bucket in the same region for throughput and egress cost. See %s",
				m.Bucket, loc.Location, region, gcsFuseProfileDocsURL)
			continue
		}
		err := fmt.Errorf("storage profile pre-flight: bucket gs://%s is located in %q but mount profile %q would be served from region %q. "+
			"GKE requires the bucket and the cluster to be in the same region for the %s profile and whenever Rapid Cache is enabled. "+
			"Use a bucket in %s, or for a non-serving profile pass attributes=%s=%s to disable Rapid Cache. See %s",
			m.Bucket, loc.Location, m.Profile, region, servingProfileStorageClass, region, anywhereCacheZonesAttribute, anywhereCacheOffValue, gcsFuseProfileDocsURL)
		if !dryRun {
			return err
		}
		logging.Warn("%v. Writing the manifest anyway because this is a dry run.", err)
	}
	return nil
}

func cacheRegions(clusterLocation string, zones []string) []string {
	if len(zones) == 0 {
		if region := normalizeToRegion(clusterLocation); region != "" {
			return []string{region}
		}
		return nil
	}

	var regions []string
	seen := map[string]bool{}
	for _, z := range zones {
		region := normalizeToRegion(z)
		if region == "" || seen[region] {
			continue
		}
		seen[region] = true
		regions = append(regions, region)
	}
	return regions
}

func normalizeToRegion(location string) string {
	loc := strings.ToLower(strings.TrimSpace(location))
	if gcpZonePattern.MatchString(loc) {
		return loc[:strings.LastIndex(loc, "-")]
	}
	return loc
}

// regionWithinBucketLocation returns true when the relationship cannot be determined.
func regionWithinBucketLocation(region string, loc bucketLocation) bool {
	bucketLoc := strings.ToUpper(strings.TrimSpace(loc.Location))
	if region == "" || bucketLoc == "" {
		return true
	}

	if len(loc.DataLocations) > 0 {
		return slices.ContainsFunc(loc.DataLocations, func(l string) bool { return strings.EqualFold(strings.TrimSpace(l), region) })
	}

	if prefixes, ok := multiRegionRegionPrefixes[bucketLoc]; ok {
		return slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(region, p) })
	}

	isRegional := gcpRegionPattern.MatchString(strings.ToLower(bucketLoc))
	if loc.LocationType != "" {
		isRegional = strings.EqualFold(loc.LocationType, bucketRegionLocationType)
	}
	if isRegional {
		return strings.EqualFold(bucketLoc, region)
	}
	return true
}

func (sm *StorageManager) getPreflightClient(ctx context.Context) (storagePreflightClient, error) {
	if sm.preflightClient != nil {
		return sm.preflightClient, nil
	}
	return newGCPPreflightClient(ctx)
}

func newGCPPreflightClient(ctx context.Context) (storagePreflightClient, error) {
	storageSvc, err := gcs.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Cloud Storage client: %w", err)
	}
	iamSvc, err := iamapi.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create IAM client: %w", err)
	}
	crmSvc, err := crm.NewService(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create Resource Manager client: %w", err)
	}
	return &gcpPreflightClient{storage: storageSvc, iam: iamSvc, crm: crmSvc}, nil
}

func (c *gcpPreflightClient) projectNumber(ctx context.Context, projectID string) (int64, error) {
	project, err := c.crm.Projects.Get(projectID).Context(ctx).Do()
	if err != nil {
		return 0, err
	}
	return project.ProjectNumber, nil
}

func (c *gcpPreflightClient) projectIAMBindings(ctx context.Context, projectID string) ([]iamBinding, error) {
	req := &crm.GetIamPolicyRequest{Options: &crm.GetPolicyOptions{RequestedPolicyVersion: 3}}
	policy, err := c.crm.Projects.GetIamPolicy(projectID, req).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	bindings := make([]iamBinding, 0, len(policy.Bindings))
	for _, b := range policy.Bindings {
		bindings = append(bindings, iamBinding{Role: b.Role, Members: b.Members})
	}
	return bindings, nil
}

func (c *gcpPreflightClient) bucketIAMBindings(ctx context.Context, bucket string) ([]iamBinding, error) {
	policy, err := c.storage.Buckets.GetIamPolicy(bucket).OptionsRequestedPolicyVersion(3).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	bindings := make([]iamBinding, 0, len(policy.Bindings))
	for _, b := range policy.Bindings {
		bindings = append(bindings, iamBinding{Role: b.Role, Members: b.Members})
	}
	return bindings, nil
}

func (c *gcpPreflightClient) bucketLocation(ctx context.Context, bucket string) (bucketLocation, error) {
	b, err := c.storage.Buckets.Get(bucket).Context(ctx).Do()
	if err != nil {
		return bucketLocation{}, err
	}
	loc := bucketLocation{Location: b.Location, LocationType: b.LocationType}
	if b.CustomPlacementConfig != nil {
		loc.DataLocations = b.CustomPlacementConfig.DataLocations
	}
	return loc, nil
}

func (c *gcpPreflightClient) rolePermissions(ctx context.Context, role string) ([]string, error) {
	switch {
	case strings.HasPrefix(role, "projects/"):
		r, err := c.iam.Projects.Roles.Get(role).Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		return r.IncludedPermissions, nil
	case strings.HasPrefix(role, "organizations/"):
		r, err := c.iam.Organizations.Roles.Get(role).Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		return r.IncludedPermissions, nil
	case strings.HasPrefix(role, "roles/"):
		r, err := c.iam.Roles.Get(role).Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		return r.IncludedPermissions, nil
	}
	return nil, fmt.Errorf("unsupported role name %q", role)
}
