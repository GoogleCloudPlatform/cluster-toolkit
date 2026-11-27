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
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	ctyjson "github.com/zclconf/go-cty/cty/json"
	yamlv3 "gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

const kubectlApplyModuleDir = "../modules/management/kubectl-apply"

type kueueEvalInput struct {
	version                  string
	install                  bool
	installCertManager       bool
	enablePathways           bool
	enableDynamicSlicing     bool
	helmValuesPath           *string
	helmValuesTemplateVar    map[string]cty.Value
	rawHelmValuesTemplateVar *cty.Value
	waitForPodsReady         map[string]cty.Value
	controllerCPU            *string
	controllerMemory         *string
	controllerReplicas       *int64
}

type kueueParsedHCL struct {
	locals        map[string]hcl.Expression
	validations   []validationRule
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
	val, diags := expr.Value(&hcl.EvalContext{Variables: vars, Functions: kueuePureEvalFunctions()})
	if diags.HasErrors() {
		return cty.NilVal, diags
	}
	return val, nil
}

func kueuePureEvalFunctions() map[string]function.Function {
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

	return map[string]function.Function{
		"alltrue":    alltrueFn,
		"anytrue":    anytrueFn,
		"can":        tryfunc.CanFunc,
		"coalesce":   terraformCoalesceFunc,
		"compact":    stdlib.CompactFunc,
		"concat":     stdlib.ConcatFunc,
		"contains":   stdlib.ContainsFunc,
		"distinct":   stdlib.DistinctFunc,
		"endswith":   endswithFn,
		"flatten":    stdlib.FlattenFunc,
		"indent":     stdlib.IndentFunc,
		"join":       stdlib.JoinFunc,
		"jsonencode": stdlib.JSONEncodeFunc,
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
}

func kueueEvalFunctions() map[string]function.Function {
	funcs := kueuePureEvalFunctions()
	maps.Copy(funcs, fileSystemCtyFuncs())
	return funcs
}

func readKubectlApplyKueueHCL(t *testing.T) kueueParsedHCL {
	t.Helper()
	tfPaths, err := filepath.Glob(filepath.Join(kubectlApplyModuleDir, "*.tf"))
	if err != nil || len(tfPaths) == 0 {
		t.Fatalf("globbing %s/*.tf failed: paths=%v err=%v", kubectlApplyModuleDir, tfPaths, err)
	}
	slices.Sort(tfPaths)

	locals := make(map[string]hcl.Expression)
	var topBlocks []*hclsyntax.Block
	var allBlocks []*hclsyntax.Block
	for _, p := range tfPaths {
		body := parseHCLFile(t, p)
		topBlocks = append(topBlocks, body.Blocks...)
		allBlocks = append(allBlocks, flattenBlocks(body)...)
	}

	var validations, preconditions []validationRule
	for _, block := range topBlocks {
		if block.Type == "locals" {
			for name, attr := range block.Body.Attributes {
				locals[name] = attr.Expr
			}
		}
		if block.Type == "variable" && slices.Equal(block.Labels, []string{"kueue"}) {
			validations = append(validations, collectValidationRules(block.Body, "validation")...)
		}
		if block.Type == "resource" && slices.Equal(block.Labels, []string{"terraform_data", "kueue_validations"}) {
			preconditions = append(preconditions, collectValidationRules(block.Body, "precondition")...)
		}
	}

	return kueueParsedHCL{
		locals:        locals,
		validations:   validations,
		preconditions: preconditions,
		valuesYAML:    findBlockAttribute(t, allBlocks, "module", "values_yaml", "install_kueue"),
	}
}

func collectValidationRules(body *hclsyntax.Body, blockType string) []validationRule {
	var rules []validationRule
	for _, sub := range flattenBlocks(body) {
		if sub.Type == blockType {
			rules = append(rules, validationRule{
				condition:    sub.Body.Attributes["condition"].Expr,
				errorMessage: sub.Body.Attributes["error_message"].Expr,
			})
		}
	}
	return rules
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

func buildKueueTemplateVarsValue(in kueueEvalInput) cty.Value {
	if in.rawHelmValuesTemplateVar != nil {
		return *in.rawHelmValuesTemplateVar
	}
	if in.helmValuesTemplateVar == nil {
		return cty.NullVal(cty.DynamicPseudoType)
	}
	return cty.ObjectVal(in.helmValuesTemplateVar)
}

func buildKueueVarValue(in kueueEvalInput) cty.Value {
	ver := in.version
	if ver == "" {
		ver = "0.19.6"
	}
	wfpr := cty.NullVal(cty.DynamicPseudoType)
	if in.waitForPodsReady != nil {
		wfpr = cty.ObjectVal(in.waitForPodsReady)
	}
	cfgTmplVars := cty.NullVal(cty.DynamicPseudoType)
	if in.enablePathways || in.enableDynamicSlicing {
		cfgTmplVars = cty.ObjectVal(map[string]cty.Value{"accelerator_type": cty.StringVal("tpu-v5p-slice")})
	}
	topoMode := cty.NullVal(cty.String)
	machineType := cty.NullVal(cty.String)
	if in.enableDynamicSlicing {
		topoMode = cty.StringVal("PROVISION_ONLY")
		machineType = cty.StringVal("ct5lp-hightpu-4t")
	}
	return cty.ObjectVal(map[string]cty.Value{
		"version":                         cty.StringVal(ver),
		"install":                         cty.BoolVal(in.install),
		"config_path":                     cty.NullVal(cty.String),
		"config_template_vars":            cfgTmplVars,
		"helm_values_path":                optStringVal(in.helmValuesPath),
		"helm_values_template_vars":       buildKueueTemplateVarsValue(in),
		"enable_pathways_for_tpus":        cty.False,
		"enable_dynamic_slicing_for_tpus": cty.BoolVal(in.enableDynamicSlicing),
		"accelerator_topology_mode":       topoMode,
		"machine_type":                    machineType,
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
				"cert_manager":             cty.ObjectVal(map[string]cty.Value{"install": cty.BoolVal(in.installCertManager)}),
				"jobset":                   cty.ObjectVal(map[string]cty.Value{"install": cty.BoolVal(in.enableDynamicSlicing)}),
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

	evaluated := map[string]bool{}
	var evalLocal func(string)
	evalExpr := func(expr hcl.Expression) cty.Value {
		for _, tr := range expr.Variables() {
			if tr.RootName() == "local" && len(tr) >= 2 {
				if attr, ok := tr[1].(hcl.TraverseAttr); ok {
					evalLocal(attr.Name)
				}
			}
		}
		val, diags := expr.Value(ctx)
		if diags.HasErrors() {
			t.Fatalf("HCL evaluation failed: %v", diags)
		}
		return val
	}
	evalLocal = func(name string) {
		if evaluated[name] {
			return
		}
		expr, ok := parsed.locals[name]
		if !ok {
			t.Fatalf("missing local.%s in kubectl-apply", name)
		}
		evaluated[name] = true
		localMap[name] = evalExpr(expr)
		ctx.Variables["local"] = cty.ObjectVal(localMap)
	}

	failedMsgs := append(
		evaluateValidationRules(parsed.validations, evalExpr),
		evaluateValidationRules(parsed.preconditions, evalExpr)...,
	)

	valsVal := evalExpr(parsed.valuesYAML)
	var valuesYAML []string
	for it := valsVal.ElementIterator(); it.Next(); {
		_, elem := it.Element()
		valuesYAML = append(valuesYAML, elem.AsString())
	}
	return valuesYAML, failedMsgs
}

func evaluateValidationRules(rules []validationRule, evalExpr func(hcl.Expression) cty.Value) []string {
	var failedMsgs []string
	for _, r := range rules {
		if !evalExpr(r.condition).True() {
			failedMsgs = append(failedMsgs, evalExpr(r.errorMessage).AsString())
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

func layerTopLevelKeys(t *testing.T, raw string) []string {
	t.Helper()
	var m map[string]any
	if err := sigsyaml.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("unmarshal layer failed: %v\n%s", err, raw)
	}
	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)
	return keys
}

func TestKueueHelmValuesDefaultBackwardCompat(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	cpu, mem, replicas := "4", "8Gi", int64(2)
	wfpr := map[string]cty.Value{
		"timeout":          cty.StringVal("45m"),
		"recovery_timeout": cty.StringVal("5m"),
		"block_admission":  cty.False,
	}

	cases := []struct {
		name           string
		in             kueueEvalInput
		wantLayerKeys  [][]string
		wantReplicas   float64
		wantCPU        string
		wantMemory     string
		wantHasManager bool
	}{
		{
			name:          "0.17.1_default",
			in:            kueueEvalInput{install: true, version: "0.17.1"},
			wantLayerKeys: [][]string{{"controllerManager"}},
		},
		{
			name:          "0.17.1_controller_sizing",
			in:            kueueEvalInput{install: true, version: "0.17.1", controllerCPU: &cpu, controllerMemory: &mem, controllerReplicas: &replicas},
			wantLayerKeys: [][]string{{"controllerManager"}, {"controllerManager"}},
			wantReplicas:  2,
			wantCPU:       "4",
			wantMemory:    "8Gi",
		},
		{
			name:          "0.18.2_default",
			in:            kueueEvalInput{install: true, version: "0.18.2"},
			wantLayerKeys: [][]string{{"controllerManager"}, {"controllerManager"}},
		},
		{
			name:          "0.18.2_controller_sizing",
			in:            kueueEvalInput{install: true, version: "0.18.2", controllerCPU: &cpu, controllerMemory: &mem, controllerReplicas: &replicas},
			wantLayerKeys: [][]string{{"controllerManager"}, {"controllerManager"}},
			wantReplicas:  2,
			wantCPU:       "4",
			wantMemory:    "8Gi",
		},
		{
			name:          "0.19.6_default",
			in:            kueueEvalInput{install: true, version: "0.19.6"},
			wantLayerKeys: [][]string{{"controllerManager"}, {"controllerManager"}},
		},
		{
			name:           "0.19.6_pathways",
			in:             kueueEvalInput{install: true, version: "0.19.6", enablePathways: true},
			wantLayerKeys:  [][]string{{"controllerManager"}, {"managerConfig"}, {"controllerManager"}},
			wantHasManager: true,
		},
		{
			name:           "0.19.6_wait_for_pods_ready",
			in:             kueueEvalInput{install: true, version: "0.19.6", waitForPodsReady: wfpr},
			wantLayerKeys:  [][]string{{"controllerManager"}, {"managerConfig"}, {"controllerManager"}},
			wantHasManager: true,
		},
		{
			name:          "0.19.6_dynamic_slicing",
			in:            kueueEvalInput{install: true, version: "0.19.6", enableDynamicSlicing: true},
			wantLayerKeys: [][]string{{"controllerManager"}, {"controllerManager"}},
			wantReplicas:  3,
			wantCPU:       "16",
			wantMemory:    "64Gi",
		},
		{
			name:           "0.19.6_dynamic_slicing_and_pathways",
			in:             kueueEvalInput{install: true, version: "0.19.6", enableDynamicSlicing: true, enablePathways: true},
			wantLayerKeys:  [][]string{{"controllerManager"}, {"managerConfig"}, {"controllerManager"}},
			wantReplicas:   3,
			wantCPU:        "16",
			wantMemory:     "64Gi",
			wantHasManager: true,
		},
		{
			name:          "0.19.6_controller_sizing",
			in:            kueueEvalInput{install: true, version: "0.19.6", controllerCPU: &cpu, controllerMemory: &mem, controllerReplicas: &replicas},
			wantLayerKeys: [][]string{{"controllerManager"}, {"controllerManager"}},
			wantReplicas:  2,
			wantCPU:       "4",
			wantMemory:    "8Gi",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertDefaultBackwardCompatCase(t, parsed, tc.in, tc.wantLayerKeys, tc.wantReplicas, tc.wantCPU, tc.wantMemory, tc.wantHasManager)
		})
	}
}

func assertDefaultBackwardCompatCase(
	t *testing.T,
	parsed kueueParsedHCL,
	in kueueEvalInput,
	wantLayerKeys [][]string,
	wantReplicas float64,
	wantCPU string,
	wantMemory string,
	wantHasManager bool,
) {
	t.Helper()
	layers, errs := evalKueueHCL(t, parsed, in)
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors: %v", errs)
	}
	if len(layers) != len(wantLayerKeys) {
		t.Fatalf("got %d values_yaml layers, want %d:\n%v", len(layers), len(wantLayerKeys), layers)
	}
	for i, wantKeys := range wantLayerKeys {
		if gotKeys := layerTopLevelKeys(t, layers[i]); !slices.Equal(gotKeys, wantKeys) {
			t.Fatalf("layer %d top-level keys = %v, want %v:\n%s", i, gotKeys, wantKeys, layers[i])
		}
	}
	merged := simulateHelmValuesMerge(t, layers)
	if _, hasMC := merged["managerConfig"]; hasMC != wantHasManager {
		t.Fatalf("has managerConfig = %t, want %t: %#v", hasMC, wantHasManager, merged["managerConfig"])
	}
	gates := extractFeatureGateMap(t, merged)
	if !gates["TopologyAwareScheduling"] {
		t.Fatalf("expected TopologyAwareScheduling=true, got %#v", gates)
	}
	draEnabled, hasDRA := gates["KueueDRAIntegration"]
	if wantDRA := semverAtLeast(in.version, 18); hasDRA != wantDRA || draEnabled {
		t.Fatalf("KueueDRAIntegration on %s: present=%t enabled=%t, want present=%t enabled=false", in.version, hasDRA, draEnabled, wantDRA)
	}
	if wantReplicas > 0 {
		assertControllerSizing(t, merged, wantReplicas, wantCPU, wantMemory)
	}
}

func assertControllerSizing(t *testing.T, helmValues map[string]any, wantReplicas float64, wantCPU, wantMemory string) {
	t.Helper()
	cm := helmValues["controllerManager"].(map[string]any)
	res := cm["manager"].(map[string]any)["resources"].(map[string]any)
	reqs := res["requests"].(map[string]any)
	lims := res["limits"].(map[string]any)
	if cm["replicas"] != wantReplicas || reqs["cpu"] != wantCPU || reqs["memory"] != wantMemory || lims["cpu"] != wantCPU || lims["memory"] != wantMemory {
		t.Fatalf("unexpected controller sizing (want replicas=%v cpu=%s memory=%s): cm=%#v", wantReplicas, wantCPU, wantMemory, cm)
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

func TestKueueHelmValuesPathwaysPartialAndNullSections(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	tmplPath := writeTempValuesFile(t, "kueue-helm-values.yaml.tftpl", `
controllerManager:
  featureGates:
    - name: TopologyAwareScheduling
      enabled: false
managerConfig:
  controllerManagerConfigYaml: |
    metrics:
      enableClusterQueueResources: ${enable_cq_metrics}
    manageJobsWithoutQueueName: true
    managedJobsNamespaceSelector:
      matchExpressions:
        - key: kubernetes.io/metadata.name
          operator: NotIn
          values:
%{ for ns in system_namespaces ~}
            - ${ns}
%{ endfor ~}
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
			"excluded_ns":       cty.StringVal("kuberay-system"),
			"enable_cq_metrics": cty.True,
			"system_namespaces": cty.ListVal([]cty.Value{cty.StringVal("kube-system"), cty.StringVal("kueue-system")}),
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
	assertRenderedNamespaceSelectorAndPathwaysWfpr(t, cfg)

	// Setting sections to null in controllerManagerConfigYaml must omit the keys completely
	// (reverting waitForPodsReady to Kueue 0.19+ chart defaults, preserving the chart's default
	// managedJobsNamespaceSelector exclusion, and removing base sections like leaderElection).
	nullSectionsPath := writeTempValuesFile(t, "null-sections.yaml", `
managerConfig:
  controllerManagerConfigYaml: |
    waitForPodsReady: null
    managedJobsNamespaceSelector: null
    leaderElection: null
`)
	nullLayers, nullErrs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        "0.19.6",
		enablePathways: true,
		helmValuesPath: nullSectionsPath,
	})
	if len(nullErrs) > 0 {
		t.Fatalf("unexpected validation errors for null sections: %v", nullErrs)
	}
	nullCfg := decodeControllerManagerConfig(t, simulateHelmValuesMerge(t, nullLayers))
	for _, omittedKey := range []string{"waitForPodsReady", "managedJobsNamespaceSelector", "leaderElection"} {
		if val, exists := nullCfg[omittedKey]; exists {
			t.Fatalf("expected key %q to be absent when set to null, got key present with value %#v", omittedKey, val)
		}
	}
}

func assertRenderedNamespaceSelectorAndPathwaysWfpr(t *testing.T, cfg map[string]any) {
	t.Helper()
	if cfg["metrics"].(map[string]any)["enableClusterQueueResources"] != true {
		t.Fatalf("expected mixed-type bool template var enableClusterQueueResources=true, got %#v", cfg["metrics"])
	}
	exprs := cfg["managedJobsNamespaceSelector"].(map[string]any)["matchExpressions"].([]any)
	rawVals := exprs[0].(map[string]any)["values"].([]any)
	var gotNS []string
	for _, v := range rawVals {
		gotNS = append(gotNS, v.(string))
	}
	wantNS := []string{"kube-system", "kueue-system", "kuberay-system"}
	if !slices.Equal(gotNS, wantNS) {
		t.Fatalf("rendered managedJobsNamespaceSelector values = %v, want %v", gotNS, wantNS)
	}
	wfpr := cfg["waitForPodsReady"].(map[string]any)
	if wfpr["timeout"] != "30m" || wfpr["recoveryTimeout"] != "0s" || wfpr["requeuingStrategy"] == nil {
		t.Fatalf("expected Pathways waitForPodsReady defaults preserved alongside requeuingStrategy, got %#v", wfpr)
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

	// Plain .yaml with empty helm_values_template_vars = {} and literal ${...} in a comment must use file(), not templatefile()
	plainWithDollar := writeTempValuesFile(t, "plain-values.yaml", "# literal ${UNSET_SHELL_VAR} in comment\nenableCertManager: true\n")
	plainLayers, plainErrs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:               true,
		version:               "0.19.6",
		helmValuesPath:        plainWithDollar,
		helmValuesTemplateVar: map[string]cty.Value{},
	})
	if len(plainErrs) > 0 || simulateHelmValuesMerge(t, plainLayers)["enableCertManager"] != true {
		t.Fatalf("plain .yaml with empty template_vars failed: errs=%v", plainErrs)
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
controllerManager:
  featureGates:
    - name: VisibilityOnDemand
      enabled: true
managerConfig:
  controllerManagerConfigYaml: |
    manageJobsWithoutQueueName: true
`)
	versionCases := []versionBoundaryExpectation{
		{ver: "0.15.2", wantDRA: false, wantCohort: false, wantPod: false, wantRayService: false, wantKubeflowV1: true},
		{ver: "0.16.0", wantDRA: false, wantCohort: false, wantPod: true, wantRayService: false, wantKubeflowV1: true},
		{ver: "0.17.1", wantDRA: false, wantCohort: false, wantPod: true, wantRayService: true, wantKubeflowV1: true},
		{ver: "0.18.2", wantDRA: true, wantCohort: false, wantPod: true, wantRayService: true, wantKubeflowV1: true},
		{ver: "0.19.6", wantDRA: true, wantCohort: true, wantPod: true, wantRayService: true, wantKubeflowV1: false},
	}
	for _, tc := range versionCases {
		t.Run(tc.ver, func(t *testing.T) {
			assertVersionBoundaryDefaults(t, parsed, partialPath, tc)
		})
	}
}

type versionBoundaryExpectation struct {
	ver            string
	wantDRA        bool
	wantCohort     bool
	wantPod        bool
	wantRayService bool
	wantKubeflowV1 bool
}

func assertVersionBoundaryDefaults(t *testing.T, parsed kueueParsedHCL, path *string, want versionBoundaryExpectation) {
	t.Helper()
	layers, errs := evalKueueHCL(t, parsed, kueueEvalInput{
		install:        true,
		version:        want.ver,
		helmValuesPath: path,
	})
	if len(errs) > 0 {
		t.Fatalf("unexpected validation errors on %s: %v", want.ver, errs)
	}
	merged := simulateHelmValuesMerge(t, layers)
	assertVersionBoundaryGates(t, want.ver, extractFeatureGateMap(t, merged), want.wantDRA)

	cfg := decodeControllerManagerConfig(t, merged)
	gkc := cfg["controller"].(map[string]any)["groupKindConcurrency"].(map[string]any)
	cohortVal, hasCohort := gkc["Cohort.kueue.x-k8s.io"]
	if hasCohort != want.wantCohort || (want.wantCohort && cohortVal != float64(1)) {
		t.Fatalf("%s Cohort.kueue.x-k8s.io: present=%t val=%v, want present=%t val=1", want.ver, hasCohort, cohortVal, want.wantCohort)
	}
	assertVersionBoundaryFrameworks(t, cfg, want)
}

func assertVersionBoundaryGates(t *testing.T, ver string, gates map[string]bool, wantDRA bool) {
	t.Helper()
	if !gates["TopologyAwareScheduling"] || !gates["VisibilityOnDemand"] {
		t.Fatalf("expected TopologyAwareScheduling=true and VisibilityOnDemand=true on %s, got %#v", ver, gates)
	}
	draEnabled, hasDRA := gates["KueueDRAIntegration"]
	wantLen := 2
	if wantDRA {
		wantLen = 6
	}
	if len(gates) != wantLen || hasDRA != wantDRA || draEnabled {
		t.Fatalf("unexpected feature gates on %s (want len=%d hasDRA=%t): %#v", ver, wantLen, wantDRA, gates)
	}
}

func assertVersionBoundaryFrameworks(t *testing.T, cfg map[string]any, want versionBoundaryExpectation) {
	t.Helper()
	var fws []string
	for _, f := range cfg["integrations"].(map[string]any)["frameworks"].([]any) {
		fws = append(fws, f.(string))
	}
	if slices.Contains(fws, "pod") != want.wantPod {
		t.Fatalf("%s pod integration = %t, want %t (%v)", want.ver, !want.wantPod, want.wantPod, fws)
	}
	if slices.Contains(fws, "ray.io/rayservice") != want.wantRayService {
		t.Fatalf("%s rayservice = %t, want %t (%v)", want.ver, !want.wantRayService, want.wantRayService, fws)
	}
	if slices.Contains(fws, "kubeflow.org/pytorchjob") != want.wantKubeflowV1 {
		t.Fatalf("%s kubeflow.org/pytorchjob = %t, want %t (%v)", want.ver, !want.wantKubeflowV1, want.wantKubeflowV1, fws)
	}
}

func TestKueueHelmValuesValidationGuardrails(t *testing.T) {
	parsed := readKubectlApplyKueueHCL(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	nonMapTmplVars := cty.ListVal([]cty.Value{cty.StringVal("not-a-map")})

	cases := []struct {
		name               string
		path               *string
		content            string
		installCertManager bool
		rawTmplVars        *cty.Value
		wantSubstr         string
	}{
		{name: "missing_file", path: &missing, wantSubstr: "does not exist"},
		{name: "non_mapping_template_vars", content: "enablePrometheus: true\n", rawTmplVars: &nonMapTmplVars, wantSubstr: "kueue.helm_values_template_vars must be a mapping"},
		{name: "enable_cert_manager_same_module", content: "enableCertManager: true\n", installCertManager: true, wantSubstr: "cannot be combined with cert_manager.install = true"},
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
		{name: "manager_config_wrong_kind", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    apiVersion: config.kueue.x-k8s.io/v1beta2\n    kind: ConfigMap\n", wantSubstr: "kind 'Configuration'"},
		{name: "manager_config_queue_apiversion", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    apiVersion: kueue.x-k8s.io/v1beta2\n    kind: Configuration\n", wantSubstr: "apiVersion 'config.kueue.x-k8s.io/v1beta2'"},
		{name: "manager_config_v1beta1_apiversion", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    apiVersion: config.kueue.x-k8s.io/v1beta1\n    kind: Configuration\n", wantSubstr: "apiVersion 'config.kueue.x-k8s.io/v1beta2'"},
		{name: "manager_config_feature_gates_conflict", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    featureGates:\n      VisibilityOnDemand: true\n", wantSubstr: "featureGates cannot be set inside managerConfig.controllerManagerConfigYaml"},
		{name: "manager_config_invalid_section_type", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    integrations:\n      - batch/job\n", wantSubstr: "Nested configuration sections"},
		{name: "manager_config_invalid_wfpr_type", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    waitForPodsReady: \"30m\"\n", wantSubstr: "Nested configuration sections"},
		{name: "manager_config_invalid_gkc_type", content: "managerConfig:\n  controllerManagerConfigYaml: |\n    controller:\n      groupKindConcurrency:\n        - Pod\n", wantSubstr: "Nested configuration sections"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := tc.path
			if p == nil {
				p = writeTempValuesFile(t, tc.name+".yaml", tc.content)
			}
			_, errs := evalKueueHCL(t, parsed, kueueEvalInput{
				install:                  true,
				installCertManager:       tc.installCertManager,
				version:                  "0.19.6",
				helmValuesPath:           p,
				rawHelmValuesTemplateVar: tc.rawTmplVars,
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
