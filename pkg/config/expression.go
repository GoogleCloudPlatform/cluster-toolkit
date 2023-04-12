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

	"github.com/zclconf/go-cty/cty"
)

// Reference is data struct that represents a reference to a variable.
// Neither checks are performed, nor context is captured, just a structural
// representation of a reference text
type Reference struct {
	GlobalVar bool
	Group     string // should be empty if GlobalVar, otherwise optional
	Module    string // should be empty if GlobalVar. otherwise required
	Name      string // required
}

// SimpleVarToReference takes a string `$(...)` and transforms it to `Reference`
func SimpleVarToReference(s string) (Reference, error) {
	if !isSimpleVariable(s) {
		return Reference{}, fmt.Errorf(
			"got %s, variables \"$(...)\" within strings are not yet implemented. remove them or add a backslash to render literally", s)
	}
	contents := simpleVariableExp.FindStringSubmatch(s)
	if len(contents) != 2 { // Should always be (match, contents) here
		return Reference{}, fmt.Errorf("%s %s, failed to extract contents: %v",
			errorMessages["invalidVar"], s, contents)
	}
	components := strings.Split(contents[1], ".")
	switch len(components) {
	case 2:
		if components[0] == "vars" {
			return Reference{
				GlobalVar: true,
				Name:      components[1]}, nil
		}
		return Reference{
			Module: components[0],
			Name:   components[1]}, nil

	case 3:
		return Reference{
			Group:  components[0],
			Module: components[1],
			Name:   components[2]}, nil
	default:
		return Reference{}, fmt.Errorf(
			"expected either 2 or 3 compontens, got %d in %#v", len(components), s)
	}
}

// VariableTranslator is an interface that provides function
// to translate "simple" variable (`$(...)`) into HCL format
type VariableTranslator interface {
	TranlateSimpleToHcl(s string) (string, error)
}

// DoNotAllowVariablesTranslator does not do any translation, it raises an error if any varaibles are met
type DoNotAllowVariablesTranslator struct {
	VariableTranslator
}

// TranlateSimpleToHcl raises an error
func (t DoNotAllowVariablesTranslator) TranlateSimpleToHcl(s string) (string, error) {
	return "", fmt.Errorf("variables aren't allowed here, got %#v", s)
}

// TransformSimpleToHcl produces a new value from passed one, replacing all occurrence
// of simple variables `$(xxx)` with HCL ones `((yyy)`, using specified translator.
func TransformSimpleToHcl(val cty.Value, translator VariableTranslator) (cty.Value, error) {
	return cty.Transform(val, func(p cty.Path, v cty.Value) (cty.Value, error) {
		if v.Type() != cty.String || !hasVariable(v.AsString()) {
			return v, nil
		}
		h, err := translator.TranlateSimpleToHcl(v.AsString())
		return cty.StringVal(h), err
	})
}
