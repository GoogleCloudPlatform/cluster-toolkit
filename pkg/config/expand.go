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
	"errors"
	"fmt"
	"regexp"

	"hpc-toolkit/pkg/modulereader"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	blueprintLabel  string = "ghpc_blueprint"
	deploymentLabel string = "ghpc_deployment"
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
	dc.combineLabels()

	if err := dc.applyUseModules(); err != nil {
		return err
	}

	dc.applyGlobalVariables()

	if err := validateInputsAllModules(dc.Config); err != nil {
		return err
	}

	if err := validateModulesAreUsed(dc.Config); err != nil {
		return err
	}

	dc.Config.populateOutputs()
	return nil
}

func validateInputsAllModules(bp Blueprint) error {
	errs := Errors{}
	bp.WalkModulesSafe(func(p ModulePath, m *Module) {
		errs.Add(validateModuleInputs(p, *m, bp))
	})
	return errs.OrNil()
}

func validateModuleInputs(mp ModulePath, m Module, bp Blueprint) error {
	mi := m.InfoOrDie()
	errs := Errors{}
	for _, input := range mi.Inputs {
		ip := mp.Settings.Dot(input.Name)

		if !m.Settings.Has(input.Name) {
			if input.Required {
				errs.At(ip, fmt.Errorf("a required setting %q is missing from a module %q", input.Name, m.ID))
			}
			continue
		}

		errs.At(ip, checkInputValueMatchesType(m.Settings.Get(input.Name), input, bp))
	}
	return errs.OrNil()
}

func attemptEvalModuleInput(val cty.Value, bp Blueprint) (cty.Value, bool) {
	v, err := evalValue(val, bp)
	// there could be a legitimate reasons for it.
	// e.g. use of modules output or unsupported (by ghpc) functions
	// TODO:
	// * substitute module outputs with an UnknownValue
	// * skip if uses functions with side-effects, e.g. `file`
	// * add implementation of all pure terraform functions
	// * add positive selection for eval-errors to bubble up
	return v, err == nil
}

func checkInputValueMatchesType(val cty.Value, input modulereader.VarInfo, bp Blueprint) error {
	v, ok := attemptEvalModuleInput(val, bp)
	if !ok || input.Type == cty.NilType {
		return nil // skip, can do nothing
	}
	// cty does panic on some edge cases, e.g. (cty.NilVal)
	// we don't anticipate any of those, but just in case, catch panic and swallow it
	defer func() { recover() }()
	// TODO: consider returning error (not panic) or logging warning
	if _, err := convert.Convert(v, input.Type); err != nil {
		return fmt.Errorf("unsuitable value for %q: %w", input.Name, err)
	}
	return nil
}

func validateModulesAreUsed(bp Blueprint) error {
	used := map[ModuleID]bool{}
	bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
		for ref := range valueReferences(m.Settings.AsObject()) {
			used[ref.Module] = true
		}
	})

	errs := Errors{}
	bp.WalkModulesSafe(func(p ModulePath, m *Module) {
		if m.InfoOrDie().Metadata.Ghpc.HasToBeUsed && !used[m.ID] {
			errs.At(p.ID, HintError{
				"you need to add it to the `use`-block of downstream modules",
				fmt.Errorf("module %q was not used", m.ID)})
		}
	})
	return errs.OrNil()
}

func (dc *DeploymentConfig) expandBackends() {
	// 1. DEFAULT: use TerraformBackend configuration (if supplied) in each
	//    resource group
	// 2. If top-level TerraformBackendDefaults is defined, insert that
	//    backend into resource groups which have no explicit
	//    TerraformBackend
	// 3. In all cases, add a prefix for GCS backends if one is not defined
	bp := &dc.Config
	defaults := bp.TerraformBackendDefaults
	if defaults.Type != "" {
		for i := range bp.DeploymentGroups {
			grp := &bp.DeploymentGroups[i]
			be := &grp.TerraformBackend
			if be.Type == "" {
				be.Type = defaults.Type
				be.Configuration = Dict{}
				for k, v := range defaults.Configuration.Items() {
					be.Configuration.Set(k, v)
				}
			}
			if be.Type == "gcs" && !be.Configuration.Has("prefix") {
				prefix := fmt.Sprintf("%s/%s/%s", bp.BlueprintName, bp.DeploymentName(), grp.Name)
				be.Configuration.Set("prefix", cty.StringVal(prefix))
			}
		}
	}
}

func getModuleInputMap(inputs []modulereader.VarInfo) map[string]cty.Type {
	modInputs := make(map[string]cty.Type)
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
//	use: "used" module as defined above
func useModule(mod *Module, use Module) {
	modInputsMap := getModuleInputMap(mod.InfoOrDie().Inputs)
	for _, useOutput := range use.InfoOrDie().Outputs {
		setting := useOutput.Name

		// Skip settings that do not have matching module inputs
		inputType, ok := modInputsMap[setting]
		if !ok {
			continue
		}

		alreadySet := mod.Settings.Has(setting)
		if alreadySet && len(IsProductOfModuleUse(mod.Settings.Get(setting))) == 0 {
			continue // set explicitly, skip
		}

		// skip settings that are not of list type, but already have a value
		// these were probably added by a previous call to this function
		isList := inputType.IsListType()
		if alreadySet && !isList {
			continue
		}

		v := AsProductOfModuleUse(
			ModuleRef(use.ID, setting).AsExpression().AsValue(),
			use.ID)

		if !isList {
			mod.Settings.Set(setting, v)
		} else {
			mod.addListValue(setting, v)
		}
	}
}

// applyUseModules applies variables from modules listed in the "use" field
// when/if applicable
func (dc *DeploymentConfig) applyUseModules() error {
	return dc.Config.WalkModules(func(_ ModulePath, m *Module) error {
		for _, u := range m.Use {
			used, err := dc.Config.Module(u)
			if err != nil { // should never happen
				return err
			}
			useModule(m, *used)
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

// combineLabels sets defaults for labels based on other variables and merges
// the global labels defined in Vars with module setting labels.
func (dc *DeploymentConfig) combineLabels() {
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

	dc.Config.WalkModulesSafe(func(_ ModulePath, mod *Module) {
		combineModuleLabels(mod, *dc)
	})
}

func combineModuleLabels(mod *Module, dc DeploymentConfig) {
	labels := "labels"
	if !moduleHasInput(*mod, labels) {
		return // no op
	}

	ref := GlobalRef(labels).AsExpression().AsValue()
	set := mod.Settings.Get(labels)

	if !set.IsNull() {
		merged := FunctionCallExpression("merge", ref, set).AsValue()
		mod.Settings.Set(labels, merged) // = merge(vars.labels, {...labels_from_settings...})
	} else {
		mod.Settings.Set(labels, ref) // = vars.labels
	}
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

func (bp Blueprint) applyGlobalVarsInModule(mod *Module) {
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

		if input.Name == mi.Metadata.Ghpc.InjectModuleId {
			mod.Settings.Set(input.Name, cty.StringVal(string(mod.ID)))
		}
	}
}

// applyGlobalVariables takes any variables defined at the global level and
// applies them to module settings if not already set.
func (dc *DeploymentConfig) applyGlobalVariables() {
	dc.Config.WalkModulesSafe(func(_ ModulePath, m *Module) {
		dc.Config.applyGlobalVarsInModule(m)
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
		mods := []string{}
		bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
			mods = append(mods, string(m.ID))
		})
		return hintSpelling(string(toID), mods, err)
	}

	if to.Kind == PackerKind {
		return fmt.Errorf("%s: %s", errMsgCannotUsePacker, to.ID)
	}

	fg := bp.ModuleGroupOrDie(from.ID)
	tg := bp.ModuleGroupOrDie(to.ID)
	fgi := slices.IndexFunc(bp.DeploymentGroups, func(g DeploymentGroup) bool { return g.Name == fg.Name })
	tgi := slices.IndexFunc(bp.DeploymentGroups, func(g DeploymentGroup) bool { return g.Name == tg.Name })
	if tgi > fgi {
		return fmt.Errorf("%s: %s is in a later group", errMsgIntergroupOrder, to.ID)
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
			err := fmt.Errorf("module %#v references unknown global variable %#v", mod.ID, r.Name)
			vars := maps.Keys(bp.Vars.Items())
			return hintSpelling(r.Name, vars, err)
		}
		return nil
	}

	if err := validateModuleReference(bp, mod, r.Module); err != nil {
		var unkModErr UnknownModuleError
		if errors.As(err, &unkModErr) {
			hints := []string{"vars"}
			bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
				hints = append(hints, string(m.ID))
			})
			return hintSpelling(string(unkModErr.ID), hints, unkModErr)
		}
		return err
	}
	tm, _ := bp.Module(r.Module) // Shouldn't error if validateModuleReference didn't
	mi, err := modulereader.GetModuleInfo(tm.Source, tm.Kind.String())
	if err != nil {
		return err
	}

	outputs := []string{}
	for _, o := range mi.Outputs {
		outputs = append(outputs, o.Name)
	}

	if !slices.Contains(outputs, r.Name) {
		err := fmt.Errorf("module %q does not have output %q", tm.ID, r.Name)
		return hintSpelling(r.Name, outputs, err)
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
	res := []Reference{}
	for r := range valueReferences(v) {
		if !r.GlobalVar && bp.ModuleGroupOrDie(r.Module).Name != g.Name {
			res = append(res, r)
		}
	}
	return res
}

// find all intergroup references and add them to source Module.Outputs
func (bp *Blueprint) populateOutputs() {
	refs := map[Reference]bool{}
	bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
		rs := FindIntergroupReferences(m.Settings.AsObject(), *m, *bp)
		for _, r := range rs {
			refs[r] = true
		}
	})

	bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
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
func OutputNamesByGroup(g DeploymentGroup, bp Blueprint) (map[GroupName][]string, error) {
	refs := g.FindAllIntergroupReferences(bp)
	inputs := make([]string, len(refs))
	for i, ref := range refs {
		inputs[i] = AutomaticOutputName(ref.Name, ref.Module)
	}

	i := bp.GroupIndex(g.Name)
	if i == -1 {
		return nil, fmt.Errorf("group %s not found in blueprint", g.Name)
	}

	res := make(map[GroupName][]string)
	for _, pg := range bp.DeploymentGroups[:i] {
		res[pg.Name] = intersection(inputs, pg.OutputNames())
	}
	return res, nil
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
