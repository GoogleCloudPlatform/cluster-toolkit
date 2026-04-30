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
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/crane"
	v1 "github.com/google/go-containerregistry/pkg/v1"
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

	tarPath, err := createFilteredTar(tempDir, matcher)
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
	got, err := BuildContainerImageFromBaseImage("test-project", "us-central1", "ubuntu", tempDir, "linux/amd64", matcher)
	if err != nil {
		t.Fatalf("BuildContainerImageFromBaseImage() error = %v", err)
	}

	if !strings.Contains(got, "us-central1-docker.pkg.dev/test-project/gcluster/") {
		t.Errorf("expected imageName to contain us-central1-docker.pkg.dev/test-project/gcluster/, got %s", got)
	}
}

func TestBuildContainerImageFromBaseImage_PlatformError(t *testing.T) {
	_, err := BuildContainerImageFromBaseImage("test-project", "us-central1", "ubuntu", "", "invalid-platform", nil)
	if err == nil {
		t.Error("expected error for invalid platform, got nil")
	}
}

func TestBuildContainerImageFromBaseImage_ParseReferenceError(t *testing.T) {
	_, err := BuildContainerImageFromBaseImage("test-project", "us-central1", "!!invalid!!", "", "linux/amd64", nil)
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

	tarPath, err := createFilteredTar(tempDir, matcher)
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
	err := writeFileContent(nil, "non-existent-file")
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

	tarPath, err := createFilteredTar(tempDir, matcher)
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
