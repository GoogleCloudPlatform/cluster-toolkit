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
	"encoding/json"
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/gocty"
	ctyJson "github.com/zclconf/go-cty/cty/json"
	"gopkg.in/yaml.v3"
)

// Dict maps string key to cty.Value.
// Zero Dict value is initialized (as oposed to nil map).
type Dict struct {
	m map[string]cty.Value
}

// NewDict constructor
func NewDict(m map[string]cty.Value) Dict {
	d := Dict{}
	for k, v := range m {
		d.Set(k, v)
	}
	return d
}

// Get returns stored value or cty.NilVal.
func (d *Dict) Get(k string) cty.Value {
	if d.m == nil {
		return cty.NilVal
	}
	return d.m[k]
}

// Has tests if key is present in map.
func (d *Dict) Has(k string) bool {
	if d.m == nil {
		return false
	}
	_, ok := d.m[k]
	return ok
}

// Set adds/overrides value by key.
// Returns reference to Dict-self.
func (d *Dict) Set(k string, v cty.Value) *Dict {
	if d.m == nil {
		d.m = map[string]cty.Value{k: v}
	} else {
		d.m[k] = v
	}
	return d
}

// Items returns instance of map[string]cty.Value
// will same set of key-value pairs as stored in Dict.
// This map is a copy, changes to returned map have no effect on the Dict.
func (d *Dict) Items() map[string]cty.Value {
	m := map[string]cty.Value{}
	if d.m != nil {
		for k, v := range d.m {
			m[k] = v
		}
	}
	return m
}

// AsObject returns Dict as cty.ObjectVal
func (d *Dict) AsObject() cty.Value {
	return cty.ObjectVal(d.Items())
}

// YamlValue is wrapper around cty.Value to handle YAML unmarshal.
type YamlValue struct {
	v cty.Value
}

// Unwrap returns wrapped cty.Value.
func (y YamlValue) Unwrap() cty.Value {
	return y.v
}

// UnmarshalYAML implements custom YAML unmarshaling.
func (y *YamlValue) UnmarshalYAML(n *yaml.Node) error {
	var err error
	switch n.Kind {
	case yaml.ScalarNode:
		err = y.unmarshalScalar(n)
	case yaml.MappingNode:
		err = y.unmarshalObject(n)
	case yaml.SequenceNode:
		err = y.unmarshalTuple(n)
	default:
		err = fmt.Errorf("line %d: cannot decode node with unknown kind %d", n.Line, n.Kind)
	}
	return err
}

func (y *YamlValue) unmarshalScalar(n *yaml.Node) error {
	var s interface{}
	if err := n.Decode(&s); err != nil {
		return err
	}
	ty, err := gocty.ImpliedType(s)
	if err != nil {
		return err
	}
	if y.v, err = gocty.ToCtyValue(s, ty); err != nil {
		return err
	}

	if l, is := IsYamlExpressionLiteral(y.v); is { // HCL literal
		var e Expression
		if e, err = ParseExpression(l); err != nil {
			return err
		}
		y.v = e.AsValue()
	} else if y.v.Type() == cty.String && hasVariable(y.v.AsString()) { // "simple" variable
		e, err := SimpleVarToExpression(y.v.AsString())
		if err != nil {
			return err
		}
		y.v = e.AsValue()
	}
	return nil
}

func (y *YamlValue) unmarshalObject(n *yaml.Node) error {
	var my map[string]YamlValue
	if err := n.Decode(&my); err != nil {
		return err
	}
	mv := map[string]cty.Value{}
	for k, y := range my {
		mv[k] = y.v
	}
	y.v = cty.ObjectVal(mv)
	return nil
}

func (y *YamlValue) unmarshalTuple(n *yaml.Node) error {
	var ly []YamlValue
	if err := n.Decode(&ly); err != nil {
		return err
	}
	lv := []cty.Value{}
	for _, y := range ly {
		lv = append(lv, y.v)
	}
	y.v = cty.TupleVal(lv)
	return nil
}

// UnmarshalYAML implements custom YAML unmarshaling.
func (d *Dict) UnmarshalYAML(n *yaml.Node) error {
	var m map[string]YamlValue
	if err := n.Decode(&m); err != nil {
		return err
	}
	for k, y := range m {
		d.Set(k, y.v)
	}
	return nil
}

// MarshalYAML implements custom YAML marshaling.
func (d Dict) MarshalYAML() (interface{}, error) {
	o, _ := cty.Transform(d.AsObject(), func(p cty.Path, v cty.Value) (cty.Value, error) {
		if e, is := IsExpressionValue(v); is {
			return e.makeYamlExpressionValue(), nil
		}
		return v, nil
	})

	j := ctyJson.SimpleJSONValue{Value: o}
	b, err := j.MarshalJSON()
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %v", err)
	}
	var g interface{}
	err = json.Unmarshal(b, &g)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON: %v", err)
	}
	return g, nil
}

// Eval returns a copy of this Dict, where all Expressions
// are evaluated and replaced by result of evaluation.
func (d Dict) Eval(bp Blueprint) (Dict, error) {
	var res Dict
	for k, v := range d.Items() {
		r, err := cty.Transform(v, func(p cty.Path, v cty.Value) (cty.Value, error) {
			if e, is := IsExpressionValue(v); is {
				return e.Eval(bp)
			}
			return v, nil
		})
		if err != nil {
			return Dict{}, fmt.Errorf("error while trying to evaluate %#v: %w", k, err)
		}
		res.Set(k, r)
	}
	return res, nil
}
