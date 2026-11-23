// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modulereader

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

const (
	constraintSourceTerraform = "terraform"
	constraintSourceMetadata  = "metadata"
)

// InputConstraints lists machine-readable limits for a module input. They are
// read from the module's own Terraform validation blocks and metadata.yaml
// validators, and only cover a few simple, exact shapes. Every limit is a
// necessary condition of those rules: any value the module accepts also
// passes it. Rules that cannot be reduced to these fields stay as messages in
// VarInfo.Validations.
type InputConstraints struct {
	AllowedValues []interface{} `json:"allowed_values,omitempty" yaml:"allowed_values,omitempty"`
	// CaseInsensitive means AllowedValues are compared without regard to case.
	CaseInsensitive bool     `json:"case_insensitive,omitempty" yaml:"case_insensitive,omitempty"`
	Min             *float64 `json:"min,omitempty" yaml:"min,omitempty"`
	MinExclusive    bool     `json:"min_exclusive,omitempty" yaml:"min_exclusive,omitempty"`
	Max             *float64 `json:"max,omitempty" yaml:"max,omitempty"`
	MaxExclusive    bool     `json:"max_exclusive,omitempty" yaml:"max_exclusive,omitempty"`
	// Pattern is an RE2 regular expression. Like Terraform's regex() and JSON
	// Schema's "pattern", it matches anywhere in the value unless it is
	// anchored with ^ and $. When rules state different patterns, only one is
	// reported; the others stay in Validations.
	Pattern string `json:"pattern,omitempty" yaml:"pattern,omitempty"`
	// AllowNull is set only when the rules explicitly accept null.
	AllowNull bool `json:"allow_null,omitempty" yaml:"allow_null,omitempty"`
	// Sources lists which kinds of rules state limits for this input:
	// "metadata" (metadata.yaml validators) and/or "terraform" (validation
	// blocks).
	Sources []string `json:"sources,omitempty" yaml:"sources,omitempty"`
}

// nullRule records what a rule says about null values.
type nullRule int

const (
	nullUnknown nullRule = iota
	// nullPassThrough means the validator skips null values (such as
	// metadata.yaml range and regex), so it neither affirms variable
	// nullability on its own nor overrides an explicit nullAllowed rule.
	nullPassThrough
	nullAllowed
	nullRejected
)

type bound struct {
	value     float64
	exclusive bool
}

// ruleConstraints holds the limits implied by one rule. Each field is a
// necessary condition of that rule.
type ruleConstraints struct {
	allowed         []cty.Value
	hasAllowed      bool
	caseInsensitive bool
	min             *bound
	max             *bound
	pattern         string
	null            nullRule
	source          string
}

func (rc ruleConstraints) hasLimits() bool {
	return len(rc.allowed) > 0 || rc.min != nil || rc.max != nil || rc.pattern != ""
}

// buildInputConstraints merges the limits of every rule on one input. Limits
// are only reported for primitive or untyped inputs: rules on collections and
// objects usually describe elements or attributes, which are not covered.
func buildInputConstraints(t cty.Type, groups ...[]ruleConstraints) *InputConstraints {
	if t != cty.DynamicPseudoType && !t.IsPrimitiveType() {
		return nil
	}
	var all []ruleConstraints
	for _, g := range groups {
		for _, rc := range g {
			all = append(all, gateRuleToType(rc, t))
		}
	}
	if len(all) == 0 {
		return nil
	}
	merged := all[0]
	for _, rc := range all[1:] {
		merged = mergeRules(merged, rc)
	}
	var sources []string
	for _, rc := range all {
		if rc.source != "" && contributesLimit(rc, merged) && !slices.Contains(sources, rc.source) {
			sources = append(sources, rc.source)
		}
	}
	sort.Strings(sources)
	return merged.toOutput(sources)
}

// gateRuleToType drops limits that cannot apply to an input of type t:
// allowed values of a different type, and numeric bounds on a non-number
// input. Dropping a limit only weakens the rule, so the result stays a
// necessary condition. Untyped (any) inputs keep every limit.
func gateRuleToType(rc ruleConstraints, t cty.Type) ruleConstraints {
	if t == cty.DynamicPseudoType {
		return rc
	}
	if rc.hasAllowed && slices.ContainsFunc(rc.allowed, func(v cty.Value) bool { return v.Type() != t }) {
		rc.allowed, rc.hasAllowed, rc.caseInsensitive = nil, false, false
	}
	if t != cty.Number {
		rc.min, rc.max = nil, nil
	}
	return rc
}

// contributesLimit reports whether rc states a limit that survives in merged.
// A pattern counts only if it is the one reported, since merged keeps one.
func contributesLimit(rc, merged ruleConstraints) bool {
	return len(rc.allowed) > 0 || rc.min != nil || rc.max != nil || (rc.pattern != "" && rc.pattern == merged.pattern)
}

func (rc ruleConstraints) toOutput(sources []string) *InputConstraints {
	if !rc.hasLimits() || (rc.hasAllowed && len(rc.allowed) == 0) || boundsContradict(rc.min, rc.max) {
		return nil
	}
	out := &InputConstraints{
		Pattern:   rc.pattern,
		AllowNull: rc.null == nullAllowed,
		Sources:   sources,
	}
	if len(rc.allowed) > 0 {
		out.CaseInsensitive = rc.caseInsensitive
		for _, v := range rc.allowed {
			out.AllowedValues = append(out.AllowedValues, ctyToPlain(v))
		}
	}
	if rc.min != nil {
		v := rc.min.value
		out.Min, out.MinExclusive = &v, rc.min.exclusive
	}
	if rc.max != nil {
		v := rc.max.value
		out.Max, out.MaxExclusive = &v, rc.max.exclusive
	}
	return out
}

func boundsContradict(minB, maxB *bound) bool {
	if minB == nil || maxB == nil {
		return false
	}
	return minB.value > maxB.value || (minB.value == maxB.value && (minB.exclusive || maxB.exclusive))
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func normalizeZero(f float64) float64 {
	if f == 0 {
		return 0
	}
	return f
}

func ctyToPlain(v cty.Value) interface{} {
	if v == cty.NilVal || v.IsNull() || !v.IsKnown() {
		return nil
	}
	switch v.Type() {
	case cty.String:
		return v.AsString()
	case cty.Number:
		f, _ := v.AsBigFloat().Float64()
		return normalizeZero(f)
	case cty.Bool:
		return v.True()
	}
	return nil
}

// mergeRules combines the limits of two rules that must both hold.
func mergeRules(a, b ruleConstraints) ruleConstraints {
	out := ruleConstraints{
		min:     tighterBound(a.min, b.min, true),
		max:     tighterBound(a.max, b.max, false),
		pattern: a.pattern,
		null:    mergeNull(a.null, b.null),
	}
	if out.pattern == "" {
		out.pattern = b.pattern
	}
	out.allowed, out.hasAllowed, out.caseInsensitive = mergeAllowed(a, b)
	return out
}

func mergeNull(a, b nullRule) nullRule {
	switch {
	case a == nullRejected || b == nullRejected:
		return nullRejected
	case a == nullUnknown || b == nullUnknown:
		return nullUnknown
	case a == nullAllowed || b == nullAllowed:
		return nullAllowed
	}
	return nullPassThrough
}

// tighterBound returns the stricter of two lower (isMin) or upper bounds.
func tighterBound(a, b *bound, isMin bool) *bound {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	if a.value == b.value {
		return &bound{value: a.value, exclusive: a.exclusive || b.exclusive}
	}
	if (a.value > b.value) == isMin {
		return a
	}
	return b
}

// mergeAllowed intersects two allowed-value lists. When only one list is case
// insensitive, the case-sensitive list is filtered against it using
// case-insensitive matching so the exact intersection is kept. An empty
// intersection means the rules contradict each other, so no limits are
// reported.
func mergeAllowed(a, b ruleConstraints) ([]cty.Value, bool, bool) {
	switch {
	case !a.hasAllowed:
		return b.allowed, b.hasAllowed, b.caseInsensitive
	case !b.hasAllowed:
		return a.allowed, a.hasAllowed, a.caseInsensitive
	case a.caseInsensitive && !b.caseInsensitive:
		return intersectAllowed(b.allowed, a.allowed, true), true, false
	case !a.caseInsensitive && b.caseInsensitive:
		return intersectAllowed(a.allowed, b.allowed, true), true, false
	default:
		return intersectAllowed(a.allowed, b.allowed, a.caseInsensitive), true, a.caseInsensitive
	}
}

func intersectAllowed(primary, filter []cty.Value, fold bool) []cty.Value {
	keep := make(map[string]bool, len(filter))
	for _, v := range filter {
		keep[valueKey(v, fold)] = true
	}
	var out []cty.Value
	for _, v := range primary {
		if keep[valueKey(v, fold)] {
			out = append(out, v)
		}
	}
	return out
}

func valueKey(v cty.Value, fold bool) string {
	if v == cty.NilVal || v.IsNull() || !v.IsKnown() {
		return ""
	}
	switch v.Type() {
	case cty.String:
		s := v.AsString()
		if fold {
			s = strings.ToLower(s)
		}
		return "s:" + s
	case cty.Number:
		bf := v.AsBigFloat()
		if bf.Sign() == 0 {
			return "n:0"
		}
		return "n:" + bf.Text('g', -1)
	case cty.Bool:
		return fmt.Sprintf("b:%t", v.True())
	}
	return ""
}

// ---------------------------------------------------------------------------
// Terraform validation conditions
// ---------------------------------------------------------------------------

// conditionAttrConstraints derives limits for var.<varName> from the
// condition attribute of a Terraform validation block. It only understands
// the shapes below and returns ok=false for anything else:
//
//	contains([literals], var.X)            -> allowed values
//	contains([lowercase], lower(var.X))    -> allowed values, case insensitive
//	can(regex("literal", var.X))           -> pattern
//	length(regexall("literal", var.X)) > 0 -> pattern
//	var.X >= / > / <= / < number           -> min / max
//	A && B                                 -> every part that is understood
//	var.X == null || S                     -> null allowed, S otherwise
//	var.X == null ? true : S               -> null allowed, S otherwise
func conditionAttrConstraints(varName string, attr *hclsyntax.Attribute) (ruleConstraints, bool) {
	if attr == nil || attr.Expr == nil {
		return ruleConstraints{}, false
	}
	rc, ok := conditionConstraints(varName, attr.Expr)
	if !ok {
		return ruleConstraints{}, false
	}
	rc.source = constraintSourceTerraform
	return rc, true
}

func conditionConstraints(varName string, expr hclsyntax.Expression) (ruleConstraints, bool) {
	switch e := unwrapParens(expr).(type) {
	case *hclsyntax.BinaryOpExpr:
		return binaryConstraints(varName, e)
	case *hclsyntax.ConditionalExpr:
		return conditionalConstraints(varName, e)
	case *hclsyntax.FunctionCallExpr:
		return callConstraints(varName, e)
	}
	return ruleConstraints{}, false
}

func binaryConstraints(varName string, e *hclsyntax.BinaryOpExpr) (ruleConstraints, bool) {
	switch e.Op {
	case hclsyntax.OpLogicalOr:
		return orConstraints(varName, e)
	case hclsyntax.OpLogicalAnd:
		return andConstraints(varName, e)
	}
	if isNullCheck(varName, e, hclsyntax.OpNotEqual) {
		return ruleConstraints{null: nullRejected}, true
	}
	if rc, ok := boundConstraints(varName, e); ok {
		return rc, true
	}
	return regexallConstraints(varName, e)
}

// orConstraints handles `var.X == null || S` with the null check on either
// side. Any other OR can accept values that S rejects, so nothing is derived
// from it. Terraform (>= 1.12, the toolkit's minimum) returns true for `||`
// as soon as either operand is true, even if the other operand would fail on
// null, so the null check allows null whichever side it is on.
func orConstraints(varName string, e *hclsyntax.BinaryOpExpr) (ruleConstraints, bool) {
	var other hclsyntax.Expression
	switch {
	case isNullCheck(varName, e.LHS, hclsyntax.OpEqual):
		other = e.RHS
	case isNullCheck(varName, e.RHS, hclsyntax.OpEqual):
		other = e.LHS
	default:
		return ruleConstraints{}, false
	}
	rc, _ := conditionConstraints(varName, other)
	rc.null = nullAllowed
	return rc, true
}

// andConstraints keeps every part it understands: each side of && is a
// necessary condition on its own.
func andConstraints(varName string, e *hclsyntax.BinaryOpExpr) (ruleConstraints, bool) {
	l, lok := conditionConstraints(varName, e.LHS)
	r, rok := conditionConstraints(varName, e.RHS)
	switch {
	case lok && rok:
		return mergeRules(l, r), true
	case lok:
		return withUnknownSide(l), true
	case rok:
		return withUnknownSide(r), true
	}
	return ruleConstraints{}, false
}

// withUnknownSide drops a claim that null is allowed when the other side of
// && was not understood, because that side might reject null.
func withUnknownSide(rc ruleConstraints) ruleConstraints {
	if rc.null == nullAllowed {
		rc.null = nullUnknown
	}
	return rc
}

// conditionalConstraints handles `var.X == null ? true : S` and
// `var.X != null ? S : true`, plus the `false` forms where null is rejected.
// Even when S is not understood, the null branch alone tells whether null is
// accepted or rejected.
func conditionalConstraints(varName string, e *hclsyntax.ConditionalExpr) (ruleConstraints, bool) {
	var whenNull, whenSet hclsyntax.Expression
	switch {
	case isNullCheck(varName, e.Condition, hclsyntax.OpEqual):
		whenNull, whenSet = e.TrueResult, e.FalseResult
	case isNullCheck(varName, e.Condition, hclsyntax.OpNotEqual):
		whenNull, whenSet = e.FalseResult, e.TrueResult
	default:
		return ruleConstraints{}, false
	}
	nullOK, isBool := boolLiteral(whenNull)
	if !isBool {
		return ruleConstraints{}, false
	}
	rc, _ := conditionConstraints(varName, whenSet)
	rc.null = nullRejected
	if nullOK {
		rc.null = nullAllowed
	}
	return rc, true
}

func callConstraints(varName string, e *hclsyntax.FunctionCallExpr) (ruleConstraints, bool) {
	switch e.Name {
	case "contains":
		return containsConstraints(varName, e)
	case "can":
		return canRegexConstraints(varName, e)
	}
	return ruleConstraints{}, false
}

// containsConstraints handles contains([literals], var.X), and
// contains([literals], lower(var.X)) or upper(var.X) when every literal is
// already in that case.
func containsConstraints(varName string, e *hclsyntax.FunctionCallExpr) (ruleConstraints, bool) {
	if len(e.Args) != 2 || e.ExpandFinal {
		return ruleConstraints{}, false
	}
	values, ok := literalPrimitiveList(e.Args[0])
	if !ok {
		return ruleConstraints{}, false
	}
	if isVarRef(varName, e.Args[1]) {
		return ruleConstraints{allowed: values, hasAllowed: true}, true
	}
	if fold, ok := foldedVarRef(varName, e.Args[1]); ok && allFolded(values, fold) {
		return ruleConstraints{allowed: values, hasAllowed: true, caseInsensitive: true}, true
	}
	return ruleConstraints{}, false
}

// canRegexConstraints handles can(regex("literal", var.X)).
func canRegexConstraints(varName string, e *hclsyntax.FunctionCallExpr) (ruleConstraints, bool) {
	if len(e.Args) != 1 || e.ExpandFinal {
		return ruleConstraints{}, false
	}
	inner, ok := unwrapParens(e.Args[0]).(*hclsyntax.FunctionCallExpr)
	if !ok || inner.Name != "regex" {
		return ruleConstraints{}, false
	}
	pattern, ok := regexCallPattern(varName, inner)
	if !ok {
		return ruleConstraints{}, false
	}
	return ruleConstraints{pattern: pattern}, true
}

// boundConstraints handles var.X compared with a constant number.
func boundConstraints(varName string, e *hclsyntax.BinaryOpExpr) (ruleConstraints, bool) {
	c, ok := splitComparison(e)
	if !ok || !isVarRef(varName, c.subject) {
		return ruleConstraints{}, false
	}
	b := &bound{value: c.number}
	switch c.op {
	case hclsyntax.OpGreaterThan:
		b.exclusive = true
		return ruleConstraints{min: b}, true
	case hclsyntax.OpGreaterThanOrEqual:
		return ruleConstraints{min: b}, true
	case hclsyntax.OpLessThan:
		b.exclusive = true
		return ruleConstraints{max: b}, true
	case hclsyntax.OpLessThanOrEqual:
		return ruleConstraints{max: b}, true
	}
	return ruleConstraints{}, false
}

// regexallConstraints handles length(regexall("literal", var.X)) compared
// with a number in a way that requires at least one match, e.g. `> 0`.
func regexallConstraints(varName string, e *hclsyntax.BinaryOpExpr) (ruleConstraints, bool) {
	c, ok := splitComparison(e)
	if !ok || !requiresAtLeastOne(c.op, c.number) {
		return ruleConstraints{}, false
	}
	pattern, ok := lengthOfRegexall(varName, c.subject)
	if !ok {
		return ruleConstraints{}, false
	}
	return ruleConstraints{pattern: pattern}, true
}

// requiresAtLeastOne reports whether `count op n` can only hold when count >= 1.
func requiresAtLeastOne(op *hclsyntax.Operation, n float64) bool {
	switch op {
	case hclsyntax.OpGreaterThan:
		return n >= 0
	case hclsyntax.OpGreaterThanOrEqual, hclsyntax.OpEqual:
		return n >= 1
	case hclsyntax.OpNotEqual:
		return n == 0
	}
	return false
}

// lengthOfRegexall returns the pattern of length(regexall("literal", var.X)).
func lengthOfRegexall(varName string, expr hclsyntax.Expression) (string, bool) {
	outer, ok := unwrapParens(expr).(*hclsyntax.FunctionCallExpr)
	if !ok || outer.Name != "length" || len(outer.Args) != 1 || outer.ExpandFinal {
		return "", false
	}
	inner, ok := unwrapParens(outer.Args[0]).(*hclsyntax.FunctionCallExpr)
	if !ok || inner.Name != "regexall" {
		return "", false
	}
	return regexCallPattern(varName, inner)
}

// regexCallPattern returns the literal pattern of regex/regexall("literal", var.X).
// Terraform uses Go's RE2 engine, so the pattern must compile with regexp.
func regexCallPattern(varName string, e *hclsyntax.FunctionCallExpr) (string, bool) {
	if len(e.Args) != 2 || e.ExpandFinal || !isVarRef(varName, e.Args[1]) {
		return "", false
	}
	pattern, ok := literalString(e.Args[0])
	if !ok || pattern == "" {
		return "", false
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return "", false
	}
	return pattern, true
}

// comparison is `subject op number`, with the number moved to the right.
type comparison struct {
	subject hclsyntax.Expression
	op      *hclsyntax.Operation
	number  float64
}

// splitComparison returns the comparison when one side is a constant number,
// flipping the operator when the number is on the left.
func splitComparison(e *hclsyntax.BinaryOpExpr) (comparison, bool) {
	if n, ok := constantNumber(e.RHS); ok {
		return comparison{subject: e.LHS, op: e.Op, number: n}, true
	}
	n, ok := constantNumber(e.LHS)
	if !ok {
		return comparison{}, false
	}
	op, ok := flipOp(e.Op)
	if !ok {
		return comparison{}, false
	}
	return comparison{subject: e.RHS, op: op, number: n}, true
}

func flipOp(op *hclsyntax.Operation) (*hclsyntax.Operation, bool) {
	switch op {
	case hclsyntax.OpGreaterThan:
		return hclsyntax.OpLessThan, true
	case hclsyntax.OpGreaterThanOrEqual:
		return hclsyntax.OpLessThanOrEqual, true
	case hclsyntax.OpLessThan:
		return hclsyntax.OpGreaterThan, true
	case hclsyntax.OpLessThanOrEqual:
		return hclsyntax.OpGreaterThanOrEqual, true
	case hclsyntax.OpEqual, hclsyntax.OpNotEqual:
		return op, true
	}
	return nil, false
}

func unwrapParens(expr hclsyntax.Expression) hclsyntax.Expression {
	for {
		p, ok := expr.(*hclsyntax.ParenthesesExpr)
		if !ok {
			return expr
		}
		expr = p.Expression
	}
}

// isVarRef reports whether expr is exactly var.<varName>.
func isVarRef(varName string, expr hclsyntax.Expression) bool {
	st, ok := unwrapParens(expr).(*hclsyntax.ScopeTraversalExpr)
	if !ok || len(st.Traversal) != 2 {
		return false
	}
	root, ok := st.Traversal[0].(hcl.TraverseRoot)
	if !ok || root.Name != "var" {
		return false
	}
	attr, ok := st.Traversal[1].(hcl.TraverseAttr)
	return ok && attr.Name == varName
}

// isNullCheck reports whether expr is `var.<varName> <op> null` (either order).
func isNullCheck(varName string, expr hclsyntax.Expression, op *hclsyntax.Operation) bool {
	e, ok := unwrapParens(expr).(*hclsyntax.BinaryOpExpr)
	if !ok || e.Op != op {
		return false
	}
	return (isVarRef(varName, e.LHS) && isNullLiteral(e.RHS)) ||
		(isNullLiteral(e.LHS) && isVarRef(varName, e.RHS))
}

func isNullLiteral(expr hclsyntax.Expression) bool {
	lit, ok := unwrapParens(expr).(*hclsyntax.LiteralValueExpr)
	return ok && lit.Val.IsNull()
}

func boolLiteral(expr hclsyntax.Expression) (bool, bool) {
	lit, ok := unwrapParens(expr).(*hclsyntax.LiteralValueExpr)
	if !ok || lit.Val.IsNull() || !lit.Val.IsKnown() || lit.Val.Type() != cty.Bool {
		return false, false
	}
	return lit.Val.True(), true
}

// foldedVarRef reports whether expr is lower(var.X) or upper(var.X) and
// returns the matching case function.
func foldedVarRef(varName string, expr hclsyntax.Expression) (func(string) string, bool) {
	call, ok := unwrapParens(expr).(*hclsyntax.FunctionCallExpr)
	if !ok || call.ExpandFinal || len(call.Args) != 1 || !isVarRef(varName, call.Args[0]) {
		return nil, false
	}
	switch call.Name {
	case "lower":
		return strings.ToLower, true
	case "upper":
		return strings.ToUpper, true
	}
	return nil, false
}

// allFolded reports whether every value is a string already in the given case.
func allFolded(values []cty.Value, fold func(string) string) bool {
	for _, v := range values {
		if v.Type() != cty.String || fold(v.AsString()) != v.AsString() {
			return false
		}
	}
	return true
}

// constantNumber evaluates expr without variables or functions and returns it
// when it is a known finite number, e.g. `0` or `-1`.
func constantNumber(expr hclsyntax.Expression) (float64, bool) {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsKnown() || v.IsNull() || v.Type() != cty.Number {
		return 0, false
	}
	f, _ := v.AsBigFloat().Float64()
	if !isFinite(f) {
		return 0, false
	}
	return normalizeZero(f), true
}

// literalString returns expr when it is a plain string with no interpolation.
func literalString(expr hclsyntax.Expression) (string, bool) {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsKnown() || v.IsNull() || v.Type() != cty.String {
		return "", false
	}
	return v.AsString(), true
}

// literalPrimitiveList returns the elements of a literal list of strings,
// finite numbers or bools. Anything else, including null elements, returns false.
func literalPrimitiveList(expr hclsyntax.Expression) ([]cty.Value, bool) {
	v, diags := expr.Value(nil)
	if diags.HasErrors() || !v.IsWhollyKnown() || v.IsNull() {
		return nil, false
	}
	if t := v.Type(); !t.IsTupleType() && !t.IsListType() && !t.IsSetType() {
		return nil, false
	}
	var out []cty.Value
	for it := v.ElementIterator(); it.Next(); {
		_, elem := it.Element()
		if !isFinitePrimitive(elem) {
			return nil, false
		}
		out = append(out, elem)
	}
	return out, len(out) > 0
}

func isFinitePrimitive(v cty.Value) bool {
	if v.IsNull() || !v.Type().IsPrimitiveType() {
		return false
	}
	if v.Type() == cty.Number {
		f, _ := v.AsBigFloat().Float64()
		return isFinite(f)
	}
	return true
}

// ---------------------------------------------------------------------------
// metadata.yaml validators
// ---------------------------------------------------------------------------

// validatorTargetKeys are the validator inputs that name module variables.
var validatorTargetKeys = []string{"vars", "trigger", "triggers", "dependent"}

type metadataVarRules struct {
	rules    []ruleConstraints
	messages []string
}

// metadataVarInfo groups metadata.yaml validators by top-level input name.
// Every rule's message is attached to each input it names. Limits are only
// attached for rules that list the input itself under "vars", not a nested
// attribute of it or a local value, and are gated by the input's declared
// Terraform type in inputTypes.
func metadataVarInfo(rules []ValidationRule, inputTypes map[string]cty.Type) map[string]metadataVarRules {
	out := map[string]metadataVarRules{}
	for _, r := range rules {
		msg := strings.Join(strings.Fields(r.ErrorMessage), " ")
		for _, key := range validatorTargetKeys {
			for _, target := range targetNames(r.Inputs[key]) {
				name, nested := targetInput(target)
				if name == "" {
					continue
				}
				info := out[name]
				if msg != "" && !slices.Contains(info.messages, msg) {
					info.messages = append(info.messages, msg)
				}
				if key == "vars" && !nested {
					if rc, ok := metadataRuleConstraints(r, inputTypes[name]); ok {
						info.rules = append(info.rules, rc)
					}
				}
				out[name] = info
			}
		}
	}
	return out
}

// targetInput returns the module input a validator target refers to, and
// whether the target is a nested attribute of it. local.* targets return "".
func targetInput(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || strings.HasPrefix(target, "local.") {
		return "", false
	}
	name, _, nested := strings.Cut(target, ".")
	return name, nested
}

// metadataRuleConstraints maps one metadata.yaml validator to limits (or a
// null-rejection rule for required/cidr). Warning-level rules, mismatched
// input types, ranges over list length and regexes over joined variables are
// skipped.
func metadataRuleConstraints(r ValidationRule, t cty.Type) (ruleConstraints, bool) {
	if strings.EqualFold(r.Level, "warning") {
		return ruleConstraints{}, false
	}
	var rc ruleConstraints
	var ok bool
	switch r.Validator {
	case "allowed_enum":
		rc, ok = enumRule(t, r.Inputs)
	case "range":
		rc, ok = rangeRule(t, r.Inputs)
	case "regex":
		rc, ok = regexRule(t, r.Inputs)
	case "required":
		rc, ok = requiredNullRule(r.Inputs)
	case "cidr":
		rc, ok = cidrNullRule(r.Inputs)
	}
	if !ok {
		return ruleConstraints{}, false
	}
	if rc.hasLimits() {
		rc.source = constraintSourceMetadata
	}
	return rc, true
}

func requiredNullRule(inputs map[string]interface{}) (ruleConstraints, bool) {
	deprecated, ok := boolInput(inputs, "deprecated", false)
	if !ok {
		return ruleConstraints{null: nullUnknown}, true
	}
	if deprecated {
		return ruleConstraints{}, false
	}
	return ruleConstraints{null: nullRejected}, true
}

func cidrNullRule(inputs map[string]interface{}) (ruleConstraints, bool) {
	if raw, ok := inputs["object_key"]; ok && raw != nil && raw != "" {
		return ruleConstraints{}, false
	}
	allowNull, ok := boolInput(inputs, "allow_null", false)
	if !ok {
		return ruleConstraints{null: nullUnknown}, true
	}
	if allowNull {
		return ruleConstraints{}, false
	}
	return ruleConstraints{null: nullRejected}, true
}

// enumRule maps metadata.yaml allowed_enum. AllowedEnumValidator only checks
// string and number values (and IterateRuleTargets flattens lists on untyped
// inputs), so only cty.String and cty.Number inputs emit limits.
func enumRule(t cty.Type, inputs map[string]interface{}) (ruleConstraints, bool) {
	values, ok := enumValuesForType(t, inputs["allowed"])
	if !ok {
		return ruleConstraints{}, false
	}
	caseSensitive, ok := boolInput(inputs, "case_sensitive", true)
	if !ok {
		return ruleConstraints{}, false
	}
	allowNull, ok := boolInput(inputs, "allow_null", false)
	if !ok {
		return ruleConstraints{}, false
	}
	rc := ruleConstraints{allowed: values, hasAllowed: true, caseInsensitive: !caseSensitive, null: nullRejected}
	if allowNull {
		rc.null = nullAllowed
	}
	return rc, true
}

// rangeRule maps metadata.yaml range. RangeValidator only checks integer
// numbers and truncates float64 bounds via int(), so only cty.Number inputs
// with integral bounds emit limits.
func rangeRule(t cty.Type, inputs map[string]interface{}) (ruleConstraints, bool) {
	if t != cty.Number {
		return ruleConstraints{}, false
	}
	lengthCheck, ok := boolInput(inputs, "length_check", false)
	if !ok || lengthCheck {
		return ruleConstraints{}, false
	}
	minV, okMin := intInput(inputs, "min")
	maxV, okMax := intInput(inputs, "max")
	if !okMin || !okMax || (minV == nil && maxV == nil) {
		return ruleConstraints{}, false
	}
	rc := ruleConstraints{null: nullPassThrough}
	if minV != nil {
		rc.min = &bound{value: *minV}
	}
	if maxV != nil {
		rc.max = &bound{value: *maxV}
	}
	if boundsContradict(rc.min, rc.max) {
		return ruleConstraints{}, false
	}
	return rc, true
}

// regexRule maps metadata.yaml regex. RegexValidator skips non-string values,
// so only cty.String inputs emit limits.
func regexRule(t cty.Type, inputs map[string]interface{}) (ruleConstraints, bool) {
	if t != cty.String {
		return ruleConstraints{}, false
	}
	concat, ok := boolInput(inputs, "concat", false)
	if !ok || concat {
		return ruleConstraints{}, false
	}
	pattern, ok := inputs["pattern"].(string)
	if !ok || pattern == "" {
		return ruleConstraints{}, false
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return ruleConstraints{}, false
	}
	return ruleConstraints{pattern: pattern, null: nullPassThrough}, true
}

// targetNames reads variable names from a string, list of strings, or
// compound triggers map (sorted keys) in metadata.yaml validator inputs.
func targetNames(raw interface{}) []string {
	if m, ok := raw.(map[string]interface{}); ok {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}
	return stringList(raw)
}

// stringList reads a string or a list of strings from validator inputs.
// Returns nil if a list contains any non-string item, matching parseStringList.
func stringList(raw interface{}) []string {
	switch v := raw.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

// boolInput reads an optional bool. ok is false when the key holds anything
// other than a bool, so the rule is skipped instead of misread.
func boolInput(inputs map[string]interface{}, key string, def bool) (bool, bool) {
	raw, present := inputs[key]
	if !present {
		return def, true
	}
	b, ok := raw.(bool)
	return b, ok
}

// maxSafeJSONInt is 2^53, the largest integer representable exactly in float64.
const maxSafeJSONInt = 1 << 53

// intInput reads an optional integral bound matching RangeValidator's
// parseIntInput semantics without silently rounding fractional floats.
func intInput(inputs map[string]interface{}, key string) (*float64, bool) {
	raw, present := inputs[key]
	if !present {
		return nil, true
	}
	var f float64
	switch v := raw.(type) {
	case int:
		if v < -maxSafeJSONInt || v > maxSafeJSONInt {
			return nil, false
		}
		f = float64(v)
	case float64:
		if !isFinite(v) || math.Trunc(v) != v || math.Abs(v) > maxSafeJSONInt {
			return nil, false
		}
		f = normalizeZero(v)
	default:
		return nil, false
	}
	return &f, true
}

// enumValuesForType converts a metadata.yaml allowed_enum list into cty values
// matching t (cty.String or cty.Number) and AllowedEnumValidator's stringified
// comparison semantics.
func enumValuesForType(t cty.Type, raw interface{}) ([]cty.Value, bool) {
	if t != cty.String && t != cty.Number {
		return nil, false
	}
	items, ok := rawList(raw)
	if !ok {
		return nil, false
	}
	out := make([]cty.Value, 0, len(items))
	for _, item := range items {
		var val cty.Value
		if t == cty.String {
			val, ok = enumStringItem(item)
		} else {
			val, ok = enumNumberItem(item)
		}
		if !ok {
			return nil, false
		}
		out = append(out, val)
	}
	return out, true
}

func rawList(raw interface{}) ([]interface{}, bool) {
	switch v := raw.(type) {
	case []interface{}:
		return v, len(v) > 0
	case []string:
		if len(v) == 0 {
			return nil, false
		}
		out := make([]interface{}, len(v))
		for i, s := range v {
			out[i] = s
		}
		return out, true
	}
	return nil, false
}

func enumStringItem(item interface{}) (cty.Value, bool) {
	switch v := item.(type) {
	case string:
		return cty.StringVal(v), true
	case int, int64, bool:
		return cty.StringVal(fmt.Sprintf("%v", v)), true
	case float64:
		if !isFinite(v) {
			return cty.NilVal, false
		}
		return cty.StringVal(fmt.Sprintf("%v", v)), true
	}
	return cty.NilVal, false
}

func enumNumberItem(item interface{}) (cty.Value, bool) {
	switch v := item.(type) {
	case int:
		return cty.NumberIntVal(int64(v)), true
	case int64:
		return cty.NumberIntVal(v), true
	case float64:
		if !isFinite(v) {
			return cty.NilVal, false
		}
		nv := cty.NumberFloatVal(normalizeZero(v))
		if nv.AsBigFloat().Text('f', -1) != fmt.Sprintf("%v", v) {
			return cty.NilVal, false
		}
		return nv, true
	case string:
		return parseEnumNumberString(v)
	}
	return cty.NilVal, false
}

func parseEnumNumberString(s string) (cty.Value, bool) {
	nv, err := cty.ParseNumberVal(s)
	if err != nil {
		return cty.NilVal, false
	}
	bf := nv.AsBigFloat()
	f, _ := bf.Float64()
	if !isFinite(f) || (bf.Sign() == 0 && s != "0") || bf.Text('f', -1) != s {
		return cty.NilVal, false
	}
	return nv, true
}
