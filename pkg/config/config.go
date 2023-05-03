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

// Package config manages and updates the ghpc input config
package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	"gopkg.in/yaml.v3"

	"hpc-toolkit/pkg/modulereader"
)

const (
	expectedVarFormat        string = "$(vars.var_name) or $(module_id.output_name)"
	expectedModFormat        string = "$(module_id) or $(group_id.module_id)"
	unexpectedConnectionKind string = "connectionKind must be useConnection or deploymentConnection"
)

var errorMessages = map[string]string{
	// general
	"appendToNonList": "cannot append to a setting whose type is not a list",
	// config
	"fileLoadError":      "failed to read the input yaml",
	"yamlUnmarshalError": "failed to parse the blueprint in %s, check YAML syntax for errors, err=%w",
	"yamlMarshalError":   "failed to export the configuration to a blueprint yaml file",
	"fileSaveError":      "failed to write the expanded yaml",
	// expand
	"missingSetting":       "a required setting is missing from a module",
	"globalLabelType":      "deployment variable 'labels' are not a map",
	"settingsLabelType":    "labels in module settings are not a map",
	"invalidVar":           "invalid variable definition in",
	"invalidMod":           "invalid module reference",
	"invalidDeploymentRef": "invalid deployment-wide reference (only \"vars\") is supported)",
	"varNotFound":          "Could not find source of variable",
	"intergroupOrder":      "References to outputs from other groups must be to earlier groups",
	"referenceWrongGroup":  "Reference specified the wrong group for the module",
	"noOutput":             "Output not found for a variable",
	"groupNotFound":        "The group ID was not found",
	"cannotUsePacker":      "Packer modules cannot be used by other modules",
	// validator
	"emptyID":            "a module id cannot be empty",
	"emptySource":        "a module source cannot be empty",
	"wrongKind":          "a module kind is invalid",
	"extraSetting":       "a setting was added that is not found in the module",
	"settingWithPeriod":  "a setting name contains a period, which is not supported; variable subfields cannot be set independently in a blueprint.",
	"settingInvalidChar": "a setting name must begin with a non-numeric character and all characters must be either letters, numbers, dashes ('-') or underscores ('_').",
	"duplicateGroup":     "group names must be unique",
	"duplicateID":        "module IDs must be unique",
	"emptyGroupName":     "group name must be set for each deployment group",
	"illegalChars":       "invalid character(s) found in group name",
	"invalidOutput":      "requested output was not found in the module",
	"varNotDefined":      "variable not defined",
	"valueNotString":     "value was not of type string",
	"valueEmptyString":   "value is an empty string",
	"labelReqs":          "value can only contain lowercase letters, numeric characters, underscores and dashes, and must be between 1 and 63 characters long.",
}

// map[moved module path]replacing module path
var movedModules = map[string]string{
	"community/modules/scheduler/cloud-batch-job":        "modules/scheduler/batch-job-template",
	"community/modules/scheduler/cloud-batch-login-node": "modules/scheduler/batch-login-node",
}

// GroupName is the name of a deployment group
type GroupName string

// Validate checks that the group name is valid
func (n GroupName) Validate() error {
	if n == "" {
		return errors.New(errorMessages["emptyGroupName"])
	}
	if hasIllegalChars(string(n)) {
		return fmt.Errorf("%s %s", errorMessages["illegalChars"], n)
	}
	return nil
}

// DeploymentGroup defines a group of Modules that are all executed together
type DeploymentGroup struct {
	Name             GroupName        `yaml:"group"`
	TerraformBackend TerraformBackend `yaml:"terraform_backend"`
	Modules          []Module         `yaml:"modules"`
	Kind             ModuleKind
}

// Module return the module with the given ID
func (bp *Blueprint) Module(id ModuleID) (*Module, error) {
	var mod *Module
	bp.WalkModules(func(m *Module) error {
		if m.ID == id {
			mod = m
		}
		return nil
	})
	if mod == nil {
		return nil, fmt.Errorf("%s: %s", errorMessages["invalidMod"], id)
	}
	return mod, nil
}

// ModuleGroup returns the group containing the module
func (bp Blueprint) ModuleGroup(mod ModuleID) (DeploymentGroup, error) {
	for _, g := range bp.DeploymentGroups {
		for _, m := range g.Modules {
			if m.ID == mod {
				return g, nil
			}
		}
	}
	return DeploymentGroup{}, fmt.Errorf("%s: %s", errorMessages["invalidMod"], mod)
}

// ModuleGroupOrDie returns the group containing the module; panics if unfound
func (bp Blueprint) ModuleGroupOrDie(mod ModuleID) DeploymentGroup {
	g, err := bp.ModuleGroup(mod)
	if err != nil {
		panic(fmt.Errorf("module %s not found in blueprint: %s", mod, err))
	}
	return g
}

// TerraformBackend defines the configuration for the terraform state backend
type TerraformBackend struct {
	Type          string
	Configuration Dict
}

// ModuleKind abstracts Toolkit module kinds (presently: packer/terraform)
type ModuleKind struct {
	kind string
}

// UnknownKind is the default value when the user has not specified module kind
var UnknownKind = ModuleKind{kind: ""}

// TerraformKind is the kind for Terraform modules (should be treated as const)
var TerraformKind = ModuleKind{kind: "terraform"}

// PackerKind is the kind for Packer modules (should be treated as const)
var PackerKind = ModuleKind{kind: "packer"}

// UnmarshalYAML implements a custom unmarshaler from YAML string to ModuleKind
func (mk *ModuleKind) UnmarshalYAML(n *yaml.Node) error {
	var kind string
	const yamlErrorMsg string = "block beginning at line %d: %s"

	err := n.Decode(&kind)
	if err == nil && IsValidModuleKind(kind) {
		mk.kind = kind
		return nil
	}
	return fmt.Errorf(yamlErrorMsg, n.Line, "kind must be \"packer\" or \"terraform\" or removed from YAML")
}

// MarshalYAML implements a custom marshaler from ModuleKind to YAML string
func (mk ModuleKind) MarshalYAML() (interface{}, error) {
	return mk.String(), nil
}

// IsValidModuleKind ensures that the user has specified a supported kind
func IsValidModuleKind(kind string) bool {
	return kind == TerraformKind.String() || kind == PackerKind.String() ||
		kind == UnknownKind.String()
}

func (mk ModuleKind) String() string {
	return mk.kind
}

type validatorName uint8

const (
	// Undefined will be default and potentially throw errors if used
	Undefined validatorName = iota
	testProjectExistsName
	testRegionExistsName
	testZoneExistsName
	testModuleNotUsedName
	testZoneInRegionName
	testApisEnabledName
	testDeploymentVariableNotUsedName
)

// this enum will be used to control how fatal validator failures will be
// treated during blueprint creation
const (
	ValidationError int = iota
	ValidationWarning
	ValidationIgnore
)

func isValidValidationLevel(level int) bool {
	return !(level > ValidationIgnore || level < ValidationError)
}

func (v validatorName) String() string {
	switch v {
	case testProjectExistsName:
		return "test_project_exists"
	case testRegionExistsName:
		return "test_region_exists"
	case testZoneExistsName:
		return "test_zone_exists"
	case testZoneInRegionName:
		return "test_zone_in_region"
	case testApisEnabledName:
		return "test_apis_enabled"
	case testModuleNotUsedName:
		return "test_module_not_used"
	case testDeploymentVariableNotUsedName:
		return "test_deployment_variable_not_used"
	default:
		return "unknown_validator"
	}
}

type validatorConfig struct {
	Validator string
	Inputs    Dict
	Skip      bool
}

func (v *validatorConfig) check(name validatorName, requiredInputs []string) error {
	if v.Validator != name.String() {
		return fmt.Errorf("passed wrong validator to %s implementation", name.String())
	}

	var errored bool
	for _, inp := range requiredInputs {
		if !v.Inputs.Has(inp) {
			log.Printf("a required input %s was not provided to %s!", inp, v.Validator)
			errored = true
		}
	}

	if errored {
		return fmt.Errorf("at least one required input was not provided to %s", v.Validator)
	}

	// ensure that no extra inputs were provided by comparing length
	if len(requiredInputs) != len(v.Inputs.Items()) {
		errStr := "only %v inputs %s should be provided to %s"
		return fmt.Errorf(errStr, len(requiredInputs), requiredInputs, v.Validator)
	}

	return nil
}

// ModuleID is a unique identifier for a module in a blueprint
type ModuleID string

// Module stores YAML definition of an HPC cluster component defined in a blueprint
type Module struct {
	Source string
	// DeploymentSource - is source to be used for this module in written deployment.
	DeploymentSource string `yaml:"-"` // "-" prevents user from specifying it
	Kind             ModuleKind
	ID               ModuleID
	Use              []ModuleID
	WrapSettingsWith map[string][]string
	Outputs          []modulereader.OutputInfo `yaml:"outputs,omitempty"`
	Settings         Dict
	RequiredApis     map[string][]string `yaml:"required_apis"`
}

// createWrapSettingsWith ensures WrapSettingsWith field is not nil, if it is
// a new map is created.
func (m *Module) createWrapSettingsWith() {
	if m.WrapSettingsWith == nil {
		m.WrapSettingsWith = make(map[string][]string)
	}
}

// InfoOrDie returns the ModuleInfo for the module or panics
func (m Module) InfoOrDie() modulereader.ModuleInfo {
	mi, err := modulereader.GetModuleInfo(m.Source, m.Kind.String())
	if err != nil {
		panic(err)
	}
	return mi
}

// Blueprint stores the contents on the User YAML
// omitempty on validation_level ensures that expand will not expose the setting
// unless it has been set to a non-default value; the implementation as an
// integer is primarily for internal purposes even if it can be set in blueprint
type Blueprint struct {
	BlueprintName            string `yaml:"blueprint_name"`
	GhpcVersion              string `yaml:"ghpc_version,omitempty"`
	Validators               []validatorConfig
	ValidationLevel          int `yaml:"validation_level,omitempty"`
	Vars                     Dict
	DeploymentGroups         []DeploymentGroup `yaml:"deployment_groups"`
	TerraformBackendDefaults TerraformBackend  `yaml:"terraform_backend_defaults"`
}

// DeploymentConfig is a container for the imported YAML data and supporting data for
// creating the blueprint from it
type DeploymentConfig struct {
	Config Blueprint
}

// ExpandConfig expands the yaml config in place
func (dc *DeploymentConfig) ExpandConfig() error {
	if err := dc.Config.checkMovedModules(); err != nil {
		return err
	}
	dc.Config.setGlobalLabels()
	dc.Config.addKindToModules()
	dc.validateConfig()
	dc.expand()
	dc.validate()
	return nil
}

func (bp *Blueprint) setGlobalLabels() {
	if !bp.Vars.Has("labels") {
		bp.Vars.Set("labels", cty.EmptyObjectVal)
	}
}

// listUnusedModules provides a list modules that are in the
// "use" field, but not actually used.
func (m Module) listUnusedModules() []ModuleID {
	used := map[ModuleID]bool{}
	// Recurse through objects/maps/lists checking each element for having `ProductOfModuleUse` mark.
	cty.Walk(m.Settings.AsObject(), func(p cty.Path, v cty.Value) (bool, error) {
		if mark, has := HasMark[ProductOfModuleUse](v); has {
			used[mark.Module] = true
		}
		return true, nil
	})

	unused := []ModuleID{}
	for _, w := range m.Use {
		if !used[w] {
			unused = append(unused, w)
		}
	}
	return unused
}

// GetUsedDeploymentVars returns a list of deployment vars used in the given value
func GetUsedDeploymentVars(val cty.Value) []string {
	res := map[string]bool{}
	// Recurse through objects/maps/lists gathering used references to deployment variables.
	cty.Walk(val, func(path cty.Path, val cty.Value) (bool, error) {
		if ex, is := IsExpressionValue(val); is {
			for _, r := range ex.References() {
				if r.GlobalVar {
					res[r.Name] = true
				}
			}
		}
		return true, nil
	})
	return maps.Keys(res)
}

func (dc *DeploymentConfig) listUnusedDeploymentVariables() []string {
	// these variables are required or automatically constructed and applied;
	// these should not be listed unused otherwise no blueprints are valid
	var usedVars = map[string]bool{
		"labels":          true,
		"deployment_name": true,
	}

	dc.Config.WalkModules(func(m *Module) error {
		for _, v := range GetUsedDeploymentVars(m.Settings.AsObject()) {
			usedVars[v] = true
		}
		return nil
	})

	unusedVars := []string{}
	for k := range dc.Config.Vars.Items() {
		if _, ok := usedVars[k]; !ok {
			unusedVars = append(unusedVars, k)
		}
	}

	return unusedVars
}

func (bp Blueprint) checkMovedModules() error {
	var err error
	bp.WalkModules(func(m *Module) error {
		if replacement, ok := movedModules[strings.Trim(m.Source, "./")]; ok {
			err = fmt.Errorf("the blueprint references modules that have moved")
			fmt.Printf(
				"A module you are using has moved. %s has been replaced with %s. Please update the source in your blueprint and try again.\n",
				m.Source, replacement)
		}
		return nil
	})
	return err
}

// NewDeploymentConfig is a constructor for DeploymentConfig
func NewDeploymentConfig(configFilename string) (DeploymentConfig, error) {
	blueprint, err := importBlueprint(configFilename)
	if err != nil {
		return DeploymentConfig{}, err
	}
	return DeploymentConfig{Config: blueprint}, nil
}

// ImportBlueprint imports the blueprint configuration provided.
func importBlueprint(blueprintFilename string) (Blueprint, error) {
	var blueprint Blueprint

	reader, err := os.Open(blueprintFilename)
	if err != nil {
		return blueprint, fmt.Errorf("%s, filename=%s: %v",
			errorMessages["fileLoadError"], blueprintFilename, err)
	}

	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)

	if err = decoder.Decode(&blueprint); err != nil {
		return blueprint, fmt.Errorf(errorMessages["yamlUnmarshalError"],
			blueprintFilename, err)
	}

	// if the validation level has been explicitly set to an invalid value
	// in YAML blueprint then silently default to validationError
	if !isValidValidationLevel(blueprint.ValidationLevel) {
		blueprint.ValidationLevel = ValidationError
	}

	return blueprint, nil
}

// ExportBlueprint exports the internal representation of a blueprint config
func (dc DeploymentConfig) ExportBlueprint(outputFilename string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString(YamlLicense)
	buf.WriteString("\n")
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err := encoder.Encode(&dc.Config)
	encoder.Close()
	d := buf.Bytes()
	if err != nil {
		return d, fmt.Errorf("%s: %w", errorMessages["yamlMarshalError"], err)
	}

	if outputFilename == "" {
		return d, nil
	}
	err = ioutil.WriteFile(outputFilename, d, 0644)
	if err != nil {
		// hitting this error writing yaml
		return d, fmt.Errorf("%s, Filename: %s: %w",
			errorMessages["fileSaveError"], outputFilename, err)
	}
	return nil, nil
}

// addKindToModules sets the kind to 'terraform' when empty.
func (bp *Blueprint) addKindToModules() {
	bp.WalkModules(func(m *Module) error {
		if m.Kind == UnknownKind {
			m.Kind = TerraformKind
		}
		return nil
	})
}

// setModulesInfo populates needed information from modules
func (bp *Blueprint) checkModulesInfo() error {
	return bp.WalkModules(func(m *Module) error {
		_, err := modulereader.GetModuleInfo(m.Source, m.Kind.String())
		return err
	})
}

// checkModuleAndGroupNames checks and imports module and resource group IDs
// and names respectively.
func checkModuleAndGroupNames(groups []DeploymentGroup) error {
	seenMod := map[ModuleID]bool{}
	seenGroups := map[GroupName]bool{}
	for ig := range groups {
		grp := &groups[ig]
		if err := grp.Name.Validate(); err != nil {
			return err
		}
		if seenGroups[grp.Name] {
			return fmt.Errorf("%s: %s used more than once", errorMessages["duplicateGroup"], grp.Name)
		}
		seenGroups[grp.Name] = true

		for _, mod := range grp.Modules {
			if seenMod[mod.ID] {
				return fmt.Errorf("%s: %s used more than once", errorMessages["duplicateID"], mod.ID)
			}
			seenMod[mod.ID] = true

			// Verify Module Kind matches group Kind
			if grp.Kind == UnknownKind {
				grp.Kind = mod.Kind
			}
			if grp.Kind != mod.Kind {
				return fmt.Errorf(
					"mixing modules of differing kinds in a deployment group is not supported: deployment group %s, got %s and %s",
					grp.Name, grp.Kind, mod.Kind)
			}
		}
	}
	return nil
}

// checkUsedModuleNames verifies that any used modules have valid names and
// are in the correct group
func checkUsedModuleNames(bp Blueprint) error {
	return bp.WalkModules(func(mod *Module) error {
		for _, used := range mod.Use {
			if err := validateModuleReference(bp, *mod, used); err != nil {
				return err
			}
		}
		return nil
	})
}

func checkBackend(b TerraformBackend) error {
	const errMsg = "can not use variables in terraform_backend block, got '%s=%s'"
	// TerraformBackend.Type is typed as string, "simple" variables and HCL literals stay "as is".
	if hasVariable(b.Type) {
		return fmt.Errorf(errMsg, "type", b.Type)
	}
	if _, is := IsYamlExpressionLiteral(cty.StringVal(b.Type)); is {
		return fmt.Errorf(errMsg, "type", b.Type)
	}
	return cty.Walk(b.Configuration.AsObject(), func(p cty.Path, v cty.Value) (bool, error) {
		if _, is := IsExpressionValue(v); is {
			return false, fmt.Errorf("can not use variables in terraform_backend block")
		}
		return true, nil
	})
}

func checkBackends(bp Blueprint) error {
	if err := checkBackend(bp.TerraformBackendDefaults); err != nil {
		return err
	}
	for _, g := range bp.DeploymentGroups {
		if err := checkBackend(g.TerraformBackend); err != nil {
			return err
		}
	}
	return nil
}

// validateConfig runs a set of simple early checks on the imported input YAML
func (dc *DeploymentConfig) validateConfig() {
	_, err := dc.Config.DeploymentName()
	if err != nil {
		log.Fatal(err)
	}
	err = dc.Config.checkBlueprintName()
	if err != nil {
		log.Fatal(err)
	}

	if err = dc.validateVars(); err != nil {
		log.Fatal(err)
	}

	if err = dc.Config.checkModulesInfo(); err != nil {
		log.Fatal(err)
	}
	if err = checkModuleAndGroupNames(dc.Config.DeploymentGroups); err != nil {
		log.Fatal(err)
	}
	if err = checkUsedModuleNames(dc.Config); err != nil {
		log.Fatal(err)
	}
	if err = checkBackends(dc.Config); err != nil {
		log.Fatal(err)
	}
	if err = checkModuleSettings(dc.Config); err != nil {
		log.Fatal(err)
	}
}

// SkipValidator marks validator(s) as skipped,
// if no validator is present, adds one, marked as skipped.
func (dc *DeploymentConfig) SkipValidator(name string) error {
	if dc.Config.Validators == nil {
		dc.Config.Validators = []validatorConfig{}
	}
	skipped := false
	for i, v := range dc.Config.Validators {
		if v.Validator == name {
			dc.Config.Validators[i].Skip = true
			skipped = true
		}
	}
	if !skipped {
		dc.Config.Validators = append(dc.Config.Validators, validatorConfig{Validator: name, Skip: true})
	}
	return nil
}

// InputValueError signifies a problem with the blueprint name.
type InputValueError struct {
	inputKey string
	cause    string
}

func (err *InputValueError) Error() string {
	return fmt.Sprintf("%v input error, cause: %v", err.inputKey, err.cause)
}

var matchLabelExp *regexp.Regexp = regexp.MustCompile(`^[\p{Ll}\p{Lo}\p{N}_-]{1,63}$`)

// isValidLabelValue checks if a string is a valid value for a GCP label.
// For more information on valid label values, see the docs at:
// https://cloud.google.com/resource-manager/docs/creating-managing-labels#requirements
func isValidLabelValue(value string) bool {
	return matchLabelExp.MatchString(value)
}

// DeploymentName returns the deployment_name from the config and does approperate checks.
func (bp *Blueprint) DeploymentName() (string, error) {
	if !bp.Vars.Has("deployment_name") {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["varNotFound"],
		}
	}

	v := bp.Vars.Get("deployment_name")
	if v.Type() != cty.String {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["valueNotString"],
		}
	}

	s := v.AsString()
	if len(s) == 0 {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["valueEmptyString"],
		}
	}

	// Check that deployment_name is a valid label
	if !isValidLabelValue(s) {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["labelReqs"],
		}
	}

	return s, nil
}

// checkBlueprintName returns an error if blueprint_name does not comply with
// requirements for correct GCP label values.
func (bp *Blueprint) checkBlueprintName() error {

	if len(bp.BlueprintName) == 0 {
		return &InputValueError{
			inputKey: "blueprint_name",
			cause:    errorMessages["valueEmptyString"],
		}
	}

	if !isValidLabelValue(bp.BlueprintName) {
		return &InputValueError{
			inputKey: "blueprint_name",
			cause:    errorMessages["labelReqs"],
		}
	}

	return nil
}

// ProductOfModuleUse is a "mark" applied to values in Module.Settings if
// this value was modified as a result of applying `use`.
type ProductOfModuleUse struct {
	Module ModuleID
}

// WalkModules walks all modules in the blueprint and calls the walker function
func (bp *Blueprint) WalkModules(walker func(*Module) error) error {
	for ig := range bp.DeploymentGroups {
		g := &bp.DeploymentGroups[ig]
		for im := range g.Modules {
			m := &g.Modules[im]
			if err := walker(m); err != nil {
				return err
			}
		}
	}
	return nil
}

// validate every module setting in the blueprint containing a reference
func checkModuleSettings(bp Blueprint) error {
	return bp.WalkModules(func(m *Module) error {
		return cty.Walk(m.Settings.AsObject(), func(p cty.Path, v cty.Value) (bool, error) {
			if e, is := IsExpressionValue(v); is {
				for _, r := range e.References() {
					if err := validateModuleSettingReference(bp, *m, r); err != nil {
						return false, err
					}
				}
			}
			return true, nil
		})
	})

}
