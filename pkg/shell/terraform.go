/**
 * Copyright 2023 Google LLC
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

package shell

import (
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/modulewriter"
	"log"
	"os/exec"
	"path/filepath"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// TfError captures Terraform errors while improving helpfulness of message
type TfError struct {
	help string
	err  error
}

func (se *TfError) Error() string {
	return fmt.Sprintf("%s (detailed error below)\n%s", se.help, se.err)
}

type outputValue struct {
	Name      string
	Sensitive bool
	Type      cty.Type
	Value     cty.Value
}

// ConfigureTerraform returns a Terraform object used to execute commands
func ConfigureTerraform(workingDir string) (*tfexec.Terraform, error) {
	path, err := exec.LookPath("terraform")
	if err != nil {
		return nil, &TfError{
			help: "must have a copy of terraform installed in PATH",
			err:  err,
		}
	}
	return tfexec.NewTerraform(workingDir, path)
}

// this function executes a lightweight "terraform init" that is designed to
// test if the root module was previously initialized and is consistent with
// the current code; it will not download modules or configure backends, but it
// will download plugins (e.g. google provider) as needed; no reliable mechanism
// has been found (e.g. tfexec.PluginDir("/dev/null")) that avoids erroring on
// properly-initialized root modules
func needsInit(tf *tfexec.Terraform) bool {
	getOpt := tfexec.Get(false)
	backendOpt := tfexec.Backend(false)
	e := tf.Init(context.Background(), getOpt, backendOpt)

	return e != nil
}

func initModule(tf *tfexec.Terraform) error {
	var err error
	if needsInit(tf) {
		log.Printf("initializing terraform directory %s", tf.WorkingDir())
		err = tf.Init(context.Background())
	}

	if err != nil {
		return &TfError{
			help: fmt.Sprintf("initialization of %s failed; manually resolve errors below", tf.WorkingDir()),
			err:  err,
		}
	}

	return err
}

func outputModule(tf *tfexec.Terraform) (map[string]cty.Value, error) {
	log.Printf("collecting terraform outputs from %s", tf.WorkingDir())
	output, err := tf.Output(context.Background())
	if err != nil {
		return map[string]cty.Value{}, &TfError{
			help: fmt.Sprintf("collecting terraform outputs from %s failed; manually resolve errors below", tf.WorkingDir()),
			err:  err,
		}
	}

	outputValues := make(map[string]cty.Value, len(output))
	for k, v := range output {
		ov := outputValue{Name: k, Sensitive: v.Sensitive}
		if err := json.Unmarshal(v.Type, &ov.Type); err != nil {
			return map[string]cty.Value{}, err
		}

		var s interface{}
		if err := json.Unmarshal(v.Value, &s); err != nil {
			return map[string]cty.Value{}, err
		}

		if ov.Value, err = gocty.ToCtyValue(s, ov.Type); err != nil {
			return map[string]cty.Value{}, err
		}
		outputValues[ov.Name] = ov.Value
	}
	return outputValues, nil
}

func getOutputs(tf *tfexec.Terraform) (map[string]cty.Value, error) {
	if err := initModule(tf); err != nil {
		return map[string]cty.Value{}, err
	}

	log.Printf("testing if terraform state of %s is in sync with cloud infrastructure", tf.WorkingDir())
	wantsChange, err := tf.Plan(context.Background())
	if err != nil {
		return map[string]cty.Value{}, &TfError{
			help: fmt.Sprintf("terraform plan for %s failed; suggest running \"ghpc export-outputs\" on previous deployment groups to define inputs", tf.WorkingDir()),
			err:  err,
		}
	}

	if wantsChange {
		return map[string]cty.Value{},
			fmt.Errorf("cloud infrastructure requires changes; please run \"terraform -chdir=%s apply\"", tf.WorkingDir())
	}

	outputValues, err := outputModule(tf)
	if err != nil {
		return map[string]cty.Value{}, err
	}
	return outputValues, nil
}

func outputsFile(artifactsDir string, groupName string) string {
	return filepath.Join(artifactsDir, fmt.Sprintf("%s_outputs.tfvars", groupName))
}

// ExportOutputs will run terraform output and capture data needed for
// subsequent deployment groups
func ExportOutputs(tf *tfexec.Terraform, artifactsDir string) error {
	thisGroup := filepath.Base(tf.WorkingDir())
	filepath := outputsFile(artifactsDir, thisGroup)

	outputValues, err := getOutputs(tf)
	if err != nil {
		return err
	}

	// TODO: confirm that outputValues has keys we would expect from the
	// blueprint; edge case is that "terraform output" can be missing keys
	// whose values are null
	if len(outputValues) == 0 {
		log.Printf("group %s contains no artifacts to export", thisGroup)
		return nil
	}

	log.Printf("writing outputs artifact from group %s to file %s", thisGroup, filepath)
	if err := modulewriter.WriteHclAttributes(outputValues, filepath); err != nil {
		return err
	}

	return nil
}

// ImportInputs will search artifactsDir for files produced by ExportOutputs and
// combine/filter them for the input values needed by the group in the Terraform
// working directory
func ImportInputs(deploymentGroupDir string, artifactsDir string, expandedBlueprintFile string) error {
	deploymentRoot := filepath.Clean(filepath.Join(deploymentGroupDir, ".."))
	thisGroup := filepath.Base(deploymentGroupDir)

	outputNamesByGroup, err := getIntergroupOutputNamesByGroup(thisGroup, expandedBlueprintFile)
	if err != nil {
		return err
	}

	kinds, err := GetDeploymentKinds(expandedBlueprintFile)
	if err != nil {
		return err
	}

	// for each prior group, read all output values and filter for those needed
	// as input values to this group; merge into a single map
	allInputValues := make(map[string]cty.Value)
	for group, intergroupOutputNames := range outputNamesByGroup {
		if len(intergroupOutputNames) == 0 {
			continue
		}
		log.Printf("collecting outputs for group %s from group %s", thisGroup, group)
		filepath := outputsFile(artifactsDir, group)
		groupOutputValues, err := modulereader.ReadHclAttributes(filepath)
		if err != nil {
			return &TfError{
				help: fmt.Sprintf("consider running \"ghpc export-outputs %s/%s\"", deploymentRoot, group),
				err:  err,
			}
		}
		intergroupValues := intersectMapKeys(intergroupOutputNames, groupOutputValues)
		mergeMapsWithoutLoss(allInputValues, intergroupValues)
	}

	if len(allInputValues) == 0 {
		return nil
	}

	var outfile string
	switch kinds[thisGroup] {
	case config.TerraformKind:
		outfile = filepath.Join(deploymentGroupDir, fmt.Sprintf("%s_inputs.auto.tfvars", thisGroup))
	case config.PackerKind:
		// outfile = filepath.Join(deploymentGroupDir, fmt.Sprintf("%s_inputs.auto.pkrvars.hcl", thisGroup))
		return fmt.Errorf("import command is not yet supported for Packer deployment groups")
	default:
		return fmt.Errorf("unexpected error: unknown module kind for group %s", thisGroup)
	}
	log.Printf("writing outputs for group %s to file %s\n", thisGroup, outfile)
	if err := modulewriter.WriteHclAttributes(allInputValues, outfile); err != nil {
		return err
	}

	return nil
}
