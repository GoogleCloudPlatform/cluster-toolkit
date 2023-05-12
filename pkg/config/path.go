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

import "fmt"

// Path is unique identifier of a piece of configuration.
type Path interface {
	String() string
	Parent() Path
}

type basePath struct {
	prev Path
	s    string
}

func (p basePath) Parent() Path { return p.prev }

func (p basePath) String() string {
	pref := ""
	if p.Parent() != nil {
		pref = p.Parent().String()
	}
	return fmt.Sprintf("%s%s", pref, p.s)
}

func (p *basePath) init(prev Path, s string) {
	if prev == nil {
		panic("prev cannot be nil")
	}
	p.prev = prev
	p.s = s
}

func (p basePath) child(s string) basePath {
	return basePath{prev: p, s: s}
}

// See https://go.googlesource.com/proposal/+/HEAD/design/43651-type-parameters.md#pointer-method-example
// For explanation of why this [T, PT] generic is needed.
type canInit[T any] interface {
	init(prev Path, s string)
	*T
}

func makeBasePath[T any, PT canInit[T]](prev Path, s string) T {
	p := new(T)
	r := PT(p)
	r.init(prev, s)
	return *p
}

type arrayPath[T Path, PT canInit[T]] struct{ basePath }
type baseArrayPath = arrayPath[basePath, *basePath]
type moduleArrayPath = arrayPath[modulePath, *modulePath]
type outputArrayPath = arrayPath[outputPath, *outputPath]
type validatorArrayPath = arrayPath[validatorPath, *validatorPath]
type groupArrayPath = arrayPath[groupPath, *groupPath]

func (p arrayPath[T, PT]) At(i int) T {
	return makeBasePath[T, PT](p, fmt.Sprintf("[%d]", i))
}

type mapPath[T Path, PT canInit[T]] struct{ basePath }
type baseMapPath = mapPath[basePath, *basePath]

func (p mapPath[T, PT]) At(k string) T {
	return makeBasePath[T, PT](p, fmt.Sprintf("[%s]", k))
}

type outputPath struct{ basePath }

func (p outputPath) Name() Path        { return p.child(".name") }
func (p outputPath) Description() Path { return p.child(".description") }
func (p outputPath) Sensitive() Path   { return p.child(".sensitive") }

type modulePath struct{ basePath }

// Intentionally do not include path `WrapSettingsWith`
func (p modulePath) Source() Path             { return p.child(".source") }
func (p modulePath) Kind() Path               { return p.child(".kind") }
func (p modulePath) ID() Path                 { return p.child(".id") }
func (p modulePath) Use() baseArrayPath       { return baseArrayPath{p.child(".use")} }
func (p modulePath) Outputs() outputArrayPath { return outputArrayPath{p.child(".outputs")} }
func (p modulePath) Settings() baseMapPath    { return baseMapPath{p.child(".settings")} }

type tbePath struct{ basePath }

func (p tbePath) Type() Path                 { return p.child(".type") }
func (p tbePath) Configuration() baseMapPath { return baseMapPath{p.child(".configuration")} }

type groupPath struct{ basePath }

func (p groupPath) Name() Path                { return p.child(".name") }
func (p groupPath) Kind() Path                { return p.child(".kind") }
func (p groupPath) Modules() moduleArrayPath  { return moduleArrayPath{p.child(".modules")} }
func (p groupPath) TerraformBackend() tbePath { return tbePath{p.child(".terraform_backend")} }

type validatorPath struct{ basePath }

func (p validatorPath) Validator() Path     { return p.child(".validator") }
func (p validatorPath) Skip() Path          { return p.child(".skip") }
func (p validatorPath) Inputs() baseMapPath { return baseMapPath{p.child(".inputs")} }

type rP struct{ basePath }

func (p rP) BlueprintName() Path               { return p.child("blueprint_name") }
func (p rP) GhpcVersion() Path                 { return p.child("ghpc_version") }
func (p rP) ValidationLevel() Path             { return p.child("validation_level") }
func (p rP) Vars() baseMapPath                 { return baseMapPath{p.child("vars")} }
func (p rP) Validators() validatorArrayPath    { return validatorArrayPath{p.child("validators")} }
func (p rP) DeploymentGroups() groupArrayPath  { return groupArrayPath{p.child("deployment_groups")} }
func (p rP) TerraformBackendDefaults() tbePath { return tbePath{p.child("terraform_backend_defaults")} }

func rootPath() rP { return rP{} }
