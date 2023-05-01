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
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/modulewriter"
	"log"
	"os/exec"
	"path"

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
	log.Printf("executing \"terraform -chdir=%s init\"\n", tf.WorkingDir())
	if needsInit(tf) {
		err = tf.Init(context.Background())
	}

	if err != nil {
		return &TfError{
			help: fmt.Sprintf("\"terraform -chdir=%s init\" failed; manually resolve errors below", tf.WorkingDir()),
			err:  err,
		}
	}

	return err
}

func outputModule(tf *tfexec.Terraform) (map[string]cty.Value, error) {
	log.Printf("executing \"terraform -chdir=%s output\"\n", tf.WorkingDir())
	output, err := tf.Output(context.Background())
	if err != nil {
		return map[string]cty.Value{}, &TfError{
			help: fmt.Sprintf("\"terraform -chdir=%s output\" failed; manually resolve errors below", tf.WorkingDir()),
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

	log.Printf("executing \"terraform -chdir=%s plan\"\n", tf.WorkingDir())
	wantsChange, err := tf.Plan(context.Background())
	if err != nil {
		return map[string]cty.Value{}, &TfError{
			help: fmt.Sprintf("\"terraform -chdir=%s init\" failed; most likely need to run \"ghpc export-outputs\" on previous deployment groups to define inputs", tf.WorkingDir()),
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
	return path.Join(artifactsDir, fmt.Sprintf("%s_outputs.tfvars", groupName))
}

// ExportOutputs will run terraform output and capture data needed for
// subsequent deployment groups
func ExportOutputs(tf *tfexec.Terraform, metadataFile string, artifactsDir string) error {
	thisGroup := path.Base(tf.WorkingDir())
	filepath := outputsFile(artifactsDir, thisGroup)

	outputValues, err := getOutputs(tf)
	if err != nil {
		return err
	}

	// TODO: confirm that outputValues has keys we would expect from the
	// blueprint; edge case is that "terraform output" can be missing keys
	// whose values are null
	if len(outputValues) == 0 {
		log.Printf("group %s contains no artifacts to export\n", thisGroup)
		return nil
	}

	log.Printf("writing outputs artifact from group %s to file %s\n", thisGroup, filepath)
	if err := modulewriter.WriteHclAttributes(outputValues, filepath); err != nil {
		return err
	}

	return nil
}

// ImportInputs will search artifactsDir for files produced by ExportOutputs and
// combine/filter them for the input values needed by the group in the Terraform
// working directory
func ImportInputs(deploymentGroupDir string, metadataFile string, artifactsDir string) error {
	deploymentRoot := path.Clean(path.Join(deploymentGroupDir, ".."))
	thisGroup := path.Base(deploymentGroupDir)

	outputNamesByGroup, err := getIntergroupOutputNamesByGroup(thisGroup, metadataFile)
	if err != nil {
		return err
	}

	// TODO: when support for writing Packer inputs (*.pkrvars.hcl) is added,
	// group kind will matter for file naming; for now, use GetDeploymentKinds
	// only to do a basic test of the deployment directory structure
	if _, err = GetDeploymentKinds(metadataFile, deploymentRoot); err != nil {
		return err
	}

	// for each prior group, read all output values and filter for those needed
	// as input values to this group; merge into a single map
	allInputValues := make(map[string]cty.Value)
	for group, intergroupOutputNames := range outputNamesByGroup {
		if len(intergroupOutputNames) == 0 {
			continue
		}
		log.Printf("collecting outputs for group %s from group %s\n", thisGroup, group)
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

	outfile := path.Join(deploymentGroupDir, fmt.Sprintf("%s_inputs.auto.tfvars", thisGroup))
	log.Printf("writing outputs for group %s to file %s\n", thisGroup, outfile)
	if err := modulewriter.WriteHclAttributes(allInputValues, outfile); err != nil {
		return err
	}

	return nil
}
