// Copyright 2026 "Google LLC"
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

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pkg/errors"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// Relative path from deployment group to the staging directory
const StagingDir = "../.ghpc/staged"

type StagedFile struct {
	AbsSrc string // absolute path
	RelDst string // relative (to deployment group folder) path
}

func (bp Blueprint) StagedFiles() []StagedFile {
	if len(bp.stagedFiles) == 0 {
		return nil
	}

	if bp.path == "" {
		panic("blueprint doesn't have known path, can't resolve staged files to absolute paths")
	}
	res := []StagedFile{}
	for src, dst := range bp.stagedFiles {
		if !filepath.IsAbs(src) { // make it absolute
			src = filepath.Join(filepath.Dir(bp.path), src)
		}
		res = append(res, StagedFile{AbsSrc: src, RelDst: dst})
	}
	return res
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
			name = "file" // shouldn't use this as a human readable name, replace with innocuous "file"
		}
		dst := filepath.Join(StagingDir, fmt.Sprintf("%s_%s", name, hash))

		if bp.stagedFiles == nil {
			bp.stagedFiles = map[string]string{}
		}
		bp.stagedFiles[src] = dst
		return dst
	}
}

// Makes an `ghpc_stage` function while capturing Blueprint
// in its closure to updade Blueprint state (stagedFiles)
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

// Partially evaluate all `ghpc_stage` expressions in the blueprint
func (bp *Blueprint) evalGhpcStage() error {
	errs := Errors{}
	ctx, err := bp.evalCtx()
	if err != nil {
		return err
	}

	bp.mutateDicts(func(dp dictPath, d *Dict) Dict {
		us := map[string]cty.Value{}
		for k, v := range d.Items() {
			uv, err := evalGhpcStageInValue(dp.Dot(k), v, ctx)
			errs.Add(err)
			us[k] = uv
		}
		return NewDict(us)
	})
	return errs.OrNil()
}

func evalGhpcStageInValue(pPref ctyPath, v cty.Value, ctx *hcl.EvalContext) (cty.Value, error) {
	return cty.Transform(v, func(pSuf cty.Path, v cty.Value) (cty.Value, error) {
		if e, is := IsExpressionValue(v); is {
			pe, err := partialEval(e, "ghpc_stage", ctx)
			if err != nil {
				return cty.NilVal, BpError{pPref.Cty(pSuf), err}
			}
			return pe.AsValue(), nil
		}
		return v, nil
	})
}

func partialEval(exp Expression, fn string, ctx *hcl.EvalContext) (Expression, error) {
	tail := exp.Tokenize()
	line := string(tail.Bytes())
	mutated := false
	acc := hclwrite.Tokens{}

	for len(tail) > 0 {
		tok := tail[0]
		if tok.Type != hclsyntax.TokenIdent || string(tok.Bytes) != fn {
			acc = append(acc, tok)
			tail = tail[1:]
			continue
		}

		var sub Expression
		var err error
		offset := len(line) - len(string(tail.Bytes()))
		sub, tail, err = trimFunctionCall(tail)
		if err != nil {
			return nil, prepareParseHclErr(err, line, offset)
		}

		for _, ref := range sub.References() {
			if !ref.GlobalVar {
				err = fmt.Errorf("function %q can only reference deployment variables, got %q", fn, ref)
				return nil, prepareParseHclErr(err, line, offset)
			}
		}

		v, err := sub.Eval(ctx)
		if err != nil {
			return nil, prepareParseHclErr(err, line, offset)
		}

		acc = append(acc, TokensForValue(v)...)
		mutated = true
	}

	if !mutated { // Not strictly necessary, but to avoid risky transformations, check if expression doesn't contain `ghpc_stage`, return as is
		return exp, nil
	}

	return ParseExpression(string(acc.Bytes()))
}

// Takes toks in form `fn(...)<TAIL>` and returns `fn(...)` and `<TAIL>`
func trimFunctionCall(toks hclwrite.Tokens) (Expression, hclwrite.Tokens, error) {
	if len(toks) < 3 {
		return nil, nil, errors.New("expected 'function_name(...)...'")
	}
	if toks[0].Type != hclsyntax.TokenIdent || toks[1].Type != hclsyntax.TokenOParen {
		return nil, nil, errors.New("expected 'function_name('")
	}

	// skip function name and opening parenthesis
	found, tail, err := greedyParseHcl(string(toks[2:].Bytes()))
	if err != nil {
		return nil, nil, err
	}

	exp, err := ParseExpression(fmt.Sprintf("%s(%s)", toks[0].Bytes, found))
	if err != nil {
		return nil, nil, err
	}
	tailToks, err := parseHcl(tail)
	return exp, tailToks, err
}
