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

package imagebuilder

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/shell"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/compression"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/moby/patternmatcher"
	"github.com/moby/patternmatcher/ignorefile"
)

var (
	cranePull       = crane.Pull
	cranePush       = crane.Push
	appendLayers    = mutate.AppendLayers
	layerFromOpener = tarball.LayerFromOpener
	defaultKeychain = authn.DefaultKeychain
)

// InsecureRegistryEnvVar enables HTTP and unverified TLS connections to OCI registries when set to true.
const InsecureRegistryEnvVar = "GCLUSTER_INSECURE_REGISTRY"

// defaultContainerWorkdir is set when a base image has no WORKDIR or uses "/".
const defaultContainerWorkdir = "/workspace"

// ErrMissingImageRepo indicates that GCLUSTER_IMAGE_REPO is unset and cannot be auto-derived from --base-image.
var ErrMissingImageRepo = errors.New("GCLUSTER_IMAGE_REPO environment variable is required")

var knownPublicRegistries = map[string]bool{
	"docker.io":            true,
	"index.docker.io":      true,
	"registry-1.docker.io": true,
	"ghcr.io":              true,
	"quay.io":              true,
	"nvcr.io":              true,
	"mcr.microsoft.com":    true,
	"public.ecr.aws":       true,
	"registry.k8s.io":      true,
	"k8s.gcr.io":           true,
}

// DestinationRepo describes the resolved OCI destination repository for a Crane build.
type DestinationRepo struct {
	RegistryHost string
	RepoPrefix   string
	GARProject   string
	IsGAR        bool
}

// DockerPlatform represents the target platform for a Docker image.
type DockerPlatform string

const (
	// LinuxAMD64 is the linux/amd64 container platform.
	LinuxAMD64 DockerPlatform = "linux/amd64"
	// LinuxARM64 is the linux/arm64 container platform.
	LinuxARM64 DockerPlatform = "linux/arm64"
)

// NormalizeRegistryHost normalizes an OCI registry host[:port] or bracketed [IPv6][:port] into a valid RFC 3986 authority.
func NormalizeRegistryHost(raw string) (string, error) {
	s := stripRegistryScheme(strings.TrimSpace(raw))
	authority, _, _ := strings.Cut(s, "/")
	if authority == "" {
		return "", fmt.Errorf("registry host cannot be empty")
	}
	normalized, err := normalizeAuthorityHostPort(authority)
	if err != nil {
		return "", err
	}
	u, err := url.Parse("//" + normalized)
	if err != nil || u.Host != normalized || strings.ContainsAny(normalized, " \t\r\n@?#") {
		return "", fmt.Errorf("invalid registry authority %q", raw)
	}
	return normalized, nil
}

func stripRegistryScheme(s string) string {
	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "https://"):
		return s[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		return s[len("http://"):]
	default:
		return s
	}
}

func normalizeAuthorityHostPort(authority string) (string, error) {
	if strings.HasPrefix(authority, "[") {
		if strings.HasSuffix(authority, "]") {
			inner := strings.Trim(authority, "[]")
			if net.ParseIP(inner) == nil {
				return "", fmt.Errorf("invalid IPv6 registry host %q", authority)
			}
			return "[" + strings.ToLower(inner) + "]", nil
		}
		h, p, err := net.SplitHostPort(authority)
		if err != nil || net.ParseIP(h) == nil || p == "" {
			return "", fmt.Errorf("invalid bracketed registry host:port %q", authority)
		}
		return net.JoinHostPort(strings.ToLower(h), p), nil
	}
	if ip := net.ParseIP(authority); ip != nil && strings.Contains(authority, ":") {
		return "[" + strings.ToLower(authority) + "]", nil
	}
	if strings.Contains(authority, ":") {
		h, p, err := net.SplitHostPort(authority)
		if err != nil || h == "" || p == "" {
			return "", fmt.Errorf("invalid registry host:port %q", authority)
		}
		return net.JoinHostPort(strings.ToLower(h), p), nil
	}
	return strings.ToLower(authority), nil
}

func registryBareHost(raw string) string {
	norm, err := NormalizeRegistryHost(raw)
	if err != nil {
		norm = strings.ToLower(strings.TrimSpace(raw))
	}
	if h, _, splitErr := net.SplitHostPort(norm); splitErr == nil {
		return h
	}
	return strings.Trim(norm, "[]")
}

// IsGoogleArtifactRegistry reports whether host targets Google Artifact Registry or Container Registry.
func IsGoogleArtifactRegistry(host string) bool {
	bare := registryBareHost(host)
	return bare == "gcr.io" || strings.HasSuffix(bare, ".gcr.io") || strings.HasSuffix(bare, ".pkg.dev")
}

// IsPrivateOrLoopbackHost reports whether host is localhost, a loopback IP, or an RFC 1918 / RFC 4193 private IP.
func IsPrivateOrLoopbackHost(host string) bool {
	bare := registryBareHost(host)
	if bare == "localhost" || strings.HasSuffix(bare, ".localhost") {
		return true
	}
	if ip := net.ParseIP(bare); ip != nil {
		return ip.IsLoopback() || ip.IsPrivate()
	}
	return false
}

// IsInsecureRegistryEnv reports whether GCLUSTER_INSECURE_REGISTRY is enabled via getenv.
func IsInsecureRegistryEnv(getenv func(string) string) bool {
	if getenv == nil {
		getenv = os.Getenv
	}
	switch strings.ToLower(strings.TrimSpace(getenv(InsecureRegistryEnvVar))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func explicitImageHost(imageRef string) (string, bool) {
	withoutDigest, _, _ := strings.Cut(strings.TrimSpace(imageRef), "@")
	first, rest, hasSlash := strings.Cut(withoutDigest, "/")
	if !hasSlash || rest == "" {
		return "", false
	}
	if first != "localhost" && !strings.ContainsAny(first, ".:") {
		return "", false
	}
	norm, err := NormalizeRegistryHost(first)
	if err != nil {
		return "", false
	}
	return norm, true
}

func isPublicRegistry(host string) bool {
	bare := registryBareHost(host)
	return knownPublicRegistries[bare] || strings.HasSuffix(bare, ".docker.io")
}

// ResolveDestinationRepo resolves the target OCI repository prefix from GCLUSTER_IMAGE_REPO or a private/local --base-image.
func ResolveDestinationRepo(project, location, baseImage, envImageRepo string) (DestinationRepo, error) {
	trimmedRepo := strings.Trim(stripRegistryScheme(strings.TrimSpace(envImageRepo)), "/")
	if trimmedRepo != "" {
		return resolveExplicitRepo(project, location, trimmedRepo)
	}
	host, ok := explicitImageHost(baseImage)
	if ok && !IsGoogleArtifactRegistry(host) && !isPublicRegistry(host) {
		return DestinationRepo{
			RegistryHost: host,
			RepoPrefix:   host + "/gcluster",
			IsGAR:        false,
		}, nil
	}
	return DestinationRepo{}, fmt.Errorf("%w. Please set it in your environment (e.g., export GCLUSTER_IMAGE_REPO=<repo>)", ErrMissingImageRepo)
}

func parseExplicitGARProject(repoPath string) (string, bool) {
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	if strings.Contains(parts[0], ".") {
		if len(parts) < 3 || parts[2] == "" {
			return "", false
		}
		return parts[0] + ":" + parts[1], true
	}
	return parts[0], true
}

func resolveExplicitRepo(project, location, trimmedRepo string) (DestinationRepo, error) {
	first, rest, hasSlash := strings.Cut(trimmedRepo, "/")
	if hasSlash || first == "localhost" || strings.ContainsAny(first, ".:") {
		host, err := NormalizeRegistryHost(first)
		if err != nil {
			return DestinationRepo{}, err
		}
		repoPath := strings.ToLower(strings.Trim(rest, "/"))
		if IsGoogleArtifactRegistry(host) {
			garProj, ok := parseExplicitGARProject(repoPath)
			if !ok {
				return DestinationRepo{}, fmt.Errorf("GCLUSTER_IMAGE_REPO %q must include <project>/<repository> when specifying an Artifact Registry host", trimmedRepo)
			}
			return DestinationRepo{RegistryHost: host, RepoPrefix: host + "/" + repoPath, GARProject: garProj, IsGAR: true}, nil
		}
		if repoPath == "" {
			repoPath = "gcluster"
		}
		return DestinationRepo{RegistryHost: host, RepoPrefix: host + "/" + repoPath, IsGAR: false}, nil
	}
	if strings.TrimSpace(project) == "" || strings.TrimSpace(location) == "" {
		return DestinationRepo{}, fmt.Errorf("GCLUSTER_IMAGE_REPO=%q is a bare Artifact Registry repository name and requires --project and --location (or set GCLUSTER_IMAGE_REPO=<registry-host>/<repository> for a non-Artifact Registry OCI registry)", trimmedRepo)
	}
	host := shell.ExtractRegion(location) + "-docker.pkg.dev"
	// Domain-scoped GCP project IDs (example.com:my-proj) map ":" to "/" in Artifact Registry paths.
	garProject := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(project)), ":", "/")
	return DestinationRepo{
		RegistryHost: host,
		RepoPrefix:   fmt.Sprintf("%s/%s/%s", host, garProject, strings.ToLower(trimmedRepo)),
		GARProject:   strings.TrimSpace(project),
		IsGAR:        true,
	}, nil
}

// BuildContainerImageFromBaseImage packages scriptDir onto baseImage aligned to its WORKDIR and pushes the built image.
func BuildContainerImageFromBaseImage(
	project string,
	location string,
	namespace string,
	workloadName string,
	baseImage string,
	scriptDir string,
	platformStr string,
	ignoreMatcher *patternmatcher.PatternMatcher,
) (string, error) {
	platform, err := parsePlatform(platformStr)
	if err != nil {
		return "", err
	}

	nameOpts := nameOptsForRef(baseImage)
	baseRef, err := name.ParseReference(baseImage, nameOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to parse base image reference %q: %w", baseImage, err)
	}

	// Validate destination configuration and OCI reference syntax before pulling the base image or walking the build context.
	preflightName, err := GenerateImageName(project, location, namespace, workloadName, baseImage, "preflight0000")
	if err != nil {
		return "", err
	}
	if _, err := name.ParseReference(preflightName, nameOptsForRef(preflightName)...); err != nil {
		return "", fmt.Errorf("invalid destination image reference %q: %w", preflightName, err)
	}

	logging.Info("Base Image: %s", baseImage)
	logging.Info("Script Directory: %s", scriptDir)
	logging.Info("Target Platform: %s/%s", platform.OS, platform.Architecture)

	baseImg, err := cranePull(baseRef.String(), craneOptionsForRef(baseRef.String(), &platform)...)
	if err != nil {
		return "", fmt.Errorf("failed to pull base image %q: %w", baseImage, err)
	}

	tarPrefix, setDefaultWorkdir, err := resolveWorkdirPrefix(baseImg)
	if err != nil {
		return "", err
	}

	tempTarballPath, contentHash, err := createFilteredTar(scriptDir, tarPrefix, ignoreMatcher)
	if err != nil {
		return "", fmt.Errorf("failed to create filtered tarball: %w", err)
	}
	defer func() {
		if tempTarballPath != "" {
			_ = os.Remove(tempTarballPath)
		}
	}()

	imageName, err := GenerateImageName(project, location, namespace, workloadName, baseImage, mixBuildHash(baseImage, platformStr, contentHash))
	if err != nil {
		return "", err
	}
	logging.Info("Starting image build process for %s", imageName)

	newImg, err := appendLayerAndWorkdir(baseImg, tempTarballPath, setDefaultWorkdir)
	if err != nil {
		return "", err
	}

	imageRef, err := name.ParseReference(imageName, nameOptsForRef(imageName)...)
	if err != nil {
		return "", fmt.Errorf("failed to parse new image reference %q: %w", imageName, err)
	}

	logging.Info("Uploading Container Image to %s", imageName)
	if err := cranePush(newImg, imageRef.String(), craneOptionsForRef(imageRef.String(), &platform)...); err != nil {
		return "", fmt.Errorf("failed to push image %q: %w", imageName, err)
	}

	logging.Info("Image %s built and uploaded successfully.", imageName)
	return imageName, nil
}

func resolveWorkdirPrefix(baseImg v1.Image) (tarPrefix string, setDefaultWorkdir bool, err error) {
	if baseImg == nil {
		return strings.TrimPrefix(defaultContainerWorkdir, "/") + "/", false, nil
	}
	cf, err := baseImg.ConfigFile()
	if err != nil {
		return "", false, fmt.Errorf("failed to read base image config: %w", err)
	}
	rawWD := ""
	if cf != nil {
		rawWD = strings.TrimSpace(cf.Config.WorkingDir)
	}
	if rawWD != "" {
		// Container paths are strictly POSIX "/"; path.Clean avoids Windows backslash corruption.
		cleaned := path.Clean(strings.ReplaceAll(rawWD, "\\", "/"))
		if cleaned != "/" && cleaned != "." {
			return strings.Trim(cleaned, "/") + "/", false, nil
		}
	}
	return strings.TrimPrefix(defaultContainerWorkdir, "/") + "/", true, nil
}

func appendLayerAndWorkdir(baseImg v1.Image, tempTarballPath string, setDefaultWorkdir bool) (v1.Image, error) {
	tarLayer, err := layerFromOpener(func() (io.ReadCloser, error) {
		file, openErr := os.Open(tempTarballPath)
		if openErr != nil {
			return nil, fmt.Errorf("failed to open temporary tarball %q: %w", tempTarballPath, openErr)
		}
		return file, nil
	}, tarball.WithCompression(compression.GZip))
	if err != nil {
		return nil, fmt.Errorf("failed to create layer from tarball: %w", err)
	}

	newImg, err := appendLayers(baseImg, tarLayer)
	if err != nil {
		return nil, fmt.Errorf("failed to append layer: %w", err)
	}
	if !setDefaultWorkdir || newImg == nil {
		return newImg, nil
	}
	cf, err := newImg.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image config for WORKDIR update: %w", err)
	}
	cfg := cf.Config.DeepCopy()
	cfg.WorkingDir = defaultContainerWorkdir
	updatedImg, err := mutate.Config(newImg, *cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to set default WORKDIR %s on image: %w", defaultContainerWorkdir, err)
	}
	return updatedImg, nil
}

func nameOptsForRef(ref string) []name.Option {
	if IsInsecureRegistryEnv(os.Getenv) {
		return []name.Option{name.Insecure}
	}
	if host, ok := explicitImageHost(ref); ok && IsPrivateOrLoopbackHost(host) {
		return []name.Option{name.Insecure}
	}
	return nil
}

func craneOptionsForRef(ref string, platform *v1.Platform) []crane.Option {
	opts := []crane.Option{
		crane.WithPlatform(platform),
		crane.WithAuthFromKeychain(defaultKeychain),
	}
	if IsInsecureRegistryEnv(os.Getenv) {
		return append(opts, crane.Insecure)
	}
	if host, ok := explicitImageHost(ref); ok && IsPrivateOrLoopbackHost(host) {
		// Enable HTTP fallback for private/loopback IPs (including IPv6 ULA) without disabling TLS verification on HTTPS.
		opts = append(opts, func(o *crane.Options) {
			o.Name = append(o.Name, name.Insecure)
		})
	}
	return opts
}

// GenerateImageName constructs the destination container image URI for either Artifact Registry or a non-GAR OCI registry.
func GenerateImageName(project, location, namespace, workloadName, baseImage, contentHash string) (string, error) {
	dest, err := ResolveDestinationRepo(project, location, baseImage, os.Getenv("GCLUSTER_IMAGE_REPO"))
	if err != nil {
		return "", err
	}
	if dest.IsGAR {
		return generateGARImageName(dest.RepoPrefix)
	}
	nsSeg := sanitizeRepoSegment(namespace, "default")
	wlSeg := sanitizeRepoSegment(workloadName, "job")
	tag := strings.ToLower(strings.TrimSpace(contentHash))
	if tag == "" {
		tag = mixBuildHash(baseImage, nsSeg+"/"+wlSeg, "")
	} else if len(tag) > 12 {
		tag = tag[:12]
	}
	return fmt.Sprintf("%s/%s/%s:%s", dest.RepoPrefix, nsSeg, wlSeg, tag), nil
}

func generateGARImageName(repoPrefix string) (string, error) {
	userName := os.Getenv("USER")
	if userName == "" {
		userName = os.Getenv("USERNAME")
	}
	if userName == "" {
		return "", fmt.Errorf("failed to determine user identity from environment (tried USER and USERNAME)")
	}
	tagRandomPrefix, err := shell.RandomString(4)
	if err != nil {
		return "", fmt.Errorf("failed to generate random prefix for image tag: %w", err)
	}
	tagDatetime := time.Now().Format("2006-01-02-15-04-05")
	return fmt.Sprintf("%s/%s-runner:%s-%s", repoPrefix, strings.ToLower(userName), tagRandomPrefix, tagDatetime), nil
}

func sanitizeRepoSegment(raw, fallback string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	cleaned := strings.Trim(b.String(), "._-")
	if cleaned == "" {
		return fallback
	}
	return cleaned
}

func mixBuildHash(baseImage, platformStr, contentHash string) string {
	sum := sha256.Sum256([]byte(baseImage + "|" + platformStr + "|" + contentHash))
	return hex.EncodeToString(sum[:])[:12]
}

// parsePlatform converts a platform string (e.g., "linux/amd64") into a v1.Platform struct.
func parsePlatform(platformStr string) (v1.Platform, error) {
	parts := strings.Split(platformStr, "/")
	if len(parts) != 2 {
		return v1.Platform{}, fmt.Errorf("invalid platform format: %q, expected \"os/arch\"", platformStr)
	}
	return v1.Platform{
		OS:           parts[0],
		Architecture: parts[1],
	}, nil
}

// ReadDockerignorePatterns reads .dockerignore rules from dir merged with defaultPatterns.
func ReadDockerignorePatterns(dir string, defaultPatterns []string) (*patternmatcher.PatternMatcher, error) {
	dockerignorePath := filepath.Join(dir, ".dockerignore")

	patterns := make([]string, len(defaultPatterns))
	copy(patterns, defaultPatterns)

	if _, err := os.Stat(dockerignorePath); err == nil {
		file, err := os.Open(dockerignorePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open .dockerignore file %q: %w", dockerignorePath, err)
		}
		defer file.Close()

		filePatterns, err := ignorefile.ReadAll(file)
		if err != nil {
			return nil, fmt.Errorf("failed to read .dockerignore file %q: %w", dockerignorePath, err)
		}
		patterns = append(patterns, filePatterns...)
		logging.Info("Found %d patterns in .dockerignore at %q", len(filePatterns), dockerignorePath)
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to stat .dockerignore file %q: %w", dockerignorePath, err)
	}

	matcher, err := patternmatcher.New(patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to create pattern matcher: %w", err)
	}
	return matcher, nil
}

func isPathIgnored(relPath string, d fs.DirEntry, matcher *patternmatcher.PatternMatcher) (bool, error) {
	if matcher == nil {
		return false, nil
	}
	relPathSlash := filepath.ToSlash(relPath)
	if d.IsDir() && !strings.HasSuffix(relPathSlash, "/") {
		relPathSlash += "/"
	}

	ignored, err := matcher.MatchesOrParentMatches(relPathSlash)
	if err != nil {
		return false, fmt.Errorf("failed to check ignore patterns: %w", err)
	}
	return ignored, nil
}

func writeFileContentWithHash(tarWriter *tar.Writer, hasher hash.Hash, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", filePath, err)
	}
	defer file.Close()

	dst := io.Writer(tarWriter)
	if hasher != nil {
		dst = io.MultiWriter(tarWriter, hasher)
	}
	if _, err := io.Copy(dst, file); err != nil {
		return fmt.Errorf("failed to write file content for %q: %w", filePath, err)
	}
	return nil
}

func buildTarHeader(entryPath, relPath, tarPrefix string, d fs.DirEntry) (*tar.Header, fs.FileInfo, string, error) {
	info, err := d.Info()
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to get info for %q: %w", entryPath, err)
	}
	var linkTarget string
	if info.Mode()&os.ModeSymlink != 0 {
		if linkTarget, err = os.Readlink(entryPath); err != nil {
			return nil, nil, "", fmt.Errorf("failed to read link for %q: %w", entryPath, err)
		}
	}
	header, err := tar.FileInfoHeader(info, linkTarget)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create tar header for %q: %w", entryPath, err)
	}
	// Normalize timestamps, ownership, and mode bits so tar digests and content hashes are reproducible across OSes and mtimes.
	header.ModTime = time.Time{}
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}
	header.Uid = 0
	header.Gid = 0
	header.Uname = ""
	header.Gname = ""
	if info.Mode()&os.ModeSymlink == 0 {
		if d.IsDir() || info.Mode()&0111 != 0 {
			header.Mode = 0755
		} else {
			header.Mode = 0644
		}
	}
	header.Name = tarPrefix + strings.TrimPrefix(filepath.ToSlash(relPath), "/")
	return header, info, linkTarget, nil
}

func processTarEntry(tarWriter *tar.Writer, hasher hash.Hash, sourceDir, tarPrefix string, ignoreMatcher *patternmatcher.PatternMatcher, entryPath string, d fs.DirEntry, errFromWalk error) error {
	if errFromWalk != nil {
		return errFromWalk
	}
	relPath, err := filepath.Rel(sourceDir, entryPath)
	if err != nil || relPath == "." {
		return err
	}
	ignored, err := isPathIgnored(relPath, d, ignoreMatcher)
	if err != nil || ignored {
		if ignored && d.IsDir() {
			return filepath.SkipDir
		}
		return err
	}
	header, info, linkTarget, err := buildTarHeader(entryPath, relPath, tarPrefix, d)
	if err != nil {
		return err
	}
	if hasher != nil {
		_, _ = fmt.Fprintf(hasher, "%s|%o|%s\n", header.Name, header.Mode, linkTarget)
	}
	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for %q: %w", entryPath, err)
	}
	if info.Mode().IsRegular() {
		return writeFileContentWithHash(tarWriter, hasher, entryPath)
	}
	return nil
}

func createFilteredTar(sourceDir, tarPrefix string, ignoreMatcher *patternmatcher.PatternMatcher) (tarPath string, contentHash string, err error) {
	tmpFile, tmpErr := os.CreateTemp("", "gcluster-build-context-*.tar.gz")
	if tmpErr != nil {
		return "", "", fmt.Errorf("failed to create temporary file for tarball: %w", tmpErr)
	}
	tmpName := tmpFile.Name()
	closed := false
	defer func() {
		if !closed {
			_ = tmpFile.Close()
		}
		if err != nil {
			_ = os.Remove(tmpName)
			tarPath = ""
			contentHash = ""
		}
	}()

	gzipWriter := gzip.NewWriter(tmpFile)
	tarWriter := tar.NewWriter(gzipWriter)
	hasher := sha256.New()

	logging.Info("Creating filtered tar from %s to temporary file %s", sourceDir, tmpName)

	if err = filepath.WalkDir(sourceDir, func(entryPath string, d fs.DirEntry, walkDirErr error) error {
		return processTarEntry(tarWriter, hasher, sourceDir, tarPrefix, ignoreMatcher, entryPath, d, walkDirErr)
	}); err != nil {
		return "", "", err
	}
	if err = tarWriter.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close tar writer: %w", err)
	}
	if err = gzipWriter.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close gzip writer: %w", err)
	}
	closed = true
	if err = tmpFile.Close(); err != nil {
		return "", "", fmt.Errorf("failed to close temporary tarball: %w", err)
	}
	return tmpName, hex.EncodeToString(hasher.Sum(nil))[:12], nil
}
