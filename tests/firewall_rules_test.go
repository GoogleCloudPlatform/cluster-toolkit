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
	"slices"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
)

// terraformCoalesceFunc wraps go-cty's stdlib.CoalesceFunc to match Terraform's
// built-in coalesce() function. Unlike stdlib.CoalesceFunc (which only skips null
// and returns "" as non-null), Terraform's coalesce() skips both null and empty
// string ("") arguments—which main.tf relies on when callers pass "" for
// var.project_id or var.network_name.
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
	modSource      hcl.Expression
	modVersion     hcl.Expression
	modProject     hcl.Expression
	modNetwork     hcl.Expression
	modIngress     hcl.Expression
	modEgress      hcl.Expression
}

type firewallEvalResult struct {
	ctx          *hcl.EvalContext
	dataCount    cty.Value
	pgaCount     cty.Value
	dataSelfLink cty.Value
	derivedProj  cty.Value
	derivedNet   cty.Value
	effProj      cty.Value
	effNet       cty.Value
	modProj      cty.Value
	modNet       cty.Value
	modIng       cty.Value
	modEg        cty.Value
}

func parseHCLFile(t *testing.T, path string) *hclsyntax.Body {
	t.Helper()
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	file, diag := hclsyntax.ParseConfig(src, path, hcl.InitialPos)
	if diag.HasErrors() {
		t.Fatal(diag)
	}
	return file.Body.(*hclsyntax.Body)
}

// findBlockAttribute locates a single attribute in blocks matching kind and all
// expected labels (unlike findAttribute in gateway_api_test.go, which checks only
// the first label). It fails if zero or multiple blocks match.
func findBlockAttribute(t *testing.T, blocks []*hclsyntax.Block, kind, attribute string, labels ...string) hcl.Expression {
	t.Helper()
	var matches []hcl.Expression
	for _, block := range blocks {
		if block.Type != kind {
			continue
		}
		if len(labels) > 0 && !slices.Equal(block.Labels, labels) {
			continue
		}
		if attr, ok := block.Body.Attributes[attribute]; ok {
			matches = append(matches, attr.Expr)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("found %d matches for %s %v.%s, want 1", len(matches), kind, labels, attribute)
	}
	return matches[0]
}

func extractValidations(t *testing.T, body *hclsyntax.Body, varName string) []validationRule {
	t.Helper()
	var validations []validationRule
	for _, block := range body.Blocks {
		if block.Type != "variable" || !slices.Equal(block.Labels, []string{varName}) {
			continue
		}
		for _, sub := range block.Body.Blocks {
			if sub.Type != "validation" {
				continue
			}
			validations = append(validations, validationRule{
				condition:    findBlockAttribute(t, []*hclsyntax.Block{sub}, "validation", "condition"),
				errorMessage: findBlockAttribute(t, []*hclsyntax.Block{sub}, "validation", "error_message"),
			})
		}
	}
	return validations
}

// readFirewallRulesExpressions parses modules/network/firewall-rules/{variables,main}.tf
// using flattenBlocks (from gateway_api_test.go) and findBlockAttribute.
func readFirewallRulesExpressions(t *testing.T) firewallRulesExprs {
	t.Helper()
	varBody := parseHCLFile(t, "../modules/network/firewall-rules/variables.tf")
	varBlocks := flattenBlocks(varBody)
	mainBody := parseHCLFile(t, "../modules/network/firewall-rules/main.tf")
	mainBlocks := flattenBlocks(mainBody)

	return firewallRulesExprs{
		useSubnetworkDefault:  findBlockAttribute(t, varBlocks, "variable", "default", "use_subnetwork_data"),
		useSubnetworkNullable: findBlockAttribute(t, varBlocks, "variable", "nullable", "use_subnetwork_data"),
		useSubnetworkValid:    extractValidations(t, varBody, "use_subnetwork_data"),
		subnetworkDefault:     findBlockAttribute(t, varBlocks, "variable", "default", "subnetwork_self_link"),
		projectDefault:        findBlockAttribute(t, varBlocks, "variable", "default", "project_id"),
		networkDefault:        findBlockAttribute(t, varBlocks, "variable", "default", "network_name"),
		ingressDefault:        findBlockAttribute(t, varBlocks, "variable", "default", "ingress_rules"),
		egressDefault:         findBlockAttribute(t, varBlocks, "variable", "default", "egress_rules"),

		dataCount:      findBlockAttribute(t, mainBlocks, "data", "count", "google_compute_subnetwork", "subnetwork"),
		dataSelfLink:   findBlockAttribute(t, mainBlocks, "data", "self_link", "google_compute_subnetwork", "subnetwork"),
		pgaCount:       findBlockAttribute(t, mainBlocks, "resource", "count", "terraform_data", "pga_check"),
		pgaCondition:   findBlockAttribute(t, mainBlocks, "precondition", "condition"),
		pgaErrorMsg:    findBlockAttribute(t, mainBlocks, "precondition", "error_message"),
		derivedProject: findBlockAttribute(t, mainBlocks, "locals", "derived_project_id"),
		derivedNetwork: findBlockAttribute(t, mainBlocks, "locals", "derived_network_name"),
		effectiveProj:  findBlockAttribute(t, mainBlocks, "locals", "effective_project_id"),
		effectiveNet:   findBlockAttribute(t, mainBlocks, "locals", "effective_network_name"),
		modSource:      findBlockAttribute(t, mainBlocks, "module", "source", "firewall_rule"),
		modVersion:     findBlockAttribute(t, mainBlocks, "module", "version", "firewall_rule"),
		modProject:     findBlockAttribute(t, mainBlocks, "module", "project_id", "firewall_rule"),
		modNetwork:     findBlockAttribute(t, mainBlocks, "module", "network_name", "firewall_rule"),
		modIngress:     findBlockAttribute(t, mainBlocks, "module", "ingress_rules", "firewall_rule"),
		modEgress:      findBlockAttribute(t, mainBlocks, "module", "egress_rules", "firewall_rule"),
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

func assertUnknownVal(t *testing.T, label string, got cty.Value, wantType cty.Type) {
	t.Helper()
	if got.IsKnown() || got.Type() != wantType {
		t.Fatalf("%s = %#v, want unknown %s", label, got, wantType.FriendlyName())
	}
}

func assertRawEquals(t *testing.T, label string, got, want cty.Value) {
	t.Helper()
	if !got.RawEquals(want) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func assertNonEmptyStringExpr(t *testing.T, label string, expr hcl.Expression) {
	t.Helper()
	val := evalExpr(t, expr, nil)
	if !val.IsKnown() || val.IsNull() || val.Type() != cty.String || val.AsString() == "" {
		t.Errorf("%s = %#v, want non-empty string", label, val)
	}
}

// evalFirewallRulesModule evaluates the module's expressions in dependency order:
// 1. Evaluates count, data.self_link, and local.derived_{project_id,network_name}.
// 2. Populates ctx.Variables["local"] with derived_* to evaluate local.effective_*.
// 3. Updates ctx.Variables["local"] with effective_* to evaluate module.firewall_rule.*.
func evalFirewallRulesModule(
	t *testing.T,
	exprs firewallRulesExprs,
	useSubnetworkData bool,
	subnetworkLink, projectID, networkName, subnetworkTuple cty.Value,
) firewallEvalResult {
	t.Helper()
	ingressVal := cty.ListVal([]cty.Value{cty.StringVal("allow-ingress-8080")})
	egressVal := cty.ListVal([]cty.Value{cty.StringVal("allow-egress-8443")})

	ctx := &hcl.EvalContext{
		Functions: map[string]function.Function{"coalesce": terraformCoalesceFunc},
		Variables: map[string]cty.Value{
			"var": cty.ObjectVal(map[string]cty.Value{
				"use_subnetwork_data":  cty.BoolVal(useSubnetworkData),
				"subnetwork_self_link": subnetworkLink,
				"project_id":           projectID,
				"network_name":         networkName,
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

	derivedProj := evalExpr(t, exprs.derivedProject, ctx)
	derivedNet := evalExpr(t, exprs.derivedNetwork, ctx)
	ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
		"derived_project_id":   derivedProj,
		"derived_network_name": derivedNet,
	})

	effProj := evalExpr(t, exprs.effectiveProj, ctx)
	effNet := evalExpr(t, exprs.effectiveNet, ctx)
	ctx.Variables["local"] = cty.ObjectVal(map[string]cty.Value{
		"derived_project_id":     derivedProj,
		"derived_network_name":   derivedNet,
		"effective_project_id":   effProj,
		"effective_network_name": effNet,
	})

	return firewallEvalResult{
		ctx:          ctx,
		dataCount:    evalExpr(t, exprs.dataCount, ctx),
		pgaCount:     evalExpr(t, exprs.pgaCount, ctx),
		dataSelfLink: evalExpr(t, exprs.dataSelfLink, ctx),
		derivedProj:  derivedProj,
		derivedNet:   derivedNet,
		effProj:      effProj,
		effNet:       effNet,
		modProj:      evalExpr(t, exprs.modProject, ctx),
		modNet:       evalExpr(t, exprs.modNetwork, ctx),
		modIng:       evalExpr(t, exprs.modIngress, ctx),
		modEg:        evalExpr(t, exprs.modEgress, ctx),
	}
}

// assertKnownCountsAndDeferredValidations verifies that data/resource counts are
// known at plan time (0 or 1), while the active variable validation rule evaluates
// to unknown (causing Terraform to defer validation until apply rather than failing
// the plan prematurely) and the inactive validation rule short-circuits to true.
func assertKnownCountsAndDeferredValidations(t *testing.T, exprs firewallRulesExprs, res firewallEvalResult, wantCount int64) {
	t.Helper()
	want := cty.NumberIntVal(wantCount)
	assertRawEquals(t, "data.google_compute_subnetwork.subnetwork.count", res.dataCount, want)
	assertRawEquals(t, "resource.terraform_data.pga_check.count", res.pgaCount, want)

	unknownCount, trueCount := 0, 0
	for i, rule := range exprs.useSubnetworkValid {
		val := evalExpr(t, rule.condition, res.ctx)
		if val.RawEquals(cty.True) {
			trueCount++
		} else if !val.IsKnown() && val.Type() == cty.Bool {
			unknownCount++
		} else {
			t.Fatalf("validation[%d] = %#v at plan time, want known true or unknown bool", i, val)
		}
	}
	if trueCount != 1 || unknownCount != 1 {
		t.Fatalf("validation states (true=%d, unknown=%d), want (1, 1)", trueCount, unknownCount)
	}
}

// Evaluate the module's actual HCL expressions in Go without provider downloads
// or embedding .tftest.hcl fixtures into user deployments.
func TestFirewallRulesVariableDefaultsAndSchema(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)

	assertRawEquals(t, "use_subnetwork_data default", evalExpr(t, exprs.useSubnetworkDefault, nil), cty.True)
	assertRawEquals(t, "use_subnetwork_data nullable", evalExpr(t, exprs.useSubnetworkNullable, nil), cty.False)
	assertRawEquals(t, "module.firewall_rule.source", evalExpr(t, exprs.modSource, nil), cty.StringVal("terraform-google-modules/network/google//modules/firewall-rules"))
	assertRawEquals(t, "module.firewall_rule.version", evalExpr(t, exprs.modVersion, nil), cty.StringVal("~> 18.3"))

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

// TestFirewallRulesInputValidations verifies both validation blocks on
// var.use_subnetwork_data, including Blueprint use: [network] cases where
// subnetwork_self_link is auto-wired even when use_subnetwork_data = false.
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
		{"lookup_enabled_all_defaults_missing_subnet", true, nullStr, nullStr, nullStr, false},
		{"lookup_enabled_missing_subnet_null", true, nullStr, validProject, validNetwork, false},
		{"lookup_enabled_empty_subnet", true, emptyStr, validProject, validNetwork, false},
		{"lookup_disabled_with_project_and_network_no_subnet", false, nullStr, validProject, validNetwork, true},
		// In blueprints with use: [network], subnetwork_self_link is auto-wired
		// even when use_subnetwork_data = false.
		{"lookup_disabled_with_project_network_and_autowired_subnet", false, validSubnet, validProject, validNetwork, true},
		{"lookup_disabled_missing_both_null", false, nullStr, nullStr, nullStr, false},
		{"lookup_disabled_missing_project_null", false, nullStr, nullStr, validNetwork, false},
		{"lookup_disabled_with_subnet_but_missing_project_null", false, validSubnet, nullStr, validNetwork, false},
		{"lookup_disabled_empty_project", false, nullStr, emptyStr, validNetwork, false},
		{"lookup_disabled_missing_network_null", false, nullStr, validProject, nullStr, false},
		{"lookup_disabled_with_subnet_but_empty_network", false, validSubnet, validProject, emptyStr, false},
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
				if val := evalExpr(t, rule.condition, ctx); !val.RawEquals(cty.True) {
					passed = false
				}
			}
			if passed != tc.wantPass {
				t.Fatalf("validation passed = %t, want %t", passed, tc.wantPass)
			}
		})
	}
}

// TestFirewallRulesPlanTimeUnknownInputs simulates a VPC/subnetwork created in the
// same deployment group as firewall-rules, where network outputs are unknown during
// terraform plan (including refined non-null unknown strings in Terraform 1.3+).
// Count expressions must still evaluate to a known 0 or 1 so planning succeeds.
func TestFirewallRulesPlanTimeUnknownInputs(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)
	nullStr := cty.NullVal(cty.String)
	emptyStr := cty.StringVal("")
	unknownStr := cty.UnknownVal(cty.String)
	refinedUnknownStr := cty.UnknownVal(cty.String).RefineNotNull()
	unknownBool := cty.UnknownVal(cty.Bool)
	unknownSubnetTuple := cty.TupleVal([]cty.Value{
		cty.ObjectVal(map[string]cty.Value{
			"project":                  unknownStr,
			"network":                  unknownStr,
			"private_ip_google_access": unknownBool,
		}),
	})

	t.Run("lookup_enabled_unknown_subnet_with_null_and_empty_overrides", func(t *testing.T) {
		for _, overrideVal := range []cty.Value{nullStr, emptyStr, refinedUnknownStr} {
			res := evalFirewallRulesModule(t, exprs, true, refinedUnknownStr, overrideVal, overrideVal, unknownSubnetTuple)
			assertKnownCountsAndDeferredValidations(t, exprs, res, 1)
			assertUnknownVal(t, "data.self_link", res.dataSelfLink, cty.String)
			assertUnknownVal(t, "local.derived_project_id", res.derivedProj, cty.String)
			assertUnknownVal(t, "local.derived_network_name", res.derivedNet, cty.String)
			assertUnknownVal(t, "local.effective_project_id", res.effProj, cty.String)
			assertUnknownVal(t, "local.effective_network_name", res.effNet, cty.String)
			assertUnknownVal(t, "module.firewall_rule.project_id", res.modProj, cty.String)
			assertUnknownVal(t, "module.firewall_rule.network_name", res.modNet, cty.String)
			assertUnknownVal(t, "pga_check.precondition", evalExpr(t, exprs.pgaCondition, res.ctx), cty.Bool)
		}
	})

	t.Run("lookup_enabled_known_overrides_with_unknown_subnet", func(t *testing.T) {
		wantProj := cty.StringVal("explicit-project")
		wantNet := cty.StringVal("explicit-network")
		res := evalFirewallRulesModule(t, exprs, true, refinedUnknownStr, wantProj, wantNet, unknownSubnetTuple)
		assertKnownCountsAndDeferredValidations(t, exprs, res, 1)
		assertRawEquals(t, "local.effective_project_id", res.effProj, wantProj)
		assertRawEquals(t, "local.effective_network_name", res.effNet, wantNet)
		assertRawEquals(t, "module.firewall_rule.project_id", res.modProj, wantProj)
		assertRawEquals(t, "module.firewall_rule.network_name", res.modNet, wantNet)
		assertRawEquals(t, "pga_check.precondition", evalExpr(t, exprs.pgaCondition, res.ctx), cty.True)
	})

	t.Run("lookup_enabled_partial_override_with_unknown_subnet", func(t *testing.T) {
		wantProj := cty.StringVal("explicit-project")
		res := evalFirewallRulesModule(t, exprs, true, refinedUnknownStr, wantProj, nullStr, unknownSubnetTuple)
		assertKnownCountsAndDeferredValidations(t, exprs, res, 1)
		assertRawEquals(t, "local.effective_project_id", res.effProj, wantProj)
		assertUnknownVal(t, "local.effective_network_name", res.effNet, cty.String)
		assertUnknownVal(t, "pga_check.precondition", evalExpr(t, exprs.pgaCondition, res.ctx), cty.Bool)
	})

	t.Run("lookup_disabled_unknown_network_inputs", func(t *testing.T) {
		res := evalFirewallRulesModule(t, exprs, false, refinedUnknownStr, refinedUnknownStr, refinedUnknownStr, cty.EmptyTupleVal)
		assertKnownCountsAndDeferredValidations(t, exprs, res, 0)
		assertRawEquals(t, "local.derived_project_id", res.derivedProj, cty.NullVal(cty.DynamicPseudoType))
		assertRawEquals(t, "local.derived_network_name", res.derivedNet, cty.NullVal(cty.DynamicPseudoType))
		assertUnknownVal(t, "local.effective_project_id", res.effProj, cty.String)
		assertUnknownVal(t, "local.effective_network_name", res.effNet, cty.String)
		assertUnknownVal(t, "module.firewall_rule.project_id", res.modProj, cty.String)
		assertUnknownVal(t, "module.firewall_rule.network_name", res.modNet, cty.String)
		// pga_check has count = 0 when use_subnetwork_data = false, so pgaCondition is not evaluated.
	})

	t.Run("lookup_disabled_unknown_project_but_null_network_fails_at_plan", func(t *testing.T) {
		res := evalFirewallRulesModule(t, exprs, false, nullStr, refinedUnknownStr, nullStr, cty.EmptyTupleVal)
		assertRawEquals(t, "validation[1] with null network_name", evalExpr(t, exprs.useSubnetworkValid[1].condition, res.ctx), cty.False)
	})
}

// TestFirewallRulesLocalsAndModuleWiring verifies derived/effective local resolution,
// coalesce override precedence, count evaluation, and module.firewall_rule wiring.
func TestFirewallRulesLocalsAndModuleWiring(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)

	nullStr := cty.NullVal(cty.String)
	emptyStr := cty.StringVal("")
	subnetLink := cty.StringVal("https://www.googleapis.com/compute/v1/projects/derived-project/regions/us-central1/subnetworks/test-subnet")

	cases := []struct {
		name              string
		useSubnetworkData bool
		subnetworkLink    cty.Value
		projectID         cty.Value
		networkName       cty.Value
		subnetProject     string
		subnetNetwork     string
		wantCount         int64
		wantDerivedNull   bool
		wantProject       string
		wantNetwork       string
	}{
		{"derive_both_when_null", true, subnetLink, nullStr, nullStr, "test-project", "test-network", 1, false, "test-project", "test-network"},
		{"derive_both_when_empty_strings", true, subnetLink, emptyStr, emptyStr, "derived-project", "derived-network", 1, false, "derived-project", "derived-network"},
		{"explicit_inputs_override_derived_subnet", true, subnetLink, cty.StringVal("explicit-project"), cty.StringVal("explicit-network"), "derived-project", "derived-network", 1, false, "explicit-project", "explicit-network"},
		{"partial_override_project_only", true, subnetLink, cty.StringVal("explicit-project"), nullStr, "derived-project", "derived-network", 1, false, "explicit-project", "derived-network"},
		{"partial_override_network_only", true, subnetLink, emptyStr, cty.StringVal("explicit-network"), "derived-project", "derived-network", 1, false, "derived-project", "explicit-network"},
		{"explicit_network_without_subnet_lookup", false, nullStr, cty.StringVal("explicit-project"), cty.StringVal("explicit-network"), "", "", 0, true, "explicit-project", "explicit-network"},
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

			res := evalFirewallRulesModule(t, exprs, tc.useSubnetworkData, tc.subnetworkLink, tc.projectID, tc.networkName, subnetworkTuple)
			wantCountVal := cty.NumberIntVal(tc.wantCount)
			assertRawEquals(t, "data.count", res.dataCount, wantCountVal)
			assertRawEquals(t, "pga_check.count", res.pgaCount, wantCountVal)
			assertRawEquals(t, "data.self_link", res.dataSelfLink, tc.subnetworkLink)

			if res.derivedProj.IsNull() != tc.wantDerivedNull || res.derivedNet.IsNull() != tc.wantDerivedNull {
				t.Fatalf("derived locals null = (%t, %t), want (%t, %t)", res.derivedProj.IsNull(), res.derivedNet.IsNull(), tc.wantDerivedNull, tc.wantDerivedNull)
			}

			assertRawEquals(t, "local.effective_project_id", res.effProj, cty.StringVal(tc.wantProject))
			assertRawEquals(t, "local.effective_network_name", res.effNet, cty.StringVal(tc.wantNetwork))
			assertRawEquals(t, "module.firewall_rule.project_id", res.modProj, res.effProj)
			assertRawEquals(t, "module.firewall_rule.network_name", res.modNet, res.effNet)
			assertRawEquals(t, "module.firewall_rule.ingress_rules", res.modIng, cty.ListVal([]cty.Value{cty.StringVal("allow-ingress-8080")}))
			assertRawEquals(t, "module.firewall_rule.egress_rules", res.modEg, cty.ListVal([]cty.Value{cty.StringVal("allow-egress-8443")}))
		})
	}
}

// TestFirewallRulesPrivateGoogleAccessPrecondition verifies that terraform_data.pga_check
// enforces private_ip_google_access when either project_id or network_name is derived,
// and bypasses the check when both project_id and network_name are explicitly set.
func TestFirewallRulesPrivateGoogleAccessPrecondition(t *testing.T) {
	exprs := readFirewallRulesExpressions(t)
	assertNonEmptyStringExpr(t, "pga_check precondition error_message", exprs.pgaErrorMsg)

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

			assertRawEquals(t, "pga_check.precondition", evalExpr(t, exprs.pgaCondition, ctx), cty.BoolVal(tc.wantPass))
		})
	}
}

func TestTerraformCoalesceFunc(t *testing.T) {
	nullStr := cty.NullVal(cty.String)
	emptyStr := cty.StringVal("")
	unknownStr := cty.UnknownVal(cty.String)

	got, err := terraformCoalesceFunc.Call([]cty.Value{nullStr, emptyStr, cty.StringVal("fallback")})
	if err != nil {
		t.Fatal(err)
	}
	assertRawEquals(t, "coalesce(null, \"\", \"fallback\")", got, cty.StringVal("fallback"))

	gotUnknown, err := terraformCoalesceFunc.Call([]cty.Value{emptyStr, unknownStr})
	if err != nil {
		t.Fatal(err)
	}
	assertUnknownVal(t, "coalesce(\"\", unknown)", gotUnknown, cty.String)

	if _, err := terraformCoalesceFunc.Call([]cty.Value{nullStr, emptyStr}); err == nil {
		t.Fatal("coalesce(null, \"\") succeeded, want error when all arguments are null or empty")
	}
}
