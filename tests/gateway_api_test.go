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
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Evaluate the module's actual expressions without provider downloads or tests
// embedded in user deployments. This covers behavior, not provider validation.
func TestGatewayAPIFlags(t *testing.T) {
	enabled, gateway, balancing, inference := readGatewayExpressions(t)
	for _, api := range []bool{false, true} {
		for _, extension := range []bool{false, true} {
			t.Run(fmt.Sprintf("gateway_%t_inference_%t", api, extension), func(t *testing.T) {
				ctx := &hcl.EvalContext{Variables: map[string]cty.Value{"var": cty.ObjectVal(map[string]cty.Value{
					"enable_gateway_api": cty.BoolVal(api), "enable_inference_gateway": cty.BoolVal(extension),
				})}}
				value, diag := enabled.Value(ctx)
				if diag.HasErrors() {
					t.Fatal(diag)
				}
				if value.True() != (api || extension) {
					t.Fatal("either flag must enable Gateway API")
				}
				ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{"enable_gateway_api": value})
				for name, expr := range map[string]hcl.Expression{"Gateway API": gateway, "HTTP load balancing": balancing, "Inference CRDs": inference} {
					result, diag := expr.Value(ctx)
					if diag.HasErrors() {
						t.Fatal(diag)
					}
					want := value.True()
					if name == "Inference CRDs" {
						want = extension
					}
					if (result.LengthInt() > 0) != want {
						t.Errorf("%s presence: got %d entries, want enabled=%t", name, result.LengthInt(), want)
					}
				}
			})
		}
	}
}

func readGatewayExpressions(t *testing.T) (hcl.Expression, hcl.Expression, hcl.Expression, hcl.Expression) {
	t.Helper()
	source, err := os.ReadFile("../modules/scheduler/gke-cluster/main.tf")
	if err != nil {
		t.Fatal(err)
	}
	file, diagnostics := hclsyntax.ParseConfig(source, "main.tf", hcl.InitialPos)
	if diagnostics.HasErrors() {
		t.Fatal(diagnostics)
	}
	blocks := flattenBlocks(file.Body.(*hclsyntax.Body))
	enabled := findAttribute(t, blocks, "locals", "", "enable_gateway_api")
	gateway := findAttribute(t, blocks, "dynamic", "gateway_api_config", "for_each")
	balancing := findAttribute(t, blocks, "dynamic", "http_load_balancing", "for_each")
	manifests := findAttribute(t, blocks, "module", "kubectl_apply", "apply_manifests")
	call, ok := manifests.(*hclsyntax.FunctionCallExpr)
	if !ok || call.Name != "concat" {
		t.Fatal("expected concatenated manifests")
	}
	inference, err := inferenceArgument(call.Args)
	if err != nil {
		t.Fatal(err)
	}
	return enabled, gateway, balancing, inference
}

func flattenBlocks(body *hclsyntax.Body) []*hclsyntax.Block {
	blocks := append([]*hclsyntax.Block{}, body.Blocks...)
	for _, block := range body.Blocks {
		blocks = append(blocks, flattenBlocks(block.Body)...)
	}
	return blocks
}

func findAttribute(t *testing.T, blocks []*hclsyntax.Block, kind, label, attribute string) hcl.Expression {
	t.Helper()
	for _, block := range blocks {
		if block.Type != kind {
			continue
		}
		if label != "" && (len(block.Labels) == 0 || block.Labels[0] != label) {
			continue
		}
		if attr := block.Body.Attributes[attribute]; attr != nil {
			return attr.Expr
		}
	}
	t.Fatalf("missing %s %s attribute %s", kind, label, attribute)
	return nil
}

func inferenceArgument(args []hclsyntax.Expression) (hcl.Expression, error) {
	var matches []hcl.Expression
	for _, arg := range args {
		for _, variable := range arg.Variables() {
			if len(variable) != 2 || variable.RootName() != "var" {
				continue
			}
			if attr, ok := variable[1].(hcl.TraverseAttr); ok && attr.Name == "enable_inference_gateway" {
				matches = append(matches, arg)
				break
			}
		}
	}
	if len(matches) != 1 {
		return nil, fmt.Errorf("expected one manifest argument referencing var.enable_inference_gateway, got %d", len(matches))
	}
	return matches[0], nil
}

func TestInferenceArgumentSelection(t *testing.T) {
	parse := func(source string) hclsyntax.Expression {
		t.Helper()
		expr, diagnostics := hclsyntax.ParseExpression([]byte(source), "test.hcl", hcl.InitialPos)
		if diagnostics.HasErrors() {
			t.Fatal(diagnostics)
		}
		return expr
	}
	inference := parse("var.enable_inference_gateway ? [1] : []")
	other := parse("var.other_manifests")
	for _, args := range [][]hclsyntax.Expression{{inference, other}, {other, inference}} {
		got, err := inferenceArgument(args)
		if err != nil || got != inference {
			t.Fatalf("selected %v, error %v", got, err)
		}
	}
	for _, args := range [][]hclsyntax.Expression{{other}, {inference, other, inference}} {
		if _, err := inferenceArgument(args); err == nil {
			t.Fatal("missing or ambiguous inference argument must fail")
		}
	}
}
