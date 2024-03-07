// Copyright 2024 "Google LLC"
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
	"crypto/md5"
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"golang.org/x/exp/maps"
)

// Relative path from deployment group to the staging directory
const StagingDir = "../.ghpc/staged"

func (bp Blueprint) StagedFiles() map[string]string {
	return maps.Clone(bp.stagedFiles)
}

func (bp *Blueprint) makeGhpcStageImpl() func(src string) string {
	// Move implementation instantiation to a separate function for easier testing
	return func(src string) string {
		// NOTE: we can't perform file validation here, because evaluation can be performed
		// on expanded blueprints, and relative `src` will not be valid at that point.
		// NOTE: this function needs to be deterministic, regardless of the invocation context.
		hash := fmt.Sprintf("%x", md5.Sum([]byte(src)))[:10]
		name := filepath.Base(src)
		if name == "." || name == ".." || filepath.ToSlash(name) == "/" {
			name = "file"
		}
		dst := filepath.Join(StagingDir, fmt.Sprintf("%s_%s", name, hash))

		if bp.stagedFiles == nil {
			bp.stagedFiles = map[string]string{}
		}
		bp.stagedFiles[src] = dst
		return dst
	}
}

// closure to capture blueprint
func (bp *Blueprint) makeGhpcStageFunc() function.Function {
	impl := bp.makeGhpcStageImpl()
	return function.New(&function.Spec{
		Description: `Copy file into the deployment directory to make it available for deployment`,
		Params:      []function.Parameter{{Name: "path", Type: cty.String}},
		Type:        function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			src := args[0].AsString()
			dst := impl(src)
			return cty.StringVal(dst), nil
		},
	})
}

// Validate that there `ghpc_stage` is only used in `vars` declarations
func (bp Blueprint) validateNoGhpcStageFuncs() error {
	errs := Errors{}
	// check modules
	bp.WalkModules(func(mp ModulePath, m *Module) error {
		for k, v := range m.Settings.Items() {
			errs.Add(validateNoGhpcStageFuncsInValue(mp.Settings.Dot(k), v))
		}
		return nil
	})
	// TODO: check terraform backends and validators inputs
	return errs.OrNil()
}

func validateNoGhpcStageFuncsInValue(vp ctyPath, val cty.Value) error {
	err := HintError{
		Err:  errors.New("ghpc_stage function can only be used in deployment Vars declarations"),
		Hint: "declare dedicated deployment variable and reference it here"}

	errs := Errors{}
	cty.Walk(val, func(p cty.Path, v cty.Value) (bool, error) {
		exp, is := IsExpressionValue(v)
		if !is { // not an expression
			return true, nil
		}
		// naive check for `ghpc_stage` identity tokens
		for _, tok := range exp.Tokenize() {
			if tok.Type == hclsyntax.TokenIdent && string(tok.Bytes) == "ghpc_stage" {
				errs.At(vp.Cty(p), err)
			}
		}
		return true, nil
	})
	return errs.OrNil()
}
