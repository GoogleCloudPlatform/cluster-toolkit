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
	"os"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// terraformCoalesceFunc wraps stdlib.CoalesceFunc to match Terraform's coalesce()
// semantics, which skip both null and empty string ("") arguments.
var terraformCoalesceFunc = function.New(&function.Spec{
	VarParam: stdlib.CoalesceFunc.VarParam(),
	Type:     stdlib.CoalesceFunc.ReturnTypeForValues,
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		normalized := make([]cty.Value, len(args))
		for i, arg := range args {
			if arg.IsKnown() && !arg.IsNull() && arg.Type() == cty.String && arg.AsString() == "" {
				normalized[i] = cty.NullVal(cty.String)
			} else {
				normalized[i] = arg
			}
		}
		return stdlib.CoalesceFunc.Call(normalized)
	},
})

type validationRule struct {
	condition    hcl.Expression
	errorMessage hcl.Expression
}

type firewallRulesExprs struct {
	useSubnetworkDefault  hcl.Expression
	useSubnetworkNullable hcl.Expression
	useSubnetworkValid    []validationRule
	subnetworkDefault     hcl.Expression
	projectDefault        hcl.Expression
	networkDefault        hcl.Expression
	ingressDefault        hcl.Expression
	egressDefault         hcl.Expression

	dataCount      hcl.Expression
	dataSelfLink   hcl.Expression
	pgaCount       hcl.Expression
	pgaCondition   hcl.Expression
	pgaErrorMsg    hcl.Expression
	derivedProject hcl.Expression
	derivedNetwork hcl.Expression
	effectiveProj  hcl.Expression
	effectiveNet   hcl.Expression
	modProject     hcl.Expression
	modNetwork     hcl.Expression
	modIngress     hcl.Expression
	modEgress      hcl.Expression
}

func parseHCLFile(t *testing.T, path, name string) *hclsyntax.Body {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, diag := hclsyntax.ParseConfig(src, name, hcl.InitialPos)
	if diag.HasErrors() {
		t.Fatal(diag)
	}
	return file.Body.(*hclsyntax.Body)
}

func extractValidations(t *testing.T, body *hclsyntax.Body, varName string) []validationRule {
	t.Helper()
	var validations []validationRule
	for _, block := range body.Blocks {
		if block.Type != "variable" || len(block.Labels) == 0 || block.Labels[0] != varName {
			continue
		}
		for _, sub := range block.Body.Blocks {
			if sub.Type != "validation" {
				continue
			}
			cond, msg := sub.Body.Attributes["condition"], sub.Body.Attributes["error_message"]
			if cond == nil || msg == nil {
				t.Fatal("validation block missing condition or error_message attribute")
			}
			validations = append(validations, validationRule{
				condition:    cond.Expr,
				errorMessage: msg.Expr,
			})
		}
	}
	return validations
}

func readFirewallRulesExpressions(t *testing.T) firewallRulesExprs {
	t.Helper()
	varBody := parseHCLFile(t, "../modules/network/firewall-rules/variables.tf", "variables.tf")
	varBlocks := flattenBlocks(varBody)
	mainBody := parseHCLFile(t, "../modules/network/firewall-rules/main.tf", "main.tf")
	mainBlocks := flattenBlocks(mainBody)

	return firewallRulesExprs{
		useSubnetworkDefault:  findAttribute(t, varBlocks, "variable", "use_subnetwork_data", "default"),
		useSubnetworkNullable: findAttribute(t, varBlocks, "variable", "use_subnetwork_data", "nullable"),
		useSubnetworkValid:    extractValidations(t, varBody, "use_subnetwork_data"),
		subnetworkDefault:     findAttribute(t, varBlocks, "variable", "subnetwork_self_link", "default"),
		projectDefault:        findAttribute(t, varBlocks, "variable", "project_id", "default"),
		networkDefault:        findAttribute(t, varBlocks, "variable", "network_name", "default"),
		ingressDefault:        findAttribute(t, varBlocks, "variable", "ingress_rules", "default"),
		egressDefault:         findAttribute(t, varBlocks, "variable", "egress_rules", "default"),

		dataCount:      findAttribute(t, mainBlocks, "data", "google_compute_subnetwork", "count"),
		dataSelfLink:   findAttribute(t, mainBlocks, "data", "google_compute_subnetwork", "self_link"),
		pgaCount:       findAttribute(t, mainBlocks, "resource", "terraform_data", "count"),
		pgaCondition:   findAttribute(t, mainBlocks, "precondition", "", "condition"),
		pgaErrorMsg:    findAttribute(t, mainBlocks, "precondition", "", "error_message"),
		derivedProject: findAttribute(t, mainBlocks, "locals", "", "derived_project_id"),
		derivedNetwork: findAttribute(t, mainBlocks, "locals", "", "derived_network_name"),
		effectiveProj:  findAttribute(t, mainBlocks, "locals", "", "effective_project_id"),
		effectiveNet:   findAttribute(t, mainBlocks, "locals", "", "effective_network_name"),
		modProject:     findAttribute(t, mainBlocks, "module", "firewall_rule", "project_id"),
		modNetwork:     findAttribute(t, mainBlocks, "module", "firewall_rule", "network_name"),
		modIngress:     findAttribute(t, mainBlocks, "module", "firewall_rule", "ingress_rules"),
		modEgress:      findAttribute(t, mainBlocks, "module", "firewall_rule", "egress_rules"),
	}
}

func evalExpr(t *testing.T, expr hcl.Expression, ctx *hcl.EvalContext) cty.Value {
	t.Helper()
	val, diag := expr.Value(ctx)
	if diag.HasErrors() {
		t.Fatal(diag)
	}
	return val
}

func assertUnknownString(t *testing.T, label string, expr hcl.Expression, ctx *hcl.EvalContext) cty.Value {
	t.Helper()
	val := evalExpr(t, expr, ctx)
	if val.IsKnown() || val.Type() != cty.String {
		t.Fatalf("%s = %#v, want unknown string", label, val)
	}
	return val
}

func assertKnownCountsAndDeferredValidations(t *testing.T, exprs firewallRulesExprs, ctx *hcl.EvalContext, wantCount int64) {
	t.Helper()
	want := cty.NumberIntVal(wantCount)
	for label, countExpr := range map[string]hcl.Expression{
		"data.google_compute_subnetwork.subnetwork": exprs.dataCount,
		"resource.terraform_data.pga_check":         exprs.pgaCount,
	} {
		val := evalExpr(t, countExpr, ctx)
		if !val.IsKnown() || val.IsNull() || !val.RawEquals(want) {
			t.Fatalf("%s count = %#v, want known %d", label, val, wantCount)
		}
	}

	hasUnknownValidation := false
	for i, rule := range exprs.useSubnetworkValid {
		val := evalExpr(t, rule.condition, ctx)
		if val.IsKnown() && !val.True() {
			t.Fatalf("validation[%d] unexpectedly failed at plan time with unknown inputs", i)
		}
		if !val.IsKnown() {
			hasUnknownValidation = true
		}
	}
	if !hasUnknownValidation {
		t.Fatal("expected active validation condition to remain unknown (deferred to apply)")
	}
}

func assertNonEmptyStringExpr(t *testing.T, label string, expr hcl.Expression) {
	t.Helper()
	val := evalExpr(t, expr, nil)
	if !val.IsKnown() || val.IsNull() || val.Type() != cty.String || val.AsString() == "" {
		t.Errorf("%s = %#v, want non-empty string", label, val)
	}
}

func TestFirewallRulesVariableDefaultsAndSchema(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)

	if defVal := evalExpr(t, exprs.useSubnetworkDefault, nil); defVal != cty.True {
		t.Fatalf("use_subnetwork_data default = %#v, want true", defVal)
	}
	if nullableVal := evalExpr(t, exprs.useSubnetworkNullable, nil); nullableVal != cty.False {
		t.Fatalf("use_subnetwork_data nullable = %#v, want false (explicit null must fall back to default true)", nullableVal)
	}
	if len(exprs.useSubnetworkValid) != 2 {
		t.Fatalf("use_subnetwork_data validation blocks = %d, want 2", len(exprs.useSubnetworkValid))
	}
	for _, rule := range exprs.useSubnetworkValid {
		assertNonEmptyStringExpr(t, "validation error_message", rule.errorMessage)
	}

	for name, expr := range map[string]hcl.Expression{
		"subnetwork_self_link": exprs.subnetworkDefault,
		"project_id":           exprs.projectDefault,
		"network_name":         exprs.networkDefault,
	} {
		if val := evalExpr(t, expr, nil); !val.IsNull() {
			t.Errorf("%s default = %#v, want null", name, val)
		}
	}

	for name, expr := range map[string]hcl.Expression{
		"ingress_rules": exprs.ingressDefault,
		"egress_rules":  exprs.egressDefault,
	} {
		if val := evalExpr(t, expr, nil); !val.IsKnown() || val.IsNull() || val.LengthInt() != 0 {
			t.Errorf("%s default = %#v, want empty list/tuple", name, val)
		}
	}
}

func TestFirewallRulesInputValidations(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)

	nullStr := cty.NullVal(cty.String)
	emptyStr := cty.StringVal("")
	validSubnet := cty.StringVal("https://www.googleapis.com/compute/v1/projects/test-project/regions/us-central1/subnetworks/test-subnet")
	validProject := cty.StringVal("explicit-project")
	validNetwork := cty.StringVal("explicit-network")

	cases := []struct {
		name              string
		useSubnetworkData bool
		subnetworkLink    cty.Value
		projectID         cty.Value
		networkName       cty.Value
		wantPass          bool
	}{
		{"lookup_enabled_with_subnet_only", true, validSubnet, nullStr, nullStr, true},
		{"lookup_enabled_with_subnet_and_explicit_overrides", true, validSubnet, validProject, validNetwork, true},
		{"lookup_enabled_missing_subnet_null", true, nullStr, validProject, validNetwork, false},
		{"lookup_enabled_empty_subnet", true, emptyStr, validProject, validNetwork, false},
		{"lookup_disabled_with_project_and_network_no_subnet", false, nullStr, validProject, validNetwork, true},
		{"lookup_disabled_with_project_network_and_autowired_subnet", false, validSubnet, validProject, validNetwork, true},
		{"lookup_disabled_missing_project_null", false, nullStr, nullStr, validNetwork, false},
		{"lookup_disabled_with_subnet_but_missing_project", false, validSubnet, nullStr, validNetwork, false},
		{"lookup_disabled_empty_project", false, nullStr, emptyStr, validNetwork, false},
		{"lookup_disabled_missing_network_null", false, nullStr, validProject, nullStr, false},
		{"lookup_disabled_with_subnet_but_missing_network", false, validSubnet, validProject, emptyStr, false},
		{"lookup_disabled_empty_network", false, nullStr, validProject, emptyStr, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"var": cty.ObjectVal(map[string]cty.Value{
						"use_subnetwork_data":  cty.BoolVal(tc.useSubnetworkData),
						"subnetwork_self_link": tc.subnetworkLink,
						"project_id":           tc.projectID,
						"network_name":         tc.networkName,
					}),
				},
			}
			passed := true
			for _, rule := range exprs.useSubnetworkValid {
				if val := evalExpr(t, rule.condition, ctx); !val.True() {
					passed = false
				}
			}
			if passed != tc.wantPass {
				t.Fatalf("validation passed = %t, want %t", passed, tc.wantPass)
			}
		})
	}
}

func TestFirewallRulesPlanTimeUnknownInputs(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)
	unknownStr := cty.UnknownVal(cty.String)
	unknownBool := cty.UnknownVal(cty.Bool)
	unknownSubnetTuple := cty.TupleVal([]cty.Value{
		cty.ObjectVal(map[string]cty.Value{
			"project":                  unknownStr,
			"network":                  unknownStr,
			"private_ip_google_access": unknownBool,
		}),
	})

	t.Run("lookup_enabled_unknown_network_inputs", func(t *testing.T) {
		ctx := &hcl.EvalContext{
			Functions: map[string]function.Function{"coalesce": terraformCoalesceFunc},
			Variables: map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"use_subnetwork_data":  cty.True,
					"subnetwork_self_link": unknownStr,
					"project_id":           unknownStr,
					"network_name":         unknownStr,
				}),
				"data": cty.ObjectVal(map[string]cty.Value{
					"google_compute_subnetwork": cty.ObjectVal(map[string]cty.Value{
						"subnetwork": unknownSubnetTuple,
					}),
				}),
			},
		}

		assertKnownCountsAndDeferredValidations(t, exprs, ctx, 1)
		assertUnknownString(t, "data.google_compute_subnetwork.subnetwork.self_link", exprs.dataSelfLink, ctx)
		derivedProj := assertUnknownString(t, "local.derived_project_id", exprs.derivedProject, ctx)
		derivedNet := assertUnknownString(t, "local.derived_network_name", exprs.derivedNetwork, ctx)

		ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
			"derived_project_id":   derivedProj,
			"derived_network_name": derivedNet,
		})
		effProj := assertUnknownString(t, "local.effective_project_id", exprs.effectiveProj, ctx)
		effNet := assertUnknownString(t, "local.effective_network_name", exprs.effectiveNet, ctx)

		ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
			"derived_project_id":     derivedProj,
			"derived_network_name":   derivedNet,
			"effective_project_id":   effProj,
			"effective_network_name": effNet,
		})
		assertUnknownString(t, "module.firewall_rule.project_id", exprs.modProject, ctx)
		assertUnknownString(t, "module.firewall_rule.network_name", exprs.modNetwork, ctx)

		if pgaVal := evalExpr(t, exprs.pgaCondition, ctx); pgaVal.IsKnown() || pgaVal.Type() != cty.Bool {
			t.Fatalf("pga_check precondition = %#v, want unknown bool", pgaVal)
		}
	})

	t.Run("lookup_enabled_known_overrides_with_unknown_subnet", func(t *testing.T) {
		ctx := &hcl.EvalContext{
			Functions: map[string]function.Function{"coalesce": terraformCoalesceFunc},
			Variables: map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"use_subnetwork_data":  cty.True,
					"subnetwork_self_link": unknownStr,
					"project_id":           cty.StringVal("explicit-project"),
					"network_name":         cty.StringVal("explicit-network"),
				}),
				"data": cty.ObjectVal(map[string]cty.Value{
					"google_compute_subnetwork": cty.ObjectVal(map[string]cty.Value{
						"subnetwork": unknownSubnetTuple,
					}),
				}),
			},
		}

		derivedProj := evalExpr(t, exprs.derivedProject, ctx)
		derivedNet := evalExpr(t, exprs.derivedNetwork, ctx)
		ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
			"derived_project_id":   derivedProj,
			"derived_network_name": derivedNet,
		})

		effProj := evalExpr(t, exprs.effectiveProj, ctx)
		effNet := evalExpr(t, exprs.effectiveNet, ctx)
		if !effProj.RawEquals(cty.StringVal("explicit-project")) || !effNet.RawEquals(cty.StringVal("explicit-network")) {
			t.Fatalf("effective locals = (%#v, %#v), want known explicit overrides", effProj, effNet)
		}

		ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
			"derived_project_id":     derivedProj,
			"derived_network_name":   derivedNet,
			"effective_project_id":   effProj,
			"effective_network_name": effNet,
		})
		modProj := evalExpr(t, exprs.modProject, ctx)
		modNet := evalExpr(t, exprs.modNetwork, ctx)
		if !modProj.RawEquals(cty.StringVal("explicit-project")) || !modNet.RawEquals(cty.StringVal("explicit-network")) {
			t.Fatalf("module.firewall_rule got (%#v, %#v), want known explicit overrides", modProj, modNet)
		}
		if pgaVal := evalExpr(t, exprs.pgaCondition, ctx); !pgaVal.IsKnown() || !pgaVal.True() {
			t.Fatalf("pga_check precondition = %#v, want known true", pgaVal)
		}
	})

	t.Run("lookup_disabled_unknown_network_inputs", func(t *testing.T) {
		ctx := &hcl.EvalContext{
			Functions: map[string]function.Function{"coalesce": terraformCoalesceFunc},
			Variables: map[string]cty.Value{
				"var": cty.ObjectVal(map[string]cty.Value{
					"use_subnetwork_data":  cty.False,
					"subnetwork_self_link": unknownStr,
					"project_id":           unknownStr,
					"network_name":         unknownStr,
				}),
				"data": cty.ObjectVal(map[string]cty.Value{
					"google_compute_subnetwork": cty.ObjectVal(map[string]cty.Value{
						"subnetwork": cty.EmptyTupleVal,
					}),
				}),
			},
		}

		assertKnownCountsAndDeferredValidations(t, exprs, ctx, 0)
		derivedProj := evalExpr(t, exprs.derivedProject, ctx)
		derivedNet := evalExpr(t, exprs.derivedNetwork, ctx)
		if !derivedProj.IsKnown() || !derivedProj.IsNull() || !derivedNet.IsKnown() || !derivedNet.IsNull() {
			t.Fatalf("derived locals = (%#v, %#v), want known nulls", derivedProj, derivedNet)
		}

		ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
			"derived_project_id":   derivedProj,
			"derived_network_name": derivedNet,
		})
		effProj := assertUnknownString(t, "local.effective_project_id", exprs.effectiveProj, ctx)
		effNet := assertUnknownString(t, "local.effective_network_name", exprs.effectiveNet, ctx)

		ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
			"derived_project_id":     derivedProj,
			"derived_network_name":   derivedNet,
			"effective_project_id":   effProj,
			"effective_network_name": effNet,
		})
		assertUnknownString(t, "module.firewall_rule.project_id", exprs.modProject, ctx)
		assertUnknownString(t, "module.firewall_rule.network_name", exprs.modNetwork, ctx)
	})
}

func TestFirewallRulesLocalsAndModuleWiring(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)

	nullStr := cty.NullVal(cty.String)
	emptyStr := cty.StringVal("")
	subnetLink := cty.StringVal("https://www.googleapis.com/compute/v1/projects/derived-project/regions/us-central1/subnetworks/test-subnet")

	cases := []struct {
		name              string
		useSubnetworkData bool
		projectID         cty.Value
		networkName       cty.Value
		subnetProject     string
		subnetNetwork     string
		wantDerivedNull   bool
		wantProject       string
		wantNetwork       string
	}{
		{"derive_both_when_null", true, nullStr, nullStr, "test-project", "test-network", false, "test-project", "test-network"},
		{"derive_both_when_empty_strings", true, emptyStr, emptyStr, "derived-project", "derived-network", false, "derived-project", "derived-network"},
		{"explicit_inputs_override_derived_subnet", true, cty.StringVal("explicit-project"), cty.StringVal("explicit-network"), "derived-project", "derived-network", false, "explicit-project", "explicit-network"},
		{"partial_override_project_only", true, cty.StringVal("explicit-project"), nullStr, "derived-project", "derived-network", false, "explicit-project", "derived-network"},
		{"partial_override_network_only", true, emptyStr, cty.StringVal("explicit-network"), "derived-project", "derived-network", false, "derived-project", "explicit-network"},
		{"explicit_network_without_subnet_lookup", false, cty.StringVal("explicit-project"), cty.StringVal("explicit-network"), "", "", true, "explicit-project", "explicit-network"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subnetworkTuple := cty.EmptyTupleVal
			if tc.useSubnetworkData {
				subnetworkTuple = cty.TupleVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"project":                  cty.StringVal(tc.subnetProject),
						"network":                  cty.StringVal(tc.subnetNetwork),
						"private_ip_google_access": cty.True,
					}),
				})
			}

			ingressVal := cty.ListValEmpty(cty.DynamicPseudoType)
			egressVal := cty.ListValEmpty(cty.DynamicPseudoType)

			ctx := &hcl.EvalContext{
				Functions: map[string]function.Function{"coalesce": terraformCoalesceFunc},
				Variables: map[string]cty.Value{
					"var": cty.ObjectVal(map[string]cty.Value{
						"use_subnetwork_data":  cty.BoolVal(tc.useSubnetworkData),
						"subnetwork_self_link": subnetLink,
						"project_id":           tc.projectID,
						"network_name":         tc.networkName,
						"ingress_rules":        ingressVal,
						"egress_rules":         egressVal,
					}),
					"data": cty.ObjectVal(map[string]cty.Value{
						"google_compute_subnetwork": cty.ObjectVal(map[string]cty.Value{
							"subnetwork": subnetworkTuple,
						}),
					}),
				},
			}

			if selfLinkVal := evalExpr(t, exprs.dataSelfLink, ctx); !selfLinkVal.RawEquals(subnetLink) {
				t.Fatalf("data.google_compute_subnetwork.subnetwork.self_link = %#v, want %#v", selfLinkVal, subnetLink)
			}

			derivedProj := evalExpr(t, exprs.derivedProject, ctx)
			derivedNet := evalExpr(t, exprs.derivedNetwork, ctx)
			if derivedProj.IsNull() != tc.wantDerivedNull || derivedNet.IsNull() != tc.wantDerivedNull {
				t.Fatalf("derived locals null = (%t, %t), want %t", derivedProj.IsNull(), derivedNet.IsNull(), tc.wantDerivedNull)
			}

			ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
				"derived_project_id":   derivedProj,
				"derived_network_name": derivedNet,
			})
			effProj := evalExpr(t, exprs.effectiveProj, ctx)
			effNet := evalExpr(t, exprs.effectiveNet, ctx)
			if effProj.AsString() != tc.wantProject || effNet.AsString() != tc.wantNetwork {
				t.Errorf("effective locals = (%q, %q), want (%q, %q)", effProj.AsString(), effNet.AsString(), tc.wantProject, tc.wantNetwork)
			}

			ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
				"derived_project_id":     derivedProj,
				"derived_network_name":   derivedNet,
				"effective_project_id":   effProj,
				"effective_network_name": effNet,
			})
			if modProj, modNet := evalExpr(t, exprs.modProject, ctx), evalExpr(t, exprs.modNetwork, ctx); !modProj.RawEquals(effProj) || !modNet.RawEquals(effNet) {
				t.Errorf("module.firewall_rule project/network = (%#v, %#v), want (%#v, %#v)", modProj, modNet, effProj, effNet)
			}
			if modIng, modEg := evalExpr(t, exprs.modIngress, ctx), evalExpr(t, exprs.modEgress, ctx); !modIng.RawEquals(ingressVal) || !modEg.RawEquals(egressVal) {
				t.Errorf("module.firewall_rule rules wiring mismatch: got (%#v, %#v)", modIng, modEg)
			}
		})
	}
}

func TestFirewallRulesPrivateGoogleAccessPrecondition(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)

	if msgVal := evalExpr(t, exprs.pgaErrorMsg, nil); msgVal.AsString() == "" {
		t.Fatal("pga_check precondition error_message must not be empty")
	}

	nullStr := cty.NullVal(cty.String)
	emptyStr := cty.StringVal("")
	explicitProject := cty.StringVal("explicit-project")
	explicitNetwork := cty.StringVal("explicit-network")

	cases := []struct {
		name        string
		projectID   cty.Value
		networkName cty.Value
		pgaEnabled  bool
		wantPass    bool
	}{
		{"derived_with_pga_enabled_passes", nullStr, nullStr, true, true},
		{"derived_with_pga_disabled_fails", nullStr, nullStr, false, false},
		{"explicit_project_and_network_bypass_disabled_pga", explicitProject, explicitNetwork, false, true},
		{"empty_project_still_requires_pga", emptyStr, explicitNetwork, false, false},
		{"null_project_still_requires_pga", nullStr, explicitNetwork, false, false},
		{"empty_network_still_requires_pga", explicitProject, emptyStr, false, false},
		{"null_network_still_requires_pga", explicitProject, nullStr, false, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &hcl.EvalContext{
				Variables: map[string]cty.Value{
					"var": cty.ObjectVal(map[string]cty.Value{
						"project_id":   tc.projectID,
						"network_name": tc.networkName,
					}),
					"data": cty.ObjectVal(map[string]cty.Value{
						"google_compute_subnetwork": cty.ObjectVal(map[string]cty.Value{
							"subnetwork": cty.TupleVal([]cty.Value{
								cty.ObjectVal(map[string]cty.Value{
									"project":                  cty.StringVal("derived-project"),
									"network":                  cty.StringVal("derived-network"),
									"private_ip_google_access": cty.BoolVal(tc.pgaEnabled),
								}),
							}),
						}),
					}),
				},
			}

			if val := evalExpr(t, exprs.pgaCondition, ctx); val.True() != tc.wantPass {
				t.Fatalf("pga_check precondition = %t, want %t", val.True(), tc.wantPass)
			}
		})
	}
}
