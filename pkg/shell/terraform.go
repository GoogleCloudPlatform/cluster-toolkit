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
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/modulewriter"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
)

// ApplyBehavior abstracts behaviors for making changes to cloud infrastructure
// when ghpc believes that they may be necessary
type ApplyBehavior uint

// 3 behaviors making changes: never, automatic, and explicit approval
const (
	NeverApply ApplyBehavior = iota
	AutomaticApply
	PromptBeforeApply
)

// TfError captures Terraform errors while improving helpfulness of message
type TfError struct {
	help string
	err  error
}

func (se *TfError) Error() string {
	return fmt.Sprintf("%s\n%s", se.help, se.err)
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
			help: "must have a copy of terraform installed in PATH (obtain at https://terraform.io)",
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
		log.Printf("Initializing deployment group %s", tf.WorkingDir())
		err = tf.Init(context.Background())
	}

	if err != nil {
		return &TfError{
			help: fmt.Sprintf("initialization of deployment group %s failed; manually resolve errors below", tf.WorkingDir()),
			err:  err,
		}
	}

	return err
}

func outputModule(tf *tfexec.Terraform) (map[string]cty.Value, error) {
	log.Printf("Collecting terraform outputs from %s", tf.WorkingDir())
	output, err := tf.Output(context.Background())
	if err != nil {
		return map[string]cty.Value{}, &TfError{
			help: fmt.Sprintf("collecting terraform outputs from deployment group %s failed; manually resolve errors below", tf.WorkingDir()),
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

// See https://github.com/hashicorp/terraform/blob/4ce385a19b93cf7f1b7780d9b2d3cadc5d0ddb31/internal/command/views/json/diagnostic.go#L34
type Diagnostic struct {
	Severity string `json:"severity"`
	Summary  string `json:"summary"`
	Detail   string `json:"detail"`
}

type JsonMessage struct {
	Level      string     `json:"@level"`
	Diagnostic Diagnostic `json:"diagnostic"`
}

func parseJsonMessages(data string) []JsonMessage {
	res := []JsonMessage{}
	// Every message is on a single line
	sc := bufio.NewScanner(strings.NewReader(data))
	for sc.Scan() {
		var msg JsonMessage
		if err := json.Unmarshal([]byte(sc.Text()), &msg); err != nil {
			continue // silently skip the message
		}
		res = append(res, msg)
	}
	return res
}

func helpOnPlanError(msgs []JsonMessage) string {
	missingVar := false
	for _, msg := range msgs {
		missingVar = missingVar || msg.Diagnostic.Summary == "No value for required variable"
	}
	if missingVar {
		// Based on assumption that the only undefined variables can possibly come from IGC references.
		// This may change in the future.
		return `run "ghpc export-outputs" on previous deployment groups to define inputs`
	} else {
		return ""
	}
}

func planModule(tf *tfexec.Terraform, path string, destroy bool) (bool, error) {
	outOpt := tfexec.Out(path)
	var jsonOut strings.Builder
	wantsChange, err := tf.PlanJSON(context.Background(), &jsonOut, outOpt, tfexec.Destroy(destroy))
	if err != nil {
		// Invoke `Plan` to get human-readable error.
		// TODO: implement rendering to avoid double-call.
		// Note planned deprecration of Plan in favor of JSON-only format
		// https://github.com/hashicorp/terraform-exec/blob/1b7714111a94813e92936051fb3014fec81218d5/tfexec/plan.go#L128-L129
		_, plainError := tf.Plan(context.Background(), tfexec.Destroy(destroy))
		if plainError == nil { // shouldn't happen
			plainError = err // fallback to original error (simple `exit status 1`)
		}
		msg := fmt.Sprintf("terraform plan for deployment group %s failed", tf.WorkingDir())
		help := helpOnPlanError(parseJsonMessages(jsonOut.String()))
		if len(help) > 0 {
			msg = fmt.Sprintf("%s; %s", msg, help)
		}
		return false, &TfError{msg, plainError}
	}

	return wantsChange, nil
}

func promptForApply(tf *tfexec.Terraform, path string, b ApplyBehavior) bool {
	switch b {
	case AutomaticApply:
		return true
	case PromptBeforeApply:
		plan, err := tf.ShowPlanFileRaw(context.Background(), path)
		if err != nil {
			return false
		}

		re := regexp.MustCompile(`Plan: .*\n`)
		summary := re.FindString(plan)

		if summary == "" {
			summary = fmt.Sprintf("Please review full proposed changes for deployment group %s", tf.WorkingDir())
		}

		changes := ProposedChanges{
			Summary: summary,
			Full:    plan,
		}

		return ApplyChangesChoice(changes)
	default:
		return false
	}
}

func applyPlanConsoleOutput(tf *tfexec.Terraform, path string) error {
	planFileOpt := tfexec.DirOrPlan(path)
	log.Printf("Running terraform apply on deployment group %s", tf.WorkingDir())
	tf.SetStdout(os.Stdout)
	tf.SetStderr(os.Stderr)
	if err := tf.Apply(context.Background(), planFileOpt); err != nil {
		return err
	}
	tf.SetStdout(nil)
	tf.SetStderr(nil)
	return nil
}

// generate a Terraform plan to apply or destroy a module
// recall "destroy" is just an alias for "apply -destroy"!
// apply the plan automatically or after prompting the user
func applyOrDestroy(tf *tfexec.Terraform, b ApplyBehavior, destroy bool) error {
	action := "adding or changing"
	pastTense := "applied"
	if destroy {
		action = "destroying"
		pastTense = "destroyed"
	}

	if err := initModule(tf); err != nil {
		return err
	}

	log.Printf("Testing if deployment group %s requires %s cloud infrastructure", tf.WorkingDir(), action)
	// capture Terraform plan in a file
	f, err := os.CreateTemp("", "plan-)")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(f.Name())
	wantsChange, err := planModule(tf, f.Name(), destroy)
	if err != nil {
		return err
	}

	var apply bool
	if wantsChange {
		log.Printf("Deployment group %s requires %s cloud infrastructure", tf.WorkingDir(), action)
		apply = b == AutomaticApply || promptForApply(tf, f.Name(), b)
	} else {
		log.Printf("Cloud infrastructure in deployment group %s is already %s", tf.WorkingDir(), pastTense)
	}

	if !apply {
		return nil
	}

	if err := applyPlanConsoleOutput(tf, f.Name()); err != nil {
		return err
	}

	return nil
}

func getOutputs(tf *tfexec.Terraform, b ApplyBehavior) (map[string]cty.Value, error) {
	err := applyOrDestroy(tf, b, false)
	if err != nil {
		return nil, err
	}

	outputValues, err := outputModule(tf)
	if err != nil {
		return nil, err
	}
	return outputValues, nil
}

func outputsFile(artifactsDir string, group config.GroupName) string {
	return filepath.Join(artifactsDir, fmt.Sprintf("%s_outputs.tfvars", string(group)))
}

// ExportOutputs will run terraform output and capture data needed for
// subsequent deployment groups
func ExportOutputs(tf *tfexec.Terraform, artifactsDir string, applyBehavior ApplyBehavior) error {
	thisGroup := config.GroupName(filepath.Base(tf.WorkingDir()))
	filepath := outputsFile(artifactsDir, thisGroup)

	outputValues, err := getOutputs(tf, applyBehavior)
	if err != nil {
		return err
	}

	// TODO: confirm that outputValues has keys we would expect from the
	// blueprint; edge case is that "terraform output" can be missing keys
	// whose values are null
	if len(outputValues) == 0 {
		log.Printf("Deployment group %s contains no artifacts to export", thisGroup)
		return nil
	}

	log.Printf("Writing outputs artifact from deployment group %s to file %s", thisGroup, filepath)
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

	dc, _, err := config.NewDeploymentConfig(expandedBlueprintFile)
	if err != nil {
		return err
	}
	g, err := dc.Config.Group(config.GroupName(filepath.Base(deploymentGroupDir)))
	if err != nil {
		return err
	}
	outputNamesByGroup, err := config.OutputNamesByGroup(g, dc)
	if err != nil {
		return err
	}

	// for each prior group, read all output values and filter for those needed
	// as input values to this group; merge into a single map
	allInputValues := make(map[string]cty.Value)
	for groupName, intergroupOutputNames := range outputNamesByGroup {
		if len(intergroupOutputNames) == 0 {
			continue
		}
		log.Printf("collecting outputs for group %s from group %s", g.Name, groupName)
		filepath := outputsFile(artifactsDir, groupName)
		groupOutputValues, err := modulereader.ReadHclAttributes(filepath)
		if err != nil {
			return &TfError{
				help: fmt.Sprintf("consider running \"ghpc export-outputs %s/%s\"", deploymentRoot, groupName),
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
	switch g.Kind() {
	case config.TerraformKind:
		outfile = filepath.Join(deploymentGroupDir, fmt.Sprintf("%s_inputs.auto.tfvars", g.Name))
	case config.PackerKind:
		thisGroupIdx := dc.Config.GroupIndex(g.Name)
		packerGroup := dc.Config.DeploymentGroups[thisGroupIdx]
		// Packer groups are enforced to have length 1
		packerModule := packerGroup.Modules[0]
		modPath, err := modulewriter.DeploymentSource(packerModule)
		if err != nil {
			return err
		}
		outfile = filepath.Join(deploymentGroupDir, modPath,
			fmt.Sprintf("%s_inputs.auto.pkrvars.hcl", packerModule.ID))

		// evaluate Packer settings that contain intergroup references in the
		// context of deployment variables and intergroup output values
		intergroupSettings := config.Dict{}
		for setting, value := range packerModule.Settings.Items() {
			igcRefs := config.FindIntergroupReferences(value, packerModule, dc.Config)
			if len(igcRefs) > 0 {
				intergroupSettings.Set(setting, value)
			}
		}

		igcVars := modulewriter.FindIntergroupVariables(packerGroup, dc.Config)
		newModule := modulewriter.SubstituteIgcReferencesInModule(config.Module{Settings: intergroupSettings}, igcVars)

		varsValues := dc.Config.Vars.Items()
		mergeMapsWithoutLoss(allInputValues, varsValues)
		evaluatedSettings, err := newModule.Settings.Eval(config.Blueprint{Vars: config.NewDict(allInputValues)})
		if err != nil {
			return err
		}
		allInputValues = evaluatedSettings.Items()
	default:
		return fmt.Errorf("unexpected error: unknown module kind for deployment group %s", g.Name)
	}
	log.Printf("Writing outputs for deployment group %s to file %s\n", g.Name, outfile)
	if err := modulewriter.WriteHclAttributes(allInputValues, outfile); err != nil {
		return err
	}

	return nil
}

// Destroy destroys all infrastructure in the module working directory
func Destroy(tf *tfexec.Terraform, b ApplyBehavior) error {
	return applyOrDestroy(tf, b, true)
}
