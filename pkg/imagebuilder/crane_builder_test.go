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
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/registry"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/moby/patternmatcher"
)

// Wrapper to simulate logic in processTarEntry
func testShouldIgnore(t *testing.T, matcher *patternmatcher.PatternMatcher, relPath string, isDir bool) bool {
	relPathSlash := filepath.ToSlash(relPath)
	if isDir && !strings.HasSuffix(relPathSlash, "/") {
		relPathSlash += "/"
	}
	// MatchesOrParentMatches is what we use in processTarEntry
	ignored, err := matcher.MatchesOrParentMatches(relPathSlash)
	if err != nil {
		t.Errorf("MatchesOrParentMatches error: %v", err)
	}
	return ignored
}

func TestPatternMatcherIntegration(t *testing.T) {
	tests := []struct {
		name           string
		ignorePatterns []string
		path           string
		isDir          bool
		wantIgnored    bool
	}{
		{
			name:           "Simple match",
			ignorePatterns: []string{"*.log"},
			path:           "foo.log",
			isDir:          false,
			wantIgnored:    true,
		},
		{
			name:           "Simple mismatch",
			ignorePatterns: []string{"*.log"},
			path:           "foo.txt",
			isDir:          false,
			wantIgnored:    false,
		},
		{
			name:           "Directory match",
			ignorePatterns: []string{"temp"},
			path:           "temp",
			isDir:          true,
			wantIgnored:    true,
		},
		{
			name:           "Negation",
			ignorePatterns: []string{"*.log", "!important.log"},
			path:           "important.log",
			isDir:          false,
			wantIgnored:    false,
		},
		{
			name:           "Double star",
			ignorePatterns: []string{"**/*.tmp"},
			path:           "a/b/c/foo.tmp",
			isDir:          false,
			wantIgnored:    true,
		},
		{
			name:           "Directory pattern with slash matching directory",
			ignorePatterns: []string{"foo/"},
			path:           "foo",
			isDir:          true,
			wantIgnored:    true,
		},
		{
			name:           "Directory pattern with slash matching file (KNOWN LIMITATION)",
			ignorePatterns: []string{"foo/"},
			path:           "foo", // file named foo
			isDir:          false,
			wantIgnored:    true, // LIMITATION: moby/patternmatcher matches this even if it shouldn't per strict Docker spec
		},
		{
			name:           "Nested file in ignored directory",
			ignorePatterns: []string{"foo/"},
			path:           "foo/bar",
			isDir:          false,
			wantIgnored:    true, // MatchesOrParentMatches should catch this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matcher, err := patternmatcher.New(tt.ignorePatterns)
			if err != nil {
				t.Fatalf("failed to create matcher: %v", err)
			}

			got := testShouldIgnore(t, matcher, tt.path, tt.isDir)
			if got != tt.wantIgnored {
				t.Errorf("testShouldIgnore(%q, isDir=%v) = %v, want %v", tt.path, tt.isDir, got, tt.wantIgnored)
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name        string
		platformStr string
		wantOS      string
		wantArch    string
		wantErr     bool
	}{
		{
			name:        "Valid platform",
			platformStr: "linux/amd64",
			wantOS:      "linux",
			wantArch:    "amd64",
			wantErr:     false,
		},
		{
			name:        "Invalid platform format",
			platformStr: "linuxamd64",
			wantOS:      "",
			wantArch:    "",
			wantErr:     true,
		},
		{
			name:        "Invalid platform parts",
			platformStr: "linux/amd64/v7",
			wantOS:      "",
			wantArch:    "",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePlatform(tt.platformStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("parsePlatform() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.OS != tt.wantOS {
					t.Errorf("parsePlatform() OS = %v, want %v", got.OS, tt.wantOS)
				}
				if got.Architecture != tt.wantArch {
					t.Errorf("parsePlatform() Architecture = %v, want %v", got.Architecture, tt.wantArch)
				}
			}
		})
	}
}

func TestReadDockerignorePatterns(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dockerignore-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dockerignorePath := filepath.Join(tempDir, ".dockerignore")
	content := "*.log\n!important.log\n"
	if err := os.WriteFile(dockerignorePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	matcher, err := ReadDockerignorePatterns(tempDir, []string{"default.tmp"})
	if err != nil {
		t.Fatalf("ReadDockerignorePatterns() error = %v", err)
	}

	if matcher == nil {
		t.Fatal("ReadDockerignorePatterns() returned nil matcher")
	}

	// Test if it matches *.log
	ignored, err := matcher.MatchesOrParentMatches("foo.log")
	if err != nil {
		t.Errorf("got error matching: %v", err)
	}
	if !ignored {
		t.Error("expected foo.log to be ignored per .dockerignore")
	}

	// Test negation
	ignored, err = matcher.MatchesOrParentMatches("important.log")
	if err != nil {
		t.Errorf("got error matching: %v", err)
	}
	if ignored {
		t.Error("expected important.log to NOT be ignored per .dockerignore")
	}

	// Test default pattern
	ignored, err = matcher.MatchesOrParentMatches("default.tmp")
	if err != nil {
		t.Errorf("got error matching: %v", err)
	}
	if !ignored {
		t.Error("expected default.tmp to be ignored per default patterns")
	}
}

func TestCreateFilteredTar(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tar-test-source")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	createTestFiles(t, tempDir)

	matcher, err := patternmatcher.New([]string{"*.log"})
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	tarPath, _, err := createFilteredTar(tempDir, "", matcher)
	if err != nil {
		t.Fatalf("createFilteredTar() error = %v", err)
	}
	defer os.Remove(tarPath)

	foundFiles := getFilesFromTar(t, tarPath)

	if !foundFiles["foo.txt"] {
		t.Error("foo.txt not found in tarball")
	}
	if foundFiles["bar.log"] {
		t.Error("bar.log should have been ignored but was found in tarball")
	}
	if !foundFiles["sub/baz.txt"] {
		t.Error("sub/baz.txt not found in tarball")
	}
}

func createTestFiles(t *testing.T, tempDir string) {
	if err := os.WriteFile(filepath.Join(tempDir, "foo.txt"), []byte("foo content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "bar.log"), []byte("bar content"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tempDir, "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "sub", "baz.txt"), []byte("baz content"), 0644); err != nil {
		t.Fatal(err)
	}
}

func getFilesFromTar(t *testing.T, tarPath string) map[string]bool {
	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatalf("failed to open generated tarball: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	foundFiles := make(map[string]bool)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("error reading tar: %v", err)
		}
		foundFiles[header.Name] = true
	}
	return foundFiles
}

func TestBuildContainerImageFromBaseImage_Success(t *testing.T) {
	// Preserve originals
	origPull := cranePull

	origRepo := os.Getenv("GCLUSTER_IMAGE_REPO")
	os.Setenv("GCLUSTER_IMAGE_REPO", "gcluster")
	defer os.Setenv("GCLUSTER_IMAGE_REPO", origRepo)

	origUser := os.Getenv("USER")
	os.Setenv("USER", "testuser")
	defer os.Setenv("USER", origUser)

	origPush := cranePush
	origAppend := appendLayers
	origLayerOpener := layerFromOpener
	defer func() {
		cranePull = origPull
		cranePush = origPush
		appendLayers = origAppend
		layerFromOpener = origLayerOpener
	}()

	// Mock implementations
	cranePull = func(ref string, opts ...crane.Option) (v1.Image, error) {
		return nil, nil // Return nil image for now, as we don't use it deeply in the mock
	}
	cranePush = func(img v1.Image, ref string, opts ...crane.Option) error {
		return nil
	}
	appendLayers = func(base v1.Image, layers ...v1.Layer) (v1.Image, error) {
		return nil, nil
	}

	tempDir, err := os.MkdirTemp("", "build-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	matcher, _ := patternmatcher.New([]string{})
	got, err := BuildContainerImageFromBaseImage("test-project", "us-central1", "default", "train-job", "ubuntu", tempDir, "linux/amd64", matcher)
	if err != nil {
		t.Fatalf("BuildContainerImageFromBaseImage() error = %v", err)
	}

	if !strings.Contains(got, "us-central1-docker.pkg.dev/test-project/gcluster/") {
		t.Errorf("expected imageName to contain us-central1-docker.pkg.dev/test-project/gcluster/, got %s", got)
	}
}

func TestBuildContainerImageFromBaseImage_PlatformError(t *testing.T) {
	_, err := BuildContainerImageFromBaseImage("test-project", "us-central1", "default", "train-job", "ubuntu", "", "invalid-platform", nil)
	if err == nil {
		t.Error("expected error for invalid platform, got nil")
	}
}

func TestBuildContainerImageFromBaseImage_ParseReferenceError(t *testing.T) {
	_, err := BuildContainerImageFromBaseImage("test-project", "us-central1", "default", "train-job", "!!invalid!!", "", "linux/amd64", nil)
	if err == nil {
		t.Error("expected error for invalid base image, got nil")
	}
}

func TestCreateFilteredTar_Symlink(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tar-symlink-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	targetPath := filepath.Join(tempDir, "target.txt")
	if err := os.WriteFile(targetPath, []byte("target content"), 0644); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(tempDir, "link.txt")
	if err := os.Symlink("target.txt", linkPath); err != nil {
		t.Fatal(err)
	}

	matcher, err := patternmatcher.New([]string{})
	if err != nil {
		t.Fatal(err)
	}

	tarPath, _, err := createFilteredTar(tempDir, "", matcher)
	if err != nil {
		t.Fatalf("createFilteredTar() error = %v", err)
	}
	defer os.Remove(tarPath)

	f, err := os.Open(tarPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	if !findSymlinkInTar(t, tr, "link.txt", "target.txt") {
		t.Error("link.txt not found or invalid in tarball")
	}
}

func findSymlinkInTar(t *testing.T, tr *tar.Reader, linkName, expectedTarget string) bool {
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == linkName {
			if header.Typeflag != tar.TypeSymlink {
				t.Errorf("expected %s to be symlink, got %v", linkName, header.Typeflag)
			}
			if header.Linkname != expectedTarget {
				t.Errorf("expected symlink target to be %q, got %q", expectedTarget, header.Linkname)
			}
			return true
		}
	}
	return false
}

func TestWriteFileContent_OpenError(t *testing.T) {
	err := writeFileContentWithHash(nil, nil, "non-existent-file")
	if err == nil {
		t.Error("expected error opening non-existent file, got nil")
	}
}

func TestReadDockerignorePatterns_OpenError(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "dockerignore-open-error")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	dockerignorePath := filepath.Join(tempDir, ".dockerignore")
	// Create a directory instead of a file to simulate a read error
	if err := os.Mkdir(dockerignorePath, 0755); err != nil {
		t.Fatal(err)
	}

	_, err = ReadDockerignorePatterns(tempDir, nil)
	if err == nil {
		t.Error("expected error reading unreadable .dockerignore, got nil")
	}
}

func TestCreateFilteredTar_IgnoreDir(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "tar-test-ignore-dir")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	// Create a directory to ignore
	ignoredDir := filepath.Join(tempDir, "ignored_dir")
	if err := os.Mkdir(ignoredDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ignoredDir, "file.txt"), []byte("ignored content"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a file that should not be ignored
	if err := os.WriteFile(filepath.Join(tempDir, "keep.txt"), []byte("keep content"), 0644); err != nil {
		t.Fatal(err)
	}

	matcher, err := patternmatcher.New([]string{"ignored_dir/"})
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	tarPath, _, err := createFilteredTar(tempDir, "", matcher)
	if err != nil {
		t.Fatalf("createFilteredTar() error = %v", err)
	}
	defer os.Remove(tarPath)

	foundFiles := getFilesFromTar(t, tarPath)

	if !foundFiles["keep.txt"] {
		t.Error("keep.txt not found in tarball")
	}
	if foundFiles["ignored_dir/file.txt"] {
		t.Error("ignored_dir/file.txt should have been ignored but was found in tarball")
	}
}

func TestGenerateImageName_NonGARScopingAndNoUserEnv(t *testing.T) {
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "")
	t.Setenv("GCLUSTER_IMAGE_REPO", "")

	got, err := GenerateImageName("", "", "Team-Alpha", "My_Job", "[fd12::1]:5000/base/jax:v5p", "abcdef0123456789")
	if err != nil {
		t.Fatalf("GenerateImageName() unexpected error: %v", err)
	}
	want := "[fd12::1]:5000/library/gcluster/team-alpha/my_job:abcdef012345"
	if got != want {
		t.Errorf("GenerateImageName() = %q, want %q", got, want)
	}

	// Dry-run fallback hash when contentHash is empty is deterministic and 12 hex chars.
	dry1, err := GenerateImageName("", "", "default", "job-a", "10.128.0.68:5000/ubuntu:24.04", "")
	if err != nil {
		t.Fatalf("GenerateImageName(dry-run) unexpected error: %v", err)
	}
	dry2, err := GenerateImageName("", "", "default", "job-a", "10.128.0.68:5000/ubuntu:24.04", "")
	if err != nil || dry1 != dry2 {
		t.Fatalf("expected deterministic dry-run URI, got %q vs %q (err=%v)", dry1, dry2, err)
	}
	if !strings.HasPrefix(dry1, "10.128.0.68:5000/library/gcluster/default/job-a:") || len(strings.Split(dry1, ":")[2]) != 12 {
		t.Errorf("unexpected dry-run URI format: %q", dry1)
	}

	// Malformed GCLUSTER_IMAGE_REPO must fail in GenerateImageName (--dry-run-out path).
	t.Setenv("GCLUSTER_IMAGE_REPO", "harbor.local/ml-team:latest")
	if _, err := GenerateImageName("", "", "default", "job-a", "10.128.0.68:5000/ubuntu:24.04", ""); err == nil {
		t.Fatalf("expected malformed GCLUSTER_IMAGE_REPO with tag to fail in GenerateImageName")
	}
}

func readFirstLayerHeaders(t *testing.T, img v1.Image) (map[string]int64, map[string]time.Time) {
	t.Helper()
	layers, err := img.Layers()
	if err != nil || len(layers) != 1 {
		t.Fatalf("img.Layers() len=%d err=%v, want 1", len(layers), err)
	}
	rc, err := layers[0].Uncompressed()
	if err != nil {
		t.Fatalf("layer.Uncompressed(): %v", err)
	}
	defer func() { _ = rc.Close() }()
	modes := map[string]int64{}
	mtimes := map[string]time.Time{}
	tr := tar.NewReader(rc)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next(): %v", err)
		}
		modes[hdr.Name] = hdr.Mode & 0777
		mtimes[hdr.Name] = hdr.ModTime.UTC()
	}
	return modes, mtimes
}

func setupWorkdirTestContext(t *testing.T) string {
	t.Helper()
	srcDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(srcDir, "train.py"), []byte("print('ok')\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "scripts"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "scripts", "run.sh"), []byte("#!/bin/sh\necho hi\n"), 0755); err != nil {
		t.Fatal(err)
	}
	return srcDir
}

func TestBuildContainerImageFromBaseImage_WorkdirAlignmentAndModes(t *testing.T) {
	srcDir := setupWorkdirTestContext(t)
	matcher, err := patternmatcher.New(nil)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name           string
		baseWorkingDir string
		wantWorkingDir string
		wantTarPrefix  string
	}{
		{name: "empty WORKDIR defaults to /workspace and mutates image config", baseWorkingDir: "", wantWorkingDir: "/workspace", wantTarPrefix: "workspace/"},
		{name: "root WORKDIR / defaults to /workspace and mutates image config", baseWorkingDir: "/", wantWorkingDir: "/workspace", wantTarPrefix: "workspace/"},
		{name: "unrooted .. WORKDIR defaults to /workspace and mutates image config", baseWorkingDir: "..", wantWorkingDir: "/workspace", wantTarPrefix: "workspace/"},
		{name: "unrooted ../app WORKDIR normalizes to /app and mutates image config", baseWorkingDir: "../app", wantWorkingDir: "/app", wantTarPrefix: "app/"},
		{name: "unrooted relative app WORKDIR normalizes to /app and mutates image config", baseWorkingDir: "app", wantWorkingDir: "/app", wantTarPrefix: "app/"},
		{name: "existing /app WORKDIR preserved", baseWorkingDir: "/app", wantWorkingDir: "/app", wantTarPrefix: "app/"},
		{name: "nested /opt/maxtext/ WORKDIR preserved without trailing double slash", baseWorkingDir: "/opt/maxtext/", wantWorkingDir: "/opt/maxtext/", wantTarPrefix: "opt/maxtext/"},
	}

	wantMtime := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GCLUSTER_IMAGE_REPO", "")
			t.Setenv("USER", "")
			t.Setenv("USERNAME", "")

			baseImg, err := mutate.Config(empty.Image, v1.Config{WorkingDir: tc.baseWorkingDir})
			if err != nil {
				t.Fatalf("mutate.Config(base): %v", err)
			}

			origPull, origPush := cranePull, cranePush
			t.Cleanup(func() {
				cranePull = origPull
				cranePush = origPush
			})

			var pushedImg v1.Image
			var pushedRef string
			var modes map[string]int64
			var mtimes map[string]time.Time
			cranePull = func(ref string, opts ...crane.Option) (v1.Image, error) {
				return baseImg, nil
			}
			cranePush = func(img v1.Image, ref string, opts ...crane.Option) error {
				pushedImg = img
				pushedRef = ref
				modes, mtimes = readFirstLayerHeaders(t, img)
				return nil
			}

			gotName, err := BuildContainerImageFromBaseImage("", "", "ml-ns", "jax-train", "10.128.0.68:5000/jax:v5p", srcDir, "linux/amd64", matcher)
			if err != nil || !strings.HasPrefix(gotName, "10.128.0.68:5000/library/gcluster/ml-ns/jax-train:") || pushedRef != gotName {
				t.Fatalf("got image %q (pushed %q, err=%v)", gotName, pushedRef, err)
			}

			cfg, err := pushedImg.ConfigFile()
			if err != nil || cfg.Config.WorkingDir != tc.wantWorkingDir {
				t.Fatalf("Config.WorkingDir = %q (err=%v), want %q", cfg.Config.WorkingDir, err, tc.wantWorkingDir)
			}
			if modes[tc.wantTarPrefix+"train.py"] != 0644 || modes[tc.wantTarPrefix+"scripts/run.sh"] != 0755 {
				t.Errorf("unexpected tar entry modes: %v", modes)
			}
			if gotMtime := mtimes[tc.wantTarPrefix+"train.py"]; !gotMtime.Equal(wantMtime) {
				t.Errorf("tar entry ModTime = %v, want %v (ZIP-safe >= 1980)", gotMtime, wantMtime)
			}
		})
	}
}

func TestBuildContainerImageFromBaseImage_BaseDigestTagInvalidation(t *testing.T) {
	srcDir := setupWorkdirTestContext(t)
	matcher, err := patternmatcher.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GCLUSTER_IMAGE_REPO", "")
	t.Setenv("USER", "")
	t.Setenv("USERNAME", "")

	baseV1, err := mutate.Config(empty.Image, v1.Config{WorkingDir: "/workspace", Env: []string{"REV=1"}})
	if err != nil {
		t.Fatal(err)
	}
	baseV2, err := mutate.Config(empty.Image, v1.Config{WorkingDir: "/workspace", Env: []string{"REV=2"}})
	if err != nil {
		t.Fatal(err)
	}

	origPull, origPush := cranePull, cranePush
	t.Cleanup(func() {
		cranePull = origPull
		cranePush = origPush
	})
	cranePush = func(v1.Image, string, ...crane.Option) error { return nil }

	var currentBase = baseV1
	cranePull = func(string, ...crane.Option) (v1.Image, error) { return currentBase, nil }

	tag1a, err := BuildContainerImageFromBaseImage("", "", "default", "train", "10.128.0.68:5000/jax:v5p", srcDir, "linux/amd64", matcher)
	if err != nil {
		t.Fatalf("build 1a failed: %v", err)
	}
	tag1b, err := BuildContainerImageFromBaseImage("", "", "default", "train", "10.128.0.68:5000/jax:v5p", srcDir, "linux/amd64", matcher)
	if err != nil || tag1a != tag1b {
		t.Fatalf("expected identical base digest + context to produce deterministic tag, got %q vs %q (err=%v)", tag1a, tag1b, err)
	}

	currentBase = baseV2
	tag2, err := BuildContainerImageFromBaseImage("", "", "default", "train", "10.128.0.68:5000/jax:v5p", srcDir, "linux/amd64", matcher)
	if err != nil {
		t.Fatalf("build 2 failed: %v", err)
	}
	if tag2 == tag1a {
		t.Fatalf("expected updated base image digest under same mutable tag to produce a distinct built tag, both were %q", tag2)
	}
}

func startBasicAuthRegistry(t *testing.T, wantUser, wantPass string, useTLS bool) string {
	t.Helper()
	regHandler := registry.New()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != wantUser || p != wantPass {
			w.Header().Set("WWW-Authenticate", `Basic realm="harbor"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		regHandler.ServeHTTP(w, r)
	})
	var srv *httptest.Server
	if useTLS {
		srv = httptest.NewTLSServer(handler)
	} else {
		srv = httptest.NewServer(handler)
	}
	t.Cleanup(srv.Close)
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Host
}

func writeTestDockerAuth(t *testing.T, hostPort, user, pass string) {
	t.Helper()
	dockerCfgDir := t.TempDir()
	authToken := base64.StdEncoding.EncodeToString([]byte(user + ":" + pass))
	cfgBytes := []byte(`{"auths":{"` + hostPort + `":{"auth":"` + authToken + `"}}}`)
	if err := os.WriteFile(filepath.Join(dockerCfgDir, "config.json"), cfgBytes, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_CONFIG", dockerCfgDir)
}

func TestBuildContainerImageFromBaseImage_LocalHTTPAndAuthKeychain(t *testing.T) {
	for _, useTLS := range []bool{false, true} {
		modeName := "plain_http_loopback"
		if useTLS {
			modeName = "self_signed_tls_with_insecure_env"
		}
		t.Run(modeName, func(t *testing.T) {
			const wantUser, wantPass = "robot$ci", "s3cr3t-token"
			hostPort := startBasicAuthRegistry(t, wantUser, wantPass, useTLS)
			writeTestDockerAuth(t, hostPort, wantUser, wantPass)
			t.Setenv("GCLUSTER_IMAGE_REPO", hostPort+"/harbor-proj")
			if useTLS {
				t.Setenv("GCLUSTER_INSECURE_REGISTRY", "true")
			} else {
				t.Setenv("GCLUSTER_INSECURE_REGISTRY", "")
			}
			t.Setenv("USER", "")
			t.Setenv("USERNAME", "")

			seedBase, err := mutate.Config(empty.Image, v1.Config{WorkingDir: ""})
			if err != nil {
				t.Fatal(err)
			}
			baseRef := hostPort + "/harbor-proj/base:v1"
			seedOpts := []crane.Option{crane.WithPlatform(&v1.Platform{OS: "linux", Architecture: "amd64"})}
			if useTLS {
				seedOpts = append(seedOpts, crane.Insecure)
			}
			if err := crane.Push(seedBase, baseRef, seedOpts...); err != nil {
				t.Fatalf("seed crane.Push(%q): %v", baseRef, err)
			}

			ctxDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(ctxDir, "app.py"), []byte("print('hello')\n"), 0644); err != nil {
				t.Fatal(err)
			}
			matcher, _ := patternmatcher.New(nil)
			builtURI, err := BuildContainerImageFromBaseImage("", "", "prod-ns", "smoke-job", baseRef, ctxDir, "linux/amd64", matcher)
			if err != nil || !strings.HasPrefix(builtURI, hostPort+"/harbor-proj/prod-ns/smoke-job:") {
				t.Fatalf("BuildContainerImageFromBaseImage() = (%q, %v)", builtURI, err)
			}

			pulled, err := crane.Pull(builtURI, seedOpts...)
			if err != nil {
				t.Fatalf("crane.Pull(%q): %v", builtURI, err)
			}
			cf, err := pulled.ConfigFile()
			if err != nil || cf.Config.WorkingDir != "/workspace" {
				t.Fatalf("pulled WorkingDir = %q (err=%v), want /workspace", cf.Config.WorkingDir, err)
			}
		})
	}
}

func TestBuildContainerImageFromBaseImage_InvalidImageRepoFailsBeforePull(t *testing.T) {
	t.Setenv("GCLUSTER_IMAGE_REPO", "harbor.local/invalid!!repo")
	origPull := cranePull
	t.Cleanup(func() { cranePull = origPull })
	pullCalled := false
	cranePull = func(string, ...crane.Option) (v1.Image, error) {
		pullCalled = true
		return nil, nil
	}

	_, err := BuildContainerImageFromBaseImage("", "", "default", "job", "10.128.0.68:5000/jax:v5p", t.TempDir(), "linux/amd64", nil)
	if err == nil {
		t.Fatalf("expected invalid GCLUSTER_IMAGE_REPO repository path to fail")
	}
	if pullCalled {
		t.Fatalf("expected destination reference validation to fail before cranePull was invoked")
	}
}

func TestBuildContainerImageFromBaseImage_PullAndPushErrors(t *testing.T) {
	t.Run("cranePull failure bubbles up with base image reference", func(t *testing.T) {
		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		origPull := cranePull
		t.Cleanup(func() { cranePull = origPull })
		cranePull = func(string, ...crane.Option) (v1.Image, error) {
			return nil, errors.New("simulated registry pull error")
		}

		_, err := BuildContainerImageFromBaseImage("", "", "default", "job", "10.128.0.68:5000/jax:v5p", t.TempDir(), "linux/amd64", nil)
		if err == nil || !strings.Contains(err.Error(), "simulated registry pull error") {
			t.Fatalf("expected pull error propagation, got: %v", err)
		}
	})

	t.Run("cranePush failure on auto-derived destination names GCLUSTER_IMAGE_REPO while explicit destination does not", func(t *testing.T) {
		origPull, origPush := cranePull, cranePush
		t.Cleanup(func() {
			cranePull = origPull
			cranePush = origPush
		})
		cranePull = func(string, ...crane.Option) (v1.Image, error) {
			return empty.Image, nil
		}
		cranePush = func(v1.Image, string, ...crane.Option) error {
			return errors.New("simulated registry push error")
		}

		ctxDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(ctxDir, "app.py"), []byte("print(1)\n"), 0644); err != nil {
			t.Fatal(err)
		}

		t.Setenv("GCLUSTER_IMAGE_REPO", "")
		_, err := BuildContainerImageFromBaseImage("", "", "default", "job", "10.128.0.68:5000/jax:v5p", ctxDir, "linux/amd64", nil)
		if err == nil || !strings.Contains(err.Error(), "simulated registry push error") || !strings.Contains(err.Error(), "GCLUSTER_IMAGE_REPO") {
			t.Fatalf("expected auto-derived push error to include GCLUSTER_IMAGE_REPO hint, got: %v", err)
		}

		t.Setenv("GCLUSTER_IMAGE_REPO", "10.128.0.68:5000/explicit-repo")
		_, explicitErr := BuildContainerImageFromBaseImage("", "", "default", "job", "10.128.0.68:5000/jax:v5p", ctxDir, "linux/amd64", nil)
		if explicitErr == nil || !strings.Contains(explicitErr.Error(), "simulated registry push error") || strings.Contains(explicitErr.Error(), "auto-derived") {
			t.Fatalf("expected explicit destination push error without auto-derived hint, got: %v", explicitErr)
		}
	})
}
