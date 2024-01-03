/*
Copyright 2022 Google LLC

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
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	. "gopkg.in/check.v1"
)

type MySuite struct {
	testDir            string
	terraformModuleDir string
}

func (s *MySuite) SetUpSuite(c *C) {
	s.testDir = c.MkDir()

	// Create dummy module in testDir
	s.terraformModuleDir = filepath.Join(s.testDir, "tfModule")
	if err := os.Mkdir(s.terraformModuleDir, 0755); err != nil {
		c.Fatal(err)
	}
}

// Test Data Producers
func (s *MySuite) getDeploymentConfigForTest() config.DeploymentConfig {
	testModule := config.Module{
		Source: s.terraformModuleDir,
		Kind:   config.TerraformKind,
		ID:     "testModule",
		Settings: config.NewDict(map[string]cty.Value{
			"deployment_name": cty.NilVal,
			"project_id":      cty.NilVal,
		}),
		Outputs: []modulereader.OutputInfo{
			{
				Name:        "test-output",
				Description: "",
				Sensitive:   false,
			},
		},
	}
	testModuleWithLabels := config.Module{
		Source: s.terraformModuleDir,
		ID:     "testModuleWithLabels",
		Kind:   config.TerraformKind,
		Settings: config.NewDict(map[string]cty.Value{
			"moduleLabel": cty.StringVal("moduleLabelValue"),
		}),
	}
	testDeploymentGroups := []config.DeploymentGroup{
		{
			Name:    "test_resource_group",
			Modules: []config.Module{testModule, testModuleWithLabels},
		},
	}
	return config.DeploymentConfig{
		Config: config.Blueprint{
			BlueprintName: "simple",
			Vars: config.NewDict(map[string]cty.Value{
				"deployment_name": cty.StringVal("deployment_name"),
				"project_id":      cty.StringVal("test-project"),
			}),
			DeploymentGroups: testDeploymentGroups,
		},
	}
}

type zeroSuite struct{}

var _ = []any{ // initialize suites
	Suite(&MySuite{}),
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

	prevModuleDir := filepath.Join(ghpcDir, prevDeploymentGroupDirName)
	if _, err := os.Stat(prevModuleDir); os.IsNotExist(err) {
		return fmt.Errorf("previous deployment group directory does not exist: %s: %w", prevModuleDir, err)
	}

	return nil
}

func (s *MySuite) TestPrepDepDir(c *C) {
	depDir := filepath.Join(s.testDir, "dep_prep_test_dir")

	// Prep a dir that does not yet exist
	c.Check(prepDepDir(depDir), IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)

	// Prep of existing dir succeeds
	c.Check(prepDepDir(depDir), IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)
}

func (s *MySuite) TestPrepDepDir_OverwriteRealDep(c *C) {
	// Test with a real deployment previously written
	testDC := s.getDeploymentConfigForTest()
	testDC.Config.Vars.Set("deployment_name", cty.StringVal("test_prep_dir"))
	depDir := filepath.Join(s.testDir, "test_prep_dir")

	// writes a full deployment w/ actual resource groups
	WriteDeployment(testDC, depDir)

	// confirm existence of resource groups (beyond .ghpc dir)
	files, _ := os.ReadDir(depDir)
	c.Check(len(files) > 1, Equals, true)

	err := prepDepDir(depDir)
	c.Check(err, IsNil)
	c.Check(isDeploymentDirPrepped(depDir), IsNil)

	// Check prev resource groups were moved
	prevModuleDir := filepath.Join(depDir, ".ghpc", prevDeploymentGroupDirName)
	files1, _ := os.ReadDir(prevModuleDir)
	c.Check(len(files1) > 0, Equals, true)

	files2, _ := os.ReadDir(depDir)
	c.Check(files2, HasLen, 3) // .ghpc, .gitignore, and instructions file
}

// modulewriter.go
func (s *MySuite) TestWriteDeployment(c *C) {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("modules/red/pink", 0755)
	afero.WriteFile(aferoFS, "modules/red/pink/main.tf", []byte("pink"), 0644)
	aferoFS.MkdirAll("community/modules/green/lime", 0755)
	afero.WriteFile(aferoFS, "community/modules/green/lime/main.tf", []byte("lime"), 0644)
	sourcereader.ModuleFS = afero.NewIOFS(aferoFS)

	dc := s.getDeploymentConfigForTest()
	dir := filepath.Join(s.testDir, "test_write_deployment")

	c.Check(WriteDeployment(dc, dir), IsNil)
	// Overwriting the deployment succeeds
	c.Check(WriteDeployment(dc, dir), IsNil)
}

func (s *MySuite) TestCreateGroupDir(c *C) {
	deplDir := c.MkDir()

	{ // Ok
		got, err := createGroupDir(deplDir, config.DeploymentGroup{Name: "ukulele"})
		c.Check(err, IsNil)
		c.Check(got, Equals, filepath.Join(deplDir, "ukulele"))
		stat, err := os.Stat(got)
		c.Check(err, IsNil)
		c.Check(stat.IsDir(), Equals, true)
	}

	{ // Dir already exists
		dir := filepath.Join(deplDir, "guitar")
		c.Assert(os.Mkdir(dir, 0755), IsNil)
		got, err := createGroupDir(deplDir, config.DeploymentGroup{Name: "guitar"})
		c.Check(err, IsNil)
		c.Check(got, Equals, dir)
	}
}

func (s *MySuite) TestRestoreTfState(c *C) {
	// set up dir structure
	//
	// └── test_restore_state
	//    ├── .ghpc
	//       └── previous_resource_groups
	//          └── fake_resource_group
	//             └── terraform.tfstate
	//    └── fake_resource_group
	depDir := filepath.Join(s.testDir, "test_restore_state")
	deploymentGroupName := "fake_resource_group"

	prevDeploymentGroup := filepath.Join(HiddenGhpcDir(depDir), prevDeploymentGroupDirName, deploymentGroupName)
	curDeploymentGroup := filepath.Join(depDir, deploymentGroupName)
	prevStateFile := filepath.Join(prevDeploymentGroup, tfStateFileName)
	prevBuStateFile := filepath.Join(prevDeploymentGroup, tfStateBackupFileName)
	os.MkdirAll(prevDeploymentGroup, 0755)
	os.MkdirAll(curDeploymentGroup, 0755)
	emptyFile, _ := os.Create(prevStateFile)
	emptyFile.Close()
	emptyFile, _ = os.Create(prevBuStateFile)
	emptyFile.Close()

	testWriter := TFWriter{}
	testWriter.restoreState(depDir)

	// check state file was moved to current resource group dir
	curStateFile := filepath.Join(curDeploymentGroup, tfStateFileName)
	curBuStateFile := filepath.Join(curDeploymentGroup, tfStateBackupFileName)
	_, err := os.Stat(curStateFile)
	c.Check(err, IsNil)
	_, err = os.Stat(curBuStateFile)
	c.Check(err, IsNil)
}

func (s *zeroSuite) TestGetTypeTokens(c *C) {
	// Success Integer
	tok := getTypeTokens(cty.NumberIntVal(-1))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	tok = getTypeTokens(cty.NumberIntVal(0))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	tok = getTypeTokens(cty.NumberIntVal(1))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	// Success Float
	tok = getTypeTokens(cty.NumberFloatVal(-99.9))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	tok = getTypeTokens(cty.NumberFloatVal(99.9))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("number")))

	// Success String
	tok = getTypeTokens(cty.StringVal("Lorum"))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("string")))

	tok = getTypeTokens(cty.StringVal(""))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("string")))

	// Success Bool
	tok = getTypeTokens(cty.BoolVal(true))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("bool")))

	tok = getTypeTokens(cty.BoolVal(false))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("bool")))

	// Success tuple
	tok = getTypeTokens(cty.TupleVal([]cty.Value{}))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("list")))

	tok = getTypeTokens(cty.TupleVal([]cty.Value{cty.StringVal("Lorum")}))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("list")))

	// Success list
	tok = getTypeTokens(cty.ListVal([]cty.Value{cty.StringVal("Lorum")}))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("list")))

	// Success object
	tok = getTypeTokens(cty.ObjectVal(map[string]cty.Value{}))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

	val := cty.ObjectVal(map[string]cty.Value{"Lorum": cty.StringVal("Ipsum")})
	tok = getTypeTokens(val)
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

	// Success Map
	val = cty.MapVal(map[string]cty.Value{"Lorum": cty.StringVal("Ipsum")})
	tok = getTypeTokens(val)
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))

	// Success any
	tok = getTypeTokens(cty.NullVal(cty.DynamicPseudoType))
	c.Assert(tok, HasLen, 1)
	c.Assert(string(tok[0].Bytes), Equals, string([]byte("any")))
}

func (s *MySuite) TestCreateBaseFile(c *C) {
	// Success
	baseFilename := "main.tf_TestCreateBaseFile"
	goodPath := filepath.Join(s.testDir, baseFilename)
	err := createBaseFile(goodPath)
	c.Assert(err, IsNil)
	fi, err := os.Stat(goodPath)
	c.Assert(err, IsNil)
	c.Assert(fi.Name(), Equals, baseFilename)
	c.Assert(fi.Size() > 0, Equals, true)
	c.Assert(fi.IsDir(), Equals, false)
	b, _ := os.ReadFile(goodPath)
	c.Assert(strings.Contains(string(b), "Licensed under the Apache License"),
		Equals, true)

	// Error: not a correct path
	fakePath := filepath.Join("not/a/real/dir", "main.tf_TestCreateBaseFile")
	err = createBaseFile(fakePath)
	c.Assert(err, ErrorMatches, ".* no such file or directory")
}

func (s *MySuite) TestAppendHCLToFile(c *C) {
	// Setup
	testFilename := "main.tf_TestAppendHCLToFile"
	testPath := filepath.Join(s.testDir, testFilename)
	_, err := os.Create(testPath)
	c.Assert(err, IsNil)
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()
	hclBody.SetAttributeValue("dummyAttributeName", cty.NumberIntVal(0))

	// Success
	err = appendHCLToFile(testPath, hclFile.Bytes())
	c.Assert(err, IsNil)
}

func stringExistsInFile(str string, filename string) (bool, error) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return false, err
	}
	return strings.Contains(string(b), str), nil
}

func (s *MySuite) TestWriteMain(c *C) {
	// Setup
	testMainDir := filepath.Join(s.testDir, "TestWriteMain")
	mainFilePath := filepath.Join(testMainDir, "main.tf")
	if err := os.Mkdir(testMainDir, 0755); err != nil {
		c.Fatal("Failed to create test dir for creating main.tf file")
	}

	// Simple success
	testModules := []config.Module{}
	testBackend := config.TerraformBackend{}
	err := writeMain(testModules, testBackend, testMainDir)
	c.Assert(err, IsNil)

	// Test with modules
	testModule := config.Module{
		ID:     "test_module",
		Kind:   config.TerraformKind,
		Source: "modules/network/vpc",
		Settings: config.NewDict(map[string]cty.Value{
			"testSetting": cty.StringVal("testValue"),
			"passthrough": config.MustParseExpression(`"${var.deployment_name}-allow"`).AsValue(),
		}),
	}
	testModules = append(testModules, testModule)
	err = writeMain(testModules, testBackend, testMainDir)
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

	// Test with Backend
	testBackend.Type = "gcs"
	testBackend.Configuration.Set("bucket", cty.StringVal("a_bucket"))

	err = writeMain(testModules, testBackend, testMainDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("a_bucket", mainFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s *MySuite) TestWriteOutputs(c *C) {
	// Setup
	testOutputsDir := filepath.Join(s.testDir, "TestWriteOutputs")
	outputsFilePath := filepath.Join(testOutputsDir, "outputs.tf")
	if err := os.Mkdir(testOutputsDir, 0755); err != nil {
		c.Fatal("Failed to create test directory for creating outputs.tf file")
	}

	// Simple success, no modules
	testModules := []config.Module{}
	err := writeOutputs(testModules, testOutputsDir)
	c.Assert(err, IsNil)

	// Success: Outputs added
	outputList := []modulereader.OutputInfo{
		{Name: "output1"},
		{Name: "output2"},
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
	c.Assert(err, ErrorMatches, "error creating outputs.tf file: .*")

}

func (s *MySuite) TestWriteVariables(c *C) {
	// Setup
	testVarDir := filepath.Join(s.testDir, "TestWriteVariables")
	varsFilePath := filepath.Join(testVarDir, "variables.tf")
	if err := os.Mkdir(testVarDir, 0755); err != nil {
		c.Fatal("Failed to create test directory for creating variables.tf file")
	}

	noIntergroupVars := []modulereader.VarInfo{}

	// Simple success, empty vars
	testVars := make(map[string]cty.Value)
	err := writeVariables(testVars, noIntergroupVars, testVarDir)
	c.Assert(err, IsNil)

	// Failure: Bad path
	err = writeVariables(testVars, noIntergroupVars, "not/a/real/path")
	c.Assert(err, ErrorMatches, "error creating variables.tf file: .*")

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

func (s *MySuite) TestWriteProviders(c *C) {
	// Setup
	testProvDir := c.MkDir()
	provFilePath := filepath.Join(testProvDir, "providers.tf")

	// Simple success, empty vars
	testVars := make(map[string]cty.Value)
	err := writeProviders(testVars, testProvDir)
	c.Assert(err, IsNil)
	exists, err := stringExistsInFile("google-beta", provFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
	exists, err = stringExistsInFile("project", provFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	// Failure: Bad Path
	err = writeProviders(testVars, "not/a/real/path")
	c.Assert(err, ErrorMatches, "error creating providers.tf file: .*")

	// Success: All vars
	testVars["project_id"] = cty.StringVal("test_project")
	testVars["zone"] = cty.StringVal("test_zone")
	testVars["region"] = cty.StringVal("test_region")
	err = writeProviders(testVars, testProvDir)
	c.Assert(err, IsNil)
	exists, err = stringExistsInFile("var.region", provFilePath)
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)
}

func (s *zeroSuite) TestKind(c *C) {
	tfw := TFWriter{}
	c.Assert(tfw.kind(), Equals, config.TerraformKind)
	pkrw := PackerWriter{}
	c.Assert(pkrw.kind(), Equals, config.PackerKind)
}

func (s *MySuite) TestWriteDeploymentGroup_PackerWriter(c *C) {
	deploymentio := deploymentio.GetDeploymentioLocal()
	testWriter := PackerWriter{}

	// No Packer modules
	deploymentName := "deployment_TestWriteModuleLevel_PackerWriter"
	deploymentDir := filepath.Join(s.testDir, deploymentName)
	if err := deploymentio.CreateDirectory(deploymentDir); err != nil {
		c.Fatal(err)
	}
	groupDir := filepath.Join(deploymentDir, "packerGroup")
	if err := deploymentio.CreateDirectory(groupDir); err != nil {
		c.Fatal(err)
	}
	moduleDir := filepath.Join(groupDir, "testPackerModule")
	if err := deploymentio.CreateDirectory(moduleDir); err != nil {
		c.Fatal(err)
	}

	testPackerModule := config.Module{
		Kind: config.PackerKind,
		ID:   "testPackerModule",
	}
	testDeploymentGroup := config.DeploymentGroup{
		Name:    "packerGroup",
		Modules: []config.Module{testPackerModule},
	}

	testDC := config.DeploymentConfig{
		Config: config.Blueprint{
			DeploymentGroups: []config.DeploymentGroup{
				testDeploymentGroup,
			},
		},
	}
	f, err := os.CreateTemp("", "tmpf")
	if err != nil {
		c.Fatal()
	}
	defer os.Remove(f.Name())
	testWriter.writeDeploymentGroup(testDC, 0, groupDir, f)
	_, err = os.Stat(filepath.Join(moduleDir, packerAutoVarFilename))
	c.Assert(err, IsNil)
}

func (s *MySuite) TestWritePackerAutoVars(c *C) {
	vars := config.Dict{}
	vars.
		Set("deployment_name", cty.StringVal("golf")).
		Set("testkey", cty.False)

	// fail writing to a bad path
	badDestPath := "not/a/real/path"
	err := writePackerAutovars(vars.Items(), badDestPath)
	expErr := fmt.Sprintf("error creating variables file %s:.*", packerAutoVarFilename)
	c.Assert(err, ErrorMatches, expErr)

	// success
	err = writePackerAutovars(vars.Items(), c.MkDir())
	c.Assert(err, IsNil)

}

func (s *zeroSuite) TestStringEscape(c *C) {
	f := func(s string) string {
		toks := config.TokensForValue(cty.StringVal(s))
		return string(toks.Bytes())
	}
	// LiteralVariables
	c.Check(f(`\((not.var))`), Equals, `"((not.var))"`)
	c.Check(f(`abc\((not.var))abc`), Equals, `"abc((not.var))abc"`)
	c.Check(f(`abc \((not.var)) abc`), Equals, `"abc ((not.var)) abc"`)
	c.Check(f(`abc \((not.var1)) abc \((not.var2)) abc`), Equals, `"abc ((not.var1)) abc ((not.var2)) abc"`)
	c.Check(f(`abc \\((escape.backslash))`), Equals, `"abc \\((escape.backslash))"`)

	// BlueprintVariables
	c.Check(f(`\$(not.var)`), Equals, `"$(not.var)"`)
	c.Check(f(`abc\$(not.var)abc`), Equals, `"abc$(not.var)abc"`)
	c.Check(f(`abc \$(not.var) abc`), Equals, `"abc $(not.var) abc"`)
	c.Check(f(`abc \$(not.var1) abc \$(not.var2) abc`), Equals, `"abc $(not.var1) abc $(not.var2) abc"`)
	c.Check(f(`abc \\$(escape.backslash)`), Equals, `"abc \\$(escape.backslash)"`)

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
		c.Check(s, Equals, "./modules/embedded/modules/x/y")
	}
	{ // embedded community
		m := config.Module{Kind: config.TerraformKind, Source: "community/modules/x/y"}
		s, err := DeploymentSource(m)
		c.Check(err, IsNil)
		c.Check(s, Equals, "./modules/embedded/community/modules/x/y")
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
}

func (s *zeroSuite) TestSubstituteIgcReferencesInModule(c *C) {
	d := config.Dict{}
	d.Set("fold", cty.TupleVal([]cty.Value{
		cty.StringVal("zebra"),
		config.MustParseExpression(`module.golf.red + 6 + module.golf.green`).AsValue(),
		config.MustParseExpression(`module.tennis.brown`).AsValue(),
	}))
	m := SubstituteIgcReferencesInModule(
		config.Module{Settings: d},
		map[config.Reference]modulereader.VarInfo{
			config.ModuleRef("golf", "red"):   {Name: "pink"},
			config.ModuleRef("golf", "green"): {Name: "lime"},
		})
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
