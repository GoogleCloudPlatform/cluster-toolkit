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

package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"hpc-toolkit/pkg/modulereader"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

// Suite that does not use any setup
type zeroSuite struct{}

// register suites
var _ = Suite(&zeroSuite{})

func Test(t *testing.T) {
	TestingT(t) // run all registered suites
}

// TODO: consider making it immutable
type modBuilder struct {
	m Module
	i modulereader.ModuleInfo
}

func tMod(id ModuleID) *modBuilder {
	return &modBuilder{m: Module{
		ID:   id,
		Kind: TerraformKind,
	}}
}

func (b *modBuilder) uses(id ...ModuleID) *modBuilder {
	b.m.Use = append(b.m.Use, id...)
	return b
}

func (b *modBuilder) set(s string, val any) *modBuilder {
	var tv cty.Value
	switch v := val.(type) {
	case string:
		tv = cty.StringVal(v)
	case cty.Value:
		tv = v
	case Reference:
		tv = v.AsValue()
	case Expression:
		tv = v.AsValue()
	default:
		panic(fmt.Sprintf("unsupported type %T", val))
	}
	b.m.Settings = b.m.Settings.With(s, tv)
	return b
}

func (b *modBuilder) outputs(o ...string) *modBuilder {
	for _, v := range o {
		b.i.Outputs = append(b.i.Outputs, modulereader.OutputInfo{Name: v})
	}
	return b
}

func (b *modBuilder) inputs(i ...interface{}) *modBuilder {
	for _, v := range i {
		var vi modulereader.VarInfo
		switch v := v.(type) {
		case string:
			vi = modulereader.VarInfo{Name: v, Type: cty.String}
		case modulereader.VarInfo:
			vi = v
		default:
			panic(fmt.Sprintf("unsupported type %T", v))
		}
		b.i.Inputs = append(b.i.Inputs, vi)
	}
	return b
}

func (b *modBuilder) packer() *modBuilder {
	b.m.Kind = PackerKind
	return b
}

var modBuilderCounter = 0

func (b modBuilder) build() Module {
	b.m.Source = fmt.Sprintf("./test_mods/%s_%d", b.m.ID, modBuilderCounter)
	modBuilderCounter++
	modulereader.SetModuleInfo(b.m.Source, b.m.Kind.String(), b.i)
	return b.m
}

func (s *zeroSuite) TestExpand(c *C) {
	mod := tMod("red").inputs("oval").set("oval", "square").build()

	bp := Blueprint{
		BlueprintName: "smurf",
		Vars:          NewDict(map[string]cty.Value{"deployment_name": cty.StringVal("green")}),
		Groups: []Group{{
			Name:    "abel",
			Modules: []Module{mod},
		}},
	}

	c.Check(bp.Expand(), IsNil)
}

func (s *zeroSuite) TestCheckModulesAndGroups(c *C) {
	pony := tMod("pony").build()
	zebra := tMod("zebra").packer().build()

	{ // Duplicate module id same group
		g := Group{Name: "ice", Modules: []Module{pony, pony}}
		err := checkModulesAndGroups(Blueprint{Groups: []Group{g}})
		c.Check(err, ErrorMatches, ".*pony.* used more than once")
	}
	{ // Duplicate module id different groups
		ice := Group{Name: "ice", Modules: []Module{pony}}
		fire := Group{Name: "fire", Modules: []Module{pony}}
		err := checkModulesAndGroups(Blueprint{Groups: []Group{ice, fire}})
		c.Check(err, ErrorMatches, ".*pony.* used more than once")
	}
	{ // Duplicate group name
		ice := Group{Name: "ice", Modules: []Module{pony}}
		ice9 := Group{Name: "ice", Modules: []Module{zebra}}
		err := checkModulesAndGroups(Blueprint{Groups: []Group{ice, ice9}})
		c.Check(err, ErrorMatches, ".*ice.* used more than once")
	}
	{ // Mixing module kinds
		g := Group{Name: "ice", Modules: []Module{pony, zebra}}
		err := checkModulesAndGroups(Blueprint{Groups: []Group{g}})
		c.Check(err, NotNil)
	}
	{ // Empty group
		g := Group{Name: "ice"}
		err := checkModulesAndGroups(Blueprint{Groups: []Group{g}})
		c.Check(err, NotNil)
	}
}

func (s *zeroSuite) TestListUnusedModules(c *C) {
	{ // No modules in "use"
		m := tMod("m").build()
		c.Check(m.ListUnusedModules(), DeepEquals, ModuleIDs{})
	}

	{ // Useful
		m := tMod("m").
			uses("w").
			set("x", AsProductOfModuleUse(cty.True, "w")).build()
		c.Check(m.ListUnusedModules(), DeepEquals, ModuleIDs{})
	}

	{ // Unused
		m := tMod("m").
			uses("w", "u").
			set("x", AsProductOfModuleUse(cty.True, "w")).build()
		c.Check(m.ListUnusedModules(), DeepEquals, ModuleIDs{"u"})
	}
}

func (s *zeroSuite) TestListUnusedVariables(c *C) {
	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"deployment_name": cty.StringVal("green"),
			"labels":          cty.False,
			"flathead_screw":  cty.NumberIntVal(1),
			"pony":            cty.NumberIntVal(2),
			"stripes":         cty.NumberIntVal(3),
			"zebra":           MustParseExpression("var.pony + var.stripes").AsValue(),
		}),
		Groups: []Group{{Modules: []Module{{
			Settings: NewDict(map[string]cty.Value{
				"circus": GlobalRef("pony").AsValue(),
			}),
		}}}},
		Validators: []Validator{{
			Inputs: NewDict(map[string]cty.Value{
				"savannah": GlobalRef("zebra").AsValue(),
			})}}}
	c.Check(bp.ListUnusedVariables(), DeepEquals, []string{"flathead_screw"})
}

func (s *zeroSuite) TestAddKindToModules(c *C) {
	bp := Blueprint{
		Groups: []Group{
			{Modules: []Module{{ID: "grain"}}}}}
	mod := &bp.Groups[0].Modules[0]

	mod.Kind = ModuleKind{} // kind is absent, set to terraform
	bp.addKindToModules()
	c.Check(mod.Kind, Equals, TerraformKind)

	mod.Kind = UnknownKind // kind is unknown, same as absent
	bp.addKindToModules()
	c.Check(mod.Kind, Equals, TerraformKind)

	mod.Kind = PackerKind // does nothing to packer types
	bp.addKindToModules()
	c.Check(mod.Kind, Equals, PackerKind)

	mod.Kind = ModuleKind{"red"} // does nothing to invalid kind
	bp.addKindToModules()
	c.Check(mod.Kind, Equals, ModuleKind{"red"})
}

func (s *zeroSuite) TestGetModule(c *C) {
	bp := Blueprint{
		Groups: []Group{{
			Modules: []Module{{ID: "blue"}}}},
	}
	{
		m, err := bp.Module("blue")
		c.Check(err, IsNil)
		c.Check(m, Equals, &bp.Groups[0].Modules[0])
	}
	{
		m, err := bp.Module("red")
		c.Check(err, NotNil)
		c.Check(m, IsNil)
	}
}

func (s *zeroSuite) TestValidateDeploymentName(c *C) {
	var e InputValueError

	h := func(val cty.Value) error {
		vars := NewDict(map[string]cty.Value{"deployment_name": val})
		return validateDeploymentName(Blueprint{Vars: vars})
	}

	// Is deployment_name a valid string?
	c.Check(h(cty.StringVal("yellow")), IsNil)

	{ // Is deployment_name an empty string?
		err := h(cty.StringVal(""))
		c.Check(errors.As(err, &e), Equals, true)
	}

	{ // Is deployment_name not a string?
		err := h(cty.NumberIntVal(100))
		c.Check(errors.As(err, &e), Equals, true)
	}

	{ // Is deployment_names longer than 63 characters?
		err := h(cty.StringVal("deployment_name-deployment_name-deployment_name-deployment_name-0123"))
		c.Check(errors.As(err, &e), Equals, true)
	}

	{ // Does deployment_name contain special characters other than dashes or underscores?
		err := h(cty.StringVal("deployment.name"))
		c.Check(errors.As(err, &e), Equals, true)
	}

	{ // Does deployment_name contain capital letters?
		err := h(cty.StringVal("Deployment_name"))
		c.Check(errors.As(err, &e), Equals, true)
	}

	{ // Is deployment_name not set?
		err := validateDeploymentName(Blueprint{})
		c.Check(errors.As(err, &e), Equals, true)
	}

	{ // Expression
		c.Check(h(MustParseExpression(`"arbuz-${5}"`).AsValue()), IsNil)
	}
}

func (s *zeroSuite) TestCheckBlueprintName(c *C) {
	bp := Blueprint{}
	var e InputValueError

	// Is blueprint_name a valid string with an underscore and dash?
	bp.BlueprintName = "blue-print_name"
	c.Check(bp.checkBlueprintName(), IsNil)

	// Is blueprint_name an empty string?
	bp.BlueprintName = ""
	c.Check(errors.As(bp.checkBlueprintName(), &e), Equals, true)

	// Is blueprint_name longer than 63 characters?
	bp.BlueprintName = "blueprint-name-blueprint-name-blueprint-name-blueprint-name-0123"
	c.Check(errors.As(bp.checkBlueprintName(), &e), Equals, true)

	// Does blueprint_name contain special characters other than dashes or underscores?
	bp.BlueprintName = "blueprint.name"
	c.Check(errors.As(bp.checkBlueprintName(), &e), Equals, true)

	// Does blueprint_name contain capital letters?
	bp.BlueprintName = "Blueprint_name"
	c.Check(errors.As(bp.checkBlueprintName(), &e), Equals, true)
}

func (s *zeroSuite) TestCheckToolkitModulesUrlAndVersion(c *C) {
	bp := Blueprint{}
	var e HintError

	// Are toolkit_modules_url and toolkit_modules_version both provided?
	bp.ToolkitModulesURL = "github.com/GoogleCloudPlatform/cluster-toolkit"
	bp.ToolkitModulesVersion = "v1.15.0"
	c.Check(bp.checkToolkitModulesUrlAndVersion(), IsNil)

	// Are toolkit_modules_url and toolkit_modules_version both empty?
	bp.ToolkitModulesURL = ""
	bp.ToolkitModulesVersion = ""
	c.Check(bp.checkToolkitModulesUrlAndVersion(), IsNil)

	// Is toolkit_modules_url provided and toolkit_modules_version empty?
	bp.ToolkitModulesURL = "github.com/GoogleCloudPlatform/cluster-toolkit"
	bp.ToolkitModulesVersion = ""
	c.Check(errors.As(bp.checkToolkitModulesUrlAndVersion(), &e), Equals, true)

	// Is toolkit_modules_version provided and toolkit_modules_url empty?
	bp.ToolkitModulesURL = ""
	bp.ToolkitModulesVersion = "v1.15.0"
	c.Check(errors.As(bp.checkToolkitModulesUrlAndVersion(), &e), Equals, true)
}

func (s *zeroSuite) TestNewBlueprint(c *C) {
	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"deployment_name": cty.StringVal("zebra")}),
		Groups: []Group{{
			Modules: []Module{{
				ID: "pony"}}}},
	}

	outFile := filepath.Join(c.MkDir(), c.TestName()+".yaml")
	c.Assert(bp.Export(outFile), IsNil)
	newBp, _, err := NewBlueprint(outFile)
	c.Assert(err, IsNil)

	bp.path = outFile // set expected path
	// NewBlueprint populates a runtime-only YamlCtx (positions in source YAML).
	// Reflect that in the expected blueprint before doing a DeepEquals compare.
	bp.YamlCtx = newBp.YamlCtx
	c.Assert(bp, DeepEquals, newBp)
}

func (s *zeroSuite) TestNewDeploymentSettings(c *C) {
	dir := c.MkDir()
	h := func(data string) (DeploymentSettings, YamlCtx, error) {
		f, err := os.CreateTemp(dir, "*.yaml")
		c.Assert(err, IsNil)
		_, err = f.Write([]byte(data))
		c.Assert(err, IsNil)
		f.Close()
		return NewDeploymentSettings(f.Name())
	}

	{ // OK
		ds, _, err := h(`
vars:
  project_id: ds-project
terraform_backend_defaults:
  type: gcs
  configuration:
    bucket: ds-tf-state-bucket
`)
		c.Assert(err, IsNil)
		c.Check(ds, DeepEquals, DeploymentSettings{
			Vars: NewDict(map[string]cty.Value{"project_id": cty.StringVal("ds-project")}),
			TerraformBackendDefaults: TerraformBackend{
				Type: "gcs",
				Configuration: NewDict(map[string]cty.Value{
					"bucket": cty.StringVal("ds-tf-state-bucket"),
				}),
			}})
	}

	{ // empty
		_, _, err := h("")
		c.Check(err, NotNil)
	}

	{ // invalid
		_, _, err := h(`
not_a_field: not_a_value
`)
		c.Check(err, NotNil)
	}
}

func (s *zeroSuite) TestValidateGlobalLabels(c *C) {

	labelName := "my_test_label_name"
	labelValue := cty.StringVal("my-valid-label-value")
	invalidName := "my_test_label_name_with_a_bad_char!"
	nameErr := ".*invalid label name.*"
	invalidLabelValue := "some/long/path/with/invalid/characters/and/with/more/than/63/characters!"

	h := func(val cty.Value) error {
		vars := NewDict(map[string]cty.Value{"labels": val})
		return validateGlobalLabels(Blueprint{Vars: vars})
	}

	{ // No labels
		c.Check(validateGlobalLabels(Blueprint{}), IsNil)
	}

	{ // Simple success case
		l := cty.MapVal(map[string]cty.Value{
			labelName: labelValue})
		c.Check(h(l), IsNil)
	}

	{ // Succeed on empty value
		l := cty.MapVal(map[string]cty.Value{
			labelName: cty.StringVal("")})
		c.Check(h(l), IsNil)
	}

	{ // Succeed on lowercase international character
		l := cty.MapVal(map[string]cty.Value{
			"ñ" + labelName: cty.StringVal("ñ")})
		c.Check(h(l), IsNil)
	}

	{ // Succeed on case-less international character
		l := cty.MapVal(map[string]cty.Value{
			"ƿ" + labelName: cty.StringVal("ƿ"), // Unicode 01BF, latin character "wynn"
		})
		c.Check(h(l), IsNil)
	}

	{ // Succeed on max number of labels
		largeLabelsMap := map[string]cty.Value{}
		for i := 0; i < 64; i++ {
			largeLabelsMap[labelName+"_"+fmt.Sprint(i)] = labelValue
		}
		c.Check(h(cty.MapVal(largeLabelsMap)), IsNil)
	}

	{ // Invalid label name
		err := h(cty.MapVal(map[string]cty.Value{
			invalidName: labelValue}))
		c.Check(err, ErrorMatches, nameErr)
	}

	{ // Invalid label value
		err := h(cty.MapVal(map[string]cty.Value{
			labelName: cty.StringVal(invalidLabelValue),
		}))
		c.Check(err, ErrorMatches, fmt.Sprintf(`.*value.*'%s: %s'.*`,
			regexp.QuoteMeta(labelName),
			regexp.QuoteMeta(invalidLabelValue)))
	}

	{ // Too many labels
		tooManyLabelsMap := map[string]cty.Value{}
		for i := 0; i < maxLabels+1; i++ {
			tooManyLabelsMap[labelName+"_"+fmt.Sprint(i)] = labelValue
		}
		c.Check(h(cty.MapVal(tooManyLabelsMap)), NotNil)
	}

	{ // Fail on uppercase international character
		err := h(cty.MapVal(map[string]cty.Value{
			labelName: cty.StringVal("Ñ"),
		}))
		c.Check(err, ErrorMatches, fmt.Sprintf(`.*value.*'%s: %s'.*`,
			regexp.QuoteMeta(labelName),
			regexp.QuoteMeta("Ñ")))
	}

	{ // Fail on empty name
		err := h(cty.MapVal(map[string]cty.Value{
			"": labelValue}))
		c.Check(err, ErrorMatches, nameErr)
	}

	{ // OK, big expression
		err := h(MustParseExpression(`2 + 5`).AsValue())
		c.Check(err, IsNil)
	}
	{ // OK, small expression
		err := h(cty.ObjectVal(map[string]cty.Value{
			"alpha": cty.StringVal("a"),
			"beta":  GlobalRef("boba").AsValue(),
		}))
		c.Check(err, IsNil)
	}
	{ // FAIL, small expression with bad name
		err := h(cty.ObjectVal(map[string]cty.Value{
			"alpha":     cty.StringVal("a"),
			invalidName: GlobalRef("boba").AsValue(),
		}))
		c.Check(err, ErrorMatches, nameErr)
	}
}

func (s *zeroSuite) TestParseBlueprint_ExtraField_ThrowsError(c *C) {
	// should fail on strict unmarshal as field does not match schema
	_, _, err := parseYaml[Blueprint]([]byte(`
blueprint_name: hpc-cluster-high-io
# line below is not in our schema
dragon: "Lews Therin Telamon"`))
	c.Check(err, NotNil)
}

func (s *zeroSuite) TestExportBlueprint(c *C) {
	bp := Blueprint{BlueprintName: "goo"}
	outFilename := c.TestName() + ".yaml"
	outFile := filepath.Join(c.MkDir(), outFilename)
	c.Assert(bp.Export(outFile), IsNil)
	fileInfo, err := os.Stat(outFile)
	c.Assert(err, IsNil)
	c.Assert(fileInfo.Name(), Equals, outFilename)
	c.Assert(fileInfo.Size() > 0, Equals, true)
	c.Assert(fileInfo.IsDir(), Equals, false)
}

func (s *zeroSuite) TestCheckMovedModules(c *C) {
	// base case should not err
	c.Check(checkMovedModule("some/module/that/has/not/moved"), IsNil)

	// embedded moved
	c.Check(checkMovedModule("community/modules/scheduler/cloud-batch-job"), NotNil)
}

func (s *zeroSuite) TestCheckStringLiteral(c *C) {
	p := Root.BlueprintName // some path

	{ // OK. Absent
		c.Check(checkStringLiteral(p, ""), IsNil)
	}
	{ // OK. No expressions
		c.Check(checkStringLiteral(p, "who"), IsNil)
	}

	{ // FAIL. Expression in type
		c.Check(checkStringLiteral(p, "$(vartype)"), NotNil)
	}

	{ // FAIL. HCL literal
		c.Check(checkStringLiteral(p, "((var.zen))"), NotNil)
	}

	{ // OK. Not an expression
		c.Check(checkStringLiteral(p, "\\$(vartype)"), IsNil)
	}
}

func (s *zeroSuite) TestCheckProviders(c *C) {
	p := Root.Groups.At(173).Provider

	{ // OK. Absent
		c.Check(checkProviders(p, map[string]TerraformProvider{}), IsNil)
	}

	{ // OK. All required values used
		tp := map[string]TerraformProvider{
			"test-provider": {
				Source:  "test-src",
				Version: "test-ver",
				Configuration: Dict{}.
					With("project", cty.StringVal("test-prj")).
					With("region", cty.StringVal("reg1")).
					With("zone", cty.StringVal("zone1")).
					With("universe_domain", cty.StringVal("test-universe.com"))}}
		c.Check(checkProviders(p, tp), IsNil)
	}

	{ // FAIL. Missing Source
		tp := map[string]TerraformProvider{
			"test-provider": {
				Version: "test-ver",
				Configuration: Dict{}.
					With("project", cty.StringVal("test-prj")).
					With("region", cty.StringVal("reg1")).
					With("zone", cty.StringVal("zone1")).
					With("universe_domain", cty.StringVal("test-universe.com"))}}
		c.Check(checkProviders(p, tp), NotNil)
	}

	{ // FAIL. Missing Version
		tp := map[string]TerraformProvider{
			"test-provider": {
				Source: "test-src",
				Configuration: Dict{}.
					With("project", cty.StringVal("test-prj")).
					With("region", cty.StringVal("reg1")).
					With("zone", cty.StringVal("zone1")).
					With("universe_domain", cty.StringVal("test-universe.com"))}}
		c.Check(checkProviders(p, tp), NotNil)
	}
}

func (s *zeroSuite) TestSkipValidator(c *C) {
	{
		bp := Blueprint{Validators: nil}
		bp.SkipValidator("zebra")
		c.Check(bp.Validators, DeepEquals, []Validator{
			{Validator: "zebra", Skip: true}})
	}
	{
		bp := Blueprint{Validators: []Validator{
			{Validator: "pony"}}}
		bp.SkipValidator("zebra")
		c.Check(bp.Validators, DeepEquals, []Validator{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}
	{
		bp := Blueprint{Validators: []Validator{
			{Validator: "pony"},
			{Validator: "zebra"}}}
		bp.SkipValidator("zebra")
		c.Check(bp.Validators, DeepEquals, []Validator{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}
	{
		bp := Blueprint{Validators: []Validator{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}}}
		bp.SkipValidator("zebra")
		c.Check(bp.Validators, DeepEquals, []Validator{
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}
	{
		bp := Blueprint{Validators: []Validator{
			{Validator: "zebra"},
			{Validator: "pony"},
			{Validator: "zebra"}}}
		bp.SkipValidator("zebra")
		c.Check(bp.Validators, DeepEquals, []Validator{
			{Validator: "zebra", Skip: true},
			{Validator: "pony"},
			{Validator: "zebra", Skip: true}})
	}

}

func (s *zeroSuite) TestModuleGroup(c *C) {
	bp := Blueprint{
		Groups: []Group{
			{Modules: []Module{
				{ID: "Waldo"},
			}}}}

	{
		got := bp.ModuleGroupOrDie("Waldo")
		c.Check(got, DeepEquals, bp.Groups[0])
	}

	{
		_, err := bp.ModuleGroup("Woof")
		c.Check(err, NotNil)
	}
}

func (s *zeroSuite) TestValidateModuleSettingReference(c *C) {
	mod11 := tMod("mod11").outputs("out11").build()
	mod21 := tMod("mod21").outputs("out21").build()
	mod22 := tMod("mod22").outputs("out22").build()
	pkr := tMod("pkr").packer().outputs("outPkr").build()

	bp := Blueprint{
		Vars: NewDict(map[string]cty.Value{
			"var1": cty.True,
		}),
		Groups: []Group{
			{Name: "group1", Modules: []Module{mod11}},
			{Name: "groupP", Modules: []Module{pkr}},
			{Name: "group2", Modules: []Module{mod21, mod22}},
		},
	}

	vld := validateModuleSettingReference
	// OK. deployment var
	c.Check(vld(bp, mod11, GlobalRef("var1")), IsNil)

	// FAIL. deployment var doesn't exist
	c.Check(vld(bp, mod11, GlobalRef("var2")), NotNil)

	// FAIL. wrong module
	c.Check(vld(bp, mod11, ModuleRef("jack", "kale")), NotNil)

	// OK. intragroup
	c.Check(vld(bp, mod22, ModuleRef("mod21", "out21")), IsNil)

	// OK. intragroup. out of module order
	c.Check(vld(bp, mod21, ModuleRef("mod22", "out22")), IsNil)

	// OK. intergroup
	c.Check(vld(bp, mod22, ModuleRef("mod11", "out11")), IsNil)

	// FAIL. out of group order
	c.Check(vld(bp, mod11, ModuleRef("mod21", "out21")), NotNil)

	// FAIL. missing output
	c.Check(vld(bp, mod22, ModuleRef("mod21", "kale")), NotNil)

	// FAIL. packer module
	c.Check(vld(bp, mod21, ModuleRef("pkr", "outPkr")), NotNil)

	// FAIL. get global hint
	mod := ModuleID("var")
	unkModErr := UnknownModuleError{mod}
	c.Check(errors.Is(vld(bp, mod11, ModuleRef(mod, "kale")), HintError{`did you mean "vars"?`, unkModErr}), Equals, true)

	// FAIL. get module ID hint
	mod = ModuleID("pkp")
	unkModErr = UnknownModuleError{mod}
	c.Check(errors.Is(vld(bp, mod11, ModuleRef(mod, "kale")), HintError{fmt.Sprintf("did you mean %q?", string(pkr.ID)), unkModErr}), Equals, true)

	// FAIL. get no hint
	mod = ModuleID("test")
	unkModErr = UnknownModuleError{mod}
	c.Check(errors.Is(vld(bp, mod11, ModuleRef(mod, "kale")), HintError{fmt.Sprintf("did you mean %q?", string(pkr.ID)), unkModErr}), Equals, false)
	c.Check(errors.Is(vld(bp, mod11, ModuleRef(mod, "kale")), unkModErr), Equals, true)
}

func (s *zeroSuite) TestValidateModuleSettingReferences(c *C) {
	m := Module{
		ID: "m",
		Settings: Dict{}.
			With("white", GlobalRef("zebra").AsValue())}
	p := Root.Groups.At(0).Modules.At(0)

	{ // No zebra
		c.Check(validateModuleSettingReferences(p, m, Blueprint{}), NotNil)
	}

	{ // Got zebra
		bp := Blueprint{Vars: Dict{}.
			With("zebra", cty.StringVal("stripes"))}
		c.Check(validateModuleSettingReferences(p, m, bp), IsNil)
	}
}

func (s *zeroSuite) TestGroupNameValidate(c *C) {
	// Invalid
	c.Check(GroupName("").Validate(), NotNil)
	c.Check(GroupName("-").Validate(), NotNil)
	c.Check(GroupName("-g").Validate(), NotNil)
	c.Check(GroupName("g-").Validate(), NotNil)
	c.Check(GroupName("g+").Validate(), NotNil)
	c.Check(GroupName("a b").Validate(), NotNil)

	// Valid
	c.Check(GroupName("g").Validate(), IsNil)
	c.Check(GroupName("gg").Validate(), IsNil)
	c.Check(GroupName("_g").Validate(), IsNil)
	c.Check(GroupName("g_dd").Validate(), IsNil)
	c.Check(GroupName("g_dd-ff").Validate(), IsNil)
	c.Check(GroupName("g-dd_ff").Validate(), IsNil)
	c.Check(GroupName("1").Validate(), IsNil)
	c.Check(GroupName("12g").Validate(), IsNil)
}

func (s *zeroSuite) TestEvalVars(c *C) {
	{ // OK
		vars := NewDict(map[string]cty.Value{
			"a":  cty.StringVal("A"),
			"b":  MustParseExpression(`"${var.a}_B"`).AsValue(),
			"c":  MustParseExpression(`"${var.b}_C"`).AsValue(),
			"bc": MustParseExpression(`"${var.b}|${var.c}"`).AsValue(),
		})
		bp := Blueprint{Vars: vars}
		got, err := bp.evalVars()
		c.Check(err, IsNil)
		c.Check(got.Items(), DeepEquals, map[string]cty.Value{
			"a":  cty.StringVal("A"),
			"b":  cty.StringVal("A_B"),
			"c":  cty.StringVal("A_B_C"),
			"bc": cty.StringVal("A_B|A_B_C"),
		})
		c.Check(bp.Vars.Items(), DeepEquals, map[string]cty.Value{ // no change
			"a":  cty.StringVal("A"),
			"b":  MustParseExpression(`"${var.a}_B"`).AsValue(),
			"c":  MustParseExpression(`"${var.b}_C"`).AsValue(),
			"bc": MustParseExpression(`"${var.b}|${var.c}"`).AsValue(),
		})
	}
	{ // Non global ref
		vars := NewDict(map[string]cty.Value{
			"a": cty.StringVal("A"),
			"b": MustParseExpression(`"${var.a}_${module.foo.ko}"`).AsValue(),
		})
		_, err := (&Blueprint{Vars: vars}).evalVars()
		var berr BpError
		if errors.As(err, &berr) {
			c.Check(berr.Path.String(), Equals, "vars.b")
		} else {
			c.Error(err, " should be BpError")
		}
	}

	{ // Cycle
		vars := NewDict(map[string]cty.Value{
			"uro": MustParseExpression(`"uro_${var.bo}_${var.ros}"`).AsValue(),
			"bo":  cty.StringVal("===="),
			"ros": MustParseExpression(`"${var.uro}_${var.bo}_ros"`).AsValue(),
		})
		_, err := (&Blueprint{Vars: vars}).evalVars()
		var berr BpError
		if errors.As(err, &berr) {
			if berr.Path.String() != "vars.uro" && berr.Path.String() != "vars.ros" {
				c.Error(berr, " should point to vars.uro or vars.ros")
			}
		} else {
			c.Error(err, " should be BpError")
		}
	}

	{ // Non-computable
		vars := NewDict(map[string]cty.Value{
			"uro": MustParseExpression("DoesHalt(var.bo)").AsValue(),
			"bo":  cty.StringVal("01_10"),
		})
		_, err := (&Blueprint{Vars: vars}).evalVars()
		var berr BpError
		if errors.As(err, &berr) {
			c.Check(berr.Error(), Matches, ".*unsupported function.*DoesHalt.*")
			c.Check(berr.Path.String(), Equals, "vars.uro")
		} else {
			c.Error(err, " should be BpError")
		}
	}
}

func (s *zeroSuite) TestValidateSlurmClusterName(c *C) {
	var e InputValueError

	h := func(val cty.Value) error {
		vars := NewDict(map[string]cty.Value{"slurm_cluster_name": val})
		return validateSlurmClusterName(Blueprint{Vars: vars})
	}

	// Valid slurm_cluster_name examples
	c.Check(h(cty.StringVal("a")), IsNil)                    // single lowercase letter
	c.Check(h(cty.StringVal("abc123")), IsNil)               // letters and numbers
	c.Check(h(cty.StringVal("slurm-cluster")), IsNil)        // hyphens
	c.Check(h(cty.StringVal("a-123456789012345678")), IsNil) // 20 chars

	{ // Is slurm_cluster_name an empty string?
		err := h(cty.StringVal(""))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*empty string.*")
	}

	{ // Is slurm_cluster_name not a string?
		err := h(cty.NumberIntVal(100))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*not.*string.*")
	}

	{ // Is slurm_cluster_name longer than 20 characters? (Updated from 10)
		err := h(cty.StringVal("slurm-12345678901234567890"))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*between 1 and 20 characters.*")
	}

	{ // Does slurm_cluster_name contain uppercase letters?
		err := h(cty.StringVal("Slurm"))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*lowercase letter.*")
	}

	{ // Does slurm_cluster_name start with a number?
		err := h(cty.StringVal("1slurm"))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*start with a lowercase letter.*")
	}

	{ // Does slurm_cluster_name start with a hyphen? (Invalid based on ^[a-z])
		err := h(cty.StringVal("-slurm"))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*start with a lowercase letter.*")
	}

	{ // Does slurm_cluster_name contain special characters (underscore)?
		err := h(cty.StringVal("slurm_gke"))
		c.Check(errors.As(err, &e), Equals, true)
		// Updated message to include "hyphens"
		c.Check(err, ErrorMatches, ".*lowercase letters, numbers and hyphens.*")
	}

	{ // Does slurm_cluster_name contain special characters (period)?
		err := h(cty.StringVal("slurm.gke"))
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*lowercase letters, numbers and hyphens.*")
	}

	{ // Is slurm_cluster_name not set? (should pass - it's optional)
		err := validateSlurmClusterName(Blueprint{})
		c.Check(err, IsNil)
	}

	{ // Expression (should pass if it evaluates correctly)
		c.Check(h(MustParseExpression(`"slurm-${1}"`).AsValue()), IsNil)
	}

	{ // Expression that results in invalid value
		err := h(MustParseExpression(`"Slurm-${1}"`).AsValue())
		c.Check(errors.As(err, &e), Equals, true)
		c.Check(err, ErrorMatches, ".*lowercase letter.*")
	}
}
