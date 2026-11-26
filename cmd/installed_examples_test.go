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
	"bytes"
	"encoding/json"
	"hpc-toolkit/pkg/catalog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	. "gopkg.in/check.v1"
)

const installedTestBlueprint = `blueprint_name: installed-test
vars:
  project_id: test-project
  deployment_name: installed-test
  region: us-central1
  zone: us-central1-a
deployment_groups:
- group: primary
  modules:
  - id: network
    source: modules/network/vpc
`

// fakeInstall creates <prefix>/share/cluster-toolkit with one core and one
// community example, makes installedShareDir return it, and changes to an
// empty working directory. It returns the share folder and a function that
// undoes the changes.
func fakeInstall(c *C) (string, func()) {
	prefix := c.MkDir()
	share := filepath.Join(prefix, "share", "cluster-toolkit")
	for _, f := range []string{
		filepath.Join("examples", "hpc-test.yaml"),
		filepath.Join("examples", "test-deployment.yaml"),
		filepath.Join("community", "examples", "community-test.yaml"),
	} {
		full := filepath.Join(share, f)
		c.Assert(os.MkdirAll(filepath.Dir(full), 0o755), IsNil)
		c.Assert(os.WriteFile(full, []byte(installedTestBlueprint), 0o644), IsNil)
	}

	origShare := installedShareDir
	installedShareDir = func() string { return share }
	origWD, err := os.Getwd()
	c.Assert(err, IsNil)
	c.Assert(os.Chdir(c.MkDir()), IsNil)
	return share, func() {
		installedShareDir = origShare
		_ = os.Chdir(origWD)
	}
}

func (s *MySuite) TestFindExampleInInstall(c *C) {
	share, restore := fakeInstall(c)
	defer restore()

	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"examples/hpc-test.yaml", filepath.Join(share, "examples", "hpc-test.yaml"), true},
		{"./examples/hpc-test.yaml", filepath.Join(share, "examples", "hpc-test.yaml"), true},
		{"community/examples/community-test.yaml", filepath.Join(share, "community", "examples", "community-test.yaml"), true},
		{"examples/missing.yaml", "", false},
		{"examples", "", false},                                 // a folder, not a file
		{"hpc-test.yaml", "", false},                            // not under examples/
		{"modules/network/vpc/main.tf", "", false},              // not an example
		{"examples/../../../etc/passwd", "", false},             // can't leave the examples folders
		{filepath.Join(share, "examples", "x.yaml"), "", false}, // absolute paths are used as given
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := findExample(tc.in)
		c.Check(ok, Equals, tc.ok, Commentf("input %q", tc.in))
		c.Check(got, Equals, tc.want, Commentf("input %q", tc.in))
	}
}

func (s *MySuite) TestFindExampleWithoutInstall(c *C) {
	_, restore := fakeInstall(c)
	defer restore()
	installedShareDir = func() string { return "" }

	_, ok := findExample("examples/hpc-test.yaml")
	c.Check(ok, Equals, false)
}

func (s *MySuite) TestResolveBlueprintPathPrefersLocalFile(c *C) {
	share, restore := fakeInstall(c)
	defer restore()

	// Not in the working directory: the installed example is used.
	c.Check(resolveBlueprintPath("examples/hpc-test.yaml"), Equals, filepath.Join(share, "examples", "hpc-test.yaml"))

	// A local file with the same path wins.
	c.Assert(os.MkdirAll("examples", 0o755), IsNil)
	c.Assert(os.WriteFile(filepath.Join("examples", "hpc-test.yaml"), []byte("x"), 0o644), IsNil)
	c.Check(resolveBlueprintPath("examples/hpc-test.yaml"), Equals, "examples/hpc-test.yaml")

	// Unknown paths are returned unchanged, so the usual "does not exist" error follows.
	c.Check(resolveBlueprintPath("examples/missing.yaml"), Equals, "examples/missing.yaml")
}

func (s *MySuite) TestCheckExistsAcceptsInstalledExample(c *C) {
	_, restore := fakeInstall(c)
	defer restore()

	c.Check(checkExists(&cobra.Command{}, []string{"examples/hpc-test.yaml"}), IsNil)
	err := checkExists(&cobra.Command{}, []string{"examples/missing.yaml"})
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, `"examples/missing.yaml" does not exist; run '.*explain' to list the available blueprints`)
	// Other paths keep the plain message.
	err = checkExists(&cobra.Command{}, []string{"missing.yaml"})
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, `"missing.yaml" does not exist`)

	// expand checks the path the same way as create and deploy.
	c.Check(expandCmd.Args(expandCmd, []string{"examples/hpc-test.yaml"}), IsNil)
	c.Check(expandCmd.Args(expandCmd, []string{"examples/missing.yaml"}), NotNil)
}

func (s *MySuite) TestIsInstalledFile(c *C) {
	share, restore := fakeInstall(c)
	defer restore()

	c.Check(isInstalledFile(filepath.Join(share, "examples", "hpc-test.yaml")), Equals, true)
	c.Check(isInstalledFile(filepath.Join(share, "..", "other.yaml")), Equals, false)
	c.Check(isInstalledFile(filepath.Join(share+"-other", "x.yaml")), Equals, false)
	c.Check(isInstalledFile("examples/hpc-test.yaml"), Equals, false) // relative to the working directory
}

// makeShare creates an installed examples folder at dir.
func makeShare(c *C, dir string) {
	for _, d := range []string{"examples", filepath.Join("community", "examples")} {
		c.Assert(os.MkdirAll(filepath.Join(dir, d), 0o755), IsNil)
	}
}

func (s *MySuite) TestShareDirFor(c *C) {
	share := func(base string) string { return filepath.Join(base, "share", "cluster-toolkit") }
	cases := []struct {
		name   string
		exe    func(root string) string // the gcluster binary, relative to a temp root
		shares func(root string) []string
		want   func(root string) string
	}{
		{
			name:   "venv or Python install (bin)",
			exe:    func(r string) string { return filepath.Join(r, "venv", "bin", "gcluster") },
			shares: func(r string) []string { return []string{share(filepath.Join(r, "venv"))} },
			want:   func(r string) string { return share(filepath.Join(r, "venv")) },
		},
		{
			name:   "Windows venv or Python install (Scripts)",
			exe:    func(r string) string { return filepath.Join(r, "venv", "Scripts", "gcluster.exe") },
			shares: func(r string) []string { return []string{share(filepath.Join(r, "venv"))} },
			want:   func(r string) string { return share(filepath.Join(r, "venv")) },
		},
		{
			name:   "Windows pip --user",
			exe:    func(r string) string { return filepath.Join(r, "Python", "Python311", "Scripts", "gcluster.exe") },
			shares: func(r string) []string { return []string{share(filepath.Join(r, "Python"))} },
			want:   func(r string) string { return share(filepath.Join(r, "Python")) },
		},
		{
			name: "the folder next to bin wins",
			exe:  func(r string) string { return filepath.Join(r, "a", "b", "bin", "gcluster") },
			shares: func(r string) []string {
				return []string{share(filepath.Join(r, "a", "b")), share(filepath.Join(r, "a"))}
			},
			want: func(r string) string { return share(filepath.Join(r, "a", "b")) },
		},
		{
			name:   "not installed with examples",
			exe:    func(r string) string { return filepath.Join(r, "usr", "local", "bin", "gcluster") },
			shares: func(r string) []string { return nil },
			want:   func(r string) string { return "" },
		},
	}
	for _, tc := range cases {
		root := c.MkDir()
		for _, d := range tc.shares(root) {
			makeShare(c, d)
		}
		c.Check(shareDirFor(tc.exe(root), "windows"), Equals, tc.want(root), Commentf(tc.name))
	}

	// A share folder without the example folders doesn't count.
	root := c.MkDir()
	c.Assert(os.MkdirAll(share(root), 0o755), IsNil)
	c.Check(shareDirFor(filepath.Join(root, "bin", "gcluster"), "linux"), Equals, "")

	// Two folders up is only for pip --user on Windows (Python3XX\Scripts).
	root = c.MkDir()
	makeShare(c, share(root))
	userExe := filepath.Join(root, "Python311", "Scripts", "gcluster.exe")
	c.Check(shareDirFor(userExe, "windows"), Equals, share(root))
	c.Check(shareDirFor(userExe, "linux"), Equals, "")
	c.Check(shareDirFor(filepath.Join(root, "x", "bin", "gcluster"), "linux"), Equals, "")
	c.Check(shareDirFor(filepath.Join(root, "x", "Scripts", "gcluster.exe"), "windows"), Equals, "")
}

func (s *MySuite) TestResolveBlueprintPathKeepsBrokenLocalSymlink(c *C) {
	_, restore := fakeInstall(c)
	defer restore()
	c.Assert(os.MkdirAll("examples", 0o755), IsNil)
	link := filepath.Join("examples", "hpc-test.yaml")
	if err := os.Symlink("missing-target.yaml", link); err != nil {
		c.Skip("symlinks not supported: " + err.Error())
	}
	// Same answer as checkExists (Lstat): the local path, so the real error shows.
	c.Check(resolveBlueprintPath(link), Equals, link)
}

func (s *MySuite) TestIsInstalledFileThroughSymlink(c *C) {
	share, restore := fakeInstall(c)
	defer restore()
	link := filepath.Join(c.MkDir(), "link")
	if err := os.Symlink(share, link); err != nil {
		c.Skip("symlinks not supported: " + err.Error())
	}
	c.Check(isInstalledFile(filepath.Join(link, "examples", "hpc-test.yaml")), Equals, true)
}

func (s *MySuite) TestFindExplainRepoRootUsesInstalledExamples(c *C) {
	share, restore := fakeInstall(c)
	defer restore()

	got, err := filepath.EvalSymlinks(findExplainRepoRoot())
	c.Assert(err, IsNil)
	want, err := filepath.EvalSymlinks(share)
	c.Assert(err, IsNil)
	c.Check(got, Equals, want)
}

func (s *MySuite) TestExplainListsInstalledExamples(c *C) {
	share, restore := fakeInstall(c)
	defer restore()
	explainCacheMu.Lock()
	explainBlueprintCache = make(map[string][]catalog.BlueprintEntry)
	explainCacheMu.Unlock()

	explainFlags.format = "json"
	defer func() { explainFlags.format = "text" }()
	var out bytes.Buffer
	explainCmd.SetOut(&out)
	defer explainCmd.SetOut(nil)
	c.Assert(runExplainCmd(explainCmd, nil), IsNil)

	var res ExplainIndexResult
	c.Assert(json.Unmarshal(out.Bytes(), &res), IsNil)
	paths := make([]string, 0, len(res.Blueprints))
	for _, bp := range res.Blueprints {
		paths = append(paths, bp.Path)
	}
	c.Check(strings.Join(paths, ","), Matches, ".*examples/hpc-test.yaml.*")
	c.Check(strings.Join(paths, ","), Matches, ".*community/examples/community-test.yaml.*")
	wantDir, err := filepath.EvalSymlinks(share)
	c.Assert(err, IsNil)
	gotDir, err := filepath.EvalSymlinks(res.BlueprintsDir)
	c.Assert(err, IsNil)
	c.Check(gotDir, Equals, wantDir)
	c.Check(res.GclusterPath, Not(Equals), "")
}

func (s *MySuite) TestSubblockMapIsNotSavedToInstalledFiles(c *C) {
	share, restore := fakeInstall(c)
	defer restore()
	installed := filepath.Join(share, "examples", "hpc-test.yaml")

	err := saveSubblockMap(installed, "subblocks", map[string]string{"pool": "block-1"})
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*installed with gcluster.*Copy it to your own folder.*")
	data, err := os.ReadFile(installed)
	c.Assert(err, IsNil)
	c.Check(string(data), Equals, installedTestBlueprint)
}

// fakeCheckout creates a repo checkout with one example and returns its root.
func fakeCheckout(c *C) string {
	root := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(root, "modules"), 0o755), IsNil)
	c.Assert(os.MkdirAll(filepath.Join(root, "examples", "sub"), 0o755), IsNil)
	c.Assert(os.WriteFile(filepath.Join(root, "examples", "hpc-test.yaml"), []byte(installedTestBlueprint), 0o644), IsNil)
	return root
}

func (s *MySuite) TestCheckoutWinsFromSubfolderForExplainAndExpand(c *C) {
	_, restore := fakeInstall(c)
	defer restore()
	checkout := fakeCheckout(c)
	c.Assert(os.Chdir(filepath.Join(checkout, "examples", "sub")), IsNil)

	want, err := filepath.EvalSymlinks(filepath.Join(checkout, "examples", "hpc-test.yaml"))
	c.Assert(err, IsNil)
	got, err := filepath.EvalSymlinks(resolveBlueprintPath("examples/hpc-test.yaml"))
	c.Assert(err, IsNil)
	c.Check(got, Equals, want) // not the installed copy

	gotRoot, err := filepath.EvalSymlinks(findExplainRepoRoot())
	c.Assert(err, IsNil)
	wantRoot, err := filepath.EvalSymlinks(checkout)
	c.Assert(err, IsNil)
	c.Check(gotRoot, Equals, wantRoot)
}

func (s *MySuite) TestReleaseBundleNextToBinary(c *C) {
	_, restore := fakeInstall(c)
	defer restore()
	installedShareDir = func() string { return "" }
	bundle := c.MkDir()
	c.Assert(os.MkdirAll(filepath.Join(bundle, "examples"), 0o755), IsNil)
	c.Assert(os.WriteFile(filepath.Join(bundle, "examples", "hpc-test.yaml"), []byte(installedTestBlueprint), 0o644), IsNil)
	c.Assert(os.WriteFile(filepath.Join(bundle, "gcluster"), []byte("#!/bin/sh\n"), 0o755), IsNil)
	origExe := gclusterExecutable
	defer func() { gclusterExecutable = origExe }()
	gclusterExecutable = func() string { return filepath.Join(bundle, "gcluster") }

	// From another folder, explain and expand both use the bundle.
	c.Check(resolveBlueprintPath("examples/hpc-test.yaml"), Equals, filepath.Join(bundle, "examples", "hpc-test.yaml"))
	c.Check(findExplainRepoRoot(), Equals, bundle)
	c.Check(checkExists(&cobra.Command{}, []string{"examples/hpc-test.yaml"}), IsNil)
}

func (s *MySuite) TestExampleDeploymentFileIsNotReadFromExamplesFolder(c *C) {
	share, restore := fakeInstall(c)
	defer restore()

	_, _, err := readDeploymentFile("examples/test-deployment.yaml")
	c.Assert(err, NotNil)
	c.Check(err.Error(), Matches, ".*not in the current folder.*"+regexpQuote(filepath.Join(share, "examples", "test-deployment.yaml"))+".*--vars.*")

	// A local file, or a path that isn't an example, is left to the normal reader.
	c.Assert(os.MkdirAll("examples", 0o755), IsNil)
	c.Assert(os.WriteFile(filepath.Join("examples", "test-deployment.yaml"), []byte("vars: {}\n"), 0o644), IsNil)
	c.Check(checkDeploymentFile("examples/test-deployment.yaml"), IsNil)
	c.Check(checkDeploymentFile("my-deployment.yaml"), IsNil)
}

func regexpQuote(s string) string { return regexp.QuoteMeta(s) }
