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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

const defaultMLDiagnosticsDaemonImage = "us-central1-docker.pkg.dev/hypercomputediagon-release/daemon-docker/mldiagnostics-daemon:latest"

func exprReferencesVar(expr hclsyntax.Expression, varName string) bool {
	for _, variable := range expr.Variables() {
		if len(variable) < 2 || variable.RootName() != "var" {
			continue
		}
		if attr, ok := variable[1].(hcl.TraverseAttr); ok && attr.Name == varName {
			return true
		}
	}
	return false
}

func ctyStartsWithFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "str", Type: cty.String},
			{Name: "prefix", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			return cty.BoolVal(strings.HasPrefix(args[0].AsString(), args[1].AsString())), nil
		},
	})
}

func ctyAllTrueFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "list", Type: cty.List(cty.Bool)},
		},
		Type: function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
			for it := args[0].ElementIterator(); it.Next(); {
				_, v := it.Element()
				if !v.True() {
					return cty.False, nil
				}
			}
			return cty.True, nil
		},
	})
}

func hclEvalFunctions() map[string]function.Function {
	return map[string]function.Function{
		"alltrue":    ctyAllTrueFunc(),
		"jsonencode": stdlib.JSONEncodeFunc,
		"length":     stdlib.LengthFunc,
		"replace":    stdlib.ReplaceFunc,
		"startswith": ctyStartsWithFunc(),
		"trimspace":  stdlib.TrimSpaceFunc,
	}
}

func findBlockByType(t *testing.T, blocks []*hclsyntax.Block, kind string) *hclsyntax.Block {
	t.Helper()
	for _, b := range blocks {
		if b.Type == kind {
			return b
		}
	}
	t.Fatalf("missing %s block", kind)
	return nil
}

func findBlockByLabel(t *testing.T, blocks []*hclsyntax.Block, kind, label string) *hclsyntax.Block {
	t.Helper()
	for _, b := range blocks {
		if b.Type == kind && len(b.Labels) > 0 && b.Labels[0] == label {
			return b
		}
	}
	t.Fatalf("missing %s %q", kind, label)
	return nil
}

func findTwoLabelBlock(t *testing.T, blocks []*hclsyntax.Block, kind, first, second string) *hclsyntax.Block {
	t.Helper()
	for _, b := range blocks {
		if b.Type == kind && len(b.Labels) == 2 && b.Labels[0] == first && b.Labels[1] == second {
			return b
		}
	}
	t.Fatalf("missing %s %q %q", kind, first, second)
	return nil
}

func findValidationCondition(t *testing.T, block *hclsyntax.Block) hclsyntax.Expression {
	t.Helper()
	for _, nested := range block.Body.Blocks {
		if nested.Type == "validation" && nested.Body.Attributes["condition"] != nil {
			return nested.Body.Attributes["condition"].Expr
		}
	}
	t.Fatalf("missing validation block on %s", strings.Join(block.Labels, " "))
	return nil
}

func findPreconditionExpr(t *testing.T, resBlock *hclsyntax.Block) hclsyntax.Expression {
	t.Helper()
	lifecycle := findBlockByType(t, resBlock.Body.Blocks, "lifecycle")
	precond := findBlockByType(t, lifecycle.Body.Blocks, "precondition")
	attr := precond.Body.Attributes["condition"]
	if attr == nil {
		t.Fatalf("missing precondition.condition in %v", resBlock.Labels)
	}
	return attr.Expr
}

func TestMLDiagnosticsDaemonVariables(t *testing.T) {
	blocks := flattenBlocks(parseHCLFile(t, "../modules/scheduler/gke-cluster/variables.tf"))

	enableBlock := findBlockByLabel(t, blocks, "variable", "enable_ml_diagnostics_daemon")
	enableNullable, diags := findAttribute(t, []*hclsyntax.Block{enableBlock}, "variable", "enable_ml_diagnostics_daemon", "nullable").Value(nil)
	if diags.HasErrors() || enableNullable.True() {
		t.Fatalf("enable_ml_diagnostics_daemon nullable = %v (diags: %v), want false", enableNullable, diags)
	}
	enableDefault, diags := findAttribute(t, []*hclsyntax.Block{enableBlock}, "variable", "enable_ml_diagnostics_daemon", "default").Value(nil)
	if diags.HasErrors() || enableDefault.True() {
		t.Fatalf("enable_ml_diagnostics_daemon default = %v (diags: %v), want false", enableDefault, diags)
	}

	imageBlock := findBlockByLabel(t, blocks, "variable", "ml_diagnostics_daemon_image")
	imageNullable, diags := findAttribute(t, []*hclsyntax.Block{imageBlock}, "variable", "ml_diagnostics_daemon_image", "nullable").Value(nil)
	if diags.HasErrors() || imageNullable.True() {
		t.Fatalf("ml_diagnostics_daemon_image nullable = %v (diags: %v), want false", imageNullable, diags)
	}
	imageDefault, diags := findAttribute(t, []*hclsyntax.Block{imageBlock}, "variable", "ml_diagnostics_daemon_image", "default").Value(nil)
	if diags.HasErrors() || imageDefault.AsString() != defaultMLDiagnosticsDaemonImage {
		t.Fatalf("ml_diagnostics_daemon_image default = %v (diags: %v), want %q", imageDefault, diags, defaultMLDiagnosticsDaemonImage)
	}

	assertDaemonImageValidation(t, findValidationCondition(t, imageBlock))
}

func assertDaemonImageValidation(t *testing.T, condExpr hclsyntax.Expression) {
	t.Helper()
	cases := []struct {
		image string
		want  bool
	}{
		{image: defaultMLDiagnosticsDaemonImage, want: true},
		{image: "us-central1-docker.pkg.dev/hypercomputediagon-release/daemon-docker/mldiagnostics-daemon:1.0.1", want: true},
		{image: "", want: false},
		{image: "   ", want: false},
	}
	for _, tc := range cases {
		ctx := &hcl.EvalContext{
			Functions: hclEvalFunctions(),
			Variables: map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"ml_diagnostics_daemon_image": cty.StringVal(tc.image),
				}),
			},
		}
		got, diags := condExpr.Value(ctx)
		if diags.HasErrors() {
			t.Fatalf("validation eval failed for %q: %v", tc.image, diags)
		}
		if got.True() != tc.want {
			t.Fatalf("validation for %q = %t, want %t", tc.image, got.True(), tc.want)
		}
	}
}

func extractDaemonManifestArg(t *testing.T, mainBlocks []*hclsyntax.Block) hclsyntax.Expression {
	t.Helper()
	manifests := findAttribute(t, mainBlocks, "module", "kubectl_apply", "apply_manifests")
	call, ok := manifests.(*hclsyntax.FunctionCallExpr)
	if !ok || call.Name != "concat" {
		t.Fatal("expected kubectl_apply.apply_manifests to call concat()")
	}
	var daemonArg hclsyntax.Expression
	for _, arg := range call.Args {
		if !exprReferencesVar(arg, "enable_ml_diagnostics_daemon") {
			continue
		}
		if daemonArg != nil {
			t.Fatal("expected single apply_manifests concat argument for enable_ml_diagnostics_daemon")
		}
		daemonArg = arg
	}
	if daemonArg == nil {
		t.Fatal("missing enable_ml_diagnostics_daemon entry in kubectl_apply.apply_manifests")
	}
	return daemonArg
}

func assertDaemonManifestValue(t *testing.T, gotManifests cty.Value, enabled bool) {
	t.Helper()
	if !enabled {
		if gotManifests.LengthInt() != 0 {
			t.Fatalf("got %d manifests when disabled, want 0", gotManifests.LengthInt())
		}
		return
	}
	if gotManifests.LengthInt() != 1 {
		t.Fatalf("got %d manifests when enabled, want 1", gotManifests.LengthInt())
	}
	item := gotManifests.Index(cty.NumberIntVal(0))
	if got := item.GetAttr("name").AsString(); got != "diagon-daemon" {
		t.Fatalf("manifest name = %q, want diagon-daemon", got)
	}
	if got := item.GetAttr("source").AsString(); got != "./templates/diagon-daemon.yaml.tftpl" {
		t.Fatalf("manifest source = %q, want ./templates/diagon-daemon.yaml.tftpl", got)
	}
	if waitRollout := item.GetAttr("wait_for_rollout"); waitRollout.True() {
		t.Fatal("expected wait_for_rollout = false on diagon-daemon manifest")
	}
	if img := item.GetAttr("template_vars").GetAttr("image").AsString(); img != defaultMLDiagnosticsDaemonImage {
		t.Fatalf("template_vars.image = %q, want %q", img, defaultMLDiagnosticsDaemonImage)
	}
}

func assertDaemonScenario(t *testing.T, daemonEnabled, diag10Enabled, versionOK bool, minVer cty.Value, exprs map[string]hcl.Expression) {
	t.Helper()
	ctx := &hcl.EvalContext{
		Functions: hclEvalFunctions(),
		Variables: map[string]cty.Value{
			"path":  cty.ObjectVal(map[string]cty.Value{"module": cty.StringVal(".")}),
			"local": cty.ObjectVal(map[string]cty.Value{"mldiagnostics_daemon_wlm_minimum_version": minVer}),
			"module": cty.ObjectVal(map[string]cty.Value{
				"mldiagnostics_daemon_version_check": cty.ObjectVal(map[string]cty.Value{
					"is_greater_than_or_equal": cty.BoolVal(versionOK),
				}),
			}),
			"var": cty.ObjectVal(map[string]cty.Value{
				"enable_ml_diagnostics":        cty.BoolVal(diag10Enabled),
				"enable_ml_diagnostics_daemon": cty.BoolVal(daemonEnabled),
				"ml_diagnostics_daemon_image":  cty.StringVal(defaultMLDiagnosticsDaemonImage),
			}),
		},
	}

	gotManifests, diags := exprs["manifests"].Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	assertDaemonManifestValue(t, gotManifests, daemonEnabled)

	gotCheck, diags := exprs["check"].Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if gotCheck.True() != (!daemonEnabled || versionOK) {
		t.Fatalf("version check = %t, want %t", gotCheck.True(), !daemonEnabled || versionOK)
	}

	gotPrecond, diags := exprs["precond"].Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if gotPrecond.True() != (!daemonEnabled || diag10Enabled) {
		t.Fatalf("enable_ml_diagnostics precondition = %t, want %t", gotPrecond.True(), !daemonEnabled || diag10Enabled)
	}

	gotMsg, diags := exprs["outMsg"].Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if (gotMsg.AsString() != "") != daemonEnabled {
		t.Fatalf("mldiagnostics_daemon_message non-empty = %t, want %t (%q)", gotMsg.AsString() != "", daemonEnabled, gotMsg.AsString())
	}
}

func TestMLDiagnosticsDaemonManifestsChecksAndPrecondition(t *testing.T) {
	mainBlocks := flattenBlocks(parseHCLFile(t, "../modules/scheduler/gke-cluster/main.tf"))
	outBlocks := flattenBlocks(parseHCLFile(t, "../modules/scheduler/gke-cluster/outputs.tf"))

	minVer, diags := findAttribute(t, mainBlocks, "locals", "", "mldiagnostics_daemon_wlm_minimum_version").Value(nil)
	if diags.HasErrors() || minVer.AsString() != "1.36.4-gke.1247000" {
		t.Fatalf("mldiagnostics_daemon_wlm_minimum_version = %v (diags: %v), want 1.36.4-gke.1247000", minVer, diags)
	}

	checkBlock := findBlockByLabel(t, mainBlocks, "check", "ml_diagnostics_daemon_gke_version")
	precondRes := findTwoLabelBlock(t, mainBlocks, "resource", "terraform_data", "validate_ml_diagnostics_daemon")
	exprs := map[string]hcl.Expression{
		"manifests": extractDaemonManifestArg(t, mainBlocks),
		"check":     findAttribute(t, checkBlock.Body.Blocks, "assert", "", "condition"),
		"precond":   findPreconditionExpr(t, precondRes),
		"outMsg":    findAttribute(t, outBlocks, "locals", "", "mldiagnostics_daemon_message"),
	}

	for _, daemonEnabled := range []bool{false, true} {
		for _, diag10Enabled := range []bool{false, true} {
			for _, versionOK := range []bool{false, true} {
				name := fmt.Sprintf("daemon_%t_diag10_%t_verOK_%t", daemonEnabled, diag10Enabled, versionOK)
				t.Run(name, func(t *testing.T) {
					assertDaemonScenario(t, daemonEnabled, diag10Enabled, versionOK, minVer, exprs)
				})
			}
		}
	}
}

func TestServiceAccountConditionalProjectRoles(t *testing.T) {
	varBlocks := flattenBlocks(parseHCLFile(t, "../modules/project/service-account/variables.tf"))
	mainBlocks := flattenBlocks(parseHCLFile(t, "../modules/project/service-account/main.tf"))

	condVar := findBlockByLabel(t, varBlocks, "variable", "conditional_project_roles")
	defaultVal, diags := findAttribute(t, []*hclsyntax.Block{condVar}, "variable", "conditional_project_roles", "default").Value(nil)
	if diags.HasErrors() || defaultVal.LengthInt() != 0 {
		t.Fatalf("conditional_project_roles default = %v (diags: %v), want []", defaultVal, diags)
	}

	valExpr := findValidationCondition(t, condVar)
	assertConditionalRolesValidation(t, valExpr)

	enabledLocal := findAttribute(t, mainBlocks, "locals", "", "enabled_conditional_roles")
	projCountExpr := findAttribute(t, mainBlocks, "data", "google_project", "count")
	iamBlock := findTwoLabelBlock(t, mainBlocks, "resource", "google_project_iam_member", "conditional_project_roles")
	condBlock := findBlockByType(t, iamBlock.Body.Blocks, "condition")
	roleAttr := iamBlock.Body.Attributes["role"].Expr
	exprAttr := condBlock.Body.Attributes["expression"].Expr

	assertConditionalRolesEvaluation(t, enabledLocal, projCountExpr, roleAttr, exprAttr)
}

func assertConditionalRolesValidation(t *testing.T, valExpr hclsyntax.Expression) {
	t.Helper()
	roleObj := func(role, title, expr string) cty.Value {
		return cty.ObjectVal(map[string]cty.Value{
			"enable":      cty.True,
			"role":        cty.StringVal(role),
			"title":       cty.StringVal(title),
			"description": cty.StringVal(""),
			"expression":  cty.StringVal(expr),
		})
	}
	cases := []struct {
		name string
		val  cty.Value
		want bool
	}{
		{name: "empty", val: cty.ListValEmpty(roleObj("a", "b", "c").Type()), want: true},
		{name: "valid", val: cty.ListVal([]cty.Value{roleObj("storage.objectUser", "diagon", "expr")}), want: true},
		{name: "blank_role", val: cty.ListVal([]cty.Value{roleObj(" ", "diagon", "expr")}), want: false},
		{name: "blank_expr", val: cty.ListVal([]cty.Value{roleObj("storage.objectUser", "diagon", "")}), want: false},
	}
	for _, tc := range cases {
		ctx := &hcl.EvalContext{
			Functions: hclEvalFunctions(),
			Variables: map[string]cty.Value{"var": cty.ObjectVal(map[string]cty.Value{"conditional_project_roles": tc.val})},
		}
		got, diags := valExpr.Value(ctx)
		if diags.HasErrors() || got.True() != tc.want {
			t.Fatalf("validation case %s = %v (diags: %v), want %t", tc.name, got, diags, tc.want)
		}
	}
}

func assertConditionalRolesEvaluation(t *testing.T, enabledLocal, projCountExpr, roleAttr, exprAttr hcl.Expression) {
	t.Helper()
	rolesInput := cty.TupleVal([]cty.Value{
		cty.ObjectVal(map[string]cty.Value{
			"enable":      cty.False,
			"role":        cty.StringVal("storage.objectUser"),
			"title":       cty.StringVal("disabled_role"),
			"description": cty.StringVal(""),
			"expression":  cty.StringVal(`resource.name.startsWith("projects/_/buckets/diagon-{project_number}-")`),
		}),
		cty.ObjectVal(map[string]cty.Value{
			"enable":      cty.True,
			"role":        cty.StringVal("storage.bucketViewer"),
			"title":       cty.StringVal("diagon_bucket_viewer"),
			"description": cty.StringVal("Bucket metadata read"),
			"expression":  cty.StringVal(`resource.name.startsWith("projects/_/buckets/diagon-{project_number}-")`),
		}),
	})
	ctx := &hcl.EvalContext{
		Functions: hclEvalFunctions(),
		Variables: map[string]cty.Value{"var": cty.ObjectVal(map[string]cty.Value{"conditional_project_roles": rolesInput})},
	}
	enabledVal, diags := enabledLocal.Value(ctx)
	if diags.HasErrors() || enabledVal.LengthInt() != 1 {
		t.Fatalf("enabled_conditional_roles = %v (diags: %v), want length 1", enabledVal, diags)
	}

	ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{"enabled_conditional_roles": enabledVal})
	countVal, diags := projCountExpr.Value(ctx)
	if diags.HasErrors() || countVal.AsBigFloat().String() != "1" {
		t.Fatalf("data.google_project.project count = %v (diags: %v), want 1", countVal, diags)
	}

	entry := enabledVal.GetAttr("1-storage.bucketViewer-diagon_bucket_viewer")
	ctx.Variables["each"] = cty.ObjectVal(map[string]cty.Value{"value": entry})
	ctx.Variables["data"] = cty.ObjectVal(map[string]cty.Value{
		"google_project": cty.ObjectVal(map[string]cty.Value{
			"project": cty.TupleVal([]cty.Value{cty.ObjectVal(map[string]cty.Value{"number": cty.StringVal("123456789012")})}),
		}),
	})

	roleVal, diags := roleAttr.Value(ctx)
	if diags.HasErrors() || roleVal.AsString() != "roles/storage.bucketViewer" {
		t.Fatalf("normalized role = %v (diags: %v), want roles/storage.bucketViewer", roleVal, diags)
	}
	exprVal, diags := exprAttr.Value(ctx)
	wantExpr := `resource.name.startsWith("projects/_/buckets/diagon-123456789012-")`
	if diags.HasErrors() || exprVal.AsString() != wantExpr {
		t.Fatalf("substituted expression = %v (diags: %v), want %q", exprVal, diags, wantExpr)
	}
}

func TestGKETPU7xBlueprintDaemonOptIn(t *testing.T) {
	data, err := os.ReadFile("../examples/gke-tpu-7x/gke-tpu-7x.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var bp struct {
		Vars             map[string]any `yaml:"vars"`
		DeploymentGroups []struct {
			Modules []struct {
				ID       string         `yaml:"id"`
				Settings map[string]any `yaml:"settings"`
			} `yaml:"modules"`
		} `yaml:"deployment_groups"`
	}
	if err := yaml.Unmarshal(data, &bp); err != nil {
		t.Fatal(err)
	}
	if got, ok := bp.Vars["enable_ml_diagnostics_daemon"].(bool); !ok || got {
		t.Fatalf("vars.enable_ml_diagnostics_daemon = %#v, want false", bp.Vars["enable_ml_diagnostics_daemon"])
	}

	settingsByID := map[string]map[string]any{}
	for _, g := range bp.DeploymentGroups {
		for _, m := range g.Modules {
			settingsByID[m.ID] = m.Settings
		}
	}
	if got := settingsByID["gke-tpu-7x-cluster"]["enable_ml_diagnostics_daemon"]; got != "$(vars.enable_ml_diagnostics_daemon)" {
		t.Fatalf("gke-tpu-7x-cluster.enable_ml_diagnostics_daemon = %#v", got)
	}
	condRoles, ok := settingsByID["node_pool_service_account"]["conditional_project_roles"].([]any)
	if !ok || len(condRoles) != 2 {
		t.Fatalf("node_pool_service_account.conditional_project_roles = %#v, want 2 entries", settingsByID["node_pool_service_account"]["conditional_project_roles"])
	}
}

func renderDiagonDaemonDocs(t *testing.T, expr hclsyntax.Expression, img string) []json.RawMessage {
	t.Helper()
	ctx := &hcl.EvalContext{
		Functions: hclEvalFunctions(),
		Variables: map[string]cty.Value{"image": cty.StringVal(img)},
	}
	val, diags := expr.Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	decoder := k8syaml.NewYAMLOrJSONDecoder(strings.NewReader(val.AsString()), 4096)
	var docs []json.RawMessage
	for {
		var raw json.RawMessage
		err := decoder.Decode(&raw)
		if err == io.EOF {
			return docs
		}
		if err != nil {
			t.Fatalf("failed to decode YAML document: %v", err)
		}
		docs = append(docs, raw)
	}
}

func decodeK8sDoc[T any](t *testing.T, raw json.RawMessage, wantKind string) T {
	t.Helper()
	var meta metav1.TypeMeta
	if err := json.Unmarshal(raw, &meta); err != nil || meta.Kind != wantKind {
		t.Fatalf("doc kind = %q (err=%v), want %s", meta.Kind, err, wantKind)
	}
	var obj T
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatal(err)
	}
	return obj
}

func assertDiagonNamespace(t *testing.T, raw json.RawMessage) {
	t.Helper()
	ns := decodeK8sDoc[corev1.Namespace](t, raw, "Namespace")
	if ns.Name != "diagon" {
		t.Fatalf("Namespace name = %q, want diagon", ns.Name)
	}
	if ns.Labels["pod-security.kubernetes.io/enforce"] != "privileged" {
		t.Fatalf("Namespace PSS label = %q, want privileged", ns.Labels["pod-security.kubernetes.io/enforce"])
	}
}

func assertDiagonServiceAccount(t *testing.T, raw json.RawMessage) {
	t.Helper()
	sa := decodeK8sDoc[corev1.ServiceAccount](t, raw, "ServiceAccount")
	if sa.Name != "diagon-daemon-sa" || sa.Namespace != "diagon" {
		t.Fatalf("ServiceAccount = %s/%s, want diagon/diagon-daemon-sa", sa.Namespace, sa.Name)
	}
}

func assertDiagonNodeAffinity(t *testing.T, affinity *corev1.Affinity) {
	t.Helper()
	if affinity == nil || affinity.NodeAffinity == nil ||
		affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution == nil {
		t.Fatal("missing requiredDuringSchedulingIgnoredDuringExecution nodeAffinity")
	}
	terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
	wantTerms := []corev1.NodeSelectorTerm{
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{Key: "kubernetes.io/arch", Operator: corev1.NodeSelectorOpIn, Values: []string{"amd64"}},
				{Key: "cloud.google.com/gke-tpu-accelerator", Operator: corev1.NodeSelectorOpExists},
			},
		},
		{
			MatchExpressions: []corev1.NodeSelectorRequirement{
				{Key: "kubernetes.io/arch", Operator: corev1.NodeSelectorOpIn, Values: []string{"amd64"}},
				{Key: "cloud.google.com/gke-accelerator", Operator: corev1.NodeSelectorOpExists},
			},
		},
	}
	if !reflect.DeepEqual(terms, wantTerms) {
		t.Fatalf("nodeSelectorTerms mismatch:\ngot:  %+v\nwant: %+v", terms, wantTerms)
	}
}

func assertDiagonContainer(t *testing.T, containers []corev1.Container, wantImage string) {
	t.Helper()
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	c := containers[0]
	if c.Name != "diagon-daemon" || c.Image != wantImage || c.ImagePullPolicy != corev1.PullAlways {
		t.Fatalf("unexpected container metadata: name=%q image=%q pullPolicy=%q", c.Name, c.Image, c.ImagePullPolicy)
	}
	if c.SecurityContext == nil || c.SecurityContext.Privileged == nil || !*c.SecurityContext.Privileged {
		t.Fatal("expected container securityContext.privileged=true")
	}
	wantCmd := []string{
		"/mldiagnostics_extension_files/bin/mldiagnostics_extension",
		"-standalone=true",
		"-channel=hypercomputecluster.googleapis.com/diagnostics",
		"-alsologtostderr=true",
	}
	if !reflect.DeepEqual(c.Command, wantCmd) {
		t.Fatalf("container command mismatch: got %v, want %v", c.Command, wantCmd)
	}
	assertDiagonResources(t, c.Resources)
}

func assertDiagonResources(t *testing.T, res corev1.ResourceRequirements) {
	t.Helper()
	if !res.Requests.Cpu().Equal(resource.MustParse("10m")) {
		t.Fatalf("cpu request = %s, want 10m", res.Requests.Cpu().String())
	}
	if !res.Requests.Memory().Equal(resource.MustParse("32Mi")) {
		t.Fatalf("memory request = %s, want 32Mi", res.Requests.Memory().String())
	}
	if !res.Limits.Memory().Equal(resource.MustParse("256Mi")) {
		t.Fatalf("memory limit = %s, want 256Mi", res.Limits.Memory().String())
	}
	if _, hasCPULimit := res.Limits[corev1.ResourceCPU]; hasCPULimit {
		t.Fatal("expected no CPU limit on diagon-daemon container")
	}
}

func assertDiagonDaemonSet(t *testing.T, raw json.RawMessage, wantImage string) {
	t.Helper()
	ds := decodeK8sDoc[appsv1.DaemonSet](t, raw, "DaemonSet")
	if ds.Name != "diagon-daemon" || ds.Namespace != "diagon" {
		t.Fatalf("DaemonSet = %s/%s, want diagon/diagon-daemon", ds.Namespace, ds.Name)
	}
	podSpec := ds.Spec.Template.Spec
	if !podSpec.HostNetwork || podSpec.DNSPolicy != corev1.DNSClusterFirstWithHostNet {
		t.Fatalf("expected hostNetwork=true and ClusterFirstWithHostNet, got hostNetwork=%t dnsPolicy=%s", podSpec.HostNetwork, podSpec.DNSPolicy)
	}
	if podSpec.ServiceAccountName != "diagon-daemon-sa" {
		t.Fatalf("serviceAccountName = %q, want diagon-daemon-sa", podSpec.ServiceAccountName)
	}
	if podSpec.TerminationGracePeriodSeconds == nil || *podSpec.TerminationGracePeriodSeconds != 30 {
		t.Fatalf("terminationGracePeriodSeconds = %v, want 30", podSpec.TerminationGracePeriodSeconds)
	}
	if len(podSpec.Tolerations) != 2 {
		t.Fatalf("expected 2 tolerations, got %d", len(podSpec.Tolerations))
	}
	assertDiagonNodeAffinity(t, podSpec.Affinity)
	assertDiagonContainer(t, podSpec.Containers, wantImage)
}

func TestDiagonDaemonTemplateYAML(t *testing.T) {
	tmplBytes, err := os.ReadFile("../modules/scheduler/gke-cluster/templates/diagon-daemon.yaml.tftpl")
	if err != nil {
		t.Fatal(err)
	}
	expr, diags := hclsyntax.ParseTemplate(tmplBytes, "diagon-daemon.yaml.tftpl", hcl.InitialPos)
	if diags.HasErrors() {
		t.Fatal(diags)
	}

	images := []string{
		defaultMLDiagnosticsDaemonImage,
		"us-central1-docker.pkg.dev/hypercomputediagon-release/daemon-docker/mldiagnostics-daemon:1.0.1",
	}
	for _, img := range images {
		t.Run(img, func(t *testing.T) {
			docs := renderDiagonDaemonDocs(t, expr, img)
			if len(docs) != 3 {
				t.Fatalf("decoded %d Kubernetes documents, want 3", len(docs))
			}
			assertDiagonNamespace(t, docs[0])
			assertDiagonServiceAccount(t, docs[1])
			assertDiagonDaemonSet(t, docs[2], img)
		})
	}
}
