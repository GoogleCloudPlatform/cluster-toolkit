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
	"reflect"

	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
)

// Path is unique identifier of a piece of configuration.
type Path interface {
	String() string
	Parent() Path
}

type basePath struct {
	InternalPrev  Path
	InternalPiece string
}

func (p basePath) Parent() Path { return p.InternalPrev }

func (p basePath) String() string {
	pref := ""
	if p.Parent() != nil {
		pref = p.Parent().String()
	}
	return fmt.Sprintf("%s%s", pref, p.InternalPiece)
}

type arrayPath[E any] struct{ basePath }

func (p arrayPath[E]) At(i int) E {
	var e E
	initPath(&e, &p, fmt.Sprintf("[%d]", i))
	return e
}

type mapPath[E any] struct{ basePath }

func (p mapPath[E]) Dot(k string) E {
	var e E
	initPath(&e, &p, fmt.Sprintf(".%s", k))
	return e
}

// ctyPath is a specialization of Path that can be extended with cty.Path
type ctyPath struct{ basePath }

// Cty builds a path chain that starts with p and each link corresponds to a step in cty.Path
// If any step in cty.Path is not supported, the path chain will be built up to that point.
// E.g.
// Root.Vars.Dot("alpha").Cty(cty.Path{}.IndexInt(6)) == "vars.alpha[6]"
func (p ctyPath) Cty(cp cty.Path) basePath {
	cur := p.basePath
	for _, s := range cp {
		prev := cur
		var nxt basePath
		piece, err := ctyStepToString(s)
		if err != nil {
			return cur // fall back to longest path build up to this point
		}
		initPath(&nxt, &prev, piece)
		cur = nxt
	}
	return cur
}

func ctyStepToString(s cty.PathStep) (string, error) {
	switch s := s.(type) {
	case cty.GetAttrStep:
		return fmt.Sprintf(".%s", s.Name), nil // equivalent to mapPath.Dot
	case cty.IndexStep:
		switch s.Key.Type() {
		case cty.Number:
			return fmt.Sprintf("[%s]", s.Key.AsBigFloat().String()), nil // equivalent to arrayPath.At
		case cty.String:
			return fmt.Sprintf(".%s", s.Key.AsString()), nil // equivalent to mapPath.Dot
		default:
			return "", errors.New("key value not number or string")
		}
	default:
		return "", errors.Errorf("unknown cty.PathStep type: %#v", s)
	}
}

// initPath walks through all child paths of p and initializes them. E.g.
// initPath(&Root, nil, "") will trigger
// -> initPath(&Root.BlueprintName, &Root, "blueprint_name")
func initPath(p any, prev any, piece string) {
	r := reflect.Indirect(reflect.ValueOf(p))
	ty := reflect.TypeOf(p).Elem()
	if !r.FieldByName("InternalPiece").IsValid() || !r.FieldByName("InternalPrev").IsValid() {
		panic(fmt.Sprintf("%s does not embed basePath", ty.Name()))
	}
	if _, ok := prev.(Path); prev != nil && !ok {
		panic(fmt.Sprintf("prev is not a Path: %#v", p))
	}

	r.FieldByName("InternalPiece").SetString(piece)
	if prev != nil {
		r.FieldByName("InternalPrev").Set(reflect.ValueOf(prev))
	}

	for i := 0; i < ty.NumField(); i++ {
		tag, ok := ty.Field(i).Tag.Lookup("path")
		if !ok {
			continue
		}
		initPath(r.Field(i).Addr().Interface(), p, tag)
	}
}

type rootPath struct {
	basePath
	BlueprintName   basePath                    `path:"blueprint_name"`
	GhpcVersion     basePath                    `path:"ghpc_version"`
	Validators      arrayPath[validatorCfgPath] `path:"validators"`
	ValidationLevel basePath                    `path:"validation_level"`
	Vars            dictPath                    `path:"vars"`
	Groups          arrayPath[groupPath]        `path:"deployment_groups"`
	Backend         backendPath                 `path:"terraform_backend_defaults"`
}

type validatorCfgPath struct {
	basePath
	Validator basePath `path:".validator"`
	Inputs    dictPath `path:".inputs"`
	Skip      basePath `path:".skip"`
}

type dictPath struct{ mapPath[ctyPath] }

type backendPath struct {
	basePath
	Type          basePath `path:".type"`
	Configuration dictPath `path:".configuration"`
}

type groupPath struct {
	basePath
	Name    basePath              `path:".group"`
	Backend backendPath           `path:".terraform_backend"`
	Modules arrayPath[modulePath] `path:".modules"`
}

type modulePath struct {
	basePath
	Source   basePath              `path:".source"`
	Kind     basePath              `path:".kind"`
	ID       basePath              `path:".id"`
	Use      arrayPath[basePath]   `path:".use"`
	Outputs  arrayPath[outputPath] `path:".outputs"`
	Settings dictPath              `path:".settings"`
}

type outputPath struct {
	basePath
	Name        basePath `path:".name"`
	Description basePath `path:".description"`
	Sensitive   basePath `path:".sensitive"`
}

// Root is a starting point for creating a Blueprint Path
var Root rootPath

// internalPath is to be used to report problems outside of Blueprint schema (e.g. YAML parsing error position)
var internalPath = mapPath[basePath]{basePath{nil, "__internal_path__"}}

func init() {
	initPath(&Root, nil, "")
}
