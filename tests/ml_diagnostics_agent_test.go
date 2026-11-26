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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
)

const defaultMLDiagnosticsAgentImage = "us-central1-docker.pkg.dev/hypercomputediagon-release/daemon-docker/mldiagnostics-daemon:latest"

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

func TestMLDiagnosticsAgentVariable(t *testing.T) {
	blocks := flattenBlocks(parseHCLFile(t, "../modules/scheduler/gke-cluster/variables.tf"))
	block := findBlockByLabel(t, blocks, "variable", "ml_diagnostics_agent")

	nullableVal, diags := findAttribute(t, []*hclsyntax.Block{block}, "variable", "ml_diagnostics_agent", "nullable").Value(nil)
	if diags.HasErrors() || nullableVal.True() {
		t.Fatalf("expected nullable = false, got %v (diags: %v)", nullableVal, diags)
	}

	defaultVal, diags := findAttribute(t, []*hclsyntax.Block{block}, "variable", "ml_diagnostics_agent", "default").Value(nil)
	if diags.HasErrors() || defaultVal.LengthInt() != 0 {
		t.Fatalf("expected empty object default = {}, got %v (diags: %v)", defaultVal, diags)
	}

	srcBytes, err := os.ReadFile("../modules/scheduler/gke-cluster/variables.tf")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(srcBytes), defaultMLDiagnosticsAgentImage) {
		t.Fatalf("expected default image %q in variables.tf", defaultMLDiagnosticsAgentImage)
	}

	condExpr := findValidationCondition(t, block)
	assertAgentImageValidation(t, condExpr)
}

func assertAgentImageValidation(t *testing.T, condExpr hclsyntax.Expression) {
	t.Helper()
	funcs := map[string]function.Function{"trimspace": stdlib.TrimSpaceFunc}
	cases := []struct {
		image string
		want  bool
	}{
		{image: defaultMLDiagnosticsAgentImage, want: true},
		{image: "us-central1-docker.pkg.dev/hypercomputediagon-release/daemon-docker/mldiagnostics-daemon:1.0.1", want: true},
		{image: "", want: false},
		{image: "   ", want: false},
	}
	for _, tc := range cases {
		ctx := &hcl.EvalContext{
			Functions: funcs,
			Variables: map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"ml_diagnostics_agent": cty.ObjectVal(map[string]cty.Value{
						"enable": cty.True,
						"image":  cty.StringVal(tc.image),
					}),
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

func extractAgentManifestArg(t *testing.T, mainBlocks []*hclsyntax.Block) hclsyntax.Expression {
	t.Helper()
	manifests := findAttribute(t, mainBlocks, "module", "kubectl_apply", "apply_manifests")
	call, ok := manifests.(*hclsyntax.FunctionCallExpr)
	if !ok || call.Name != "concat" {
		t.Fatal("expected kubectl_apply.apply_manifests to call concat()")
	}
	var agentArg hclsyntax.Expression
	for _, arg := range call.Args {
		if !exprReferencesVar(arg, "ml_diagnostics_agent") {
			continue
		}
		if agentArg != nil {
			t.Fatal("expected single apply_manifests concat argument for ml_diagnostics_agent")
		}
		agentArg = arg
	}
	if agentArg == nil {
		t.Fatal("missing ml_diagnostics_agent entry in kubectl_apply.apply_manifests")
	}
	return agentArg
}

func assertAgentManifestValue(t *testing.T, gotManifests cty.Value, enabled bool) {
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
	if img := item.GetAttr("template_vars").GetAttr("image").AsString(); img != defaultMLDiagnosticsAgentImage {
		t.Fatalf("template_vars.image = %q, want %q", img, defaultMLDiagnosticsAgentImage)
	}
}

func assertAgentScenario(t *testing.T, enabled, versionOK bool, minVer cty.Value, agentArg, checkCond, outMsg hcl.Expression) {
	t.Helper()
	ctx := &hcl.EvalContext{
		Functions: map[string]function.Function{"trimspace": stdlib.TrimSpaceFunc},
		Variables: map[string]cty.Value{
			"path":  cty.ObjectVal(map[string]cty.Value{"module": cty.StringVal(".")}),
			"local": cty.ObjectVal(map[string]cty.Value{"mldiagnostics_agent_wlm_minimum_version": minVer}),
			"module": cty.ObjectVal(map[string]cty.Value{
				"mldiagnostics_agent_version_check": cty.ObjectVal(map[string]cty.Value{
					"is_greater_than_or_equal": cty.BoolVal(versionOK),
				}),
			}),
			"var": cty.ObjectVal(map[string]cty.Value{
				"ml_diagnostics_agent": cty.ObjectVal(map[string]cty.Value{
					"enable": cty.BoolVal(enabled),
					"image":  cty.StringVal(defaultMLDiagnosticsAgentImage),
				}),
			}),
		},
	}

	gotManifests, diags := agentArg.Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	assertAgentManifestValue(t, gotManifests, enabled)

	gotCheck, diags := checkCond.Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if gotCheck.True() != (!enabled || versionOK) {
		t.Fatalf("version check = %t, want %t", gotCheck.True(), !enabled || versionOK)
	}

	gotMsg, diags := outMsg.Value(ctx)
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	if (gotMsg.AsString() != "") != enabled {
		t.Fatalf("mldiagnostics_agent_message non-empty = %t, want %t (%q)", gotMsg.AsString() != "", enabled, gotMsg.AsString())
	}
}

func TestMLDiagnosticsAgentManifestsAndCheck(t *testing.T) {
	mainBlocks := flattenBlocks(parseHCLFile(t, "../modules/scheduler/gke-cluster/main.tf"))
	outBlocks := flattenBlocks(parseHCLFile(t, "../modules/scheduler/gke-cluster/outputs.tf"))

	minVer, diags := findAttribute(t, mainBlocks, "locals", "", "mldiagnostics_agent_wlm_minimum_version").Value(nil)
	if diags.HasErrors() || minVer.AsString() != "1.36.4-gke.1247000" {
		t.Fatalf("mldiagnostics_agent_wlm_minimum_version = %v (diags: %v), want 1.36.4-gke.1247000", minVer, diags)
	}

	agentArg := extractAgentManifestArg(t, mainBlocks)
	checkBlock := findBlockByLabel(t, mainBlocks, "check", "ml_diagnostics_agent_gke_version")
	checkCond := findAttribute(t, checkBlock.Body.Blocks, "assert", "", "condition")
	outMsg := findAttribute(t, outBlocks, "locals", "", "mldiagnostics_agent_message")

	for _, enabled := range []bool{false, true} {
		for _, versionOK := range []bool{false, true} {
			t.Run(fmt.Sprintf("enable_%t_versionOK_%t", enabled, versionOK), func(t *testing.T) {
				assertAgentScenario(t, enabled, versionOK, minVer, agentArg, checkCond, outMsg)
			})
		}
	}
}

func renderDiagonDaemonDocs(t *testing.T, expr hclsyntax.Expression, img string) []json.RawMessage {
	t.Helper()
	ctx := &hcl.EvalContext{
		Functions: map[string]function.Function{"jsonencode": stdlib.JSONEncodeFunc},
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
		defaultMLDiagnosticsAgentImage,
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
