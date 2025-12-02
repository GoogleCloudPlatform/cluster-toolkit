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

	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	blueprintLabel  string = "ghpc_blueprint"
	deploymentLabel string = "ghpc_deployment"
)

func validateModuleInputs(mp ModulePath, m Module, bp Blueprint) error {
	mi := m.InfoOrDie()
	errs := Errors{}
	for _, input := range mi.Inputs {
		ip := mp.Settings.Dot(input.Name)

		if !m.Settings.Has(input.Name) {
			if input.Required {
				errs.At(ip,
					HintError{
						Err:  fmt.Errorf("a required setting %q is missing from a module %q", input.Name, m.ID),
						Hint: fmt.Sprintf("%q description: %s", input.Name, input.Description)})
			}
			continue
		}

		errs.At(ip, checkInputValueMatchesType(m.Settings.Get(input.Name), input, bp))
	}
	return errs.OrNil()
}

func attemptEvalModuleInput(val cty.Value, bp Blueprint) (cty.Value, bool) {
	v, err := bp.Eval(val)
	// there could be a legitimate reasons for it.
	// e.g. use of modules output or unsupported (by gcluster) functions
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

func (bp *Blueprint) expandVars() error {
	if err := validateVars(*bp); err != nil {
		return err
	}
	bp.expandGlobalLabels()
	return nil
}

func (bp *Blueprint) substituteModuleSources() {
	bp.WalkModulesSafe(func(_ ModulePath, m *Module) {
		m.Source = bp.transformSource(m.Source)
	})
}

func (bp Blueprint) transformSource(s string) string {
	if sourcereader.IsEmbeddedPath(s) && bp.ToolkitModulesURL != "" && bp.ToolkitModulesVersion != "" {
		return fmt.Sprintf("%s//%s?ref=%s&depth=1", bp.ToolkitModulesURL, s, bp.ToolkitModulesVersion)
	}
	return s
}

func (bp *Blueprint) expandGroups() error {
	bp.addKindToModules()
	bp.substituteModuleSources()
	if err := checkModulesAndGroups(*bp); err != nil {
		return err
	}

	var errs Errors
	for ig := range bp.Groups {
		errs.Add(bp.expandGroup(Root.Groups.At(ig), &bp.Groups[ig]))
	}

	if errs.Any() {
		return errs
	}

	// Following actions depend on whole blueprint being expanded
	// run it after all groups are expanded
	if err := validateModulesAreUsed(*bp); err != nil {
		return err
	}
	bp.populateOutputs()
	return nil
}

func (bp Blueprint) expandGroup(gp groupPath, g *Group) error {
	var errs Errors
	bp.expandBackend(g)
	if g.Kind() == TerraformKind {
		bp.expandProviders(g)
	}
	for im := range g.Modules {
		errs.Add(bp.expandModule(gp.Modules.At(im), &g.Modules[im]))
	}
	return errs.OrNil()
}

func (bp Blueprint) expandModule(mp ModulePath, m *Module) error {
	bp.applyUseModules(m)
	bp.applyGlobalVarsInModule(m)
	return validateModuleInputs(mp, *m, bp)
}

func (bp Blueprint) expandBackend(grp *Group) {
	// 1. DEFAULT: use TerraformBackend configuration (if supplied)
	// 2. If top-level TerraformBackendDefaults is defined, insert that
	//    backend into resource groups which have no explicit
	//    TerraformBackend
	// 3. In all cases, add a prefix for GCS backends if one is not defined
	defaults := bp.TerraformBackendDefaults
	if defaults.Type == "" {
		return
	}

	be := &grp.TerraformBackend
	if be.Type == "" {
		(*be) = defaults
	}

	if be.Type == "gcs" && !be.Configuration.Has("prefix") {
		prefix := MustParseExpression(
			fmt.Sprintf(`"%s/${var.deployment_name}/%s"`, bp.BlueprintName, grp.Name))
		be.Configuration = be.Configuration.With("prefix", prefix.AsValue())
	}
}

func getDefaultGoogleProviders(bp Blueprint) map[string]TerraformProvider {
	gglConf := Dict{}
	for s, v := range map[string]string{
		"project": "project_id",
		"region":  "region",
		"zone":    "zone"} {
		if bp.Vars.Has(v) {
			gglConf = gglConf.With(s, GlobalRef(v).AsValue())
		}
	}
	return map[string]TerraformProvider{
		"google": {
			Source:        "hashicorp/google",
			Version:       ">= 6.9.0, <= 7.11.0",
			Configuration: gglConf},
		"google-beta": {
			Source:        "hashicorp/google-beta",
			Version:       ">= 6.9.0, <= 7.11.0",
			Configuration: gglConf}}
}

func (bp Blueprint) expandProviders(grp *Group) {
	// 1. DEFAULT: use TerraformProviders provider dictionary (if supplied)
	// 2. If top-level TerraformProviders is defined, insert that
	//    provider dictionary into resource groups which have no explicit
	//    TerraformProviders
	defaults := bp.TerraformProviders
	pv := &grp.TerraformProviders
	if defaults == nil {
		defaults = getDefaultGoogleProviders(bp)
	}
	if (*pv) == nil {
		(*pv) = maps.Clone(defaults)
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
func (mod *Module) addListValue(setting string, value cty.Value) {
	args := []cty.Value{value}
	mods := map[ModuleID]bool{}
	for _, mod := range IsProductOfModuleUse(value) {
		mods[mod] = true
	}

	if mod.Settings.Has(setting) {
		cur := mod.Settings.Get(setting)
		for _, mod := range IsProductOfModuleUse(cur) {
			mods[mod] = true
		}
		args = append(args, cur)
	}

	exp := FunctionCallExpression("flatten", cty.TupleVal(args))
	val := AsProductOfModuleUse(exp.AsValue(), maps.Keys(mods)...)
	mod.Settings = mod.Settings.With(setting, val)
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
		if !ok || setting == "labels" { // also do not "use" module labels
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

		v := AsProductOfModuleUse(ModuleRef(use.ID, setting).AsValue(), use.ID)

		if !isList {
			mod.Settings = mod.Settings.With(setting, v)
		} else {
			mod.addListValue(setting, v)
		}
	}
}

// applyUseModules applies variables from modules listed in the "use" field
// when/if applicable
func (bp Blueprint) applyUseModules(m *Module) error {
	for _, u := range m.Use {
		used, err := bp.Module(u)
		if err != nil { // should never happen
			panic(err)
		}
		useModule(m, *used)
	}
	return nil
}

// expandGlobalLabels sets defaults for labels based on other variables.
func (bp *Blueprint) expandGlobalLabels() {
	defaults := cty.ObjectVal(map[string]cty.Value{
		blueprintLabel:  cty.StringVal(bp.BlueprintName),
		deploymentLabel: GlobalRef("deployment_name").AsValue()})

	labels := "labels"
	var gl cty.Value
	if !bp.Vars.Has(labels) {
		gl = defaults
	} else {
		gl = FunctionCallExpression("merge", defaults, bp.Vars.Get(labels)).AsValue()
	}
	bp.Vars = bp.Vars.With(labels, gl)
}

func combineModuleLabels(mod Module) cty.Value {
	ref := GlobalRef("labels").AsValue()
	set := mod.Settings.Get("labels")

	if !set.IsNull() {
		// = merge(vars.labels, {...labels_from_settings...})
		return FunctionCallExpression("merge", ref, set).AsValue()

	}
	return ref // = vars.labels
}

func (bp Blueprint) applyGlobalVarsInModule(mod *Module) {
	mi := mod.InfoOrDie()
	for _, input := range mi.Inputs {
		if input.Name == "labels" && bp.Vars.Has("labels") {
			// labels are special case, always make use of global labels
			mod.Settings = mod.Settings.With("labels", combineModuleLabels(*mod))
		}

		// Module setting exists? Nothing more needs to be done.
		if mod.Settings.Has(input.Name) {
			continue
		}

		// If it's not set, is there a global we can use?
		if bp.Vars.Has(input.Name) {
			mod.Settings = mod.Settings.With(input.Name, GlobalRef(input.Name).AsValue())
			continue
		}

		if input.Name == mi.Metadata.Ghpc.InjectModuleId {
			mod.Settings = mod.Settings.With(input.Name, cty.StringVal(string(mod.ID)))
		}
	}
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
		return HintSpelling(string(toID), mods, err)
	}

	if to.Kind == PackerKind {
		return fmt.Errorf("packer modules cannot be used by other modules: %s", to.ID)
	}

	fg := bp.ModuleGroupOrDie(from.ID)
	tg := bp.ModuleGroupOrDie(to.ID)
	fgi := slices.IndexFunc(bp.Groups, func(g Group) bool { return g.Name == fg.Name })
	tgi := slices.IndexFunc(bp.Groups, func(g Group) bool { return g.Name == tg.Name })
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
			err := fmt.Errorf("module %q references unknown global variable %q", mod.ID, r.Name)
			return HintSpelling(r.Name, bp.Vars.Keys(), err)
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
			return HintSpelling(string(unkModErr.ID), hints, unkModErr)
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
		return HintSpelling(r.Name, outputs, err)
	}
	return nil
}

// FindAllIntergroupReferences finds all intergroup references within the group
func (g Group) FindAllIntergroupReferences(bp Blueprint) []Reference {
	igcRefs := map[Reference]bool{}
	for _, mod := range g.Modules {
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
func (dg Group) OutputNames() []string {
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
func OutputNamesByGroup(g Group, bp Blueprint) (map[GroupName][]string, error) {
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
	for _, pg := range bp.Groups[:i] {
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
