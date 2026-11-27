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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/logging"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// realExecutable returns the path of the running gcluster binary with symlinks
// resolved. On Linux and macOS, uv and pipx put a symlink in ~/.local/bin that
// points into the environment they installed gcluster in.
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

// gclusterExecutable returns the running gcluster binary. It is a variable so
// tests can replace it.
var gclusterExecutable = realExecutable

// shareDirFor returns the folder where the examples were installed with the
// gcluster binary at exe on the given OS (runtime.GOOS), or "" if there is none:
//   - <prefix>/share/cluster-toolkit for <prefix>/bin/gcluster or
//     <prefix>\Scripts\gcluster.exe: pip into a venv or a Python install, and
//     pip --user, uv and pipx on Linux and macOS;
//   - <userbase>\share\cluster-toolkit for
//     <userbase>\Python3XX\Scripts\gcluster.exe: pip --user on Windows.
//
// uv and pipx on Windows copy gcluster.exe out of the environment they
// installed it in, so its examples can't be found from the binary. That isn't
// supported yet.
func shareDirFor(exe, goos string) string {
	bin := filepath.Dir(exe)
	ups := []string{".."}
	if goos == "windows" && strings.EqualFold(filepath.Base(bin), "Scripts") &&
		strings.HasPrefix(strings.ToLower(filepath.Base(filepath.Dir(bin))), "python3") {
		ups = append(ups, filepath.Join("..", ".."))
	}
	for _, up := range ups {
		dir := filepath.Clean(filepath.Join(bin, up, "share", "cluster-toolkit"))
		if looksLikeToolkitRoot(dir) {
			return dir
		}
	}
	return ""
}

// installedShareDir returns the folder where the running gcluster's examples
// were installed (see shareDirFor), or "". It is a variable so tests can
// replace it.
var installedShareDir = func() string {
	exe := gclusterExecutable()
	if exe == "" {
		return ""
	}
	return shareDirFor(exe, runtime.GOOS)
}

// exampleRoots returns the folders that example blueprints are read from, in
// order, without duplicates:
//  1. the repo checkout or release bundle that contains the current folder;
//  2. the release bundle that contains the gcluster binary;
//  3. the folder the examples were installed in with pip, uv or pipx.
//
// explain and create/deploy/expand use the same list, so they always pick the
// same file for a path like examples/hpc-slurm.yaml.
func exampleRoots() []string {
	var roots []string
	add := func(root string) {
		if root != "" && !slices.Contains(roots, root) {
			roots = append(roots, root)
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		add(walkUpForExplainRoot(cwd))
	}
	if exe := gclusterExecutable(); exe != "" {
		add(walkUpForExplainRoot(filepath.Dir(exe)))
	}
	add(installedShareDir())
	return roots
}

// isExamplePath reports whether p is a relative path under examples/ or
// community/examples/ that doesn't point outside them.
func isExamplePath(p string) bool {
	if p == "" || filepath.IsAbs(p) {
		return false
	}
	rel := filepath.ToSlash(filepath.Clean(p))
	return strings.HasPrefix(rel, "examples/") || strings.HasPrefix(rel, "community/examples/")
}

// findExample returns the file that an example path like
// examples/hpc-slurm.yaml names in the first of exampleRoots that has it.
func findExample(p string) (string, bool) {
	if !isExamplePath(p) {
		return "", false
	}
	rel := filepath.FromSlash(filepath.ToSlash(filepath.Clean(p)))
	for _, root := range exampleRoots() {
		candidate := filepath.Join(root, rel)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, true
		}
	}
	return "", false
}

// resolveBlueprintPath lets `gcluster deploy examples/hpc-slurm.yaml` work from
// any folder. A path that exists in the current folder always wins, even a
// broken symlink (like checkExists, it uses Lstat). Otherwise, an example path
// is looked up in exampleRoots. Anything else is returned unchanged, so the
// usual "does not exist" error follows.
func resolveBlueprintPath(p string) string {
	if _, err := os.Lstat(p); err == nil {
		return p
	}
	found, ok := findExample(p)
	if !ok {
		return p
	}
	if isInstalledFile(found) {
		logging.Info("Using %s installed with gcluster: %s. It is replaced when gcluster is "+
			"upgraded; copy it to your own folder before editing it.", p, found)
	} else {
		logging.Info("Using %s from %s", p, found)
	}
	return found
}

// checkDeploymentFile returns an error for a deployment file (--deployment-file)
// that isn't in the current folder but is one of the examples. Example
// deployment files have empty values (project_id, zone, ...) that must be
// filled in, so they are never read from an examples folder.
func checkDeploymentFile(p string) error {
	if _, err := os.Lstat(p); err == nil {
		return nil
	}
	found, ok := findExample(p)
	if !ok {
		return nil // the usual read error follows
	}
	return fmt.Errorf("deployment file %q is not in the current folder. Example deployment files must be "+
		"filled in before use: copy %s to your folder and edit the copy, or set the values with --vars instead", p, found)
}

// readDeploymentFile reads a --deployment-file after checkDeploymentFile.
func readDeploymentFile(p string) (config.DeploymentSettings, config.YamlCtx, error) {
	if err := checkDeploymentFile(p); err != nil {
		return config.DeploymentSettings{}, config.YamlCtx{}, err
	}
	return config.NewDeploymentSettings(p)
}

// isInstalledFile reports whether path is inside the folder where gcluster's
// examples are installed. Files there belong to the installed package and must
// not be changed: pip replaces them on upgrade.
func isInstalledFile(path string) bool {
	share := installedShareDir()
	if share == "" || path == "" {
		return false
	}
	rel, err := filepath.Rel(realPath(share), realPath(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// realPath returns the absolute path of p with symlinks resolved, or as much of
// that as can be worked out.
func realPath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	}
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return p
}
