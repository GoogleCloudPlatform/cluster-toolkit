/**
* Copyright 2021 Google LLC
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

package reswriter

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	ctyJson "github.com/zclconf/go-cty/cty/json"

	"hpc-toolkit/pkg/config"
)

// TFWriter writes terraform to the blueprint folder
type TFWriter struct {
	numResources int
}

// interfaceStruct is a struct wrapper for converting interface data structures
// to yaml flow style: one line wrapped in {} for maps and [] for lists.
type interfaceStruct struct {
	Elem interface{} `yaml:",flow"`
}

// GetNumResources getter for resource count
func (w *TFWriter) getNumResources() int {
	return w.numResources
}

// AddNumResources add value to resource count
func (w *TFWriter) addNumResources(value int) {
	w.numResources += value
}

// createBaseFile creates a baseline file for all terraform/hcl including a
// license and any other boilerplate
func createBaseFile(path string) error {
	baseFile, err := os.Create(path)
	defer baseFile.Close()
	if err != nil {
		return err
	}
	_, err = baseFile.WriteString(license)
	return err
}

func handlePassthroughVariables(hclBytes []byte) []byte {
	re := regexp.MustCompile(`"\(\((.*?)\)\)"`)
	return re.ReplaceAll(hclBytes, []byte(`${1}`))
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

func convertToCty(iMap map[string]interface{}) (map[string]cty.Value, error) {
	cMap := make(map[string]cty.Value)

	for k, v := range iMap {
		// Convert to JSON bytes
		jsonBytes, err := json.Marshal(v)
		if err != nil {
			return cMap, err
		}

		// Unmarshal JSON into cty
		simpleJSON := ctyJson.SimpleJSONValue{}
		simpleJSON.UnmarshalJSON(jsonBytes)
		cMap[k] = simpleJSON.Value
	}
	return cMap, nil
}

func writeTfvars(vars map[string]cty.Value, dst string) error {
	// Create file
	tfvarsPath := path.Join(dst, "terraform.tfvars")
	if err := createBaseFile(tfvarsPath); err != nil {
		return fmt.Errorf("error creating terraform.tfvars file: %v", err)
	}

	// Create hcl body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// for each variable
	for k, v := range vars {
		// Write attribute
		hclBody.SetAttributeValue(k, v)
	}

	// Write file
	err := appendHCLToFile(tfvarsPath, hclFile.Bytes())
	if err != nil {
		return fmt.Errorf("error writing HCL to terraform.tfvars file: %v", err)
	}
	return err
}

func getTypeTokens(ctyVal cty.Value) hclwrite.Tokens {
	typeToken := hclwrite.Token{
		Type: hclsyntax.TokenIdent,
	}

	typeName := ctyVal.Type().FriendlyName()
	if strings.HasPrefix(typeName, "list of") {
		typeToken.Bytes = []byte("list")
		return []*hclwrite.Token{&typeToken}
	}
	if strings.HasPrefix(typeName, "map of") {
		typeToken.Bytes = []byte("map")
		return []*hclwrite.Token{&typeToken}
	}
	switch typeName {
	case "number", "string", "bool":
		typeToken.Bytes = []byte(typeName)
	case "tuple", "list":
		typeToken.Bytes = []byte("list")
	case "object", "map":
		typeToken.Bytes = []byte("map")
	default:
		return hclwrite.Tokens{}
	}
	return []*hclwrite.Token{&typeToken}
}

func writeVariables(vars map[string]cty.Value, dst string) error {
	// Create file
	variablesPath := path.Join(dst, "variables.tf")
	if err := createBaseFile(variablesPath); err != nil {
		return fmt.Errorf("error creating variables.tf file: %v", err)
	}

	// Create HCL Body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// for each variable
	for k, v := range vars {
		// Create variable block
		hclBlock := hclBody.AppendNewBlock("variable", []string{k})
		blockBody := hclBlock.Body()

		// Add attributes (description, type, etc)
		blockBody.SetAttributeValue("description", cty.StringVal(""))
		typeTok := getTypeTokens(v)
		if len(typeTok) == 0 {
			return fmt.Errorf("error determining type of variable %s", k)
		}
		blockBody.SetAttributeRaw("type", typeTok)
		hclBody.AppendNewline()
	}
	// Write file
	if err := appendHCLToFile(variablesPath, hclFile.Bytes()); err != nil {
		return fmt.Errorf("error writing HCL to variables.tf file: %v", err)
	}
	return nil
}

func writeMain(resources []config.Resource, dst string) error {
	// Create file
	mainPath := path.Join(dst, "main.tf")
	if err := createBaseFile(mainPath); err != nil {
		return fmt.Errorf("error creating main.tf file: %v", err)
	}

	// Create HCL Body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// For each resource:
	for _, res := range resources {
		// Convert settings to cty.Value
		ctySettings, err := convertToCty(res.Settings)
		if err != nil {
			return fmt.Errorf(
				"error converting setting in resource %s to cty when writing main.tf: %v",
				res.ID, err)
		}

		// Add block
		moduleBlock := hclBody.AppendNewBlock("module", []string{res.ID})
		moduleBody := moduleBlock.Body()

		// Add source attribute
		moduleSource := cty.StringVal(fmt.Sprintf("./modules/%s", res.ResourceName))
		moduleBody.SetAttributeValue("source", moduleSource)

		// For each Setting
		for setting, value := range ctySettings {
			// Add attributes
			moduleBody.SetAttributeValue(setting, value)
		}
		hclBody.AppendNewline()
	}
	// Write file
	hclBytes := handlePassthroughVariables(hclFile.Bytes())
	if err := appendHCLToFile(mainPath, hclBytes); err != nil {
		return fmt.Errorf("error writing HCL to main.tf file: %v", err)
	}
	return nil
}

func simpleTokenFromString(str string) hclwrite.Token {
	return hclwrite.Token{
		Type:  hclsyntax.TokenIdent,
		Bytes: []byte(str),
	}
}

func writeProviders(vars map[string]cty.Value, dst string) error {
	// Create file
	providersPath := path.Join(dst, "providers.tf")
	if err := createBaseFile(providersPath); err != nil {
		return fmt.Errorf("error creating providers.tf file: %v", err)
	}

	// Create HCL Body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	for _, prov := range []string{"google", "google-beta"} {
		provBlock := hclBody.AppendNewBlock("provider", []string{prov})
		provBody := provBlock.Body()
		if _, ok := vars["project_id"]; ok {
			pidToken := simpleTokenFromString("var.project_id")
			pidTokens := []*hclwrite.Token{&pidToken}
			provBody.SetAttributeRaw("project", pidTokens)
		}
		if _, ok := vars["zone"]; ok {
			zoneToken := simpleTokenFromString("var.zone")
			zoneTokens := []*hclwrite.Token{&zoneToken}
			provBody.SetAttributeRaw("zone", zoneTokens)
		}
		if _, ok := vars["region"]; ok {
			regToken := simpleTokenFromString("var.region")
			regTokens := []*hclwrite.Token{&regToken}
			provBody.SetAttributeRaw("region", regTokens)
		}
		hclBody.AppendNewline()
	}

	// Write file
	hclBytes := handlePassthroughVariables(hclFile.Bytes())
	if err := appendHCLToFile(providersPath, hclBytes); err != nil {
		return fmt.Errorf("error writing HCL to providers.tf file: %v", err)
	}
	return nil
}

func writeVersions(dst string) error {
	// Create file
	versionsPath := path.Join(dst, "versions.tf")
	if err := createBaseFile(versionsPath); err != nil {
		return fmt.Errorf("error creating versions.tf file: %v", err)
	}
	// Write hard-coded version information
	if err := appendHCLToFile(versionsPath, []byte(tfversions)); err != nil {
		return fmt.Errorf("error writing HCL to versions.tf file: %v", err)
	}
	return nil
}

// writeTopLevel writes any needed files to the top layer of the blueprint
func (w TFWriter) writeResourceGroups(yamlConfig *config.YamlConfig) error {
	bpName := yamlConfig.BlueprintName
	ctyVars, err := convertToCty(yamlConfig.Vars)
	if err != nil {
		return fmt.Errorf(
			"error converting global vars to cty for writing: %v", err)
	}
	for _, resGroup := range yamlConfig.ResourceGroups {
		if !resGroup.HasKind("terraform") {
			continue
		}
		writePath := path.Join(bpName, resGroup.Name)

		// Write main.tf file
		if err := writeMain(resGroup.Resources, writePath); err != nil {
			return fmt.Errorf("error writing main.tf file for resource group %s: %v",
				resGroup.Name, err)
		}

		// Write variables.tf file
		if err := writeVariables(ctyVars, writePath); err != nil {
			return fmt.Errorf(
				"error writing variables.tf file for resource group %s: %v",
				resGroup.Name, err)
		}

		// Write terraform.tfvars file
		if err := writeTfvars(ctyVars, writePath); err != nil {
			return fmt.Errorf(
				"error writing terraform.tfvars file for resource group %s: %v",
				resGroup.Name, err)
		}

		// Write providers.tf file
		if err := writeProviders(ctyVars, writePath); err != nil {
			return fmt.Errorf(
				"error writing providers.tf file for resource group %s: %v",
				resGroup.Name, err)
		}

		// Write versions.tf file
		if err := writeVersions(writePath); err != nil {
			return fmt.Errorf(
				"error writing versions.tf file for resource group %s: %v",
				resGroup.Name, err)
		}

		// License only file, outputs.tf
		if err = createBaseFile(path.Join(writePath, "outputs.tf")); err != nil {
			return fmt.Errorf(
				"error creating outputs.tf file for resource group %s: %v",
				resGroup.Name, err)
		}
	}
	return nil
}
