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

package config

import (
	"fmt"
	"log"
	"regexp"
	"strings"

	"path/filepath"

	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	blueprintLabel  string = "ghpc_blueprint"
	deploymentLabel string = "ghpc_deployment"
	roleLabel       string = "ghpc_role"
)

var (
	// Checks if a variable exists only as a substring, ex:
	// Matches: "a$(vars.example)", "word $(vars.example)", "word$(vars.example)", "$(vars.example)"
	// Doesn't match: "\$(vars.example)", "no variable in this string"
	anyVariableExp    *regexp.Regexp = regexp.MustCompile(`(^|[^\\])\$\((.*?)\)`)
	literalExp        *regexp.Regexp = regexp.MustCompile(`^\(\((.*)\)\)$`)
	simpleVariableExp *regexp.Regexp = regexp.MustCompile(`^\$\((.*)\)$`)
	// the greediness and non-greediness of expression below is important
	// consume all whitespace at beginning and end
	// consume only up to first period to get variable source
	// consume only up to whitespace to get variable name
	literalSplitExp *regexp.Regexp = regexp.MustCompile(`^\(\([[:space:]]*(.*?)\.(.*?)[[:space:]]*\)\)$`)
)

// expand expands variables and strings in the yaml config. Used directly by
// ExpandConfig for the create and expand commands.
func (dc *DeploymentConfig) expand() {
	if err := dc.addMetadataToModules(); err != nil {
		log.Printf("could not determine required APIs: %v", err)
	}

	if err := dc.expandBackends(); err != nil {
		log.Fatalf("failed to apply default backend to deployment groups: %v", err)
	}

	if err := dc.addDefaultValidators(); err != nil {
		log.Fatalf(
			"failed to update validators when expanding the config: %v", err)
	}

	if err := dc.combineLabels(); err != nil {
		log.Fatalf(
			"failed to update module labels when expanding the config: %v", err)
	}

	if err := dc.applyUseModules(); err != nil {
		log.Fatalf(
			"failed to apply \"use\" modules when expanding the config: %v", err)
	}

	if err := dc.applyGlobalVariables(); err != nil {
		log.Fatalf(
			"failed to apply deployment variables in modules when expanding the config: %v",
			err)
	}

	dc.Config.populateOutputs()
}

func (dc *DeploymentConfig) addMetadataToModules() error {
	for iGrp, grp := range dc.Config.DeploymentGroups {
		for iMod, mod := range grp.Modules {
			if mod.RequiredApis == nil {
				if dc.Config.Vars.Get("project_id").Type() != cty.String {
					return fmt.Errorf("global variable project_id must be defined")
				}

				// handle possibility that ModulesInfo does not have this module in it
				// this occurs in unit testing because they do not run dc.ExpandConfig()
				// and dc.setModulesInfo()
				requiredAPIs := dc.ModulesInfo[grp.Name][mod.Source].RequiredApis
				if requiredAPIs == nil {
					requiredAPIs = []string{}
				}
				dc.Config.DeploymentGroups[iGrp].Modules[iMod].RequiredApis = map[string][]string{
					"$(vars.project_id)": requiredAPIs,
				}
			}
		}
	}
	return nil
}

func (dc *DeploymentConfig) expandBackends() error {
	// 1. DEFAULT: use TerraformBackend configuration (if supplied) in each
	//    resource group
	// 2. If top-level TerraformBackendDefaults is defined, insert that
	//    backend into resource groups which have no explicit
	//    TerraformBackend
	// 3. In all cases, add a prefix for GCS backends if one is not defined
	blueprint := &dc.Config
	defaults := blueprint.TerraformBackendDefaults
	if defaults.Type != "" {
		for i := range blueprint.DeploymentGroups {
			grp := &blueprint.DeploymentGroups[i]
			be := &grp.TerraformBackend
			if be.Type == "" {
				be.Type = defaults.Type
				be.Configuration = Dict{}
				for k, v := range defaults.Configuration.Items() {
					be.Configuration.Set(k, v)
				}
			}
			if be.Type == "gcs" && !be.Configuration.Has("prefix") {
				prefix := blueprint.BlueprintName
				if deployment, err := blueprint.DeploymentName(); err == nil {
					prefix += "/" + deployment
				}
				prefix += "/" + grp.Name
				be.Configuration.Set("prefix", cty.StringVal(prefix))
			}
		}
	}
	return nil
}

func getModuleInputMap(inputs []modulereader.VarInfo) map[string]string {
	modInputs := make(map[string]string)
	for _, input := range inputs {
		modInputs[input.Name] = input.Type
	}
	return modInputs
}

// initialize a Toolkit setting that corresponds to a module input of type list
// create new list if unset, append if already set, error if value not a list
func (mod *Module) addListValue(settingName string, value cty.Value) error {
	var cur []cty.Value
	if !mod.Settings.Has(settingName) {
		mod.createWrapSettingsWith()
		mod.WrapSettingsWith[settingName] = []string{"flatten([", "])"}
		cur = []cty.Value{}
	} else {
		v := mod.Settings.Get(settingName)
		ty := v.Type()
		if !ty.IsTupleType() && !ty.IsSetType() && !ty.IsSetType() {
			return fmt.Errorf("%s: module %s, setting %s", errorMessages["appendToNonList"], mod.ID, settingName)
		}
		cur = mod.Settings.Get(settingName).AsValueSlice()
	}
	mod.Settings.Set(settingName, cty.TupleVal(append(cur, value)))
	return nil
}

// useModule matches input variables in a "using" module to output values
// from a "used" module. It may be used iteratively to successively apply used
// modules in order of precedence. New input variables are added to the using
// module as Toolkit variable references (in same format as a blueprint). If
// the input variable already has a setting, it is ignored, unless the value is
// a list, in which case output values are appended and flattened using HCL.
//
//	mod: "using" module as defined above
//	useMod: "used" module as defined above
//	useModGroupID: deployment group ID to which useMod belongs
//	modInputs: input variables as defined by the using module code
//	useOutputs: output values as defined by the used module code
//	settingsToIgnore: a list of module settings not to modify for any reason;
//	 typical usage will be to leave explicit blueprint settings unmodified
func useModule(
	mod *Module,
	useMod Module,
	modInputs []modulereader.VarInfo,
	useOutputs []modulereader.OutputInfo,
	settingsToIgnore []string,
) error {
	modInputsMap := getModuleInputMap(modInputs)
	for _, useOutput := range useOutputs {
		settingName := useOutput.Name

		// explicitly ignore these settings (typically those in blueprint)
		if slices.Contains(settingsToIgnore, settingName) {
			continue
		}

		// Skip settings that do not have matching module inputs
		inputType, ok := modInputsMap[settingName]
		if !ok {
			continue
		}

		// skip settings that are not of list type, but already have a value
		// these were probably added by a previous call to this function
		alreadySet := mod.Settings.Has(settingName)
		isList := strings.HasPrefix(inputType, "list")
		if alreadySet && !isList {
			continue
		}

		v := ModuleRef(useMod.ID, settingName).
			AsExpression().
			AsValue().
			Mark(ProductOfModuleUse{Module: useMod.ID})

		if !isList {
			mod.Settings.Set(settingName, v)
		} else {
			if err := mod.addListValue(settingName, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// applyUseModules applies variables from modules listed in the "use" field
// when/if applicable
func (dc *DeploymentConfig) applyUseModules() error {
	for iGrp := range dc.Config.DeploymentGroups {
		group := &dc.Config.DeploymentGroups[iGrp]
		grpModsInfo := dc.ModulesInfo[group.Name]
		for iMod := range group.Modules {
			fromMod := &group.Modules[iMod]
			fromModInfo := grpModsInfo[fromMod.Source]
			settingsInBlueprint := maps.Keys(fromMod.Settings.Items())
			for _, toModID := range fromMod.Use {
				// turn the raw string into a modReference struct
				// which was previously validated by checkUsedModuleNames
				// this will enable us to get structs about the module being
				// used and search it for outputs that match inputs in the
				// current module (the iterator)
				modRef, err := identifyModuleByReference(toModID, dc.Config, fromMod.ID)
				if err != nil {
					return err
				}

				// to get the module struct, we first needs its group
				toGroup, err := dc.getGroupByID(modRef.toGroupID)
				if err != nil {
					return err
				}

				// this module contains information about the target module that
				// was specified by the user in the blueprint
				toMod, err := toGroup.getModuleByID(modRef.toModuleID)
				if err != nil {
					return err
				}

				// Packer modules cannot be used because they do not have a
				// native concept of outputs. Without this, the validator
				// that checks for matching inputs will always trigger
				if toMod.Kind == PackerKind {
					return fmt.Errorf("%s: %s", errorMessages["cannotUsePacker"], toMod.ID)
				}

				// this struct contains the underlying module implementation,
				// not just what the user specified in blueprint. e.g. module
				// input variables and output values
				// this line should probably be tested for success and unit
				// tested but it our unit test infrastructure does not support
				// running dc.setModulesInfo() on our test configurations
				toModInfo := dc.ModulesInfo[toGroup.Name][toMod.Source]
				err = useModule(fromMod, toMod,
					fromModInfo.Inputs, toModInfo.Outputs, settingsInBlueprint)
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (dc DeploymentConfig) moduleHasInput(
	depGroup string, source string, inputName string) bool {
	for _, input := range dc.ModulesInfo[depGroup][source].Inputs {
		if input.Name == inputName {
			return true
		}
	}
	return false
}

// Returns enclosing directory of source directory.
func getRole(source string) string {
	role := filepath.Base(filepath.Dir(source))
	// Returned by base if containing directory was not explicit
	invalidRoles := []string{"..", ".", "/"}
	for _, ir := range invalidRoles {
		if role == ir {
			return "other"
		}
	}
	return role
}

// combineLabels sets defaults for labels based on other variables and merges
// the global labels defined in Vars with module setting labels. It also
// determines the role and sets it for each module independently.
func (dc *DeploymentConfig) combineLabels() error {
	vars := &dc.Config.Vars
	defaults := map[string]cty.Value{
		blueprintLabel:  cty.StringVal(dc.Config.BlueprintName),
		deploymentLabel: vars.Get("deployment_name"),
	}
	labels := "labels"
	if !vars.Has(labels) { // Shouldn't happen if blueprint was properly constructed
		vars.Set(labels, cty.EmptyObjectVal)
	}
	gl := mergeLabels(vars.Get(labels).AsValueMap(), defaults)
	vars.Set(labels, cty.ObjectVal(gl))

	return dc.Config.WalkModules(func(mod *Module) error {
		return combineModuleLabels(mod, *dc)
	})
}

func combineModuleLabels(mod *Module, dc DeploymentConfig) error {
	grp := dc.Config.ModuleGroupOrDie(mod.ID)
	mod.createWrapSettingsWith()
	labels := "labels"

	// previously expanded blueprint, user written BPs do not use `WrapSettingsWith`
	if _, ok := mod.WrapSettingsWith[labels]; ok {
		return nil // Do nothing
	}

	// Check if labels are set for this module
	if !dc.moduleHasInput(grp.Name, mod.Source, labels) {
		return nil
	}

	modLabels := map[string]cty.Value{}
	if mod.Settings.Has(labels) {
		// Cast into map so we can index into them
		v := mod.Settings.Get(labels)
		ty := v.Type()
		if !ty.IsObjectType() && !ty.IsMapType() {
			return fmt.Errorf("%s, Module %s, labels type: %s",
				errorMessages["settingsLabelType"], mod.ID, ty.FriendlyName())
		}
		if v.AsValueMap() != nil {
			modLabels = v.AsValueMap()
		}
	}
	// Add the role (e.g. compute, network, etc)
	if _, exists := modLabels[roleLabel]; !exists {
		modLabels[roleLabel] = cty.StringVal(getRole(mod.Source))
	}

	if mod.Kind == TerraformKind {
		// Terraform module labels to be expressed as
		// `merge(var.labels, { ghpc_role=..., **settings.labels })`
		mod.WrapSettingsWith[labels] = []string{"merge(", ")"}
		ref := GlobalRef(labels).AsExpression()
		args := []cty.Value{ref.AsValue(), cty.ObjectVal(modLabels)}
		mod.Settings.Set(labels, cty.TupleVal(args))
	} else if mod.Kind == PackerKind {
		g := dc.Config.Vars.Get(labels).AsValueMap()
		mod.Settings.Set(labels, cty.ObjectVal(mergeLabels(modLabels, g)))
	}
	return nil
}

// mergeLabels returns a new map with the keys from both maps. If a key exists in both maps,
// the value from the first map is used.
func mergeLabels(a map[string]cty.Value, b map[string]cty.Value) map[string]cty.Value {
	r := map[string]cty.Value{}
	for k, v := range a {
		r[k] = v
	}
	for k, v := range b {
		if _, exists := a[k]; !exists {
			r[k] = v
		}
	}
	return r
}

func (dc *DeploymentConfig) applyGlobalVarsInGroup(groupIndex int) error {
	deploymentGroup := dc.Config.DeploymentGroups[groupIndex]
	modInfo := dc.ModulesInfo[deploymentGroup.Name]

	for im := range deploymentGroup.Modules {
		mod := &deploymentGroup.Modules[im]
		for _, input := range modInfo[mod.Source].Inputs {
			// Module setting exists? Nothing more needs to be done.
			if mod.Settings.Has(input.Name) {
				continue
			}

			// If it's not set, is there a global we can use?
			if dc.Config.Vars.Has(input.Name) {
				ref := GlobalRef(input.Name)
				mod.Settings.Set(input.Name, ref.AsExpression().AsValue())
				continue
			}

			if input.Required {
				// It's not explicitly set, and not global is set
				// Fail if no default has been set
				return fmt.Errorf("%s: Module ID: %s Setting: %s",
					errorMessages["missingSetting"], mod.ID, input.Name)
			}
			// Default exists, the module will handle it
		}
	}
	return nil
}

// applyGlobalVariables takes any variables defined at the global level and
// applies them to module settings if not already set.
func (dc *DeploymentConfig) applyGlobalVariables() error {
	for groupIndex := range dc.Config.DeploymentGroups {
		err := dc.applyGlobalVarsInGroup(groupIndex)
		if err != nil {
			return err
		}
	}
	return nil
}

/*
A module reference is made by the use keyword and is subject to IGC constraints
of references (ordering, explicitness). It has the following fields:
  - toModuleID: the target module ID
  - fromModuleID: the source module ID
  - toGroupID: the deployment group in which the module is *expected* to be found
  - fromGroupID: the deployment group from which the reference is made
  - explicit: a boolean value indicating whether the user made a reference that
    explicitly identified toGroupID rather than inferring it using fromGroupID
*/
type modReference struct {
	toModuleID   string
	fromModuleID string
	toGroupID    string
	fromGroupID  string
}

/*
This function performs only the most rudimentary conversion of an input
string into a modReference struct as defined above. This function does not
ensure the existence of the module!
*/
func identifyModuleByReference(yamlReference string, bp Blueprint, fromMod string) (modReference, error) {
	// struct defaults: empty strings and false booleans
	var ref modReference
	ref.fromModuleID = fromMod
	ref.toModuleID = yamlReference

	fromG, err := bp.ModuleGroup(fromMod)
	if err != nil {
		return modReference{}, err
	}
	ref.fromGroupID = fromG.Name

	toG, err := bp.ModuleGroup(ref.toModuleID)
	if err != nil {
		return modReference{}, err
	}
	ref.toGroupID = toG.Name

	// should consider more sophisticated definition of valid values here.
	// for now check that no fields are the empty string; due to the default
	// zero values for strings in the "ref" struct, this will also cover the
	// case that modComponents has wrong # of fields
	if ref.fromModuleID == "" || ref.toModuleID == "" || ref.fromGroupID == "" || ref.toGroupID == "" {
		return ref, fmt.Errorf("%s: %s, expected %s",
			errorMessages["invalidMod"], yamlReference, expectedModFormat)
	}

	return ref, nil
}

// AutomaticOutputName generates unique deployment-group-level output names
func AutomaticOutputName(outputName string, moduleID string) string {
	return outputName + "_" + moduleID
}

func (ref modReference) validate(bp Blueprint) error {
	callingModuleGroupIndex := slices.IndexFunc(bp.DeploymentGroups, func(d DeploymentGroup) bool { return d.Name == ref.fromGroupID })
	if callingModuleGroupIndex == -1 {
		return fmt.Errorf("%s: %s", errorMessages["groupNotFound"], ref.fromGroupID)
	}

	targetModuleGroupIndex, err := modToGrp(bp.DeploymentGroups, ref.toModuleID)
	if err != nil {
		return err
	}
	targetModuleGroupName := bp.DeploymentGroups[targetModuleGroupIndex].Name

	// Ensure module is from the correct group
	isInterGroupReference := callingModuleGroupIndex != targetModuleGroupIndex
	isRefToLaterGroup := targetModuleGroupIndex > callingModuleGroupIndex
	isCorrectToGroup := ref.toGroupID == targetModuleGroupName

	if isInterGroupReference {
		if isRefToLaterGroup {
			return fmt.Errorf("%s: %s is in a later group",
				errorMessages["intergroupOrder"], ref.toModuleID)
		}
	}

	// at this point, the reference may be intergroup or intragroup. now we
	// only care about correctness of target group ID. better to order this
	// error after enforcing explicitness of intergroup references
	if !isCorrectToGroup {
		return fmt.Errorf("%s: %s.%s",
			errorMessages["referenceWrongGroup"], ref.toGroupID, ref.toModuleID)
	}

	return nil
}

// Validates that references in module settings are valid:
// * referenced deployment variable does exist;
// * referenced module output does exist;
// * doesn't reference an output of module in a later group.
func validateModuleSettingReference(bp Blueprint, mod Module, r Reference) error {
	// simplest case to evaluate is a deployment variable's existence
	if r.GlobalVar {
		if !bp.Vars.Has(r.Name) {
			return fmt.Errorf("module %#v references unknown global variable %#v", mod.ID, r.Name)
		}
		return nil
	}
	g := bp.ModuleGroupOrDie(mod.ID)
	callingModuleGroupIndex := slices.IndexFunc(bp.DeploymentGroups, func(d DeploymentGroup) bool { return d.Name == g.Name })

	targetModuleGroupIndex, err := modToGrp(bp.DeploymentGroups, r.Module)
	if err != nil {
		return err
	}
	targetModuleGroup := bp.DeploymentGroups[targetModuleGroupIndex]

	// references must refer to the same or an earlier group;
	if targetModuleGroupIndex > callingModuleGroupIndex {
		return fmt.Errorf("%s: %s is in the later group %s", errorMessages["intergroupOrder"], r.Module, targetModuleGroup.Name)
	}

	// at this point, we have a valid intragroup or intergroup references to a
	// module. must now determine whether the output value actually exists in
	// the module.
	refModIndex := slices.IndexFunc(targetModuleGroup.Modules, func(m Module) bool { return m.ID == r.Module })
	if refModIndex == -1 {
		log.Fatalf("Could not find module %s", r.Module)
	}
	refMod := targetModuleGroup.Modules[refModIndex]
	if refMod.Kind == PackerKind {
		return fmt.Errorf("module %s cannot be referenced because packer modules have no outputs", refMod.ID)
	}

	modInfo, err := modulereader.GetModuleInfo(refMod.Source, refMod.Kind.String())
	if err != nil {
		log.Fatalf("failed to get info for module at %s: %v", refMod.Source, err)
	}
	found := slices.ContainsFunc(modInfo.Outputs, func(o modulereader.OutputInfo) bool { return o.Name == r.Name })
	if !found {
		return fmt.Errorf("%s: module %s did not have output %s",
			errorMessages["noOutput"], refMod.ID, r.Name)
	}
	return nil
}

// isSimpleVariable checks if the entire string is just a single variable
func isSimpleVariable(str string) bool {
	return simpleVariableExp.MatchString(str)
}

// hasVariable checks to see if any variable exists in a string
func hasVariable(str string) bool {
	return anyVariableExp.MatchString(str)
}

// this function adds default validators to the blueprint.
// default validators are only added for global variables that exist
func (dc *DeploymentConfig) addDefaultValidators() error {
	if dc.Config.Validators == nil {
		dc.Config.Validators = []validatorConfig{}
	}

	projectIDExists := dc.Config.Vars.Has("project_id")
	projectRef := GlobalRef("project_id").AsExpression().AsValue()

	regionExists := dc.Config.Vars.Has("region")
	regionRef := GlobalRef("region").AsExpression().AsValue()

	zoneExists := dc.Config.Vars.Has("zone")
	zoneRef := GlobalRef("zone").AsExpression().AsValue()

	defaults := []validatorConfig{
		{Validator: testModuleNotUsedName.String()},
		{Validator: testDeploymentVariableNotUsedName.String()}}

	// always add the project ID validator before subsequent validators that can
	// only succeed if credentials can access the project. If the project ID
	// validator fails, all remaining validators are not executed.
	if projectIDExists {
		defaults = append(defaults, validatorConfig{
			Validator: testProjectExistsName.String(),
			Inputs:    NewDict(map[string]cty.Value{"project_id": projectRef}),
		})
	}

	// it is safe to run this validator even if vars.project_id is undefined;
	// it will likely fail but will do so helpfully to the user
	defaults = append(defaults,
		validatorConfig{Validator: "test_apis_enabled"})

	if projectIDExists && regionExists {
		defaults = append(defaults, validatorConfig{
			Validator: testRegionExistsName.String(),
			Inputs: NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"region":     regionRef,
			},
			)})
	}

	if projectIDExists && zoneExists {
		defaults = append(defaults, validatorConfig{
			Validator: testZoneExistsName.String(),
			Inputs: NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"zone":       zoneRef,
			}),
		})
	}

	if projectIDExists && regionExists && zoneExists {
		defaults = append(defaults, validatorConfig{
			Validator: testZoneInRegionName.String(),
			Inputs: NewDict(map[string]cty.Value{
				"project_id": projectRef,
				"region":     regionRef,
				"zone":       zoneRef,
			}),
		})
	}

	used := map[string]bool{}
	for _, v := range dc.Config.Validators {
		used[v.Validator] = true
	}

	for _, v := range defaults {
		if used[v.Validator] {
			continue
		}
		dc.Config.Validators = append(dc.Config.Validators, v)
	}

	return nil
}

// FindIntergroupReferences finds all references to other groups used in the given value
func FindIntergroupReferences(v cty.Value, mod Module, bp Blueprint) []Reference {
	g := bp.ModuleGroupOrDie(mod.ID)
	res := map[Reference]bool{}
	cty.Walk(v, func(p cty.Path, v cty.Value) (bool, error) {
		e, is := IsExpressionValue(v)
		if !is {
			return true, nil
		}
		for _, r := range e.References() {
			if !r.GlobalVar && bp.ModuleGroupOrDie(r.Module).Name != g.Name {
				res[r] = true
			}
		}
		return true, nil
	})
	return maps.Keys(res)
}

// find all intergroup references and add them to source Module.Outputs
func (bp *Blueprint) populateOutputs() {
	refs := map[Reference]bool{}
	bp.WalkModules(func(m *Module) error {
		rs := FindIntergroupReferences(m.Settings.AsObject(), *m, *bp)
		for _, r := range rs {
			refs[r] = true
		}
		return nil
	})

	bp.WalkModules(func(m *Module) error {
		for r := range refs {
			if r.Module != m.ID {
				continue // find IGC references pointing to this module
			}
			if slices.ContainsFunc(m.Outputs, func(o modulereader.OutputInfo) bool { return o.Name == r.Name }) {
				continue // output is already registered
			}
			m.Outputs = append(m.Outputs, modulereader.OutputInfo{
				Name:        r.Name,
				Description: "Automatically-generated output exported for use by later deployment groups",
				Sensitive:   true,
			})

		}
		return nil
	})
}
