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
	"fmt"
	"net"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/orchestrator"

	filestore "cloud.google.com/go/filestore/apiv1"
	"cloud.google.com/go/filestore/apiv1/filestorepb"
	"google.golang.org/api/iterator"

	"gopkg.in/yaml.v2"
)

const (
	// Excludes '/' deliberately: it blocks the kubelet-reserved
	// csi.storage.k8s.io/* volume attributes, such as serviceAccount.name.
	volumeAttributeKeyCharset = `[A-Za-z0-9][A-Za-z0-9._-]*`
	maxGeneratedPVCNameLength = 189

	gcsFuseGatewayPrefix   = "gcluster-gcsfuse"
	gcsFuseGatewayCapacity = "5Gi" // Ignored by GCSFuse CSI driver, required by Kubernetes.

	gatewayNameDigestLength = 10
)

func newMountBuildState() *mountBuildState {
	return &mountBuildState{gatewayVolumeNames: map[string]string{}}
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
		if pm.Profile != "" {
			sourceKey = pm.Src + ";profile=" + pm.Profile
		}

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
	if pm.Options != "" {
		return fmt.Errorf("options= is currently only supported for GCS fuse volumes (gs://...)")
	}
	if profileSet {
		return fmt.Errorf("profile= is only supported for GCS fuse volumes (gs://...)")
	}
	if len(pm.Attributes) > 0 {
		return fmt.Errorf("attributes= is only supported for GCS fuse volumes (gs://...)")
	}
	return nil
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

	if segments.ProfileSet {
		sc, err := normalizeProfileName(segments.RawProfile)
		if err != nil {
			return parsedMount{}, err
		}
		pm.Profile = sc
		if _, _, err := splitGCSSource(pm.Src); err != nil {
			return parsedMount{}, err
		}
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
	pvName := sanitizePVCName(fmt.Sprintf("%s-%s", pvcName, ns))

	info := MountInfo{
		Source:    pvcName,
		MountPath: dest,
		Type:      "pvc",
		ReadOnly:  readOnly,
	}

	if state != nil {
		if existing, ok := state.gatewayVolumeNames[pvName]; ok {
			info.Name = existing
			return info, "", nil
		}
	}
	info.Name = fmt.Sprintf("vol-%d", idx)
	if state != nil {
		state.gatewayVolumeNames[pvName] = info.Name
	}

	filestoreTmpl, err := sm.orchestrator.parseGKETextTemplate("filestore.tmpl")
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to parse filestore template: %w", err)
	}

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

// customGatewayOptionsHash returns a deterministic suffix for custom options/attributes, or "" when none are supplied.
func customGatewayOptionsHash(options string, attrs map[string]string) string {
	if options == "" && len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	sb.WriteString(options)
	for _, k := range keys {
		sb.WriteString("\x00")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(attrs[k])
	}
	sum := sha256.Sum256([]byte(sb.String()))
	return hex.EncodeToString(sum[:])[:6]
}

// gcsFuseGatewayPVCName builds the deterministic PVC name for a (bucket, profile) gateway.
func gcsFuseGatewayPVCName(bucket, profileShortName, options string, attrs map[string]string) string {
	suffix := sanitizePVCName(profileShortName)
	if hash := customGatewayOptionsHash(options, attrs); hash != "" {
		suffix += "-" + hash
	}

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

func gcsFuseGatewayPVName(claim, namespace string) string {
	return sanitizePVCName(fmt.Sprintf("%s-%s", claim, namespace))
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
	bucket, subPath, err := splitGCSSource(pm.Src)
	if err != nil {
		return MountInfo{}, "", err
	}

	profileShortName := strings.TrimPrefix(pm.Profile, "gcsfusecsi-")
	pvcName := gcsFuseGatewayPVCName(bucket, profileShortName, pm.Options, pm.Attributes)
	ns, err := sm.resolveNamespace(job)
	if err != nil {
		return MountInfo{}, "", err
	}
	pvName := gcsFuseGatewayPVName(pvcName, ns)

	info := MountInfo{
		Source:              pvcName,
		MountPath:           pm.Dest,
		Type:                "pvc",
		ReadOnly:            pm.ReadOnly,
		SubPath:             subPath,
		NeedsGCSFuseSidecar: true,
	}

	if existing, ok := state.gatewayVolumeNames[pvName]; ok {
		info.Name = existing
		return info, "", nil
	}
	info.Name = fmt.Sprintf("vol-%d", idx)
	state.gatewayVolumeNames[pvName] = info.Name

	tmpl, err := sm.orchestrator.parseGKETextTemplate("gcs_fuse_pv_pvc.tmpl")
	if err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to parse GCSFuse PV/PVC template: %w", err)
	}

	params := GCSFusePVPVCTemplateParams{
		PVName:           pvName,
		PVCName:          pvcName,
		Namespace:        ns,
		StorageClassName: pm.Profile,
		Capacity:         gcsFuseGatewayCapacity,
		VolumeHandle:     bucket,
		MountOptions:     splitMountOptions(pm.Options),
		VolumeAttributes: pm.Attributes,
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return MountInfo{}, "", fmt.Errorf("failed to execute GCSFuse PV/PVC template: %w", err)
	}

	return info, buf.String(), nil
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
