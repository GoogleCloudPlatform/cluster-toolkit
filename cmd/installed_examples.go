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

package cmd

import (
	"hpc-toolkit/pkg/logging"
	"os"
	"path/filepath"
	"strings"
)

// realExecutable returns the path of the running gcluster binary with symlinks
// resolved. uv and pipx put a symlink in ~/.local/bin that points into the
// environment they installed gcluster in.
func realExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return exe
}

// installedShareDir returns the folder where a pip, uv or pipx install puts the
// example blueprints, <prefix>/share/cluster-toolkit, for a gcluster binary at
// <prefix>/bin/gcluster (<prefix>\Scripts\gcluster.exe on Windows). It returns
// "" if gcluster wasn't installed that way. It is a variable so tests can
// replace it.
var installedShareDir = func() string {
	exe := realExecutable()
	if exe == "" {
		return ""
	}
	dir := filepath.Join(filepath.Dir(exe), "..", "share", "cluster-toolkit")
	if !looksLikeToolkitRoot(dir) {
		return ""
	}
	return filepath.Clean(dir)
}

// installedExamplePath returns the installed copy of an example blueprint (or
// deployment file) named like examples/... or community/examples/..., if
// gcluster was installed with its examples and the file exists there.
func installedExamplePath(p string) (string, bool) {
	if p == "" || filepath.IsAbs(p) {
		return "", false
	}
	rel := filepath.ToSlash(filepath.Clean(p))
	if !strings.HasPrefix(rel, "examples/") && !strings.HasPrefix(rel, "community/examples/") {
		return "", false
	}
	share := installedShareDir()
	if share == "" {
		return "", false
	}
	candidate := filepath.Join(share, filepath.FromSlash(rel))
	if info, err := os.Stat(candidate); err != nil || info.IsDir() {
		return "", false
	}
	return candidate, true
}

// resolveBlueprintPath lets `gcluster deploy examples/hpc-slurm.yaml` work from
// any folder after a pip, uv or pipx install. A path that exists locally always
// wins. Otherwise, if p names an example installed with gcluster, the installed
// file is used. Anything else is returned unchanged, so the usual "does not
// exist" error follows.
func resolveBlueprintPath(p string) string {
	if _, err := os.Stat(p); err == nil {
		return p
	}
	if installed, ok := installedExamplePath(p); ok {
		logging.Info("Using %s installed with gcluster: %s", p, installed)
		return installed
	}
	return p
}

// isInstalledFile reports whether path is inside the folder where gcluster's
// examples are installed. Files there belong to the installed package and must
// not be changed: pip replaces them on upgrade.
func isInstalledFile(path string) bool {
	share := installedShareDir()
	if share == "" || path == "" {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(share, absPath)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
