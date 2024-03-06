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
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/maps"
)

// Dict maps string key to cty.Value.
// Zero Dict value is initialized (as opposed to nil map).
type Dict struct {
	m map[string]cty.Value
}

// NewDict constructor
func NewDict(m map[string]cty.Value) Dict {
	return Dict{m: maps.Clone(m)}
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

func (d Dict) With(k string, v cty.Value) Dict {
	m := d.Items()
	m[k] = v
	return Dict{m: m}
}

// Items returns instance of map[string]cty.Value
// will same set of key-value pairs as stored in Dict.
// This map is a copy, changes to returned map have no effect on the Dict.
func (d *Dict) Items() map[string]cty.Value {
	m := maps.Clone(d.m)
	if m == nil { // never return nil
		return map[string]cty.Value{}
	}
	return m
}

func (d *Dict) Keys() []string {
	return maps.Keys(d.m)
}

// AsObject returns Dict as cty.ObjectVal
func (d *Dict) AsObject() cty.Value {
	return cty.ObjectVal(d.Items())
}

// IsZero determine whether it should be omitted when YAML marshaling
// with the `omitempty“ flag.
func (d Dict) IsZero() bool {
	return len(d.m) == 0
}

// Eval returns a copy of this Dict, where all Expressions
// are evaluated and replaced by result of evaluation.
func (d Dict) Eval(bp Blueprint) (Dict, error) {
	res, err := bp.Eval(d.AsObject())
	if err != nil {
		return Dict{}, err
	}
	return NewDict(res.AsValueMap()), nil
}
