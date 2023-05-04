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
	Module    ModuleID // should be empty if GlobalVar. otherwise required
	Name      string   // required
}

// GlobalRef returns a reference to a global variable
func GlobalRef(n string) Reference {
	return Reference{GlobalVar: true, Name: n}
}

// ModuleRef returns a reference to a module output
func ModuleRef(m ModuleID, n string) Reference {
	return Reference{Module: m, Name: n}
}

// AsExpression returns a expression that represents the reference
func (r Reference) AsExpression() Expression {
	if r.GlobalVar {
		return MustParseExpression(fmt.Sprintf("var.%s", r.Name))
	}
	return MustParseExpression(fmt.Sprintf("module.%s.%s", r.Module, r.Name))
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
func SimpleVarToReference(s string) (Reference, error) {
	fmtErr := VarFormatError{s}
	if !hasVariable(s) {
		return Reference{}, fmtErr
	}
	if !isSimpleVariable(s) {
		return Reference{}, MakeStringInterpolationError(s)
	}
	contents := simpleVariableExp.FindStringSubmatch(s)
	if len(contents) != 2 { // Should always be (match, contents) here
		return Reference{}, fmtErr
	}
	components := strings.Split(contents[1], ".")
	if len(components) != 2 {
		return Reference{}, fmtErr
	}
	if components[0] == "vars" {
		return Reference{
			GlobalVar: true,
			Name:      components[1]}, nil
	}
	return Reference{
		Module: ModuleID(components[0]),
		Name:   components[1]}, nil
}

// SimpleVarToExpression takes a string `$(...)` and transforms it to `Expression`
func SimpleVarToExpression(s string) (Expression, error) {
	ref, err := SimpleVarToReference(s)
	if err != nil {
		return nil, err
	}
	var ex Expression
	if ref.GlobalVar {
		ex = MustParseExpression(fmt.Sprintf("var.%s", ref.Name))
	} else {
		ex = MustParseExpression(fmt.Sprintf("module.%s.%s", ref.Module, ref.Name))
	}
	return ex, nil
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
		return GlobalRef(n), nil
	case "module":
		m, err := getAttrName(1)
		if err != nil {
			return Reference{}, fmt.Errorf("expected second component of module var reference to be a module name, got %w", err)
		}
		n, err := getAttrName(2)
		if err != nil {
			return Reference{}, fmt.Errorf("expected third component of module var reference to be a variable name, got %w", err)
		}
		return ModuleRef(ModuleID(m), n), nil
	default:
		return Reference{}, fmt.Errorf("unexpected first component of reference: %#v", root)
	}
}

// IsYamlExpressionLiteral checks if passed value of type cty.String
// and its content starts with "((" and ends with "))".
// Returns trimmed string and result of test.
func IsYamlExpressionLiteral(v cty.Value) (string, bool) {
	if v.Type() != cty.String {
		return "", false
	}
	s := v.AsString()
	if len(s) < 4 || s[:2] != "((" || s[len(s)-2:] != "))" {
		return "", false
	}
	return s[2 : len(s)-2], true
}

// Expression is a representation of expressions in Blueprint
type Expression interface {
	// Eval evaluates the expression in the context of Blueprint
	Eval(bp Blueprint) (cty.Value, error)
	// Tokenize returns Tokens to be used for marshalling HCL
	Tokenize() hclwrite.Tokens
	// References return Reference for all variables used in the expression
	References() []Reference
	// AsValue returns a cty.Value that represents the expression.
	// This function is the ONLY way to get an Expression as a cty.Value,
	// do not attempt to build it by other means.
	AsValue() cty.Value
	// makeYamlExpressionValue returns a cty.Value, that is rendered as
	// HCL literal in Blueprint syntax. Returned value isn't functional,
	// as it doesn't reference an Expression.
	// This method should only be used for marshaling Blueprint YAML.
	makeYamlExpressionValue() cty.Value
	// key returns unique identifier of this expression in universe of all possible expressions.
	// `ex1.key() == ex2.key()` => `ex1` and `ex2` are identical.
	key() expressionKey
}

// ParseExpression returns Expression
func ParseExpression(s string) (Expression, error) {
	pos := hcl.Pos{Column: 2} // set offset to 2 to account for "((" in literal expressions
	e, diag := hclsyntax.ParseExpression([]byte(s), "", pos)
	if diag.HasErrors() {
		return nil, diag
	}
	sToks, _ := hclsyntax.LexExpression([]byte(s), "", hcl.Pos{})
	wToks := make(hclwrite.Tokens, len(sToks))
	for i, st := range sToks {
		wToks[i] = &hclwrite.Token{Type: st.Type, Bytes: st.Bytes}
	}

	ts := e.Variables()
	rs := make([]Reference, len(ts))
	for i, t := range ts {
		var err error
		if rs[i], err = TraversalToReference(t); err != nil {
			return nil, err
		}
	}
	return BaseExpression{e: e, toks: wToks, rs: rs}, nil
}

// MustParseExpression is "errorless" version of ParseExpression
// NOTE: only use it if passed expression is guaranteed to be correct
func MustParseExpression(s string) Expression {
	if exp, err := ParseExpression(s); err != nil {
		panic(fmt.Errorf("error while parsing %#v: %w", s, err))
	} else {
		return exp
	}
}

// BaseExpression is a base implementation of Expression interface
type BaseExpression struct {
	// Those fields should be accessed by Expression methods ONLY.
	e    hclsyntax.Expression
	toks hclwrite.Tokens
	rs   []Reference
}

// Eval evaluates the expression in the context of Blueprint
func (e BaseExpression) Eval(bp Blueprint) (cty.Value, error) {
	ctx := hcl.EvalContext{
		Variables: map[string]cty.Value{"var": bp.Vars.AsObject()},
	}
	v, diag := e.e.Value(&ctx)
	if diag.HasErrors() {
		return cty.NilVal, diag
	}
	return v, nil
}

// Tokenize returns Tokens to be used for marshalling HCL
func (e BaseExpression) Tokenize() hclwrite.Tokens {
	return e.toks
}

// References return Reference for all variables used in the expression
func (e BaseExpression) References() []Reference {
	c := make([]Reference, len(e.rs))
	for i, r := range e.rs {
		c[i] = r
	}
	return c
}

// makeYamlExpressionValue returns a cty.Value, that is rendered as
// HCL literal in Blueprint syntax. Returned value isn't functional,
// as it doesn't reference an Expression.
// This method should only be used for marshaling Blueprint YAML.
func (e BaseExpression) makeYamlExpressionValue() cty.Value {
	s := string(hclwrite.Format(e.Tokenize().Bytes()))
	return cty.StringVal("((" + s + "))")
}

// key returns unique identifier of this expression in universe of all possible expressions.
// `ex1.key() == ex2.key()` => `ex1` and `ex2` are identical.
func (e BaseExpression) key() expressionKey {
	s := string(e.Tokenize().Bytes())
	return expressionKey{k: s}
}

// AsValue returns a cty.Value that represents the expression.
// This function is the ONLY way to get an Expression as a cty.Value,
// do not attempt to build it by other means.
func (e BaseExpression) AsValue() cty.Value {
	k := e.key()
	// we don't care if ot overrides as expressions are identical
	globalExpressions[k] = e
	return cty.DynamicVal.Mark(k)
}

// To associate cty.Value with Expression we use cty.Value.Mark
// See: https://pkg.go.dev/github.com/zclconf/go-cty/cty#Value.Mark
// "Marks" should be of hashable type, sadly Expression isn't one.
// We work it around by using `expressionKey` - unique identifier
// of Expression and global map of used expressions `globalExpressions`.
// There are two guarantees to hold:
// * `ex1.key() == ex2.key()` => `ex1` and `ex2` are identical.
// It's achieved by using HCL literal as a key.
// * Every "mark" is in agreement with `globalExpressions`.
// Achieved by declaring `Expression.AsValue()` as the ONLY way to produce "marks".
type expressionKey struct {
	k string
}

var globalExpressions = map[expressionKey]Expression{}

// IsExpressionValue checks if the value is result of `Expression.AsValue()`.
// Returns original expression and result of check.
// It will panic if the value is expression-marked but not a result of `Expression.AsValue()`
func IsExpressionValue(v cty.Value) (Expression, bool) {
	key, ok := HasMark[expressionKey](v)
	if !ok {
		return nil, false
	}
	expr, stored := globalExpressions[key]
	if !stored { // shouldn't happen
		panic(fmt.Errorf("Expression isn't present in global state, while being referenced by value %#v", v))
	}
	return expr, true
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
