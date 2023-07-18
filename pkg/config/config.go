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
	"regexp"
	"sort"
	"strings"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
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
	"missingSetting":    "a required setting is missing from a module",
	"settingsLabelType": "labels in module settings are not a map",
	"invalidVar":        "invalid variable definition in",
	"invalidMod":        "invalid module reference",
	"varNotFound":       "Could not find source of variable",
	"intergroupOrder":   "References to outputs from other groups must be to earlier groups",
	"noOutput":          "Output not found for a variable",
	"cannotUsePacker":   "Packer modules cannot be used by other modules",
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
	"invalidOutput":      "requested output was not found in the module",
	"valueNotString":     "value was not of type string",
	"valueEmptyString":   "value is an empty string",
	"labelNameReqs":      "name must begin with a lowercase letter, can only contain lowercase letters, numeric characters, underscores and dashes, and must be between 1 and 63 characters long",
	"labelValueReqs":     "value can only contain lowercase letters, numeric characters, underscores and dashes, and must be between 0 and 63 characters long",
}

// map[moved module path]replacing module path
var movedModules = map[string]string{
	"community/modules/scheduler/cloud-batch-job":        "modules/scheduler/batch-job-template",
	"community/modules/scheduler/cloud-batch-login-node": "modules/scheduler/batch-login-node",
	"community/modules/scheduler/htcondor-configure":     "community/modules/scheduler/htcondor-base",
}

// GroupName is the name of a deployment group
type GroupName string

// Validate checks that the group name is valid
func (n GroupName) Validate() error {
	if n == "" {
		return errors.New(errorMessages["emptyGroupName"])
	}

	if !regexp.MustCompile(`^\w(-*\w)*$`).MatchString(string(n)) {
		return fmt.Errorf("invalid character(s) found in group name %q.\n"+
			"Allowed : alphanumeric, '_', and '-'; can not start/end with '-'", n)
	}
	return nil
}

// DeploymentGroup defines a group of Modules that are all executed together
type DeploymentGroup struct {
	Name             GroupName        `yaml:"group"`
	TerraformBackend TerraformBackend `yaml:"terraform_backend,omitempty"`
	Modules          []Module         `yaml:"modules"`
	// DEPRECATED fields, keep in the struct for backwards compatibility
	deprecatedKind interface{} `yaml:"kind,omitempty"`
}

// Kind returns the kind of all the modules in the group.
// If the group contains modules of different kinds, it returns UnknownKind
func (g DeploymentGroup) Kind() ModuleKind {
	if len(g.Modules) == 0 {
		return UnknownKind
	}
	k := g.Modules[0].Kind
	for _, m := range g.Modules {
		if m.Kind != k {
			return UnknownKind
		}
	}
	return k
}

// Module return the module with the given ID
func (bp *Blueprint) Module(id ModuleID) (*Module, error) {
	var mod *Module
	bp.WalkModules(func(_ modulePath, m *Module) error {
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

// GroupIndex returns the index of the input group in the blueprint
// return -1 if not found
func (bp Blueprint) GroupIndex(n GroupName) int {
	for i, g := range bp.DeploymentGroups {
		if g.Name == n {
			return i
		}
	}
	return -1
}

// Group returns the deployment group with a given name
func (bp Blueprint) Group(n GroupName) (DeploymentGroup, error) {
	idx := bp.GroupIndex(n)
	if idx == -1 {
		return DeploymentGroup{}, fmt.Errorf("could not find group %s in blueprint", n)
	}
	return bp.DeploymentGroups[idx], nil
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
	Inputs    Dict `yaml:"inputs,omitempty"`
	Skip      bool `yaml:"skip,omitempty"`
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

// ModuleIDs is a list of ModuleID
type ModuleIDs []ModuleID

// Module stores YAML definition of an HPC cluster component defined in a blueprint
type Module struct {
	Source   string
	Kind     ModuleKind
	ID       ModuleID
	Use      ModuleIDs                 `yaml:"use,omitempty"`
	Outputs  []modulereader.OutputInfo `yaml:"outputs,omitempty"`
	Settings Dict                      `yaml:"settings,omitempty"`
	// DEPRECATED fields, keep in the struct for backwards compatibility
	RequiredApis     interface{} `yaml:"required_apis,omitempty"`
	WrapSettingsWith interface{} `yaml:"wrapsettingswith,omitempty"`
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
	BlueprintName            string            `yaml:"blueprint_name"`
	GhpcVersion              string            `yaml:"ghpc_version,omitempty"`
	Validators               []validatorConfig `yaml:"validators,omitempty"`
	ValidationLevel          int               `yaml:"validation_level,omitempty"`
	Vars                     Dict
	DeploymentGroups         []DeploymentGroup `yaml:"deployment_groups"`
	TerraformBackendDefaults TerraformBackend  `yaml:"terraform_backend_defaults,omitempty"`
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
	if err := dc.validateConfig(); err != nil {
		return err
	}
	if err := dc.expand(); err != nil {
		return err
	}
	if err := dc.validate(); err != nil {
		return err
	}
	return nil
}

func (bp *Blueprint) setGlobalLabels() {
	if !bp.Vars.Has("labels") {
		bp.Vars.Set("labels", cty.EmptyObjectVal)
	}
}

// listUnusedModules provides a list modules that are in the
// "use" field, but not actually used.
func (m Module) listUnusedModules() ModuleIDs {
	used := map[ModuleID]bool{}
	// Recurse through objects/maps/lists checking each element for having `ProductOfModuleUse` mark.
	cty.Walk(m.Settings.AsObject(), func(p cty.Path, v cty.Value) (bool, error) {
		for _, mod := range IsProductOfModuleUse(v) {
			used[mod] = true
		}
		return true, nil
	})

	unused := ModuleIDs{}
	for _, w := range m.Use {
		if !used[w] {
			unused = append(unused, w)
		}
	}
	return unused
}

// GetUsedDeploymentVars returns a list of deployment vars used in the given value
func GetUsedDeploymentVars(val cty.Value) []string {
	res := []string{}
	for _, ref := range valueReferences(val) {
		if ref.GlobalVar {
			res = append(res, ref.Name)
		}
	}
	return res
}

func (dc *DeploymentConfig) listUnusedDeploymentVariables() []string {
	// these variables are required or automatically constructed and applied;
	// these should not be listed unused otherwise no blueprints are valid
	var usedVars = map[string]bool{
		"labels":          true,
		"deployment_name": true,
	}

	dc.Config.WalkModules(func(_ modulePath, m *Module) error {
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
	errs := Errors{}

	bp.WalkModules(func(p modulePath, m *Module) error {
		if replacement, ok := movedModules[strings.Trim(m.Source, "./")]; ok {
			err := fmt.Errorf("a module has moved. %s has been replaced with %s; "+
				"update the source in your blueprint and read %s/README.md for migration instructions",
				m.Source, replacement, replacement)
			errs.At(p.Source, err)
		}
		return nil
	})

	return errs.OrNil()
}

// NewDeploymentConfig is a constructor for DeploymentConfig
func NewDeploymentConfig(configFilename string) (DeploymentConfig, YamlCtx, error) {
	bp, ctx, err := importBlueprint(configFilename)
	if err != nil {
		return DeploymentConfig{}, YamlCtx{}, err
	}
	// if the validation level has been explicitly set to an invalid value
	// in YAML blueprint then silently default to validationError
	if !isValidValidationLevel(bp.ValidationLevel) {
		bp.ValidationLevel = ValidationError
	}
	return DeploymentConfig{Config: bp}, ctx, nil
}

// ExportBlueprint exports the internal representation of a blueprint config
func (dc DeploymentConfig) ExportBlueprint(outputFilename string) error {
	var buf bytes.Buffer
	buf.WriteString(YamlLicense)
	buf.WriteString("\n")
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	err := encoder.Encode(&dc.Config)
	encoder.Close()
	d := buf.Bytes()

	if err != nil {
		return fmt.Errorf("%s: %w", errorMessages["yamlMarshalError"], err)
	}

	err = ioutil.WriteFile(outputFilename, d, 0644)
	if err != nil {
		// hitting this error writing yaml
		return fmt.Errorf("%s, Filename: %s: %w",
			errorMessages["fileSaveError"], outputFilename, err)
	}
	return nil
}

// addKindToModules sets the kind to 'terraform' when empty.
func (bp *Blueprint) addKindToModules() {
	bp.WalkModules(func(_ modulePath, m *Module) error {
		if m.Kind == UnknownKind {
			m.Kind = TerraformKind
		}
		return nil
	})
}

// checkModulesInfo ensures each module in the blueprint has known detailed
// metadata (inputs, outputs)
func (bp *Blueprint) checkModulesInfo() error {
	errs := Errors{}
	bp.WalkModules(func(p modulePath, m *Module) error {
		_, err := modulereader.GetModuleInfo(m.Source, m.Kind.String())
		errs.At(p.Source, err)
		return nil
	})
	return errs.OrNil()
}

// checkModulesAndGroups ensures:
//   - all module IDs are unique across all groups
//   - if deployment group kind is unknown (not explicit in blueprint), then it is
//     set to th kind of the first module that has a known kind (a prior func sets
//     module kind to Terraform if unset)
//   - all modules must be of the same kind and all modules must be of the same
//     kind as the group
//   - all group names are unique and do not have illegal characters
func checkModulesAndGroups(groups []DeploymentGroup) error {
	seenMod := map[ModuleID]bool{}
	seenGroups := map[GroupName]bool{}
	errs := Errors{}

	for ig := range groups {
		pg := Root.Groups.At(ig)
		grp := &groups[ig]
		errs.At(pg.Name, grp.Name.Validate())

		if seenGroups[grp.Name] {
			errs.At(pg.Name, fmt.Errorf("%s: %s used more than once", errorMessages["duplicateGroup"], grp.Name))
		}
		seenGroups[grp.Name] = true

		if len(grp.Modules) == 0 {
			errs.At(pg.Modules, errors.New("deployment group must have at least one module"))
		} else if grp.Kind() == UnknownKind {
			errs.At(pg.Modules, errors.New("mixing modules of differing kinds in a deployment group is not supported"))
		}

		for im, mod := range grp.Modules {
			pm := pg.Modules.At(im)
			if seenMod[mod.ID] {
				errs.At(pm.ID, fmt.Errorf("%s: %s used more than once", errorMessages["duplicateID"], mod.ID))
			}
			seenMod[mod.ID] = true
		}
	}
	return errs.OrNil()
}

// checkUsedModuleNames verifies that any used modules have valid names and
// are in the correct group
func checkUsedModuleNames(bp Blueprint) error {
	errs := Errors{}
	bp.WalkModules(func(p modulePath, mod *Module) error {
		for iu, used := range mod.Use {
			errs.At(p.Use.At(iu), validateModuleReference(bp, *mod, used))
		}
		return nil
	})
	return errs.OrNil()
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
	errs := Errors{}
	errs.At(Root.Backend, checkBackend(bp.TerraformBackendDefaults))
	for ig, g := range bp.DeploymentGroups {
		errs.At(Root.Groups.At(ig).Backend, checkBackend(g.TerraformBackend))
	}
	return errs.OrNil()
}

// validateConfig runs a set of simple early checks on the imported input YAML
func (dc *DeploymentConfig) validateConfig() error {

	if _, err := dc.Config.DeploymentName(); err != nil {
		return err
	}

	if err := dc.Config.checkBlueprintName(); err != nil {
		return err
	}

	if err := dc.validateVars(); err != nil {
		return err
	}

	if err := dc.Config.checkModulesInfo(); err != nil {
		return err
	}

	if err := checkModulesAndGroups(dc.Config.DeploymentGroups); err != nil {
		return err
	}

	// checkPackerGroups must come after checkModulesAndGroups, in which group
	// Kind is set and aligned with module Kinds
	if err := checkPackerGroups(dc.Config.DeploymentGroups); err != nil {
		return err
	}

	if err := checkUsedModuleNames(dc.Config); err != nil {
		return err
	}

	if err := checkBackends(dc.Config); err != nil {
		return err
	}

	if err := checkModuleSettings(dc.Config); err != nil {
		return err
	}
	return nil
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

func (err InputValueError) Error() string {
	return fmt.Sprintf("%v input error, cause: %v", err.inputKey, err.cause)
}

var matchLabelNameExp *regexp.Regexp = regexp.MustCompile(`^[\p{Ll}\p{Lo}][\p{Ll}\p{Lo}\p{N}_-]{0,62}$`)
var matchLabelValueExp *regexp.Regexp = regexp.MustCompile(`^[\p{Ll}\p{Lo}\p{N}_-]{0,63}$`)

// isValidLabelName checks if a string is a valid name for a GCP label.
// For more information on valid label names, see the docs at:
// https://cloud.google.com/resource-manager/docs/creating-managing-labels#requirements
func isValidLabelName(name string) bool {
	return matchLabelNameExp.MatchString(name)
}

// isValidLabelValue checks if a string is a valid value for a GCP label.
// For more information on valid label values, see the docs at:
// https://cloud.google.com/resource-manager/docs/creating-managing-labels#requirements
func isValidLabelValue(value string) bool {
	return matchLabelValueExp.MatchString(value)
}

// DeploymentName returns the deployment_name from the config and does approperate checks.
func (bp *Blueprint) DeploymentName() (string, error) {
	if !bp.Vars.Has("deployment_name") {
		return "", InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["varNotFound"],
		}
	}

	path := Root.Vars.Dot("deployment_name")
	v := bp.Vars.Get("deployment_name")
	if v.Type() != cty.String {
		return "", BpError{path, InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["valueNotString"],
		}}
	}

	s := v.AsString()
	if len(s) == 0 {
		return "", BpError{path, InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["valueEmptyString"],
		}}
	}

	// Check that deployment_name is a valid label
	if !isValidLabelValue(s) {
		return "", BpError{path, InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["labelValueReqs"],
		}}
	}

	return s, nil
}

// checkBlueprintName returns an error if blueprint_name does not comply with
// requirements for correct GCP label values.
func (bp *Blueprint) checkBlueprintName() error {
	if len(bp.BlueprintName) == 0 {
		return BpError{Root.BlueprintName, InputValueError{
			inputKey: "blueprint_name",
			cause:    errorMessages["valueEmptyString"],
		}}
	}

	if !isValidLabelValue(bp.BlueprintName) {
		return BpError{Root.BlueprintName, InputValueError{
			inputKey: "blueprint_name",
			cause:    errorMessages["labelValueReqs"],
		}}
	}

	return nil
}

// productOfModuleUseMark is a "mark" applied to values that are result of `use`.
// Should not be used directly, use AsProductOfModuleUse and IsProductOfModuleUse instead.
type productOfModuleUseMark struct {
	mods string
}

// AsProductOfModuleUse marks a value as a result of `use` of given modules.
func AsProductOfModuleUse(v cty.Value, mods ...ModuleID) cty.Value {
	s := make([]string, len(mods))
	for i, m := range mods {
		s[i] = string(m)
	}
	sort.Strings(s)
	return v.Mark(productOfModuleUseMark{strings.Join(s, ",")})
}

// IsProductOfModuleUse returns list of modules that contributed (by `use`) to this value.
func IsProductOfModuleUse(v cty.Value) []ModuleID {
	mark, marked := HasMark[productOfModuleUseMark](v)
	if !marked {
		return []ModuleID{}
	}

	s := strings.Split(mark.mods, ",")
	mods := make([]ModuleID, len(s))
	for i, m := range s {
		mods[i] = ModuleID(m)
	}
	return mods
}

// WalkModules walks all modules in the blueprint and calls the walker function
func (bp *Blueprint) WalkModules(walker func(modulePath, *Module) error) error {
	for ig := range bp.DeploymentGroups {
		g := &bp.DeploymentGroups[ig]
		for im := range g.Modules {
			p := Root.Groups.At(ig).Modules.At(im)
			m := &g.Modules[im]
			if err := walker(p, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// validate every module setting in the blueprint containing a reference
func checkModuleSettings(bp Blueprint) error {
	errs := Errors{}
	bp.WalkModules(func(p modulePath, m *Module) error {
		for k, v := range m.Settings.Items() {
			for _, r := range valueReferences(v) {
				// TODO: add a cty.Path suffix to the errors path for better location
				errs.At(p.Settings.Dot(k), validateModuleSettingReference(bp, *m, r))
			}
		}
		return nil
	})
	return errs.OrNil()
}

func checkPackerGroups(groups []DeploymentGroup) error {
	errs := Errors{}
	for ig, group := range groups {
		if group.Kind() == PackerKind && len(group.Modules) != 1 {
			errs.At(Root.Groups.At(ig),
				fmt.Errorf("group %s is \"kind: packer\" but has more than 1 module; separate each packer module into its own deployment group", group.Name))
		}
	}
	return errs.OrNil()
}

func (bp *Blueprint) evalVars() error {
	// 0 - unvisited
	// 1 - on stack
	// 2 - done
	used := map[string]int{}
	res := Dict{}

	// walk vars in reverse topological order, and evaluate them
	var dfs func(string) error
	dfs = func(n string) error {
		used[n] = 1 // put on stack
		v := bp.Vars.Get(n)
		for _, ref := range valueReferences(v) {
			if !ref.GlobalVar {
				return BpError{
					Root.Vars.Dot(n),
					fmt.Errorf("non-global variable %q referenced in expression", ref.Name),
				}
			}
			if used[ref.Name] == 1 {
				return BpError{
					Root.Vars.Dot(n),
					fmt.Errorf("cyclic dependency detected: %q -> %q", n, ref.Name),
				}
			}
			if used[ref.Name] == 0 {
				if err := dfs(ref.Name); err != nil {
					return err
				}
			}
		}

		used[n] = 2 // remove from stack and evaluate
		ev, err := evalValue(v, Blueprint{Vars: res})
		res.Set(n, ev)
		return err
	}

	for n := range bp.Vars.Items() {
		if used[n] == 0 { // unvisited
			if err := dfs(n); err != nil {
				return err
			}
		}
	}

	bp.Vars = res
	return nil
}
