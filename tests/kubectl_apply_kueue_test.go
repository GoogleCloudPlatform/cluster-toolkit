// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package tests

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	yamlv3 "gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

const kubectlApplyModuleDir = "../modules/management/kubectl-apply"

var kueueOrderedLocalNames = []string{
	"kueue_supported_versions",
	"enable_pathways",
	"enable_slicing",
	"install_kueue",
	"kueue_controller_cpu",
	"kueue_controller_memory",
	"kueue_controller_replicas",
	"kueue_enable_wait_for_pods_ready",
	"kueue_pods_ready_recovery_timeout",
	"kueue_wait_for_pods_ready",
	"kueue_helm_values_path",
	"kueue_helm_values_file_exists",
	"kueue_user_helm_values_raw",
	"kueue_user_helm_values_is_blank",
	"kueue_user_helm_values_content",
	"kueue_helm_values_is_mapping",
	"kueue_user_helm_values_parsed",
	"kueue_helm_values_not_k8s_manifest",
	"kueue_helm_values_check_fields",
	"kueue_helm_values_preserves_fullname",
	"kueue_user_controller_manager_valid",
	"kueue_user_manager_config_container_valid",
	"kueue_has_user_feature_gates_field",
	"kueue_user_feature_gates_valid",
	"kueue_user_feature_gates",
	"kueue_user_feature_gates_by_name",
	"kueue_base_feature_gates",
	"kueue_merged_feature_gates",
	"kueue_has_user_tolerations_field",
	"kueue_user_tolerations_valid",
	"kueue_has_user_tolerations",
	"kueue_user_tolerations",
	"kueue_merged_tolerations",
	"kueue_has_user_manager_config_field",
	"kueue_user_manager_config_valid_yaml",
	"kueue_user_manager_config",
	"kueue_user_manager_config_valid_kind",
	"kueue_user_manager_config_no_feature_gates",
	"kueue_user_manager_config_sections_valid",
	"kueue_raw_base_manager_config",
	"kueue_base_manager_config",
	"kueue_merged_wfpr_entry",
	"kueue_merged_manager_config",
}

type kueueEvalInput struct {
	version               string
	install               bool
	enablePathways        bool
	helmValuesPath        *string
	helmValuesTemplateVar map[string]cty.Value
	waitForPodsReady      map[string]cty.Value
	controllerCPU         *string
	controllerMemory      *string
	controllerReplicas    *int64
}

type kueueParsedHCL struct {
	locals        map[string]hcl.Expression
	preconditions []validationRule
	valuesYAML    hcl.Expression
}

// yamlDecodeCtyFunc matches Terraform's yamldecode(): rejects empty/comment-only
// input (EOF) and multi-document streams ('---'), and decodes explicit null (~ / null).
func yamlDecodeCtyFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "src", Type: cty.String}},
		Type:   function.StaticReturnType(cty.DynamicPseudoType),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			raw := args[0].AsString()
			dec := yamlv3.NewDecoder(strings.NewReader(raw))
			var first yamlv3.Node
			if err := dec.Decode(&first); err != nil {
				return cty.NilVal, err
			}
			var extra yamlv3.Node
			if err := dec.Decode(&extra); err != io.EOF {
				if err == nil {
					return cty.NilVal, fmt.Errorf("cannot decode multi-document YAML")
				}
				return cty.NilVal, err
			}
			return singleYAMLDocToCty(raw)
		},
	})
}

func singleYAMLDocToCty(raw string) (cty.Value, error) {
	jsonBytes, err := sigsyaml.YAMLToJSON([]byte(raw))
	if err != nil {
		return cty.NilVal, err
	}
	if bytes.Equal(bytes.TrimSpace(jsonBytes), []byte("null")) {
		return cty.NullVal(cty.DynamicPseudoType), nil
	}
	ty, err := ctyjson.ImpliedType(jsonBytes)
	if err != nil {
		return cty.NilVal, err
	}
	return ctyjson.Unmarshal(jsonBytes, ty)
}

func yamlEncodeCtyFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "val", Type: cty.DynamicPseudoType, AllowNull: true}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			if args[0].IsNull() {
				return cty.StringVal("null\n"), nil
			}
			jsonBytes, err := ctyjson.Marshal(args[0], args[0].Type())
			if err != nil {
				return cty.NilVal, err
			}
			yamlBytes, err := sigsyaml.JSONToYAML(jsonBytes)
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(string(yamlBytes)), nil
		},
	})
}

func fileSystemCtyFuncs() map[string]function.Function {
	fileFn := function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			b, err := os.ReadFile(args[0].AsString())
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(string(b)), nil
		},
	})
	fileExistsFn := function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			p := args[0].AsString()
			if p == "" {
				return cty.False, nil
			}
			info, err := os.Stat(p)
			return cty.BoolVal(err == nil && !info.IsDir()), nil
		},
	})
	templateFileFn := function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
			{Name: "vars", Type: cty.DynamicPseudoType},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return renderTemplateFile(args[0].AsString(), args[1])
		},
	})
	return map[string]function.Function{
		"file":         fileFn,
		"fileexists":   fileExistsFn,
		"templatefile": templateFileFn,
	}
}

func renderTemplateFile(path string, varsVal cty.Value) (cty.Value, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return cty.NilVal, err
	}
	expr, diags := hclsyntax.ParseTemplate(src, path, hcl.InitialPos)
	if diags.HasErrors() {
		return cty.NilVal, diags
	}
	vars := map[string]cty.Value{}
	if !varsVal.IsNull() {
		maps.Copy(vars, varsVal.AsValueMap())
	}
	val, diags := expr.Value(&hcl.EvalContext{Variables: vars, Functions: stubTemplateFuncs()})
	if diags.HasErrors() {
		return cty.NilVal, diags
	}
	return val, nil
}

func kueueEvalFunctions() map[string]function.Function {
	endswithFn := function.New(&function.Spec{
		Params: []function.Parameter{{Type: cty.String}, {Type: cty.String}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.BoolVal(strings.HasSuffix(args[0].AsString(), args[1].AsString())), nil
		},
	})
	alltrueFn := function.New(&function.Spec{
		Params: []function.Parameter{{Type: cty.List(cty.Bool)}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			for it := args[0].ElementIterator(); it.Next(); {
				if _, v := it.Element(); v.IsNull() || !v.True() {
					return cty.False, nil
				}
			}
			return cty.True, nil
		},
	})
	anytrueFn := function.New(&function.Spec{
		Params: []function.Parameter{{Type: cty.List(cty.Bool)}},
		Type:   function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			for it := args[0].ElementIterator(); it.Next(); {
				if _, v := it.Element(); !v.IsNull() && v.True() {
					return cty.True, nil
				}
			}
			return cty.False, nil
		},
	})

	funcs := map[string]function.Function{
		"alltrue":    alltrueFn,
		"anytrue":    anytrueFn,
		"can":        tryfunc.CanFunc,
		"coalesce":   terraformCoalesceFunc,
		"compact":    stdlib.CompactFunc,
		"concat":     stdlib.ConcatFunc,
		"contains":   stdlib.ContainsFunc,
		"distinct":   stdlib.DistinctFunc,
		"endswith":   endswithFn,
		"join":       stdlib.JoinFunc,
		"keys":       stdlib.KeysFunc,
		"length":     stdlib.LengthFunc,
		"lookup":     stdlib.LookupFunc,
		"merge":      stdlib.MergeFunc,
		"regex":      stdlib.RegexFunc,
		"regexall":   stdlib.RegexAllFunc,
		"tostring":   stdlib.MakeToFunc(cty.String),
		"tobool":     stdlib.MakeToFunc(cty.Bool),
		"trimspace":  stdlib.TrimSpaceFunc,
		"try":        tryfunc.TryFunc,
		"yamldecode": yamlDecodeCtyFunc(),
		"yamlencode": yamlEncodeCtyFunc(),
	}
	maps.Copy(funcs, fileSystemCtyFuncs())
	return funcs
}

func readKubectlApplyKueueHCL(t *testing.T) kueueParsedHCL {
	t.Helper()
	varBody := parseHCLFile(t, filepath.Join(kubectlApplyModuleDir, "variables.tf"))
	mainBody := parseHCLFile(t, filepath.Join(kubectlApplyModuleDir, "main.tf"))

	locals := make(map[string]hcl.Expression)
	for _, block := range append(varBody.Blocks, mainBody.Blocks...) {
		if block.Type != "locals" {
			continue
		}
		for name, attr := range block.Body.Attributes {
			locals[name] = attr.Expr
		}
	}

	var preconditions []validationRule
	for _, block := range varBody.Blocks {
		if block.Type != "resource" || !slices.Equal(block.Labels, []string{"terraform_data", "kueue_validations"}) {
			continue
		}
		for _, sub := range flattenBlocks(block.Body) {
			if sub.Type != "precondition" {
				continue
			}
			preconditions = append(preconditions, validationRule{
				condition:    sub.Body.Attributes["condition"].Expr,
				errorMessage: sub.Body.Attributes["error_message"].Expr,
			})
		}
	}

	mainBlocks := flattenBlocks(mainBody)
	valuesYAML := findBlockAttribute(t, mainBlocks, "module", "values_yaml", "install_kueue")
	return kueueParsedHCL{
		locals:        locals,
		preconditions: preconditions,
		valuesYAML:    valuesYAML,
	}
}

func semverAtLeast(ver string, minMinor int) bool {
	parts := strings.Split(strings.TrimPrefix(ver, "v"), ".")
	if len(parts) < 2 || parts[0] != "0" {
		return true
	}
	var minor int
	_, _ = fmt.Sscanf(parts[1], "%d", &minor)
	return minor >= minMinor
}

func optStringVal(s *string) cty.Value {
	if s == nil {
		return cty.NullVal(cty.String)
	}
	return cty.StringVal(*s)
}

func optIntVal(n *int64) cty.Value {
	if n == nil {
		return cty.NullVal(cty.Number)
	}
	return cty.NumberIntVal(*n)
}

func buildKueueVarValue(in kueueEvalInput) cty.Value {
	ver := in.version
	if ver == "" {
		ver = "0.19.6"
	}
	tmplVars := cty.NullVal(cty.DynamicPseudoType)
	if in.helmValuesTemplateVar != nil {
		tmplVars = cty.ObjectVal(in.helmValuesTemplateVar)
	}
	wfpr := cty.NullVal(cty.DynamicPseudoType)
	if in.waitForPodsReady != nil {
		wfpr = cty.ObjectVal(in.waitForPodsReady)
	}
	cfgTmplVars := cty.NullVal(cty.DynamicPseudoType)
	if in.enablePathways {
		cfgTmplVars = cty.ObjectVal(map[string]cty.Value{"accelerator_type": cty.StringVal("tpu-v5p-slice")})
	}
	return cty.ObjectVal(map[string]cty.Value{
		"version":                         cty.StringVal(ver),
		"install":                         cty.BoolVal(in.install),
		"config_path":                     cty.NullVal(cty.String),
		"config_template_vars":            cfgTmplVars,
		"helm_values_path":                optStringVal(in.helmValuesPath),
		"helm_values_template_vars":       tmplVars,
		"enable_pathways_for_tpus":        cty.False,
		"enable_dynamic_slicing_for_tpus": cty.False,
		"accelerator_topology_mode":       cty.NullVal(cty.String),
		"machine_type":                    cty.NullVal(cty.String),
		"controller_cpu":                  optStringVal(in.controllerCPU),
		"controller_memory":               optStringVal(in.controllerMemory),
		"controller_replicas":             optIntVal(in.controllerReplicas),
		"wait_for_pods_ready":             wfpr,
	})
}

func evalKueueHCL(t *testing.T, parsed kueueParsedHCL, in kueueEvalInput) ([]string, []string) {
	t.Helper()
	ver := in.version
	if ver == "" {
		ver = "0.19.6"
	}
	localMap := map[string]cty.Value{}
	ctx := &hcl.EvalContext{
		Functions: kueueEvalFunctions(),
		Variables: map[string]cty.Value{
			"path": cty.ObjectVal(map[string]cty.Value{"module": cty.StringVal(kubectlApplyModuleDir)}),
			"var": cty.ObjectVal(map[string]cty.Value{
				"enable_pathways_for_tpus": cty.BoolVal(in.enablePathways),
				"jobset":                   cty.ObjectVal(map[string]cty.Value{"install": cty.False}),
				"kueue":                    buildKueueVarValue(in),
			}),
			"module": cty.ObjectVal(map[string]cty.Value{
				"kueue_pods_ready_default_on_check": cty.ObjectVal(map[string]cty.Value{
					"is_greater_than_or_equal": cty.BoolVal(semverAtLeast(ver, 19)),
				}),
				"kueue_dra_version_check": cty.ObjectVal(map[string]cty.Value{
					"is_greater_than_or_equal": cty.BoolVal(semverAtLeast(ver, 18)),
				}),
			}),
			"local": cty.ObjectVal(localMap),
		},
	}

	for _, name := range kueueOrderedLocalNames {
		expr, ok := parsed.locals[name]
		if !ok {
			t.Fatalf("missing local.%s in kubectl-apply", name)
		}
		val, diags := expr.Value(ctx)
		if diags.HasErrors() {
			t.Fatalf("evaluating local.%s failed: %v", name, diags)
		}
		localMap[name] = val
		ctx.Variables["local"] = cty.ObjectVal(localMap)
	}

	failedMsgs := evaluatePreconditions(t, parsed.preconditions, ctx)

	valsVal, diags := parsed.valuesYAML.Value(ctx)
	if diags.HasErrors() {
		t.Fatalf("evaluating install_kueue.values_yaml failed: %v", diags)
	}
	var valuesYAML []string
	for it := valsVal.ElementIterator(); it.Next(); {
		_, elem := it.Element()
		valuesYAML = append(valuesYAML, elem.AsString())
	}
	return valuesYAML, failedMsgs
}

func evaluatePreconditions(t *testing.T, preconditions []validationRule, ctx *hcl.EvalContext) []string {
	t.Helper()
	var failedMsgs []string
	for _, pre := range preconditions {
		cond, diags := pre.condition.Value(ctx)
		if diags.HasErrors() {
			t.Fatalf("precondition evaluation failed: %v", diags)
		}
		if !cond.True() {
			msgVal, msgDiags := pre.errorMessage.Value(ctx)
			if msgDiags.HasErrors() {
				t.Fatalf("precondition error_message evaluation failed: %v", msgDiags)
			}
			failedMsgs = append(failedMsgs, msgVal.AsString())
		}
	}
	return failedMsgs
}

// simulateHelmValuesMerge mimics Helm's -f values merging (maps deep-merge, nil deletes keys, slices/scalars replace).
func simulateHelmValuesMerge(t *testing.T, layers []string) map[string]any {
	t.Helper()
	merged := make(map[string]any)
	for i, raw := range layers {
		var layer map[string]any
		if err := sigsyaml.Unmarshal([]byte(raw), &layer); err != nil {
			t.Fatalf("layer %d YAML unmarshal failed: %v\n%s", i, err, raw)
		}
		deepMergeMaps(merged, layer)
	}
	return merged
}

func deepMergeMaps(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			delete(dst, k)
			continue
		}
		srcMap, srcIsMap := v.(map[string]any)
		dstMap, dstIsMap := dst[k].(map[string]any)
		if srcIsMap && dstIsMap {
			deepMergeMaps(dstMap, srcMap)
		} else {
			dst[k] = v
		}
	}
}

func writeTempValuesFile(t *testing.T, name, content string) *string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return &p
}

func extractFeatureGateMap(t *testing.T, helmValues map[string]any) map[string]bool {
	t.Helper()
	cm, ok := helmValues["controllerManager"].(map[string]any)
	if !ok {
		t.Fatalf("missing controllerManager in merged Helm values: %#v", helmValues)
	}
	rawGates, ok := cm["featureGates"].([]any)
	if !ok {
		t.Fatalf("missing controllerManager.featureGates list: %#v", cm)
	}
	gates := make(map[string]bool, len(rawGates))
	for _, item := range rawGates {
		m := item.(map[string]any)
		name := m["name"].(string)
		if _, dup := gates[name]; dup {
			t.Fatalf("duplicate featureGate %q in %#v", name, rawGates)
		}
		gates[name] = m["enabled"].(bool)
	}
	return gates
}

func extractTolerationKeys(t *testing.T, helmValues map[string]any) []string {
	t.Helper()
	cm := helmValues["controllerManager"].(map[string]any)
	tols := cm["tolerations"].([]any)
	out := make([]string, 0, len(tols))
	for _, item := range tols {
		out = append(out, item.(map[string]any)["key"].(string))
	}
	return out
}

func decodeControllerManagerConfig(t *testing.T, helmValues map[string]any) map[string]any {
	t.Helper()
	mc, ok := helmValues["managerConfig"].(map[string]any)
	if !ok {
		t.Fatalf("missing managerConfig in merged Helm values: %#v", helmValues)
	}
	raw, ok := mc["controllerManagerConfigYaml"].(string)
	if !ok {
		t.Fatalf("missing controllerManagerConfigYaml string in %#v", mc)
	}
	var cfg map[string]any
	if err := sigsyaml.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("invalid controllerManagerConfigYaml: %v\n%s", err, raw)
	}
	return cfg
}

func TestKueueHCLFormatting(t *testing.T) {
	for _, name := range []string{"variables.tf", "main.tf"} {
		p := filepath.Join(kubectlApplyModuleDir, name)
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		formatted := hclwrite.Format(src)
		if !bytes.Equal(src, formatted) {
			t.Fatalf("%s is not canonically formatted", p)
		}
	}
}

func TestKueueHelmValuesDefaultBackwardCompat(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{install: true, version: "0.19.6"})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if len(layers) != 2 {
		t.Fatalf("got %d values_yaml layers, want 2: %v", len(layers), layers)
	}
	merged := simulateHelmValuesMerge(t, layers)
	if _, hasMC := merged["managerConfig"]; hasMC {
		t.Fatalf("default non-Pathways install must not override managerConfig: %#v", merged["managerConfig"])
	}
	gates := extractFeatureGateMap(t, merged)
	if !gates["TopologyAwareScheduling"] || gates["KueueDRAIntegration"] {
		t.Fatalf("unexpected default feature gates: %#v", gates)
	}
}

func TestKueueHelmValuesSharedClusterFullOverride(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	valuesPath := writeTempValuesFile(t, "kueue-helm-values.yaml", `
enablePrometheus: true
controllerManager:
  featureGates:
    - name: VisibilityOnDemand
      enabled: true
  tolerations:
    - key: dedicated
      operator: Exists
      effect: NoSchedule
managerConfig:
  controllerManagerConfigYaml: |
    apiVersion: config.kueue.x-k8s.io/v1beta2
    kind: Configuration
    metrics:
      enableClusterQueueResources: true
    controller:
      groupKindConcurrency:
        Pod: 10
    manageJobsWithoutQueueName: true
    managedJobsNamespaceSelector:
      matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: In
          values:
            - default
    resources:
      excludeResourcePrefixes:
        - networking.gke.io.networks/
    waitForPodsReady:
      timeout: 90m
      recoveryTimeout: 5m
      blockAdmission: false
      requeuingStrategy:
        timestamp: Creation
        backoffLimitCount: 5
    integrations:
      frameworks:
        - "pod"
        - "batch/job"
        - "jobset.x-k8s.io/jobset"
`)
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        "0.19.6",
		helmValuesPath: valuesPath,
		waitForPodsReady: map[string]cty.Value{
			"timeout":          cty.StringVal("30m"),
			"recovery_timeout": cty.StringVal("0s"),
			"block_admission":  cty.NullVal(cty.Bool),
		},
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}

	merged := simulateHelmValuesMerge(t, layers)
	if merged["enablePrometheus"] != true {
		t.Fatalf("expected enablePrometheus=true, got %#v", merged["enablePrometheus"])
	}

	gates := extractFeatureGateMap(t, merged)
	wantGates := map[string]bool{
		"TopologyAwareScheduling":                 true,
		"VisibilityOnDemand":                      true,
		"KueueDRAIntegration":                     false,
		"KueueDRARejectWorkloadsWhenDRADisabled":  false,
		"KueueDRAIntegrationExtendedResource":     false,
		"KueueDRAIntegrationPartitionableDevices": false,
	}
	if !maps.Equal(gates, wantGates) {
		t.Fatalf("featureGates mismatch:\ngot:  %#v\nwant: %#v", gates, wantGates)
	}

	wantTolKeys := []string{"components.gke.io/gke-managed-components", "dedicated"}
	if gotTolKeys := extractTolerationKeys(t, merged); !slices.Equal(gotTolKeys, wantTolKeys) {
		t.Fatalf("toleration keys = %v, want %v", gotTolKeys, wantTolKeys)
	}

	cfg := decodeControllerManagerConfig(t, merged)
	assertSharedClusterManagerConfig(t, cfg)
}

func assertSharedClusterManagerConfig(t *testing.T, cfg map[string]any) {
	t.Helper()
	metrics := cfg["metrics"].(map[string]any)
	if metrics["bindAddress"] != ":8443" || metrics["enableClusterQueueResources"] != true {
		t.Fatalf("expected metrics deep-merge to preserve bindAddress and add enableClusterQueueResources, got %#v", metrics)
	}
	if cfg["manageJobsWithoutQueueName"] != true {
		t.Fatalf("expected manageJobsWithoutQueueName=true, got %#v", cfg["manageJobsWithoutQueueName"])
	}
	wfpr := cfg["waitForPodsReady"].(map[string]any)
	if wfpr["timeout"] != "90m" || wfpr["recoveryTimeout"] != "5m" || wfpr["requeuingStrategy"] == nil {
		t.Fatalf("expected user waitForPodsReady values and requeuingStrategy, got %#v", wfpr)
	}
	frameworks := cfg["integrations"].(map[string]any)["frameworks"].([]any)
	if len(frameworks) != 3 {
		t.Fatalf("expected integrations.frameworks list replacement (3 items, no deployment/statefulset), got %#v", frameworks)
	}
	gkc := cfg["controller"].(map[string]any)["groupKindConcurrency"].(map[string]any)
	if gkc["Pod"] != float64(10) || gkc["Job.batch"] != float64(5) {
		t.Fatalf("expected groupKindConcurrency merge (Pod=10, Job.batch=5), got %#v", gkc)
	}
}

func TestKueueHelmValuesControllerSizingAndTolerationOverride(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	cpu, mem, replicas := "4", "8Gi", int64(2)
	valuesPath := writeTempValuesFile(t, "sizing-and-tol.yaml", `
fullnameOverride: "kueue"
nameOverride: ""
controllerManager:
  replicas: 3
  tolerations:
    - key: "components.gke.io/gke-managed-components"
      operator: "Equal"
      value: "true"
      effect: "PreferNoSchedule"
`)
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:            true,
		version:            "0.19.6",
		helmValuesPath:     valuesPath,
		controllerCPU:      &cpu,
		controllerMemory:   &mem,
		controllerReplicas: &replicas,
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	merged := simulateHelmValuesMerge(t, layers)
	cm := merged["controllerManager"].(map[string]any)
	if cm["replicas"] != float64(3) {
		t.Fatalf("expected helm_values_path replicas=3 to override controller_replicas=2, got %#v", cm["replicas"])
	}
	reqs := cm["manager"].(map[string]any)["resources"].(map[string]any)["requests"].(map[string]any)
	if reqs["cpu"] != "4" || reqs["memory"] != "8Gi" {
		t.Fatalf("expected controller_cpu/memory preserved when unset in helm_values_path, got %#v", reqs)
	}
	tols := cm["tolerations"].([]any)
	if len(tols) != 1 || tols[0].(map[string]any)["effect"] != "PreferNoSchedule" {
		t.Fatalf("expected explicit GKE toleration override without duplication, got %#v", tols)
	}
}

func TestKueueHelmValuesPathwaysPartialAndNullWaitForPodsReady(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	tmplPath := writeTempValuesFile(t, "kueue-helm-values.yaml.tftpl", `
controllerManager:
  featureGates:
    - name: TopologyAwareScheduling
      enabled: false
managerConfig:
  controllerManagerConfigYaml: |
    manageJobsWithoutQueueName: true
    managedJobsNamespaceSelector:
      matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: NotIn
          values:
            - ${excluded_ns}
    waitForPodsReady:
      requeuingStrategy:
        timestamp: Creation
`)
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        "0.19.6",
		enablePathways: true,
		helmValuesPath: tmplPath,
		helmValuesTemplateVar: map[string]cty.Value{
			"excluded_ns": cty.StringVal("kuberay-system"),
		},
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	merged := simulateHelmValuesMerge(t, layers)
	if extractFeatureGateMap(t, merged)["TopologyAwareScheduling"] {
		t.Fatalf("explicit user TopologyAwareScheduling=false should win")
	}
	cfg := decodeControllerManagerConfig(t, merged)
	wfpr := cfg["waitForPodsReady"].(map[string]any)
	if wfpr["timeout"] != "30m" || wfpr["recoveryTimeout"] != "0s" || wfpr["requeuingStrategy"] == nil {
		t.Fatalf("expected Pathways waitForPodsReady defaults preserved alongside requeuingStrategy, got %#v", wfpr)
	}

	// Explicit waitForPodsReady: null in controllerManagerConfigYaml must suppress Pathways waitForPodsReady
	nullWfprPath := writeTempValuesFile(t, "null-wfpr.yaml", `
managerConfig:
  controllerManagerConfigYaml: |
    waitForPodsReady: null
`)
	nullLayers, nullErrs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        "0.19.6",
		enablePathways: true,
		helmValuesPath: nullWfprPath,
	})
	if len(nullErrs) > 0 {
		t.Fatalf("unexpected validation errors for waitForPodsReady: null: %v", nullErrs)
	}
	nullCfg := decodeControllerManagerConfig(t, simulateHelmValuesMerge(t, nullLayers))
	if nullCfg["waitForPodsReady"] != nil {
		t.Fatalf("expected waitForPodsReady to remain nil when explicitly nulled, got %#v", nullCfg["waitForPodsReady"])
	}
}

func TestKueueHelmValuesStagedTftplHashAndBlankFiles(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)

	// ghpc_stage appends _<10-hex-chars> after .tftpl; must still render via templatefile() when template vars are nil
	stagedPath := writeTempValuesFile(t, "kueue-helm-values.yaml.tftpl_a1b2c3d4e5", `
%{ if true ~}
enablePrometheus: true
%{ endif ~}
`)
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        "0.19.6",
		helmValuesPath: stagedPath,
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors on staged .tftpl_<hash>: %v", errs)
	}
	if simulateHelmValuesMerge(t, layers)["enablePrometheus"] != true {
		t.Fatalf("staged .tftpl_<hash> template directive was not rendered")
	}

	// Comment-only and conditionally empty templates must succeed as no-ops
	for name, content := range map[string]string{
		"comments.yaml":               "# comment only\n   # another comment\n",
		"empty.yaml.tftpl_0123456789": "%{ if false }enablePrometheus: true%{ endif }\n",
		"explicit-null.yaml":          "---\n~\n",
	} {
		t.Run(name, func(t *testing.T) {
			p := writeTempValuesFile(t, name, content)
			gotLayers, gotErrs := evalKueueHCL(t, parsed, kueueEvalInput{
				install:        true,
				version:        "0.19.6",
				helmValuesPath: p,
			})
			if len(gotErrs) > 0 {
				t.Fatalf("blank/comment-only file %s failed validation: %v", name, gotErrs)
			}
			if len(gotLayers) != 2 {
				t.Fatalf("expected 2 default layers for blank file %s, got %d", name, len(gotLayers))
			}
		})
	}
}

func TestKueueHelmValuesOlderVersionCompatAndInstallFalse(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)

	missing := "/nonexistent/path/kueue-values.yaml"
	if _, errs := evalKueueHCL(t, parsed, kueueEvalInput{install: false, helmValuesPath: &missing}); len(errs) > 0 {
		t.Fatalf("install=false must skip file checks, got %v", errs)
	}

	partialPath := writeTempValuesFile(t, "partial.yaml", `
managerConfig:
  controllerManagerConfigYaml: |
    manageJobsWithoutQueueName: true
`)
	assertOlderVersionDefaults(t, parsed, partialPath, "0.15.2", false, false)
	assertOlderVersionDefaults(t, parsed, partialPath, "0.16.0", true, false)
	assertOlderVersionDefaults(t, parsed, partialPath, "0.17.1", true, true)
}

func assertOlderVersionDefaults(t *testing.T, parsed kueueParsedHCL, path *string, ver string, wantPodIntegration, wantRayService bool) {
	t.Helper()
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        ver,
		helmValuesPath: path,
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors on %s: %v", ver, errs)
	}
	merged := simulateHelmValuesMerge(t, layers)
	gates := extractFeatureGateMap(t, merged)
	if len(gates) != 1 || !gates["TopologyAwareScheduling"] {
		t.Fatalf("expected only TopologyAwareScheduling on %s, got %#v", ver, gates)
	}
	cfg := decodeControllerManagerConfig(t, merged)
	gkc := cfg["controller"].(map[string]any)["groupKindConcurrency"].(map[string]any)
	if _, hasCohort := gkc["Cohort.kueue.x-k8s.io"]; hasCohort {
		t.Fatalf("%s base config must not inject Cohort.kueue.x-k8s.io: %#v", ver, gkc)
	}
	var fws []string
	for _, f := range cfg["integrations"].(map[string]any)["frameworks"].([]any) {
		fws = append(fws, f.(string))
	}
	if slices.Contains(fws, "pod") != wantPodIntegration {
		t.Fatalf("%s pod integration = %t, want %t (%v)", ver, !wantPodIntegration, wantPodIntegration, fws)
	}
	if slices.Contains(fws, "ray.io/rayservice") != wantRayService {
		t.Fatalf("%s rayservice = %t, want %t (%v)", ver, !wantRayService, wantRayService, fws)
	}
	if !slices.Contains(fws, "kubeflow.org/pytorchjob") {
		t.Fatalf("%s must retain Kubeflow Training v1 pytorchjob default (%v)", ver, fws)
	}
}

func TestKueueHelmValuesValidationGuardrails(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")

	cases := []struct {
		name       string
		path       *string
		content    string
		wantSubstr string
	}{
		{name: "missing_file", path: &missing, wantSubstr: "does not exist"},
		{name: "invalid_yaml_syntax", content: "controllerManager: [unclosed", wantSubstr: "single-document YAML mapping"},
		{name: "yaml_list_not_mapping", content: "- item1\n- item2\n", wantSubstr: "single-document YAML mapping"},
		{name: "multi_doc_non_k8s_yaml", content: "enablePrometheus: true\n---\nenableCertManager: true\n", wantSubstr: "single-document YAML mapping"},
		{name: "raw_configmap_manifest", content: "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: kueue-manager-config\n", wantSubstr: "not a Kubernetes manifest"},
		{name: "raw_clusterqueue_manifest", content: "apiVersion: kueue.x-k8s.io/v1beta2\nkind: ClusterQueue\nmetadata:\n  name: cq\n", wantSubstr: "kueue.config_path"},
		{name: "multi_doc_k8s_manifests", content: "apiVersion: kueue.x-k8s.io/v1beta2\nkind: ResourceFlavor\nmetadata:\n  name: rf\n---\napiVersion: kueue.x-k8s.io/v1beta2\nkind: ClusterQueue\nmetadata:\n  name: cq\n", wantSubstr: "kueue.config_path"},
		{name: "fullname_override_custom", content: "fullnameOverride: custom-kueue\n", wantSubstr: "fullnameOverride and nameOverride"},
		{name: "name_override_custom", content: "nameOverride: custom-kueue\n", wantSubstr: "fullnameOverride and nameOverride"},
		{name: "controller_manager_null", content: "controllerManager: null\n", wantSubstr: "controllerManager in kueue.helm_values_path must be a YAML mapping"},
		{name: "feature_gates_null", content: "controllerManager:\n  featureGates:\n", wantSubstr: "controllerManager.featureGates"},
		{name: "feature_gates_as_map", content: "controllerManager:\n  featureGates:\n    VisibilityOnDemand: true\n", wantSubstr: "controllerManager.featureGates"},
		{name: "feature_gates_null_name", content: "controllerManager:\n  featureGates:\n    - name: null\n      enabled: true\n", wantSubstr: "controllerManager.featureGates"},
		{name: "feature_gates_blank_name", content: "controllerManager:\n  featureGates:\n    - name: \"   \"\n      enabled: true\n", wantSubstr: "controllerManager.featureGates"},
		{name: "feature_gates_null_enabled", content: "controllerManager:\n  featureGates:\n    - name: VisibilityOnDemand\n      enabled: null\n", wantSubstr: "controllerManager.featureGates"},
		{name: "feature_gates_string_enabled", content: "controllerManager:\n  featureGates:\n    - name: VisibilityOnDemand\n      enabled: \"true\"\n", wantSubstr: "controllerManager.featureGates"},
		{name: "tolerations_null", content: "controllerManager:\n  tolerations:\n", wantSubstr: "controllerManager.tolerations"},
		{name: "tolerations_string", content: "controllerManager:\n  tolerations: \"invalid\"\n", wantSubstr: "controllerManager.tolerations"},
		{name: "tolerations_map", content: "controllerManager:\n  tolerations:\n    key: foo\n", wantSubstr: "controllerManager.tolerations"},
		{name: "manager_config_null", content: "managerConfig: null\n", wantSubstr: "managerConfig in kueue.helm_values_path must be a YAML mapping"},
		{name: "manager_config_yaml_null", content: "managerConfig:\n  controllerManagerConfigYaml: null\n", wantSubstr: "must be a YAML string"},
		{name: "manager_config_as_nested_map_not_string", content: "managerConfig:\n  controllerManagerConfigYaml:\n    manageJobsWithoutQueueName: true\n", wantSubstr: "must be a YAML string"},
		{name: "manager_config_wrong_kind", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    apiVersion: v1\n    kind: ConfigMap\n", wantSubstr: "kind 'Configuration'"},
		{name: "manager_config_feature_gates_conflict", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    featureGates:\n      VisibilityOnDemand: true\n", wantSubstr: "featureGates cannot be set inside managerConfig.controllerManagerConfigYaml"},
		{name: "manager_config_invalid_section_type", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    integrations:\n      - batch/job\n", wantSubstr: "Nested configuration sections"},
		{name: "manager_config_invalid_gkc_type", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    controller:\n      groupKindConcurrency:\n        - Pod\n", wantSubstr: "Nested configuration sections"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.path
			if p == nil {
				p = writeTempValuesFile(t, tc.name+".yaml", tc.content)
			}
			_, errs := evalKueueHCL(t, parsed, kueueEvalInput{
				install:        true,
				version:        "0.19.6",
				helmValuesPath: p,
			})
			if len(errs) != 1 {
				t.Fatalf("got %d validation errors, want 1:\n%s", len(errs), strings.Join(errs, "\n"))
			}
			if !strings.Contains(errs[0], tc.wantSubstr) {
				t.Fatalf("expected error containing %q, got:\n%s", tc.wantSubstr, errs[0])
			}
		})
	}
}
