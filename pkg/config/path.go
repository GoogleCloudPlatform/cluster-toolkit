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
)

// Path is unique identifier of a piece of configuration.
type Paath interface {
	String() string
	Parent() Paath
}

type basePath struct {
	Prev  Paath
	Piece string
}

func (p basePath) Parent() Paath { return p.Prev }

func (p basePath) String() string {
	pref := ""
	if p.Parent() != nil {
		pref = p.Parent().String()
	}
	return fmt.Sprintf("%s%s", pref, p.Piece)
}

type arrayPath[E any] struct{ basePath }

func (p arrayPath[E]) At(i int) E {
	var e E
	initPath(&e, &p, fmt.Sprintf("[%d]", i))
	return e
}

type mapPath[E any] struct{ basePath }

func (p mapPath[E]) At(k string) E {
	var e E
	initPath(&e, &p, fmt.Sprintf("[%s]", k))
	return e
}

func initPath(p any, prev any, piece string) {
	// Couldn't figure out how constrain it using `initPath` signature
	if _, ok := p.(Paath); !ok {
		panic(fmt.Sprintf("p is not a Paath: %#v", p))
	}
	if _, ok := prev.(Paath); prev != nil && !ok {
		panic(fmt.Sprintf("prev is not a Paath: %#v", p))
	}

	if base, ok := p.(*basePath); ok {
		base.Piece = piece
		base.Prev = prev.(Paath)
		return
	}

	pref := reflect.Indirect(reflect.ValueOf(p))
	ty := reflect.TypeOf(p).Elem()

	// !!! Get base in a better way
	//  E.g.
	// bref, ok := pref.Field(0).Addr().Interface().(basePath)
	// if !ok {
	// 	panic(fmt.Sprintf("%s does not embed basePath", ty.Name()))
	// } else {
	// 	bref.Piece = piece
	// 	bref.Prev = prev.(Paath)
	// 	return
	// }

	base := pref.Field(0)
	base.FieldByName("Piece").SetString(piece)
	if prev != nil {
		base.FieldByName("Prev").Set(reflect.ValueOf(prev))
	}

	for i := 0; i < ty.NumField(); i++ {
		field := ty.Field(i)
		tag, ok := field.Tag.Lookup("path")
		if !ok {
			continue
		}
		ref, ok := pref.FieldByName(field.Name).Addr().Interface().(Paath)
		if !ok {
			panic(fmt.Sprintf("field %s.%s is not a Path", ty.Name(), field.Name))
		}
		initPath(ref, p, tag)
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

type dictPath struct{ mapPath[basePath] }

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
	Kind    basePath              `path:".kind"`
}

type modulePath struct {
	basePath
	Source   basePath               `path:".source"`
	Kind     basePath               `path:".kind"`
	ID       basePath               `path:".id"`
	Use      arrayPath[backendPath] `path:".use"`
	Outputs  arrayPath[outputPath]  `path:".outputs"`
	Settings dictPath               `path:".settings"`
}

type outputPath struct {
	basePath
	Name        basePath `path:".name"`
	Description basePath `path:".description"`
	Sensitive   basePath `path:".sensitive"`
}

var Root rootPath = rootPath{}

func init() {
	initPath(&Root, nil, "")
}
