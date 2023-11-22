// Copyright 2022 Google LLC
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

package modulereader

import (
	"embed"
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	. "gopkg.in/check.v1"
	"gopkg.in/yaml.v3"
)

const (
	pkrKindString = "packer"
	tfKindString  = "terraform"
)

//go:embed modules
var testModuleFS embed.FS

type MySuite struct {
	tmpModuleDir string
	terraformDir string
	packerDir    string
}

func (s *MySuite) SetUpSuite(c *C) {
	var err error
	if s.tmpModuleDir, err = os.MkdirTemp("", "modulereader_tests_*"); err != nil {
		c.Fatal(err)
	}
	sourcereader.ModuleFS = testModuleFS
	rdr := sourcereader.EmbeddedSourceReader{}
	if err = rdr.CopyDir("modules", s.tmpModuleDir); err != nil {
		c.Fatal(err)
	}

	s.terraformDir = filepath.Join(s.tmpModuleDir, "test_role", "test_module")
	s.packerDir = filepath.Join(s.tmpModuleDir, "imaginarium", "zebra")
}

func (s *MySuite) TearDownSuite(c *C) {
	if err := os.RemoveAll(s.tmpModuleDir); err != nil {
		c.Fatal(err)
	}
}

type zeroSuite struct{}

var _ = []any{ // initialize suites
	Suite(&MySuite{}),
	Suite(&zeroSuite{})}

func Test(t *testing.T) {
	TestingT(t)
}

func (s *zeroSuite) TestGetOutputsAsMap(c *C) {
	// Simple: empty outputs
	modInfo := ModuleInfo{}
	outputMap := modInfo.GetOutputsAsMap()
	c.Assert(len(outputMap), Equals, 0)

	testDescription := "This is a test description"
	testName := "testName"
	outputInfo := OutputInfo{Name: testName, Description: testDescription}
	modInfo.Outputs = []OutputInfo{outputInfo}
	outputMap = modInfo.GetOutputsAsMap()
	c.Assert(len(outputMap), Equals, 1)
	c.Assert(outputMap[testName].Description, Equals, testDescription)
}

func (s *zeroSuite) TestFactory(c *C) {
	pkrReader := Factory(pkrKindString)
	c.Assert(reflect.TypeOf(pkrReader), Equals, reflect.TypeOf(PackerReader{}))
	tfReader := Factory(tfKindString)
	c.Assert(reflect.TypeOf(tfReader), Equals, reflect.TypeOf(TFReader{}))
}

func (s *MySuite) TestGetModuleInfo_Embedded(c *C) {
	{ // Success
		mi, err := GetModuleInfo("modules/test_role/test_module", tfKindString)
		c.Assert(err, IsNil)
		c.Check(mi, DeepEquals, ModuleInfo{
			Inputs: []VarInfo{{
				Name:        "test_variable",
				Type:        "string",
				Description: "This is just a test",
				Required:    true}},
			Outputs: []OutputInfo{{
				Name:        "test_output",
				Description: "This is just a test",
				Sensitive:   false}},
			Metadata: Metadata{
				Spec: MetadataSpec{
					Requirements: MetadataRequirements{
						Services: []string{
							"room.service.vip",
							"protection.service.GCPD",
						}}}}})
	}

	{ // Invalid: No embedded modules
		_, err := GetModuleInfo("modules/does/not/exist", tfKindString)
		c.Check(err, ErrorMatches, "failed to get info using tfconfig for terraform module at .*")
	}

	{ // Invalid: Unsupported source
		_, err := GetModuleInfo("wut::hpc-toolkit/modules", tfKindString)
		c.Check(err, NotNil)
	}
}

func (s *zeroSuite) TestGetModuleInfo_Git(c *C) {

	// Invalid git repository - path does not exists
	badGitRepo := "github.com:not/exist.git"
	_, err := GetModuleInfo(badGitRepo, tfKindString)
	c.Check(err, NotNil)

	// Invalid: Unsupported Module Source
	badSource := "gcs::https://www.googleapis.com/storage/v1/GoogleCloudPlatform/hpc-toolkit/modules"
	_, err = GetModuleInfo(badSource, tfKindString)
	c.Check(err, NotNil)
}

func (s *MySuite) TestGetModuleInfo_Local(c *C) {
	{ // Success
		mi, err := GetModuleInfo(s.terraformDir, tfKindString)
		c.Assert(err, IsNil)
		c.Check(mi, DeepEquals, ModuleInfo{
			Inputs: []VarInfo{{
				Name:        "test_variable",
				Type:        "string",
				Description: "This is just a test",
				Required:    true}},
			Outputs: []OutputInfo{{
				Name:        "test_output",
				Description: "This is just a test",
				Sensitive:   false}},
			Metadata: Metadata{
				Spec: MetadataSpec{
					Requirements: MetadataRequirements{
						Services: []string{
							"room.service.vip",
							"protection.service.GCPD",
						}}}}})
	}

	{ // Invalid source path - path does not exists
		_, err := GetModuleInfo("./not/a/real/path", tfKindString)
		c.Assert(err, ErrorMatches, "failed to get info using tfconfig for terraform module at .*")
	}

	{ // Invalid: Unsupported Module Source
		_, err := GetModuleInfo("wut:://hpc-toolkit/modules", tfKindString)
		c.Assert(err, NotNil)
	}
}

func (s *MySuite) TestGetHCLInfo(c *C) {
	// Invalid source path - path does not exists
	fakePath := "./not/a/real/path"
	_, err := getHCLInfo(fakePath)
	expectedErr := "source to module does not exist: .*"
	c.Assert(err, ErrorMatches, expectedErr)
	// Invalid source path - points to a file
	pathToFile := filepath.Join(s.terraformDir, "main.tf")
	_, err = getHCLInfo(pathToFile)
	expectedErr = "source of module must be a directory: .*"
	c.Assert(err, ErrorMatches, expectedErr)

	// Invalid source path - points to directory with no .tf files
	pathToEmptyDir := filepath.Join(s.packerDir, "emptyDir")
	err = os.Mkdir(pathToEmptyDir, 0755)
	if err != nil {
		c.Fatal("TestGetHCLInfo: Failed to create test directory.")
	}
	_, err = getHCLInfo(pathToEmptyDir)
	expectedErr = "source is not a terraform or packer module: .*"
	c.Assert(err, ErrorMatches, expectedErr)
}

func (s *MySuite) TestGetInfo_TFReder(c *C) {
	reader := NewTFReader()
	info, err := reader.GetInfo(s.terraformDir)
	c.Assert(err, IsNil)
	c.Check(info, DeepEquals, ModuleInfo{
		Inputs:  []VarInfo{{Name: "test_variable", Type: "string", Description: "This is just a test", Required: true}},
		Outputs: []OutputInfo{{Name: "test_output", Description: "This is just a test"}},
	})

}

func (s *MySuite) TestGetInfo_PackerReader(c *C) {
	reader := NewPackerReader()
	exp := ModuleInfo{
		Inputs: []VarInfo{{
			Name:        "test_variable",
			Type:        "string",
			Description: "This is just a test",
			Required:    true}}}

	{ // Didn't already exist, succeeds
		info, err := reader.GetInfo(s.packerDir)
		c.Assert(err, IsNil)
		c.Check(info, DeepEquals, exp)
	}

	{ // Already exists, succeeds
		info, err := reader.GetInfo(s.packerDir)
		c.Assert(err, IsNil)
		c.Check(info, DeepEquals, exp)
	}
}

func (s *zeroSuite) TestGetInfo_MetaReader(c *C) {
	// Not implemented, expect that error
	reader := MetaReader{}
	_, err := reader.GetInfo("")
	expErr := "meta GetInfo not implemented: .*"
	c.Assert(err, ErrorMatches, expErr)
}

// module outputs can be specified as a simple string for the output name or as
// a YAML mapping of name/description/sensitive (str,str,bool)
func (s *zeroSuite) TestUnmarshalOutputInfo(c *C) {
	var oinfo OutputInfo
	var y string

	y = "foo"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "", Sensitive: false})

	y = "{ name: foo }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "", Sensitive: false})

	y = "{ name: foo, description: bar }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "bar", Sensitive: false})

	y = "{ name: foo, description: bar, sensitive: true }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), IsNil)
	c.Check(oinfo, DeepEquals, OutputInfo{Name: "foo", Description: "bar", Sensitive: true})

	// extra key should generate error
	y = "{ name: foo, description: bar, sensitive: true, extrakey: extraval }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)

	// missing required key name should generate error
	y = "{ description: bar, sensitive: true }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)

	// should not ummarshal a sequence
	y = "[ foo ]"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)

	// should not ummarshal an object with non-boolean sensitive type
	y = "{ name: foo, description: bar, sensitive: contingent }"
	c.Check(yaml.Unmarshal([]byte(y), &oinfo), NotNil)
}
