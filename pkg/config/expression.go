// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package config

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// Reference is data struct that represents a reference to a variable.
// Neither checks are performed, nor context is captured, just a structural
// representation of a reference text
type Reference struct {
	GlobalVar bool
	Module    string // should be empty if GlobalVar. otherwise required
	Name      string // required
}

// MakeStringInterpolationError generates an error message guiding the user to proper escape syntax
func MakeStringInterpolationError(s string) error {
	matchall := anyVariableExp.FindAllString(s, -1)
	hint := ""
	for _, element := range matchall {
		// the regex match will include the first matching character
		// this might be (1) "^" or (2) any character EXCEPT "\"
		// if (2), we have to remove the first character from the match
		if element[0:2] != "$(" {
			element = strings.Replace(element, element[0:1], "", 1)
		}
		hint += "\\" + element + " will be rendered as " + element + "\n"
	}
	return fmt.Errorf(
		"variables \"$(...)\" within strings are not yet implemented. remove them or add a backslash to render literally. \n%s", hint)
}

// SimpleVarToReference takes a string `$(...)` and transforms it to `Reference`
// Returns `Reference` and name of explicit group if one was set.
func SimpleVarToReference(s string) (Reference, string, error) {
	if !hasVariable(s) {
		return Reference{}, "", fmt.Errorf("%#v is not a variable", s)
	}
	if !isSimpleVariable(s) {
		return Reference{}, "", MakeStringInterpolationError(s)
	}
	contents := simpleVariableExp.FindStringSubmatch(s)
	if len(contents) != 2 { // Should always be (match, contents) here
		return Reference{}, "", fmt.Errorf("%s %s, failed to extract contents: %v",
			errorMessages["invalidVar"], s, contents)
	}
	components := strings.Split(contents[1], ".")
	switch len(components) {
	case 2:
		if components[0] == "vars" {
			return Reference{
				GlobalVar: true,
				Name:      components[1]}, "", nil
		}
		return Reference{
			Module: components[0],
			Name:   components[1]}, "", nil

	case 3:
		return Reference{
			Module: components[1],
			Name:   components[2]}, components[0], nil
	default:
		return Reference{}, "", fmt.Errorf(
			"expected either 2 or 3 components, got %d in %#v", len(components), s)
	}
}

// SimpleVarToHclExpression takes a string `$(...)` and transforms it to `HclExpression`
// Returns `HclExpression` and name of explicit group if one was set.
func SimpleVarToHclExpression(s string) (HclExpression, string, error) {
	ref, gr, err := SimpleVarToReference(s)
	if err != nil {
		return HclExpression{}, "", err
	}
	var ex HclExpression
	if ref.GlobalVar {
		ex, err = ParseExpression(fmt.Sprintf("var.%s", ref.Name))
	} else {
		ex, err = ParseExpression(fmt.Sprintf("module.%s.%s", ref.Module, ref.Name))
	}
	if err != nil {
		return HclExpression{}, "", err
	}
	return ex, gr, nil
}

// TraversalToReference takes HCL traversal and returns `Reference`
func TraversalToReference(t hcl.Traversal) (Reference, error) {
	if t.IsRelative() {
		return Reference{}, fmt.Errorf("got relative traversal")
	}
	getAttrName := func(i int) (string, error) {
		if i >= len(t) {
			return "", fmt.Errorf("traversal does not have enough components")
		}
		if a, ok := t[i].(hcl.TraverseAttr); ok {
			return a.Name, nil
		}
		return "", fmt.Errorf("got unexpected traversal component: %#v", t[i])
	}
	switch root := t.RootName(); root {
	case "var":
		n, err := getAttrName(1)
		if err != nil {
			return Reference{}, fmt.Errorf("expected second component of global var reference to be a variable name, got %w", err)
		}
		return Reference{GlobalVar: true, Name: n}, nil
	case "module":
		m, err := getAttrName(1)
		if err != nil {
			return Reference{}, fmt.Errorf("expected second component of module var reference to be a module name, got %w", err)
		}
		n, err := getAttrName(2)
		if err != nil {
			return Reference{}, fmt.Errorf("expected third component of module var reference to be a variable name, got %w", err)
		}
		return Reference{Module: m, Name: n}, nil
	default:
		return Reference{}, fmt.Errorf("unexpected first component of reference: %#v", root)
	}
}

// IsYamlHclLiteral checks if passed value of type cty.String
// and its content starts with "((" and ends with "))".
// Returns trimmed string and result of test.
func IsYamlHclLiteral(v cty.Value) (string, bool) {
	if v.Type() != cty.String {
		return "", false
	}
	s := v.AsString()
	if len(s) < 4 || s[:2] != "((" || s[len(s)-2:] != "))" {
		return "", false
	}
	return s[2 : len(s)-2], true
}

// HclExpression is a representation of HCL literals in Blueprint
type HclExpression struct {
	// Those fields should be accessed by HclExpression methods ONLY.
	e  hclsyntax.Expression
	s  string
	rs []Reference
}

// ParseExpression returns HclExpression
func ParseExpression(s string) (HclExpression, error) {
	e, diag := hclsyntax.ParseExpression([]byte(s), "", hcl.Pos{})
	if diag.HasErrors() {
		return HclExpression{}, diag
	}
	ts := e.Variables()
	rs := make([]Reference, len(ts))
	for i, t := range ts {
		var err error
		if rs[i], err = TraversalToReference(t); err != nil {
			return HclExpression{}, err
		}
	}
	return HclExpression{e: e, s: s, rs: rs}, nil
}

// MustParseExpression is "errorless" version of ParseExpression
// NOTE: only use it if passed expression is guaranteed to be correct
func MustParseExpression(s string) HclExpression {
	if exp, err := ParseExpression(s); err != nil {
		panic(fmt.Errorf("error while parsing %#v: %w", s, err))
	} else {
		return exp
	}
}

// Tokenize returns Tokens to be used for marshalling HCL
func (e HclExpression) Tokenize() hclwrite.Tokens {
	return hclwrite.TokensForIdentifier(e.s)
}

// References return Reference for all variables used in the expression
func (e HclExpression) References() []Reference {
	c := make([]Reference, len(e.rs))
	for i, r := range e.rs {
		c[i] = r
	}
	return c
}

// makeYamlLiteralValue returns a cty.Value, that is rendered as
// HCL literal in Blueprint syntax. Returned value isn't functional,
// as it doesn't reference HclExpression.
// This method should only be used for marshaling Blueprint YAML.
func (e HclExpression) makeYamlLiteralValue() cty.Value {
	return cty.StringVal("((" + e.s + "))")
}

// To associate cty.Value with HclExpression we use cty.Value.Mark
// See: https://pkg.go.dev/github.com/zclconf/go-cty/cty#Value.Mark
// "Marks" should be of hashable type, sadly HclExpression isn't one.
// We work it around by using `hclExpressionKey` - unique identifier
// of HclExpression and global map of used expressions `globalHclExpressions`.
// There are two guarantees to hold:
// * `ex1.key() == ex2.key()` => `ex1` and `ex2` are identical.
// It's achieved by using HCL literal as a key.
// * Every "mark" is in agreement with `globalHclExpressions`.
// Achieved by declaring `HclExpression.AsValue()` as the ONLY way to produce "marks".
type hclExpressionKey struct {
	k string
}

var globalHclExpressions = map[hclExpressionKey]HclExpression{}

// key returns unique identifier of this expression in universe of all possible HCL expressions.
// `ex1.key() == ex2.key()` => `ex1` and `ex2` are identical.
func (e HclExpression) key() hclExpressionKey {
	return hclExpressionKey{k: e.s}
}

// AsValue returns a cty.Value that represents the expression.
// This function should be the ONLY way to get HCL expression as a cty.Value.
func (e HclExpression) AsValue() cty.Value {
	k := e.key()
	// we don't care if ot overrides as expressions are identical
	globalHclExpressions[k] = e
	return cty.DynamicVal.Mark(e.key())
}

// IsHclValue checks if the value is result of `HclExpression.AsValue()`.
// Returns original expression and result of check.
// It will panic if the value is HCL-marked but not a result of `HclExpression.AsValue()`
func IsHclValue(v cty.Value) (HclExpression, bool) {
	key, ok := HasMark[hclExpressionKey](v)
	if !ok {
		return HclExpression{}, false
	}
	expr, stored := globalHclExpressions[key]
	if !stored { // shouldn't happen
		panic(fmt.Errorf("HclExpression isn't present in global state, while being referenced by value %#v", v))
	}
	return expr, true
}

// ExplicitGroupMark is mark attached to HclExpression if expression
// was produced from "simple" variable and had group set explicitly.
// This mark has no effect on expression and only may be used for validation of
// original "simple" variable down the road.
type ExplicitGroupMark struct {
	Group string
}

// HasMark checks if cty.Value has mark of specified type T.
// Returns found mark and result of check.
// Panics if value has multiple marks of such type.
func HasMark[T any](val cty.Value) (T, bool) {
	var tgt T
	found := false
	for m := range val.Marks() {
		t, ok := m.(T)
		if !ok {
			continue
		}
		if found { // shouldn't happen
			panic(fmt.Errorf("more than one %T mark at %#v", tgt, val))
		}
		found, tgt = true, t
	}
	return tgt, found
}
