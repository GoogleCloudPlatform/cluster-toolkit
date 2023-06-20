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
	simpleVariableExp *regexp.Regexp = regexp.MustCompile(`^\$\((.*)\)$`)
)

// expand expands variables and strings in the yaml config. Used directly by
// ExpandConfig for the create and expand commands.
func (dc *DeploymentConfig) expand() error {
	dc.expandBackends()
	dc.addDefaultValidators()

	if err := dc.combineLabels(); err != nil {
		return err
	}

	if err := dc.applyUseModules(); err != nil {
		return err
	}

	if err := dc.applyGlobalVariables(); err != nil {
		return err
	}

	dc.Config.populateOutputs()
	return nil
}

func (dc *DeploymentConfig) expandBackends() {
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
				prefix += "/" + string(grp.Name)
				be.Configuration.Set("prefix", cty.StringVal(prefix))
			}
		}
	}
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
func (mod *Module) addListValue(settingName string, value cty.Value) {
	args := []cty.Value{value}
	mods := map[ModuleID]bool{}
	for _, mod := range IsProductOfModuleUse(value) {
		mods[mod] = true
	}

	if mod.Settings.Has(settingName) {
		cur := mod.Settings.Get(settingName)
		for _, mod := range IsProductOfModuleUse(cur) {
			mods[mod] = true
		}
		args = append(args, cur)
	}

	exp := FunctionCallExpression("flatten", cty.TupleVal(args))
	val := AsProductOfModuleUse(exp.AsValue(), maps.Keys(mods)...)
	mod.Settings.Set(settingName, val)
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
//	settingsToIgnore: a list of module settings not to modify for any reason;
//	 typical usage will be to leave explicit blueprint settings unmodified
func useModule(
	mod *Module,
	useMod Module,
	settingsToIgnore []string,
) {
	modInputsMap := getModuleInputMap(mod.InfoOrDie().Inputs)
	for _, useOutput := range useMod.InfoOrDie().Outputs {
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

		v := AsProductOfModuleUse(
			ModuleRef(useMod.ID, settingName).AsExpression().AsValue(),
			useMod.ID)

		if !isList {
			mod.Settings.Set(settingName, v)
		} else {
			mod.addListValue(settingName, v)
		}
	}
}

// applyUseModules applies variables from modules listed in the "use" field
// when/if applicable
func (dc *DeploymentConfig) applyUseModules() error {
	return dc.Config.WalkModules(func(m *Module) error {
		settingsInBlueprint := maps.Keys(m.Settings.Items())
		for _, u := range m.Use {
			used, err := dc.Config.Module(u)
			if err != nil {
				return err
			}
			useModule(m, *used, settingsInBlueprint)
		}
		return nil
	})
}

func moduleHasInput(m Module, n string) bool {
	for _, input := range m.InfoOrDie().Inputs {
		if input.Name == n {
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
	gl := mergeMaps(defaults, vars.Get(labels).AsValueMap())
	vars.Set(labels, cty.ObjectVal(gl))

	return dc.Config.WalkModules(func(mod *Module) error {
		return combineModuleLabels(mod, *dc)
	})
}

func combineModuleLabels(mod *Module, dc DeploymentConfig) error {
	labels := "labels"

	if !moduleHasInput(*mod, labels) {
		return nil // no op
	}

	cur := mod.Settings.Get(labels)
	extra := map[string]cty.Value{roleLabel: cty.StringVal(getRole(mod.Source))}

	if mod.Kind == TerraformKind {
		mod.Settings.Set(labels, mergeLabelsTf(extra, cur))
	} else if mod.Kind == PackerKind {
		gl := dc.Config.Vars.Get(labels).AsValueMap()
		merged, err := mergeLabelsPkr(gl, extra, cur)
		if err != nil {
			return err
		}
		mod.Settings.Set(labels, merged)
	}
	return nil
}

// Terraform labels are `merge(var.labels, {ghpc_role="foo"}, [module labels])`
func mergeLabelsTf(extra map[string]cty.Value, cur cty.Value) cty.Value {
	args := []cty.Value{
		GlobalRef("labels").AsExpression().AsValue(),
		cty.ObjectVal(extra),
	}
	if !cur.IsNull() {
		args = append(args, cur)
	}
	return FunctionCallExpression("merge", args...).AsValue()
}

// Packer doesn't support `merge`, so merge it here.
func mergeLabelsPkr(global map[string]cty.Value, extra map[string]cty.Value, cur cty.Value) (cty.Value, error) {
	modLabels := map[string]cty.Value{}
	if !cur.IsNull() {
		ty := cur.Type()
		if !ty.IsObjectType() && !ty.IsMapType() {
			return cty.NilVal, fmt.Errorf("%s,labels type: %s", errorMessages["settingsLabelType"], ty.FriendlyName())
		}
		if cur.AsValueMap() != nil {
			modLabels = cur.AsValueMap()
		}
	}
	return cty.ObjectVal(mergeMaps(global, extra, modLabels)), nil
}

// mergeMaps takes an arbitrary number of maps, and returns a single map that contains
// a merged set of elements from all arguments.
// If more than one given map defines the same key, then the one that is later in the argument sequence takes precedence.
// See https://developer.hashicorp.com/terraform/language/functions/merge
func mergeMaps(ms ...map[string]cty.Value) map[string]cty.Value {
	r := map[string]cty.Value{}
	for _, m := range ms {
		for k, v := range m {
			r[k] = v
		}
	}
	return r
}

func (bp Blueprint) applyGlobalVarsInModule(mod *Module) error {
	mi := mod.InfoOrDie()
	for _, input := range mi.Inputs {
		// Module setting exists? Nothing more needs to be done.
		if mod.Settings.Has(input.Name) {
			continue
		}

		// If it's not set, is there a global we can use?
		if bp.Vars.Has(input.Name) {
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
	return nil
}

// applyGlobalVariables takes any variables defined at the global level and
// applies them to module settings if not already set.
func (dc *DeploymentConfig) applyGlobalVariables() error {
	return dc.Config.WalkModules(func(mod *Module) error {
		return dc.Config.applyGlobalVarsInModule(mod)
	})
}

// AutomaticOutputName generates unique deployment-group-level output names
func AutomaticOutputName(outputName string, moduleID ModuleID) string {
	return outputName + "_" + string(moduleID)
}

// Checks validity of reference to a module:
// * module exists;
// * module is not a Packer module;
// * module is not in a later deployment group.
func validateModuleReference(bp Blueprint, from Module, toID ModuleID) error {
	to, err := bp.Module(toID)
	if err != nil {
		return err
	}

	if to.Kind == PackerKind {
		return fmt.Errorf("%s: %s", errorMessages["cannotUsePacker"], to.ID)
	}

	fg := bp.ModuleGroupOrDie(from.ID)
	tg := bp.ModuleGroupOrDie(to.ID)
	fgi := slices.IndexFunc(bp.DeploymentGroups, func(g DeploymentGroup) bool { return g.Name == fg.Name })
	tgi := slices.IndexFunc(bp.DeploymentGroups, func(g DeploymentGroup) bool { return g.Name == tg.Name })
	if tgi > fgi {
		return fmt.Errorf("%s: %s is in a later group", errorMessages["intergroupOrder"], to.ID)
	}
	return nil
}

// Checks validity of reference to a module output:
// * reference to an existing global variable;
// * reference to a module is valid;
// * referenced module output exists.
func validateModuleSettingReference(bp Blueprint, mod Module, r Reference) error {
	// simplest case to evaluate is a deployment variable's existence
	if r.GlobalVar {
		if !bp.Vars.Has(r.Name) {
			return fmt.Errorf("module %#v references unknown global variable %#v", mod.ID, r.Name)
		}
		return nil
	}

	if err := validateModuleReference(bp, mod, r.Module); err != nil {
		return err
	}
	tm, _ := bp.Module(r.Module)
	mi := tm.InfoOrDie()
	found := slices.ContainsFunc(mi.Outputs, func(o modulereader.OutputInfo) bool { return o.Name == r.Name })
	if !found {
		return fmt.Errorf("%s: module %s did not have output %s", errorMessages["noOutput"], tm.ID, r.Name)
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
func (dc *DeploymentConfig) addDefaultValidators() {
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
}

// FindAllIntergroupReferences finds all intergroup references within the group
func (dg DeploymentGroup) FindAllIntergroupReferences(bp Blueprint) []Reference {
	igcRefs := map[Reference]bool{}
	for _, mod := range dg.Modules {
		for _, ref := range FindIntergroupReferences(mod.Settings.AsObject(), mod, bp) {
			igcRefs[ref] = true
		}
	}
	return maps.Keys(igcRefs)
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

// OutputNames returns the group-level output names constructed from module ID
// and module-level output name; by construction, all elements are unique
func (dg DeploymentGroup) OutputNames() []string {
	outputs := []string{}
	for _, mod := range dg.Modules {
		for _, output := range mod.Outputs {
			outputs = append(outputs, AutomaticOutputName(output.Name, mod.ID))
		}
	}
	return outputs
}

// OutputNamesByGroup returns the outputs from prior groups that match input
// names for this group as a map
func OutputNamesByGroup(g DeploymentGroup, dc DeploymentConfig) (map[GroupName][]string, error) {
	refs := g.FindAllIntergroupReferences(dc.Config)
	inputNames := make([]string, len(refs))
	for i, ref := range refs {
		inputNames[i] = AutomaticOutputName(ref.Name, ref.Module)
	}

	i := dc.Config.GroupIndex(g.Name)
	if i == -1 {
		return nil, fmt.Errorf("group %s not found in blueprint", g.Name)
	}
	outputNamesByGroup := make(map[GroupName][]string)
	for _, g := range dc.Config.DeploymentGroups[:i] {
		outputNamesByGroup[g.Name] = intersection(inputNames, g.OutputNames())
	}
	return outputNamesByGroup, nil
}

// return sorted list of elements common to s1 and s2
func intersection(s1 []string, s2 []string) []string {
	first := make(map[string]bool)

	for _, v := range s1 {
		first[v] = true
	}

	both := map[string]bool{}
	for _, v := range s2 {
		if first[v] {
			both[v] = true
		}
	}
	is := maps.Keys(both)
	slices.Sort(is)
	return is
}
