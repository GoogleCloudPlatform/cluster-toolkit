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

func (s *MySuite) TestInstalledExamplePath(c *C) {
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
		got, ok := installedExamplePath(tc.in)
		c.Check(ok, Equals, tc.ok, Commentf("input %q", tc.in))
		c.Check(got, Equals, tc.want, Commentf("input %q", tc.in))
	}
}

func (s *MySuite) TestInstalledExamplePathWithoutInstall(c *C) {
	orig := installedShareDir
	defer func() { installedShareDir = orig }()
	installedShareDir = func() string { return "" }

	_, ok := installedExamplePath("examples/hpc-slurm.yaml")
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
	c.Check(err.Error(), Equals, `"examples/missing.yaml" does not exist`)
}

func (s *MySuite) TestIsInstalledFile(c *C) {
	share, restore := fakeInstall(c)
	defer restore()

	c.Check(isInstalledFile(filepath.Join(share, "examples", "hpc-test.yaml")), Equals, true)
	c.Check(isInstalledFile(filepath.Join(share, "..", "other.yaml")), Equals, false)
	c.Check(isInstalledFile(filepath.Join(share+"-other", "x.yaml")), Equals, false)
	c.Check(isInstalledFile("examples/hpc-test.yaml"), Equals, false) // relative to the working directory
}

func (s *MySuite) TestInstalledShareDirLayout(c *C) {
	// The real lookup: <prefix>/bin/gcluster -> <prefix>/share/cluster-toolkit.
	prefix := c.MkDir()
	c.Check(looksLikeToolkitRoot(filepath.Join(prefix, "share", "cluster-toolkit")), Equals, false)
	for _, d := range []string{"examples", filepath.Join("community", "examples")} {
		c.Assert(os.MkdirAll(filepath.Join(prefix, "share", "cluster-toolkit", d), 0o755), IsNil)
	}
	c.Check(looksLikeToolkitRoot(filepath.Join(prefix, "bin", "..", "share", "cluster-toolkit")), Equals, true)
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
