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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyJson "github.com/zclconf/go-cty/cty/json"
	"golang.org/x/exp/slices"
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
	"varInAnotherGroup":    "References to other groups are not yet supported",
	"intergroupImplicit":   "References to outputs from other groups must explicitly identify the group",
	"intergroupOrder":      "References to outputs from other groups must be to earlier groups",
	"referenceWrongGroup":  "Reference specified the wrong group for the module",
	"noOutput":             "Output not found for a variable",
	"varWithinStrings":     "variables \"$(...)\" within strings are not yet implemented. remove them or add a backslash to render literally.",
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

// DeploymentGroup defines a group of Modules that are all executed together
type DeploymentGroup struct {
	Name             string           `yaml:"group"`
	TerraformBackend TerraformBackend `yaml:"terraform_backend"`
	Modules          []Module         `yaml:"modules"`
	Kind             string
}

func (g DeploymentGroup) getModuleByID(modID string) (Module, error) {
	idx := slices.IndexFunc(g.Modules, func(m Module) bool { return m.ID == modID })
	if idx == -1 {
		return Module{}, fmt.Errorf("%s: %s", errorMessages["invalidMod"], modID)
	}
	return g.Modules[idx], nil
}

func (dc DeploymentConfig) getGroupByID(groupID string) (DeploymentGroup, error) {
	groupIndex := slices.IndexFunc(dc.Config.DeploymentGroups, func(d DeploymentGroup) bool { return d.Name == groupID })
	if groupIndex == -1 {
		return DeploymentGroup{}, fmt.Errorf("%s: %s", errorMessages["groupNotFound"], groupID)
	}
	group := dc.Config.DeploymentGroups[groupIndex]
	return group, nil
}

// TerraformBackend defines the configuration for the terraform state backend
type TerraformBackend struct {
	Type          string
	Configuration map[string]interface{}
}

type validatorName int64

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
	validationError int = iota
	validationWarning
	validationIgnore
)

func isValidValidationLevel(level int) bool {
	return !(level > validationIgnore || level < validationError)
}

// SetValidationLevel allows command-line tools to set the validation level
func (dc *DeploymentConfig) SetValidationLevel(level string) error {
	switch level {
	case "ERROR":
		dc.Config.ValidationLevel = validationError
	case "WARNING":
		dc.Config.ValidationLevel = validationWarning
	case "IGNORE":
		dc.Config.ValidationLevel = validationIgnore
	default:
		return fmt.Errorf("invalid validation level (\"ERROR\", \"WARNING\", \"IGNORE\")")
	}

	return nil
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
	Inputs    map[string]interface{}
	Skip      bool
}

func (v *validatorConfig) check(name validatorName, requiredInputs []string) error {
	if v.Validator != name.String() {
		return fmt.Errorf("passed wrong validator to %s implementation", name.String())
	}

	var errored bool
	for _, inp := range requiredInputs {
		if _, found := v.Inputs[inp]; !found {
			log.Printf("a required input %s was not provided to %s!", inp, v.Validator)
			errored = true
		}
	}

	if errored {
		return fmt.Errorf("at least one required input was not provided to %s", v.Validator)
	}

	// ensure that no extra inputs were provided by comparing length
	if len(requiredInputs) != len(v.Inputs) {
		errStr := "only %v inputs %s should be provided to %s"
		return fmt.Errorf(errStr, len(requiredInputs), requiredInputs, v.Validator)
	}

	return nil
}

// HasKind checks to see if a resource group contains any modules of the given
// kind. Note that a DeploymentGroup should never have more than one kind, this
// function is used in the validation step to ensure that is true.
func (g DeploymentGroup) HasKind(kind string) bool {
	for _, mod := range g.Modules {
		if mod.Kind == kind {
			return true
		}
	}
	return false
}

// Module stores YAML definition of an HPC cluster component defined in a blueprint
type Module struct {
	Source string
	// DeploymentSource - is source to be used for this module in written deployment.
	DeploymentSource string `yaml:"-"` // "-" prevents user from specifying it
	Kind             string
	ID               string
	Use              []string
	WrapSettingsWith map[string][]string
	Outputs          []string `yaml:"outputs,omitempty"`
	Settings         map[string]interface{}
	RequiredApis     map[string][]string `yaml:"required_apis"`
}

// createWrapSettingsWith ensures WrapSettingsWith field is not nil, if it is
// a new map is created.
func (m *Module) createWrapSettingsWith() {
	if m.WrapSettingsWith == nil {
		m.WrapSettingsWith = make(map[string][]string)
	}
}

// Blueprint stores the contents on the User YAML
// omitempty on validation_level ensures that expand will not expose the setting
// unless it has been set to a non-default value; the implementation as an
// integer is primarily for internal purposes even if it can be set in blueprint
type Blueprint struct {
	BlueprintName            string `yaml:"blueprint_name"`
	Validators               []validatorConfig
	ValidationLevel          int `yaml:"validation_level,omitempty"`
	Vars                     map[string]interface{}
	DeploymentGroups         []DeploymentGroup `yaml:"deployment_groups"`
	TerraformBackendDefaults TerraformBackend  `yaml:"terraform_backend_defaults"`
}

// connectionKind defines tracks graph edges between modules and from modules to
// deployment variables:
//
// use: created via module-module use keyword
// deployment: created by a module setting equal to $(vars.name)
// explicit: created by a module setting equal to $(mod_id.output)
//
// no attempt is made to track edges made via Toolkit literal strings presently
// required when wanting to index a list or map "((mod_id.output[0]))"
type connectionKind int

const (
	undefinedConnection connectionKind = iota
	useConnection
	deploymentConnection
	explicitConnection
)

func (c connectionKind) IsValid() bool {
	return c == useConnection || c == deploymentConnection || c == explicitConnection
}

// ModConnection defines details about connections between modules. Currently,
// only modules connected with "use" are tracked.
type ModConnection struct {
	ref             reference
	kind            connectionKind
	sharedVariables []string
}

// Returns true if a connection does not functionally link the outputs and
// inputs of the modules. This can happen when a module is connected with "use"
// but none of the outputs of fromID match the inputs of toID.
func (mc *ModConnection) isUnused() bool {
	return mc.kind == useConnection && len(mc.sharedVariables) == 0
}

// DeploymentConfig is a container for the imported YAML data and supporting data for
// creating the blueprint from it
type DeploymentConfig struct {
	Config Blueprint
	// Indexed by Resource Group name and Module Source
	ModulesInfo       map[string]map[string]modulereader.ModuleInfo
	expanded          bool
	moduleConnections map[string][]ModConnection
}

// ExpandConfig expands the yaml config in place
func (dc *DeploymentConfig) ExpandConfig() error {
	if err := dc.checkMovedModules(); err != nil {
		return err
	}
	dc.addKindToModules()
	dc.setModulesInfo()
	dc.validateConfig()
	dc.expand()
	dc.validate()
	dc.expanded = true
	return nil
}

func (dc *DeploymentConfig) addModuleConnection(ref reference, kind connectionKind, sharedVariables []string) error {
	if dc.moduleConnections == nil {
		dc.moduleConnections = make(map[string][]ModConnection)
	}

	if !kind.IsValid() {
		log.Fatal(unexpectedConnectionKind)
	}

	conn := ModConnection{
		ref:             ref,
		kind:            kind,
		sharedVariables: sharedVariables,
	}

	fromModID := ref.FromModuleID()
	dc.moduleConnections[fromModID] = append(dc.moduleConnections[fromModID], conn)
	return nil
}

// listUnusedModules provides a mapping of modules to modules that are in the
// "use" field, but not actually used.
func (dc *DeploymentConfig) listUnusedModules() map[string][]string {
	unusedModules := make(map[string][]string)
	for _, connections := range dc.moduleConnections {
		for _, conn := range connections {
			if conn.isUnused() {
				fromMod := conn.ref.FromModuleID()
				unusedModules[fromMod] = append(unusedModules[fromMod], conn.ref.ToModuleID())
			}
		}
	}
	return unusedModules
}

func (dc *DeploymentConfig) listUnusedDeploymentVariables() []string {
	// these variables are required or automatically constructed and applied;
	// these should not be listed unused otherwise no blueprints are valid
	var usedVars = map[string]bool{
		"labels":          true,
		"deployment_name": true,
	}

	for _, connections := range dc.moduleConnections {
		for _, conn := range connections {
			if conn.kind == deploymentConnection {
				for _, v := range conn.sharedVariables {
					usedVars[v] = true
				}
			}
		}
	}

	unusedVars := []string{}
	for k := range dc.Config.Vars {
		if _, ok := usedVars[k]; !ok {
			unusedVars = append(unusedVars, k)
		}
	}

	return unusedVars
}

func (dc *DeploymentConfig) checkMovedModules() error {
	var err error
	for _, grp := range dc.Config.DeploymentGroups {
		for _, mod := range grp.Modules {
			if replacingMod, ok := movedModules[strings.Trim(mod.Source, "./")]; ok {
				err = fmt.Errorf("the blueprint references modules that have moved")
				fmt.Printf(
					"A module you are using has moved. %s has been replaced with %s. Please update the source in your blueprint and try again.\n",
					mod.Source, replacingMod)
			}
		}
	}
	return err
}

// NewDeploymentConfig is a constructor for DeploymentConfig
func NewDeploymentConfig(configFilename string) (DeploymentConfig, error) {
	var newDeploymentConfig DeploymentConfig
	blueprint, err := importBlueprint(configFilename)
	if err != nil {
		return newDeploymentConfig, err
	}

	newDeploymentConfig = DeploymentConfig{
		Config:            blueprint,
		moduleConnections: make(map[string][]ModConnection),
	}
	return newDeploymentConfig, nil
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

	// Ensure Vars is not a nil map if not set by the user
	if len(blueprint.Vars) == 0 {
		blueprint.Vars = make(map[string]interface{})
	}

	// if the validation level has been explicitly set to an invalid value
	// in YAML blueprint then silently default to validationError
	if !isValidValidationLevel(blueprint.ValidationLevel) {
		blueprint.ValidationLevel = validationError
	}

	return blueprint, nil
}

// ExportBlueprint exports the internal representation of a blueprint config
func (dc DeploymentConfig) ExportBlueprint(outputFilename string) ([]byte, error) {
	var buf bytes.Buffer
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

func createModuleInfo(
	deploymentGroup DeploymentGroup) map[string]modulereader.ModuleInfo {
	modsInfo := make(map[string]modulereader.ModuleInfo)
	for _, mod := range deploymentGroup.Modules {
		if _, exists := modsInfo[mod.Source]; !exists {
			ri, err := modulereader.GetModuleInfo(mod.Source, mod.Kind)
			if err != nil {
				log.Fatalf(
					"failed to get info for module at %s while setting dc.ModulesInfo: %e",
					mod.Source, err)
			}
			modsInfo[mod.Source] = ri
		}
	}
	return modsInfo
}

// addKindToModules sets the kind to 'terraform' when empty.
func (dc *DeploymentConfig) addKindToModules() {
	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			if mod.Kind == "" {
				dc.Config.DeploymentGroups[iGrp].Modules[iMod].Kind =
					"terraform"
			}
		}
	}
}

// setModulesInfo populates needed information from modules
func (dc *DeploymentConfig) setModulesInfo() {
	dc.ModulesInfo = make(map[string]map[string]modulereader.ModuleInfo)
	for _, grp := range dc.Config.DeploymentGroups {
		dc.ModulesInfo[grp.Name] = createModuleInfo(grp)
	}
}

func validateGroupName(name string, usedNames map[string]bool) {
	if name == "" {
		log.Fatal(errorMessages["emptyGroupName"])
	}
	if hasIllegalChars(name) {
		log.Fatalf("%s %s", errorMessages["illegalChars"], name)
	}
	if _, ok := usedNames[name]; ok {
		log.Fatalf(
			"%s: %s used more than once", errorMessages["duplicateGroup"], name)
	}
	usedNames[name] = true
}

// checkModuleAndGroupNames checks and imports module and resource group IDs
// and names respectively.
func checkModuleAndGroupNames(groups []DeploymentGroup) error {
	seenMod := map[string]bool{}
	groupNames := make(map[string]bool)
	for ig := range groups {
		grp := &groups[ig]
		validateGroupName(grp.Name, groupNames)
		for _, mod := range grp.Modules {
			if seenMod[mod.ID] {
				return fmt.Errorf("%s: %s used more than once", errorMessages["duplicateID"], mod.ID)
			}
			seenMod[mod.ID] = true

			// Verify Module Kind matches group Kind
			if grp.Kind == "" {
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

func modToGrp(groups []DeploymentGroup, modID string) (int, error) {
	i := slices.IndexFunc(groups, func(g DeploymentGroup) bool {
		return slices.ContainsFunc(g.Modules, func(m Module) bool {
			return m.ID == modID
		})
	})
	if i == -1 {
		return -1, fmt.Errorf("module %s was not found", modID)
	}
	return i, nil
}

// checkUsedModuleNames verifies that any used modules have valid names and
// are in the correct group
func checkUsedModuleNames(bp Blueprint) error {
	for _, grp := range bp.DeploymentGroups {
		for _, mod := range grp.Modules {
			for _, usedMod := range mod.Use {
				ref, err := identifyModuleByReference(usedMod, grp, mod)
				if err != nil {
					return err
				}

				if err = ref.validate(bp); err != nil {
					return err
				}

				// TODO: remove this when support is added!
				if ref.fromGroupID != ref.toGroupID {
					return fmt.Errorf("%s: %s is an intergroup reference",
						errorMessages["varInAnotherGroup"], usedMod)
				}
			}
		}
	}
	return nil
}

func checkBackend(b TerraformBackend) error {
	const errMsg = "can not use variables in backend block, got '%s=%s'"
	if hasVariable(b.Type) {
		return fmt.Errorf(errMsg, "type", b.Type)
	}
	for k, v := range b.Configuration {
		if s, ok := v.(string); ok && hasVariable(s) {
			return fmt.Errorf(errMsg, k, s)
		}
	}
	return nil
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
	if err = checkModuleAndGroupNames(dc.Config.DeploymentGroups); err != nil {
		log.Fatal(err)
	}
	if err = checkUsedModuleNames(dc.Config); err != nil {
		log.Fatal(err)
	}
	if err = checkBackends(dc.Config); err != nil {
		log.Fatal(err)
	}
}

// SetCLIVariables sets the variables at CLI
func (dc *DeploymentConfig) SetCLIVariables(cliVariables []string) error {
	for _, cliVar := range cliVariables {
		arr := strings.SplitN(cliVar, "=", 2)

		if len(arr) != 2 {
			return fmt.Errorf("invalid format: '%s' should follow the 'name=value' format", cliVar)
		}

		// Convert the variable's string litteral to its equivalent default type.
		var out interface{}
		err := yaml.Unmarshal([]byte(arr[1]), &out)
		if err != nil {
			return fmt.Errorf("invalid input: unable to convert '%s' value '%s' to known type", arr[0], arr[1])
		}

		key, value := arr[0], out
		dc.Config.Vars[key] = value
	}

	return nil
}

// SetBackendConfig sets the backend config variables at CLI
func (dc *DeploymentConfig) SetBackendConfig(cliBEConfigVars []string) error {
	// Set "gcs" as default value when --backend-config is specified at CLI
	if len(cliBEConfigVars) > 0 {
		dc.Config.TerraformBackendDefaults.Type = "gcs"
		dc.Config.TerraformBackendDefaults.Configuration = make(map[string]interface{})
	}

	for _, config := range cliBEConfigVars {
		arr := strings.SplitN(config, "=", 2)

		if len(arr) != 2 {
			return fmt.Errorf("invalid format: '%s' should follow the 'name=value' format", config)
		}

		key, value := arr[0], arr[1]
		switch key {
		case "type":
			dc.Config.TerraformBackendDefaults.Type = value
		default:
			dc.Config.TerraformBackendDefaults.Configuration[key] = value
		}

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

// IsLiteralVariable returns true if string matches variable ((ctx.name))
func IsLiteralVariable(str string) bool {
	return literalExp.MatchString(str)
}

// IdentifyLiteralVariable returns
// string: variable source (e.g. global "vars" or module "modname")
// string: variable name (e.g. "project_id")
// bool: true/false reflecting success
func IdentifyLiteralVariable(str string) (string, string, bool) {
	contents := literalSplitExp.FindStringSubmatch(str)
	if len(contents) != 3 {
		return "", "", false
	}

	return contents[1], contents[2], true
}

// HandleLiteralVariable is exported for use in modulewriter as well
func HandleLiteralVariable(str string) string {
	contents := literalExp.FindStringSubmatch(str)
	if len(contents) != 2 {
		log.Fatalf("Incorrectly formatted literal variable: %s", str)
	}

	return strings.TrimSpace(contents[1])
}

// ConvertToCty convert interface directly to a cty.Value
func ConvertToCty(val interface{}) (cty.Value, error) {
	// Convert to JSON bytes
	jsonBytes, err := json.Marshal(val)
	if err != nil {
		return cty.Value{}, err
	}

	// Unmarshal JSON into cty
	simpleJSON := ctyJson.SimpleJSONValue{}
	simpleJSON.UnmarshalJSON(jsonBytes)
	return simpleJSON.Value, nil
}

// ConvertMapToCty convert an interface map to a map of cty.Values
func ConvertMapToCty(iMap map[string]interface{}) (map[string]cty.Value, error) {
	cMap := make(map[string]cty.Value)
	for k, v := range iMap {
		convertedVal, err := ConvertToCty(v)
		if err != nil {
			return cMap, err
		}
		cMap[k] = convertedVal
	}
	return cMap, nil
}

// ResolveVariables is given two maps of strings to cty.Value types, one
// representing a list of settings or variables to resolve (ctyMap) and other
// representing variables used to resolve (origin). This function will
// examine all cty.Values that are of type cty.String. If they are literal
// global variables, then they are replaced by the cty.Value of the
// corresponding entry in the origin. All other cty.Values are unmodified.
// ERROR: if (somehow) the cty.String cannot be converted to a Go string
// ERROR: rely on HCL TraverseAbs to bubble up "diagnostics" when the global
// variable being resolved does not exist in b.Vars
func ResolveVariables(
	ctyMap map[string]cty.Value,
	origin map[string]cty.Value,
) error {
	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{"var": cty.ObjectVal(origin)},
	}
	for key, val := range ctyMap {
		if val.Type() == cty.String {
			var valString string
			if err := gocty.FromCtyValue(val, &valString); err != nil {
				return err
			}
			ctx, varName, found := IdentifyLiteralVariable(valString)
			// only attempt resolution on global literal variables
			// leave all other strings alone (including non-global)
			if found && ctx == "var" {
				varTraversal := hcl.Traversal{
					hcl.TraverseRoot{Name: ctx},
					hcl.TraverseAttr{Name: varName},
				}
				newVal, diags := varTraversal.TraverseAbs(evalCtx)
				if diags.HasErrors() {
					return diags
				}
				ctyMap[key] = newVal
			}
		}
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
func (b *Blueprint) DeploymentName() (string, error) {
	nameInterface, found := b.Vars["deployment_name"]
	if !found {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["varNotFound"],
		}
	}

	deploymentName, ok := nameInterface.(string)
	if !ok {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["valueNotString"],
		}
	}

	if len(deploymentName) == 0 {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["valueEmptyString"],
		}
	}

	// Check that deployment_name is a valid label
	if !isValidLabelValue(deploymentName) {
		return "", &InputValueError{
			inputKey: "deployment_name",
			cause:    errorMessages["labelReqs"],
		}
	}

	return deploymentName, nil
}

// checkBlueprintName returns an error if blueprint_name does not comply with
// requirements for correct GCP label values.
func (b *Blueprint) checkBlueprintName() error {

	if len(b.BlueprintName) == 0 {
		return &InputValueError{
			inputKey: "blueprint_name",
			cause:    errorMessages["valueEmptyString"],
		}
	}

	if !isValidLabelValue(b.BlueprintName) {
		return &InputValueError{
			inputKey: "blueprint_name",
			cause:    errorMessages["labelReqs"],
		}
	}

	return nil
}
