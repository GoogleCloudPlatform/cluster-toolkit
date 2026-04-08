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
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hpc-toolkit/pkg/shell"

	"hpc-toolkit/pkg/logging"

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
)

// DockerPlatform represents the target platform for a Docker image.
type DockerPlatform string

const (
	LinuxAMD64 DockerPlatform = "linux/amd64"
	LinuxARM64 DockerPlatform = "linux/arm64"
)

// BuildContainerImageFromBaseImage builds and pushes a container image.
// It appends a new layer created from the scriptDir, filtered by ignorePatterns,
// to a base Docker image.
func BuildContainerImageFromBaseImage(
	project string,
	baseImage string,
	scriptDir string,
	platformStr string,
	ignoreMatcher *patternmatcher.PatternMatcher,
) (string, error) {
	platform, err := parsePlatform(platformStr)
	if err != nil {
		return "", err
	}

	userName := os.Getenv("USER")
	if userName == "" {
		userName = "unknown"
	}

	tagRandomPrefix := shell.RandomString(4)
	tagDatetime := time.Now().Format("2006-01-02-15-04-05") // YYYY-MM-DD-HH-MM-SS
	imageName := fmt.Sprintf("gcr.io/%s/%s-runner:%s-%s", project, userName, tagRandomPrefix, tagDatetime)

	logging.Info("Starting image build process for %s", imageName)
	logging.Info("Base Image: %s", baseImage)
	logging.Info("Script Directory: %s", scriptDir)
	logging.Info("Target Platform: %s/%s", platform.OS, platform.Architecture)

	// Create a tarball in a temporary file from the scriptDir, applying ignore patterns.
	tempTarballPath, err := createFilteredTar(scriptDir, ignoreMatcher)
	if err != nil {
		return "", fmt.Errorf("failed to create filtered tarball: %w", err)
	}
	// Ensure the temporary file is cleaned up after use.
	defer func() {
		if tempTarballPath != "" {
			os.Remove(tempTarballPath)
		}
	}()

	// Create a v1.Layer from the tarball.
	tarLayer, err := layerFromOpener(func() (io.ReadCloser, error) {
		file, openErr := os.Open(tempTarballPath)
		if openErr != nil {
			return nil, fmt.Errorf("failed to open temporary tarball %q: %w", tempTarballPath, openErr)
		}
		return file, nil
	}, tarball.WithCompression(compression.GZip))
	if err != nil {
		return "", fmt.Errorf("failed to create layer from tarball: %w", err)
	}

	baseRef, err := name.ParseReference(baseImage)
	if err != nil {
		return "", fmt.Errorf("failed to parse base image reference %q: %w", baseImage, err)
	}

	baseImg, err := cranePull(baseRef.String(), crane.WithPlatform(&platform))
	if err != nil {
		return "", fmt.Errorf("failed to pull base image %q: %w", baseImage, err)
	}

	newImg, err := appendLayers(baseImg, tarLayer)
	if err != nil {
		return "", fmt.Errorf("failed to append layer: %w", err)
	}

	// Push the new image.
	imageRef, err := name.ParseReference(imageName)
	if err != nil {
		return "", fmt.Errorf("failed to parse new image reference %q: %w", imageName, err)
	}

	logging.Info("Uploading Container Image to %s", imageName)

	err = cranePush(newImg, imageRef.String(), crane.WithPlatform(&platform))
	if err != nil {
		return "", fmt.Errorf("failed to push image %q: %w", imageName, err)
	}

	logging.Info("Image %s built and uploaded successfully.", imageName)
	return imageName, nil
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

func writeFileContent(tarWriter *tar.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open file %q: %w", path, err)
	}
	defer file.Close()

	if _, err := io.Copy(tarWriter, file); err != nil {
		return fmt.Errorf("failed to write file content for %q: %w", path, err)
	}
	return nil
}

func processTarEntry(tarWriter *tar.Writer, sourceDir string, ignoreMatcher *patternmatcher.PatternMatcher, path string, d fs.DirEntry, errFromWalk error) error {
	if errFromWalk != nil {
		return errFromWalk
	}

	relPath, err := filepath.Rel(sourceDir, path)
	if err != nil || relPath == "." {
		return err
	}

	ignored, err := isPathIgnored(relPath, d, ignoreMatcher)
	if err != nil {
		return err
	}
	if ignored {
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}

	info, err := d.Info()
	if err != nil {
		return fmt.Errorf("failed to get info for %q: %w", path, err)
	}

	header, err := tar.FileInfoHeader(info, relPath)
	if err != nil {
		return fmt.Errorf("failed to create tar header for %q: %w", path, err)
	}
	header.Name = relPath

	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header for %q: %w", path, err)
	}

	if info.Mode().IsRegular() {
		return writeFileContent(tarWriter, path)
	}

	return nil
}

func createFilteredTar(sourceDir string, ignoreMatcher *patternmatcher.PatternMatcher) (string, error) {
	tmpFile, err := os.CreateTemp("", "gcluster-build-context-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("failed to create temporary file for tarball: %w", err)
	}
	defer tmpFile.Close()

	gzipWriter := gzip.NewWriter(tmpFile)
	tarWriter := tar.NewWriter(gzipWriter)

	logging.Info("Creating filtered tar from %s to temporary file %s", sourceDir, tmpFile.Name())

	var walkErr error
	defer func() {
		// Ensure tar and gzip writers are closed to flush any buffered data
		if closeErr := tarWriter.Close(); closeErr != nil && walkErr == nil {
			walkErr = fmt.Errorf("failed to close tar writer: %w", closeErr)
		}
		if closeErr := gzipWriter.Close(); closeErr != nil && walkErr == nil {
			walkErr = fmt.Errorf("failed to close gzip writer: %w", closeErr)
		}
	}()

	walkErr = filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		return processTarEntry(tarWriter, sourceDir, ignoreMatcher, path, d, err)
	})

	if walkErr != nil {
		os.Remove(tmpFile.Name())
		return "", walkErr
	}

	return tmpFile.Name(), nil
}
