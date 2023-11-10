/**
* Copyright 2022 Google LLC
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

package modulewriter

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"

	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
)

const (
	tfStateFileName       = "terraform.tfstate"
	tfStateBackupFileName = "terraform.tfstate.backup"
)

// TFWriter writes terraform to the blueprint folder
type TFWriter struct{}

// createBaseFile creates a baseline file for all terraform/hcl including a
// license and any other boilerplate
func createBaseFile(path string) error {
	baseFile, err := os.Create(path)
	if err != nil {
		return err
	}
	defer baseFile.Close()
	_, err = baseFile.WriteString(license)
	return err
}

func appendHCLToFile(path string, hclBytes []byte) error {
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err = file.Write(hclBytes); err != nil {
		return err
	}
	return nil
}

func writeOutputs(
	modules []config.Module,
	dst string,
) error {
	// Create hcl body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	outputs := []string{}
	// Add all outputs from each module
	for _, mod := range modules {
		for _, output := range mod.Outputs {
			outputName := config.AutomaticOutputName(output.Name, mod.ID)
			outputs = append(outputs, outputName)

			hclBody.AppendNewline()
			hclBlock := hclBody.AppendNewBlock("output", []string{outputName})
			blockBody := hclBlock.Body()

			desc := output.Description
			if desc == "" {
				desc = fmt.Sprintf("Generated output from module '%s'", mod.ID)
			}
			blockBody.SetAttributeValue("description", cty.StringVal(desc))
			value := fmt.Sprintf("module.%s.%s", mod.ID, output.Name)
			blockBody.SetAttributeRaw("value", simpleTokens(value))
			if output.Sensitive {
				blockBody.SetAttributeValue("sensitive", cty.BoolVal(output.Sensitive))
			}
		}
	}

	if len(outputs) == 0 {
		return nil
	}
	hclBytes := hclFile.Bytes()
	outputsPath := filepath.Join(dst, "outputs.tf")
	if err := createBaseFile(outputsPath); err != nil {
		return fmt.Errorf("error creating outputs.tf file: %v", err)
	}
	err := appendHCLToFile(outputsPath, hclBytes)
	if err != nil {
		return fmt.Errorf("error writing HCL to outputs.tf file: %v", err)
	}
	return nil
}

func writeTfvars(vars map[string]cty.Value, dst string) error {
	// Create file
	tfvarsPath := filepath.Join(dst, "terraform.tfvars")
	err := WriteHclAttributes(vars, tfvarsPath)
	return err
}

func getHclType(t cty.Type) string {
	if t.IsPrimitiveType() {
		return typeexpr.TypeString(t)
	}
	if t.IsListType() || t.IsTupleType() || t.IsSetType() {
		return "list"
	}
	return typeexpr.TypeString(cty.DynamicPseudoType) // any
}

func getTypeTokens(v cty.Value) hclwrite.Tokens {
	return simpleTokens(getHclType(v.Type()))
}

func writeVariables(vars map[string]cty.Value, extraVars []modulereader.VarInfo, dst string) error {
	// Create file
	variablesPath := filepath.Join(dst, "variables.tf")
	if err := createBaseFile(variablesPath); err != nil {
		return fmt.Errorf("error creating variables.tf file: %v", err)
	}

	var inputs []modulereader.VarInfo
	for k, v := range vars {
		typeStr := getHclType(v.Type())
		newInput := modulereader.VarInfo{
			Name:        k,
			Type:        typeStr,
			Description: fmt.Sprintf("Toolkit deployment variable: %s", k),
		}
		inputs = append(inputs, newInput)
	}
	inputs = append(inputs, extraVars...)
	slices.SortFunc(inputs, func(i, j modulereader.VarInfo) bool { return i.Name < j.Name })

	// Create HCL Body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// create variable block for each input
	for _, k := range inputs {
		hclBody.AppendNewline()
		hclBlock := hclBody.AppendNewBlock("variable", []string{k.Name})
		blockBody := hclBlock.Body()
		blockBody.SetAttributeValue("description", cty.StringVal(k.Description))
		blockBody.SetAttributeRaw("type", simpleTokens(k.Type))
	}

	// Write file
	if err := appendHCLToFile(variablesPath, hclFile.Bytes()); err != nil {
		return fmt.Errorf("error writing HCL to variables.tf file: %v", err)
	}
	return nil
}

func writeMain(
	modules []config.Module,
	tfBackend config.TerraformBackend,
	dst string,
) error {
	// Create file
	mainPath := filepath.Join(dst, "main.tf")
	if err := createBaseFile(mainPath); err != nil {
		return fmt.Errorf("error creating main.tf file: %v", err)
	}

	// Create HCL Body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// Write Terraform backend if needed
	if tfBackend.Type != "" {
		hclBody.AppendNewline()
		tfBody := hclBody.AppendNewBlock("terraform", []string{}).Body()
		backendBlock := tfBody.AppendNewBlock("backend", []string{tfBackend.Type})
		backendBody := backendBlock.Body()
		vals := tfBackend.Configuration.Items()
		for _, setting := range orderKeys(vals) {
			backendBody.SetAttributeValue(setting, vals[setting])
		}
	}

	for _, mod := range modules {
		hclBody.AppendNewline()
		// Add block
		moduleBlock := hclBody.AppendNewBlock("module", []string{string(mod.ID)})
		moduleBody := moduleBlock.Body()

		// Add source attribute
		ds, err := DeploymentSource(mod)
		if err != nil {
			return err
		}
		moduleBody.SetAttributeValue("source", cty.StringVal(ds))

		// For each Setting
		for _, setting := range orderKeys(mod.Settings.Items()) {
			value := mod.Settings.Get(setting)
			moduleBody.SetAttributeRaw(setting, config.TokensForValue(value))
		}
	}
	// Write file
	hclBytes := hclFile.Bytes()
	hclBytes = hclwrite.Format(hclBytes)
	if err := appendHCLToFile(mainPath, hclBytes); err != nil {
		return fmt.Errorf("error writing HCL to main.tf file: %v", err)
	}
	return nil
}

var simpleTokens = hclwrite.TokensForIdentifier

func writeProviders(vars map[string]cty.Value, dst string) error {
	// Create file
	providersPath := filepath.Join(dst, "providers.tf")
	if err := createBaseFile(providersPath); err != nil {
		return fmt.Errorf("error creating providers.tf file: %v", err)
	}

	// Create HCL Body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	for _, prov := range []string{"google", "google-beta"} {
		hclBody.AppendNewline()
		provBlock := hclBody.AppendNewBlock("provider", []string{prov})
		provBody := provBlock.Body()
		if _, ok := vars["project_id"]; ok {
			provBody.SetAttributeRaw("project", simpleTokens("var.project_id"))
		}
		if _, ok := vars["zone"]; ok {
			provBody.SetAttributeRaw("zone", simpleTokens("var.zone"))
		}
		if _, ok := vars["region"]; ok {
			provBody.SetAttributeRaw("region", simpleTokens("var.region"))
		}
	}

	// Write file
	hclBytes := hclFile.Bytes()
	if err := appendHCLToFile(providersPath, hclBytes); err != nil {
		return fmt.Errorf("error writing HCL to providers.tf file: %v", err)
	}
	return nil
}

func writeVersions(dst string) error {
	// Create file
	versionsPath := filepath.Join(dst, "versions.tf")
	if err := createBaseFile(versionsPath); err != nil {
		return fmt.Errorf("error creating versions.tf file: %v", err)
	}
	// Write hard-coded version information
	if err := appendHCLToFile(versionsPath, []byte(tfversions)); err != nil {
		return fmt.Errorf("error writing HCL to versions.tf file: %v", err)
	}
	return nil
}

func writeTerraformInstructions(w io.Writer, grpPath string, n config.GroupName, printExportOutputs bool, printImportInputs bool) {
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Terraform group '%s' was successfully created in directory %s\n", n, grpPath)
	fmt.Fprintln(w, "To deploy, run the following commands:")
	fmt.Fprintln(w)
	if printImportInputs {
		fmt.Fprintf(w, "ghpc import-inputs %s\n", grpPath)
	}
	fmt.Fprintf(w, "terraform -chdir=%s init\n", grpPath)
	fmt.Fprintf(w, "terraform -chdir=%s validate\n", grpPath)
	fmt.Fprintf(w, "terraform -chdir=%s apply\n", grpPath)
	if printExportOutputs {
		fmt.Fprintf(w, "ghpc export-outputs %s\n", grpPath)
	}
}

// writeDeploymentGroup creates and sets up the provided terraform deployment
// group in the provided deployment directory
// depGroup: The deployment group that is being written
// globalVars: The top-level variables, needed for writing terraform.tfvars and
// variables.tf
// groupDir: The path to the directory the resource group will be created in
func (w TFWriter) writeDeploymentGroup(
	dc config.DeploymentConfig,
	groupIndex int,
	deploymentDir string,
	instructionsFile io.Writer,
) error {
	depGroup := dc.Config.DeploymentGroups[groupIndex]
	deploymentVars := getUsedDeploymentVars(depGroup, dc.Config)
	intergroupVars := FindIntergroupVariables(depGroup, dc.Config)
	intergroupInputs := make(map[string]bool)
	for _, igVar := range intergroupVars {
		intergroupInputs[igVar.Name] = true
	}

	groupPath := filepath.Join(deploymentDir, string(depGroup.Name))

	// Write main.tf file
	doctoredModules := substituteIgcReferences(depGroup.Modules, intergroupVars)
	if err := writeMain(
		doctoredModules, depGroup.TerraformBackend, groupPath,
	); err != nil {
		return fmt.Errorf("error writing main.tf file for deployment group %s: %v",
			depGroup.Name, err)
	}

	// Write variables.tf file
	if err := writeVariables(deploymentVars, maps.Values(intergroupVars), groupPath); err != nil {
		return fmt.Errorf(
			"error writing variables.tf file for deployment group %s: %v",
			depGroup.Name, err)
	}

	// Write outputs.tf file
	if err := writeOutputs(depGroup.Modules, groupPath); err != nil {
		return fmt.Errorf(
			"error writing outputs.tf file for deployment group %s: %v",
			depGroup.Name, err)
	}

	// Write terraform.tfvars file
	if err := writeTfvars(deploymentVars, groupPath); err != nil {
		return fmt.Errorf(
			"error writing terraform.tfvars file for deployment group %s: %v",
			depGroup.Name, err)
	}

	// Write providers.tf file
	if err := writeProviders(deploymentVars, groupPath); err != nil {
		return fmt.Errorf(
			"error writing providers.tf file for deployment group %s: %v",
			depGroup.Name, err)
	}

	// Write versions.tf file
	if err := writeVersions(groupPath); err != nil {
		return fmt.Errorf(
			"error writing versions.tf file for deployment group %s: %v",
			depGroup.Name, err)
	}

	multiGroupDeployment := len(dc.Config.DeploymentGroups) > 1
	printImportInputs := multiGroupDeployment && groupIndex > 0
	printExportOutputs := multiGroupDeployment && groupIndex < len(dc.Config.DeploymentGroups)-1

	writeTerraformInstructions(instructionsFile, groupPath, depGroup.Name, printExportOutputs, printImportInputs)

	return nil
}

// Transfers state files from previous resource groups (in .ghpc/) to a newly written blueprint
func (w TFWriter) restoreState(deploymentDir string) error {
	prevDeploymentGroupPath := filepath.Join(
		deploymentDir, HiddenGhpcDirName, prevDeploymentGroupDirName)
	files, err := os.ReadDir(prevDeploymentGroupPath)
	if err != nil {
		return fmt.Errorf(
			"Error trying to read previous modules in %s, %w",
			prevDeploymentGroupPath, err)
	}

	for _, f := range files {
		var tfStateFiles = []string{tfStateFileName, tfStateBackupFileName}
		for _, stateFile := range tfStateFiles {
			src := filepath.Join(prevDeploymentGroupPath, f.Name(), stateFile)
			dest := filepath.Join(deploymentDir, f.Name(), stateFile)

			if bytesRead, err := os.ReadFile(src); err == nil {
				err = os.WriteFile(dest, bytesRead, 0644)
				if err != nil {
					return fmt.Errorf("failed to write previous state file %s, %w", dest, err)
				}
			}
		}

	}
	return nil
}

func orderKeys[T any](settings map[string]T) []string {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func getUsedDeploymentVars(group config.DeploymentGroup, bp config.Blueprint) map[string]cty.Value {
	// labels must always be written as a variable as it is implicitly added
	groupInputs := map[string]bool{
		"labels": true,
	}

	for _, mod := range group.Modules {
		for _, v := range config.GetUsedDeploymentVars(mod.Settings.AsObject()) {
			groupInputs[v] = true
		}
	}

	filteredVars := make(map[string]cty.Value)
	for key, val := range bp.Vars.Items() {
		if groupInputs[key] {
			filteredVars[key] = val
		}
	}
	return filteredVars
}

func substituteIgcReferences(mods []config.Module, igcRefs map[config.Reference]modulereader.VarInfo) []config.Module {
	doctoredMods := make([]config.Module, len(mods))
	for i, mod := range mods {
		doctoredMods[i] = SubstituteIgcReferencesInModule(mod, igcRefs)
	}
	return doctoredMods
}

// SubstituteIgcReferencesInModule updates expressions in Module settings to use
// special IGC var name instead of the module reference
func SubstituteIgcReferencesInModule(mod config.Module, igcRefs map[config.Reference]modulereader.VarInfo) config.Module {
	v, _ := cty.Transform(mod.Settings.AsObject(), func(p cty.Path, v cty.Value) (cty.Value, error) {
		e, is := config.IsExpressionValue(v)
		if !is {
			return v, nil
		}
		ue := string(e.Tokenize().Bytes())
		for _, r := range e.References() {
			oi, exists := igcRefs[r]
			if !exists {
				continue
			}
			s := fmt.Sprintf("module.%s.%s", r.Module, r.Name)
			rs := fmt.Sprintf("var.%s", oi.Name)
			ue = strings.ReplaceAll(ue, s, rs)
		}
		return config.MustParseExpression(ue).AsValue(), nil
	})
	mod.Settings = config.NewDict(v.AsValueMap())
	return mod
}

// FindIntergroupVariables returns all unique intergroup references made by
// each module settings in a group
func FindIntergroupVariables(group config.DeploymentGroup, bp config.Blueprint) map[config.Reference]modulereader.VarInfo {
	res := map[config.Reference]modulereader.VarInfo{}
	igcRefs := group.FindAllIntergroupReferences(bp)
	for _, r := range igcRefs {
		n := config.AutomaticOutputName(r.Name, r.Module)
		res[r] = modulereader.VarInfo{
			Name:        n,
			Type:        getHclType(cty.DynamicPseudoType),
			Description: "Automatically generated input from previous groups (ghpc import-inputs --help)",
			Required:    true,
		}
	}
	return res
}

func (w TFWriter) kind() config.ModuleKind {
	return config.TerraformKind
}
