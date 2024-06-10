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

package inspect

import (
	"fmt"
	"hpc-toolkit/pkg/modulereader"
	"regexp"

	"github.com/zclconf/go-cty/cty"
)

func walkFields(p string, ty cty.Type, pattern *regexp.Regexp, cb func(string, cty.Type)) {
	if ty.IsObjectType() {
		for key, ety := range ty.AttributeTypes() {
			pref := fmt.Sprintf("%s.%s", p, key)
			walkFields(pref, ety, pattern, cb)
			if pattern.MatchString(key) {
				cb(pref, ety)
			}
		}
	}
	if ty.IsListType() || ty.IsMapType() || ty.IsSetType() {
		walkFields(p+"[*]", ty.ElementType(), pattern, cb)
	}
	if ty.IsTupleType() {
		for i, ety := range ty.TupleElementTypes() {
			walkFields(fmt.Sprintf("%s[%d]", p, i), ety, pattern, cb)
		}
	}
}

func FindField(inputs []modulereader.VarInfo, field string) map[string]cty.Type {
	pattern := regexp.MustCompile(field)
	res := map[string]cty.Type{}
	for _, input := range inputs {
		pref := input.Name
		walkFields(pref, input.Type, pattern, func(p string, ty cty.Type) {
			res[p] = ty
		})
		if pattern.MatchString(input.Name) {
			res[pref] = input.Type
		}
	}
	return res
}
