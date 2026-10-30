/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package modulewriter

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/modulereader"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/exp/slices"

	"github.com/google/go-cmp/cmp"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"

	. "gopkg.in/check.v1"
)

type zeroSuite struct{}

var _ = []any{ // initialize suites
	Suite(&zeroSuite{})}

func Test(t *testing.T) {
	TestingT(t)
}

// Tests

func isDeploymentDirPrepped(depDirectoryPath string) error {
	if _, err := os.Stat(depDirectoryPath); os.IsNotExist(err) {
		return fmt.Errorf("deployment dir does not exist: %s: %w", depDirectoryPath, err)
	}

	ghpcDir := filepath.Join(depDirectoryPath, ".ghpc")
	if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
		return fmt.Errorf(".ghpc working dir does not exist: %s: %w", ghpcDir, err)
	}

	prevModuleDir := filepath.Join(ghpcDir, prevGroupDirName)
	if _, err := os.Stat(prevModuleDir); os.IsNotExist(err) {
		return fmt.Errorf("previous deployment group directory does not exist: %s: %w", prevModuleDir, err)
	}

	return nil
}

func (s *zeroSuite) TestPrepDepDir(c *C) {
	depDir := filepath.Join(c.MkDir(), "test_prep_dir")

	// Prep a dir that does not yet exist
	c.Check(prepDepDir(depDir), IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)

	// Prep of existing dir succeeds
	c.Check(prepDepDir(depDir), IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)
}

func (s *zeroSuite) TestPrepDepDir_OverwriteRealDep(c *C) {
	// Test with a real deployment previously written
	bp := config.Blueprint{
		Vars:   config.Dict{}.With("deployment_name", cty.StringVal("test_prep_dir")),
		Groups: []config.Group{{Name: "ozon"}},
	}
	depDir := filepath.Join(c.MkDir(), "test_prep_dir")

	// writes a full deployment w/ actual resource groups
	WriteDeployment(bp, depDir)

	// confirm existence of resource groups (beyond .ghpc dir)
	files, _ := os.ReadDir(depDir)
	c.Check(len(files) > 1, Equals, true)

	err := prepDepDir(depDir)
	c.Check(err, IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)

	// Check prev resource groups were moved
	prevModuleDir := filepath.Join(depDir, ".ghpc", prevGroupDirName)
	files1, _ := os.ReadDir(prevModuleDir)
	c.Check(len(files1) > 0, Equals, true)

	files2, _ := os.ReadDir(depDir)
	c.Check(files2, HasLen, 3) // .ghpc, .gitignore, and instructions file
}

// modulewriter.go
func (s *zeroSuite) TestWriteDeployment(c *C) {
	bp := config.Blueprint{
		Vars: config.Dict{}.With("deployment_name", cty.StringVal("green")),
		Groups: []config.Group{{
			Name: "ozon",
			Modules: []config.Module{{
				Source: "some/path",
				ID:     "whole",
				Kind:   config.TerraformKind,
			}},
		}},
	}
	dir := filepath.Join(c.MkDir(), "depl")

	c.Check(WriteDeployment(bp, dir), IsNil)
	// Overwriting the deployment succeeds
	c.Check(WriteDeployment(bp, dir), IsNil)
}

func (s *zeroSuite) TestCreateGroupDir(c *C) {
	deplDir := c.MkDir()

	{ // Ok
		got, err := createGroupDir(deplDir, config.Group{Name: "ukulele"})
		c.Check(err, IsNil)
		c.Check(got, Equals, filepath.Join(deplDir, "ukulele"))
		stat, err := os.Stat(got)
		c.Check(err, IsNil)
		c.Check(stat.IsDir(), Equals, true)
	}

	{ // Dir already exists
		dir := filepath.Join(deplDir, "guitar")
		c.Assert(os.Mkdir(dir, 0755), IsNil)
		got, err := createGroupDir(deplDir, config.Group{Name: "guitar"})
		c.Check(err, IsNil)
		c.Check(got, Equals, dir)
	}
}

func (s *zeroSuite) TestRestoreTfState(c *C) {
	// set up dir structure
	//
	// └── test_dir
	//    ├── .ghpc
	//       └── previous_resource_groups
	//          └── fake_resource_group
	//             └── terraform.tfstate
	//    └── fake_resource_group
	depDir := c.MkDir()
	groupName := "fake_resource_group"

	prevGroup := filepath.Join(HiddenGhpcDir(depDir), prevGroupDirName, groupName)
	curGroup := filepath.Join(depDir, groupName)
	prevStateFile := filepath.Join(prevGroup, tfStateFileName)
	prevBuStateFile := filepath.Join(prevGroup, tfStateBackupFileName)
	os.MkdirAll(prevGroup, 0755)
	os.MkdirAll(curGroup, 0755)
	emptyFile, _ := os.Create(prevStateFile)
	emptyFile.Close()
	emptyFile, _ = os.Create(prevBuStateFile)
	emptyFile.Close()

	testWriter := TFWriter{}
	testWriter.restoreState(depDir)

	// check state file was moved to current resource group dir
	curStateFile := filepath.Join(curGroup, tfStateFileName)
	curBuStateFile := filepath.Join(curGroup, tfStateBackupFileName)
	_, err := os.Stat(curStateFile)
	c.Check(err, IsNil)
	_, err = os.Stat(curBuStateFile)
	c.Check(err, IsNil)
}

func TestGetTypeTokensRelaxed(t *testing.T) {
	type test struct {
		input cty.Type
		want  string
	}
	tests := []test{
		{cty.Number, "number"},
		{cty.String, "string"},
		{cty.Bool, "bool"},
		{cty.Tuple([]cty.Type{}), "list(any)"},
		{cty.Tuple([]cty.Type{cty.String}), "list(any)"},
		{cty.List(cty.String), "list(any)"},
		{cty.Object(map[string]cty.Type{}), "any"},
		{cty.Object(map[string]cty.Type{"Lorum": cty.String}), "any"},
		{cty.Map(cty.String), "any"},
		{cty.DynamicPseudoType, "any"},
	}
	for _, tc := range tests {
		t.Run(typeexpr.TypeString(tc.input), func(t *testing.T) {
			got := string(getTypeTokens(relaxVarType(tc.input)).Bytes())
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("diff (-want +got):\n%s", diff)
			}
		})
	}
}

func (s *zeroSuite) TestWriteHclFile(c *C) {
	hclF := hclwrite.NewEmptyFile()
	hclF.Body().SetAttributeValue("zebra", cty.NumberIntVal(0))

	// Success
	baseFilename := "main.tf_TestCreateBaseFile"
	goodPath := filepath.Join(c.MkDir(), baseFilename)
	c.Assert(writeHclFile(goodPath, hclF), IsNil)

	fi, err := os.Stat(goodPath)
	c.Assert(err, IsNil)
	c.Assert(fi.Name(), Equals, baseFilename)
	c.Assert(fi.Size() > 0, Equals, true)
	c.Assert(fi.IsDir(), Equals, false)
	b, _ := os.ReadFile(goodPath)
	c.Assert(strings.Contains(string(b), "Licensed under the Apache License"),
		Equals, true)
	c.Assert(strings.Contains(string(b), "zebra"), Equals, true)

	// Error: not a correct path
	fakePath := filepath.Join("not/a/real/dir", "main.tf_TestCreateBaseFile")
	c.Assert(writeHclFile(fakePath, hclF), ErrorMatches, ".* no such file or directory")
}

func stringExistsInFile(str string, filename string) (bool, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), str), nil
}

func (s *zeroSuite) TestWriteMain(c *C) {
	// Setup
	testMainDir := c.MkDir()
	mainFilePath := filepath.Join(testMainDir, "main.tf")

	mods := []config.Module{{
		ID:     "test_module",
		Kind:   config.TerraformKind,
		Source: "modules/network/vpc",
		Settings: config.Dict{}.
			With("testSetting", cty.StringVal("testValue")).
			With("passthrough", config.MustParseExpression(`"${var.deployment_name}-allow"`).AsValue())}}

	be := config.TerraformBackend{
		Type:          "gcs",
		Configuration: config.Dict{}.With("bucket", cty.StringVal("a_bucket"))}

	noMods, noBe := []config.Module{}, config.TerraformBackend{}

	{ // Simple success
		err := writeMain(noMods, noBe, testMainDir)
		c.Check(err, IsNil)
	}

	{ // Test with modules
		err := writeMain(mods, noBe, testMainDir)
		c.Assert(err, IsNil)
		exists, err := stringExistsInFile("testSetting", mainFilePath)
		c.Assert(err, IsNil)
		c.Assert(exists, Equals, true)

		exists, err = stringExistsInFile(`"${var.deployment_name}-allow"`, mainFilePath)
		c.Assert(err, IsNil)
		c.Assert(exists, Equals, true)

		exists, err = stringExistsInFile(`("${var.deployment_name}-allow")`, mainFilePath)
		c.Assert(err, IsNil)
		c.Assert(exists, Equals, false)
	}

	{ // Test with Backend
		err := writeMain(mods, be, testMainDir)
		c.Assert(err, IsNil)
		exists, err := stringExistsInFile("a_bucket", mainFilePath)
		c.Assert(err, IsNil)
		c.Assert(exists, Equals, true)
	}
}

func (s *zeroSuite) TestWriteOutputs(c *C) {
	// Setup
	testOutputsDir := c.MkDir()
	outputsFilePath := filepath.Join(testOutputsDir, "outputs.tf")

	// Simple success, no modules
	testModules := []config.Module{}
	err := writeOutputs(testModules, testOutputsDir)
	c.Assert(err, IsNil)

	// Success: Outputs added
	outputList := []modulereader.OutputInfo{
		{Name: "output1"},
		{
			Name:      "output2",
			Sensitive: true,
		},
	}
	moduleWithOutputs := config.Module{Outputs: outputList, ID: "testMod"}
	testModules = []config.Module{moduleWithOutputs}
	err = writeOutputs(testModules, testOutputsDir)
	c.Assert(err, IsNil)

	exists, err := stringExistsInFile("output1", outputsFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
	exists, err = stringExistsInFile("output2", outputsFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Failure: Bad path
	err = writeOutputs(testModules, "not/a/real/path")
	c.Assert(err, ErrorMatches, ".*outputs.tf.*")

}

func (s *zeroSuite) TestWriteVariables(c *C) {
	// Setup
	testVarDir := c.MkDir()
	varsFilePath := filepath.Join(testVarDir, "variables.tf")

	noIntergroupVars := []modulereader.VarInfo{}

	// Simple success, empty vars
	testVars := make(map[string]cty.Value)
	err := writeVariables(testVars, noIntergroupVars, testVarDir)
	c.Assert(err, IsNil)

	// Failure: Bad path
	err = writeVariables(testVars, noIntergroupVars, "not/a/real/path")
	c.Assert(err, NotNil)

	// Success, common vars
	testVars["deployment_name"] = cty.StringVal("test_deployment")
	testVars["project_id"] = cty.StringVal("test_project")
	err = writeVariables(testVars, noIntergroupVars, testVarDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile("\"deployment_name\"", varsFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	// Success, "dynamic type"
	testVars = make(map[string]cty.Value)
	testVars["project_id"] = cty.NullVal(cty.DynamicPseudoType)
	err = writeVariables(testVars, noIntergroupVars, testVarDir)
	c.Assert(err, IsNil)
}

func (s *zeroSuite) TestWriteProviders(c *C) {
	// Setup
	dir := c.MkDir()
	providers := map[string]config.TerraformProvider{
		"elephant": config.TerraformProvider{
			Source:  "savannah/elephant",
			Version: "~> 8",
			Configuration: config.NewDict(map[string]cty.Value{
				"smeller":   config.GlobalRef("long").AsValue(),
				"listeners": config.GlobalRef("spacious").AsValue()})},
		"zebra": config.TerraformProvider{
			Source:        "hashicorp/zebra",
			Version:       "~> 2",
			Configuration: config.Dict{}}}
	{ // FAIL, non existing path
		c.Check(writeProviders(providers, "not/a/real/path"), NotNil)
	}

	{ // OK
		c.Check(writeProviders(providers, dir), IsNil)
		b, err := os.ReadFile(filepath.Join(dir, "providers.tf"))
		c.Assert(err, IsNil)
		c.Check(string(b), Equals, license+`
provider "elephant" {
  listeners = var.spacious
  smeller   = var.long
}

provider "zebra" {
}
`)
	}
}

func (s *zeroSuite) TestWriteVersions(c *C) {
	// Setup
	dir := c.MkDir()
	providers := map[string]config.TerraformProvider{
		"elephant": config.TerraformProvider{
			Source:  "savannah/elephant",
			Version: "~> 8",
			Configuration: config.NewDict(map[string]cty.Value{
				"smeller":   config.GlobalRef("long").AsValue(),
				"listeners": config.GlobalRef("spacious").AsValue()})},
		"zebra": config.TerraformProvider{
			Source:        "hashicorp/zebra",
			Version:       "~> 2",
			Configuration: config.Dict{}}}

	{ // FAIL, non existing path
		c.Check(writeVersions(providers, "not/a/real/path"), NotNil)
	}

	{ // OK
		c.Check(writeVersions(providers, dir), IsNil)
		b, err := os.ReadFile(filepath.Join(dir, "versions.tf"))
		c.Assert(err, IsNil)
		c.Check(string(b), Equals, license+`
terraform {
  required_version = ">= 1.12.2"

  required_providers {
    elephant = {
      source  = "savannah/elephant"
      version = "~> 8"
    }
    zebra = {
      source  = "hashicorp/zebra"
      version = "~> 2"
    }
  }
}
`)
	}
}

func (s *zeroSuite) TestKind(c *C) {
	tfw := TFWriter{}
	c.Assert(tfw.kind(), Equals, config.TerraformKind)
	pkrw := PackerWriter{}
	c.Assert(pkrw.kind(), Equals, config.PackerKind)
}

func (s *zeroSuite) TestWriteDeploymentGroup_PackerWriter(c *C) {
	deploymentio := deploymentio.GetDeploymentioLocal()
	testWriter := PackerWriter{}

	otherMod := config.Module{ID: "tortoise"}

	mod := config.Module{
		Kind: config.PackerKind,
		ID:   "prince",
		Settings: config.NewDict(map[string]cty.Value{
			"zebra":  cty.StringVal("checker"),                                      // const
			"salmon": config.GlobalRef("golf").AsValue(),                            // var
			"bear":   config.Reference{Module: otherMod.ID, Name: "rome"}.AsValue(), // IGC
		}),
	}

	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"golf": cty.NumberIntVal(17),
		}),
		Groups: []config.Group{
			{Name: "bread", Modules: []config.Module{otherMod}},
			{Name: "green", Modules: []config.Module{mod}},
		},
	}

	dir := c.MkDir()
	moduleDir := filepath.Join(dir, string(mod.ID))
	if err := deploymentio.CreateDirectory(moduleDir); err != nil {
		c.Fatal(err)
	}
	instructions := new(strings.Builder)

	c.Assert(testWriter.writeGroup(bp, 1, dir, instructions), IsNil)
	_, err := os.Stat(filepath.Join(moduleDir, packerAutoVarFilename))
	c.Assert(err, IsNil)
}

func (s *zeroSuite) TestWritePackerAutoVars(c *C) {
	vars := config.Dict{}.
		With("deployment_name", cty.StringVal("golf")).
		With("testkey", cty.False)

	// fail writing to a bad path
	badDestPath := "not/a/real/path"
	err := writePackerAutovars(vars.Items(), badDestPath)
	expErr := fmt.Sprintf(".*%s.*", packerAutoVarFilename)
	c.Assert(err, ErrorMatches, expErr)

	// success
	err = writePackerAutovars(vars.Items(), c.MkDir())
	c.Assert(err, IsNil)

}

func (s *zeroSuite) TestDeploymentSource(c *C) {
	{ // git
		m := config.Module{Kind: config.TerraformKind, Source: "github.com/x/y.git"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "github.com/x/y.git")
	}
	{ // packer
		m := config.Module{Kind: config.PackerKind, Source: "modules/packer/custom-image", ID: "image-id"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "image-id")
	}
	{ // remote packer non-package
		m := config.Module{Kind: config.PackerKind, Source: "github.com/GoogleCloudPlatform/modules/packer/custom-image", ID: "image-id"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "image-id")
	}
	{ // remote packer package
		m := config.Module{Kind: config.PackerKind, Source: "github.com/GoogleCloudPlatform//modules/packer/custom-image?ref=main", ID: "image-id"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "image-id/modules/packer/custom-image")
	}
	{ // embedded core
		m := config.Module{Kind: config.TerraformKind, Source: "modules/x/y"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "../"+config.SharedModulesDirName+"/embedded/modules/x/y")
	}
	{ // embedded community
		m := config.Module{Kind: config.TerraformKind, Source: "community/modules/x/y"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "../"+config.SharedModulesDirName+"/embedded/community/modules/x/y")
	}
	{ // local rel in repo
		m := config.Module{Kind: config.TerraformKind, Source: "./modules/x/y"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Matches, `^\./modules/y-\w\w\w\w$`)
	}
	{ // local rel
		m := config.Module{Kind: config.TerraformKind, Source: "./../../../../x/y"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Matches, `^\./modules/y-\w\w\w\w$`)
	}
	{ // local abs
		m := config.Module{Kind: config.TerraformKind, Source: "/tmp/x/y"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Matches, `^\./modules/y-\w\w\w\w$`)
	}
	{ // equivalent paths produce same hash after cleaning
		m1 := config.Module{Kind: config.TerraformKind, Source: "./modules/x/y"}
		m2 := config.Module{Kind: config.TerraformKind, Source: "./modules/x/z/../y"}
		s1, err1 := DeploymentSource(m1)
		c.Check(err1, IsNil)
		s2, err2 := DeploymentSource(m2)
		c.Check(err2, IsNil)
		c.Check(s1, Equals, s2)
	}
}

func (s *zeroSuite) TestSubstituteIgcReferencesInModule(c *C) {
	d := config.Dict{}.With("fold", cty.TupleVal([]cty.Value{
		cty.StringVal("zebra"),
		config.MustParseExpression(`module.golf.red + 6 + module.golf.green`).AsValue(),
		config.MustParseExpression(`module.tennis.brown`).AsValue(),
	}))
	m, err := SubstituteIgcReferencesInModule(
		config.Module{Settings: d},
		map[config.Reference]modulereader.VarInfo{
			config.ModuleRef("golf", "red"):   {Name: "pink"},
			config.ModuleRef("golf", "green"): {Name: "lime"},
		})
	c.Assert(err, IsNil)
	c.Check(m.Settings.Items(), DeepEquals, map[string]cty.Value{"fold": cty.TupleVal([]cty.Value{
		cty.StringVal("zebra"),
		config.MustParseExpression(`var.pink + 6 + var.lime`).AsValue(),
		config.MustParseExpression(`module.tennis.brown`).AsValue(),
	})})
}

func (s *zeroSuite) TestWritePackerDestroyInstructions(c *C) {
	{ // no manifest
		b := new(strings.Builder)
		WritePackerDestroyInstructions(b, nil)
		c.Check(b.String(), Equals, "")
	}
	{ // with manifest
		b := new(strings.Builder)
		WritePackerDestroyInstructions(b, []string{"Aldebaran", "Betelgeuse"})
		got := strings.ReplaceAll(b.String(), "\n", "") // one-line to simplify matcher
		c.Check(got, Matches, ".*Aldebaran.*Betelgeuse.*")
	}
}

func (s *zeroSuite) TestStagingDirConsistency(c *C) {
	// pkg/config can't use modulewriter, ensure consistency by testing.
	want := filepath.Join("..", HiddenGhpcDirName, "staged")
	c.Check(config.StagingDir, Equals, want)
}

func (s *zeroSuite) TestStageFile(c *C) {
	srcDir := c.MkDir()

	deplDir := c.MkDir()
	stagedDir := filepath.Join(deplDir, "staged")
	if err := os.Mkdir(stagedDir, 0755); err != nil {
		c.Fatal(err)
	}

	{ // src doesn't exist
		f := config.StagedFile{
			AbsSrc: filepath.Join(srcDir, "bush"),
			RelDst: "../staged/bush_44"}
		c.Assert(stageFile(deplDir, f), ErrorMatches, ".*bush does not exists.*")
	}

	{ // src exists, dst doesn't
		f := config.StagedFile{
			AbsSrc: filepath.Join(srcDir, "ugg"),
			RelDst: "../staged/ugg_44"}

		if err := os.WriteFile(f.AbsSrc, []byte("riddle"), 0644); err != nil {
			c.Fatal(err)
		}

		c.Assert(stageFile(deplDir, f), IsNil)
		dat, err := os.ReadFile(filepath.Join(deplDir, "any_group", f.RelDst))
		c.Assert(err, IsNil)
		c.Assert(string(dat), Equals, "riddle")
	}

	{ // src exists, dst exists and get overwritten
		f := config.StagedFile{
			AbsSrc: filepath.Join(srcDir, "clement"),
			RelDst: "../staged/clement_44"}

		if err := os.WriteFile(f.AbsSrc, []byte("barrel"), 0644); err != nil {
			c.Fatal(err)
		}

		absDst := filepath.Join(deplDir, "any_group", f.RelDst)
		if err := os.WriteFile(absDst, []byte("not_barrel"), 0644); err != nil {
			c.Fatal(err)
		}

		c.Assert(stageFile(deplDir, f), IsNil)
		dat, err := os.ReadFile(absDst)
		c.Assert(err, IsNil)
		c.Assert(string(dat), Equals, "barrel")
	}

	{ // src doesn't exists, but dst exists
		f := config.StagedFile{
			AbsSrc: filepath.Join(srcDir, "orange"),
			RelDst: "../staged/orange_44"}

		absDst := filepath.Join(deplDir, "any_group", f.RelDst)
		if err := os.WriteFile(absDst, []byte("pulp"), 0644); err != nil {
			c.Fatal(err)
		}

		c.Assert(stageFile(deplDir, f), IsNil)
		dat, err := os.ReadFile(absDst)
		c.Assert(err, IsNil)
		c.Assert(string(dat), Equals, "pulp")
	}

}

func (s *zeroSuite) TestWriteGcsDestroyInstructions(c *C) {
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id": cty.StringVal("my-project"),
		}),
		Groups: []config.Group{
			{
				Name: "group1",
				TerraformBackend: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.StringVal("my-bucket-name"),
					}),
				},
			},
		},
	}

	buf := new(bytes.Buffer)
	writeDestroyInstructions(buf, bp, "deployment-dir")
	out := buf.String()

	c.Check(strings.Contains(out, "my-bucket-name"), Equals, true)
}

func (s *zeroSuite) TestGetUniqueGcsBuckets_Errors(c *C) {
	bp := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "group_empty",
				TerraformBackend: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.StringVal(""),
					}),
				},
			},
		},
	}
	_, err := GetUniqueGcsBuckets(bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name for group \"group_empty\" cannot be empty")

	bp.Groups[0].Name = "group_null"
	bp.Groups[0].TerraformBackend.Configuration = config.NewDict(map[string]cty.Value{
		"bucket": cty.NullVal(cty.String),
	})
	_, err = GetUniqueGcsBuckets(bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name for group \"group_null\" cannot be empty or unknown")
}

func TestWriteModuleReconfigureOverrides(t *testing.T) {
	tmpDir := t.TempDir()

	mainTf := `
resource "google_container_node_pool" "node_pool" {
  name = "np1"
}
resource "google_container_node_pool" "subblock_node_pool" {
  name = "np2"
}
resource "null_resource" "dummy" {
  triggers = {}
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(mainTf), 0644); err != nil {
		t.Fatalf("failed to write main.tf: %v", err)
	}

	reconfigIgnores := map[string][]string{
		"google_container_node_pool": {
			"node_config[0].labels",
			"initial_node_count",
		},
		"null_resource.dummy": {
			"triggers",
		},
	}

	if err := writeModuleReconfigureOverrides(tmpDir, reconfigIgnores); err != nil {
		t.Fatalf("unexpected error from writeModuleReconfigureOverrides: %v", err)
	}

	overrideFile := filepath.Join(tmpDir, "override.tf.json")
	data, err := os.ReadFile(overrideFile)
	if err != nil {
		t.Fatalf("failed to read override.tf.json: %v", err)
	}

	var parsed tfOverrideFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal override.tf.json: %v", err)
	}

	np := parsed.Resource["google_container_node_pool"]["node_pool"]
	expectedNP := []string{"node_config[0].labels", "initial_node_count"}
	if !slices.Equal(np.Lifecycle.IgnoreChanges, expectedNP) {
		t.Errorf("got %v for node_pool, want %v", np.Lifecycle.IgnoreChanges, expectedNP)
	}

	snp := parsed.Resource["google_container_node_pool"]["subblock_node_pool"]
	if !slices.Equal(snp.Lifecycle.IgnoreChanges, expectedNP) {
		t.Errorf("got %v for subblock_node_pool, want %v", snp.Lifecycle.IgnoreChanges, expectedNP)
	}

	nr := parsed.Resource["null_resource"]["dummy"]
	expectedNR := []string{"triggers"}
	if !slices.Equal(nr.Lifecycle.IgnoreChanges, expectedNR) {
		t.Errorf("got %v for dummy null_resource, want %v", nr.Lifecycle.IgnoreChanges, expectedNR)
	}
}

func TestWriteModuleReconfigureOverrides_Validation(t *testing.T) {
	tmpDir := t.TempDir()

	mainTf := `
resource "google_container_node_pool" "node_pool" {
  name = "np1"
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(mainTf), 0644); err != nil {
		t.Fatalf("failed to write main.tf: %v", err)
	}

	// Target with invalid resource name should fail with explicit error
	reconfigIgnores := map[string][]string{
		"google_container_node_pool.nonexistent": {
			"node_config[0].labels",
		},
	}

	if err := writeModuleReconfigureOverrides(tmpDir, reconfigIgnores); err == nil {
		t.Fatalf("expected error for nonexistent resource target, got nil")
	}

	// Target with invalid resource type should fail with explicit error
	reconfigIgnoresType := map[string][]string{
		"invalid_resource_type": {
			"foo",
		},
	}

	if err := writeModuleReconfigureOverrides(tmpDir, reconfigIgnoresType); err == nil {
		t.Fatalf("expected error for invalid resource type target, got nil")
	}
}

func TestWriteModuleReconfigureOverrides_MergeWithExisting(t *testing.T) {
	tmpDir := t.TempDir()

	mainTf := `
resource "google_container_node_pool" "node_pool" {
  name = "np1"

  lifecycle {
    ignore_changes = [
      node_config[0].labels,
      placement_policy[0].type,
    ]
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(mainTf), 0644); err != nil {
		t.Fatalf("failed to write main.tf: %v", err)
	}

	reconfigIgnores := map[string][]string{
		"google_container_node_pool": {
			"initial_node_count",
			"node_config[0].labels", // Duplicate, should be deduplicated
		},
	}

	if err := writeModuleReconfigureOverrides(tmpDir, reconfigIgnores); err != nil {
		t.Fatalf("unexpected error from writeModuleReconfigureOverrides: %v", err)
	}

	overrideFile := filepath.Join(tmpDir, "override.tf.json")
	data, err := os.ReadFile(overrideFile)
	if err != nil {
		t.Fatalf("failed to read override.tf.json: %v", err)
	}

	var parsed tfOverrideFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal override.tf.json: %v", err)
	}

	np := parsed.Resource["google_container_node_pool"]["node_pool"]
	expected := []string{"node_config[0].labels", "placement_policy[0].type", "initial_node_count"}
	if diff := cmp.Diff(expected, np.Lifecycle.IgnoreChanges); diff != "" {
		t.Errorf("merged ignore_changes mismatch (-want +got):\n%s", diff)
	}
}

func TestCheckLocalModuleIgnoresConflict(t *testing.T) {
	modulesConflicting := []config.Module{
		{
			Kind:   config.TerraformKind,
			Source: "./local-mod",
			ReconfigureIgnores: map[string][]string{
				"res": {"field_a"},
			},
		},
		{
			Kind:   config.TerraformKind,
			Source: "./local-mod",
			ReconfigureIgnores: map[string][]string{
				"res": {"field_b"},
			},
		},
	}
	if err := checkLocalModuleIgnoresConflict(modulesConflicting); err == nil {
		t.Fatalf("expected error for conflicting local module ignores, got nil")
	}

	modulesMatching := []config.Module{
		{
			Kind:   config.TerraformKind,
			Source: "./local-mod",
			ReconfigureIgnores: map[string][]string{
				"res": {"field_a"},
			},
		},
		{
			Kind:   config.TerraformKind,
			Source: "./local-mod",
			ReconfigureIgnores: map[string][]string{
				"res": {"field_a"},
			},
		},
	}
	if err := checkLocalModuleIgnoresConflict(modulesMatching); err != nil {
		t.Fatalf("unexpected error for matching local module ignores: %v", err)
	}
}

func TestWriteModuleReconfigureOverrides_IgnoreChangesAll(t *testing.T) {
	tmpDir := t.TempDir()

	mainTf := `
resource "google_compute_disk" "disk" {
  name = "disk"
  lifecycle {
    ignore_changes = all
  }
}
`
	if err := os.WriteFile(filepath.Join(tmpDir, "main.tf"), []byte(mainTf), 0644); err != nil {
		t.Fatalf("failed to write main.tf: %v", err)
	}

	reconfigIgnores := map[string][]string{
		"google_compute_disk": {
			"labels",
		},
	}

	if err := writeModuleReconfigureOverrides(tmpDir, reconfigIgnores); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Because resource already ignores all changes, no conflicting list override should be emitted
	overrideFile := filepath.Join(tmpDir, "override.tf.json")
	if _, err := os.Stat(overrideFile); !os.IsNotExist(err) {
		t.Errorf("expected override.tf.json to not be created for ignore_changes = all, but it was")
	}
}

func TestFormatTargetNotFoundError(t *testing.T) {
	discovered := map[string][]string{
		"google_compute_disk": {"disk_a", "disk_b"},
	}
	err := formatTargetNotFoundError("google_compute_disk.wrong", "/path/to/mod", discovered)
	expectedSub := "Available resources in this module: google_compute_disk.disk_a, google_compute_disk.disk_b"
	if !strings.Contains(err.Error(), expectedSub) {
		t.Errorf("got error %q, want substring %q", err.Error(), expectedSub)
	}
}

func TestNormalizeIgnoreAttribute(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"metadata['ssh-keys']", `metadata["ssh-keys"]`},
		{"metadata[ 'ssh-keys' ]", `metadata["ssh-keys"]`},
		{"node_config[0].labels['app']", `node_config[0].labels["app"]`},
		{`node_config[0].labels["app"]`, `node_config[0].labels["app"]`},
		{"labels", "labels"},
	}
	for _, tc := range tests {
		got := normalizeIgnoreAttribute(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeIgnoreAttribute(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

func TestValidateModuleIgnoresRemoteSource(t *testing.T) {
	bpRemote := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "remote_mod",
						Kind:   config.TerraformKind,
						Source: "git::https://github.com/terraform-google-modules/network.git",
						ReconfigureIgnores: map[string][]string{
							"google_compute_network": {"mtu"},
						},
					},
				},
			},
		},
	}
	if err := validateModuleIgnoresRemoteSource(bpRemote); err == nil {
		t.Fatalf("expected error for remote module with reconfigure_ignores, got nil")
	}

	bpLocal := config.Blueprint{
		Groups: []config.Group{
			{
				Name: "primary",
				Modules: []config.Module{
					{
						ID:     "local_mod",
						Kind:   config.TerraformKind,
						Source: "modules/network/vpc",
						ReconfigureIgnores: map[string][]string{
							"google_compute_network": {"mtu"},
						},
					},
				},
			},
		},
	}
	if err := validateModuleIgnoresRemoteSource(bpLocal); err != nil {
		t.Fatalf("unexpected error for embedded module: %v", err)
	}
}

func TestReconfigIgnoresEqual_Unordered(t *testing.T) {
	a := map[string][]string{
		"google_compute_disk": {"size", "labels", "metadata['ssh-keys']"},
	}
	b := map[string][]string{
		"google_compute_disk": {"labels", `metadata["ssh-keys"]`, "size"},
	}
	if !reconfigIgnoresEqual(a, b) {
		t.Errorf("reconfigIgnoresEqual(a, b) = false, want true for unordered and quote-normalized matches")
	}

	c := map[string][]string{
		"google_compute_disk": {"size", "type"},
	}
	if reconfigIgnoresEqual(a, c) {
		t.Errorf("reconfigIgnoresEqual(a, c) = true, want false for different attributes")
	}
}

func TestWriteModuleReconfigureOverrides_TypeAndSpecificMerge(t *testing.T) {
	dir := t.TempDir()
	mainTf := `
resource "google_compute_disk" "disk_a" {
  name = "disk-a"
  type = "pd-standard"
}
`
	if err := os.WriteFile(filepath.Join(dir, "main.tf"), []byte(mainTf), 0644); err != nil {
		t.Fatal(err)
	}

	ignores := map[string][]string{
		"google_compute_disk":        {"type"},
		"google_compute_disk.disk_a": {"size"},
	}

	if err := writeModuleReconfigureOverrides(dir, ignores); err != nil {
		t.Fatalf("writeModuleReconfigureOverrides failed: %v", err)
	}

	overrideBytes, err := os.ReadFile(filepath.Join(dir, "override.tf.json"))
	if err != nil {
		t.Fatalf("failed to read override.tf.json: %v", err)
	}

	var parsed tfOverrideFile
	if err := json.Unmarshal(overrideBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal override.tf.json: %v", err)
	}

	diskA, ok := parsed.Resource["google_compute_disk"]["disk_a"]
	if !ok {
		t.Fatalf("expected override for google_compute_disk.disk_a")
	}

	expected := []string{"type", "size"}
	if !stringSlicesEqual(diskA.Lifecycle.IgnoreChanges, expected) && !stringSlicesEqual(diskA.Lifecycle.IgnoreChanges, []string{"size", "type"}) {
		t.Errorf("got ignore_changes %v, want union of type and specific ignores (%v)", diskA.Lifecycle.IgnoreChanges, expected)
	}
}
