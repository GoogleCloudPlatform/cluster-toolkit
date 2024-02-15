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

package cmd

import (
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/validators"
	"strings"
)

func findPos(path config.Path, ctx config.YamlCtx) (config.Pos, bool) {
	pos, ok := ctx.Pos(path)
	for !ok && path.Parent() != nil {
		path = path.Parent()
		pos, ok = ctx.Pos(path)
	}
	return pos, ok
}

func renderError(err error, ctx config.YamlCtx) string {
	switch te := err.(type) {
	case config.Errors:
		return renderMultiError(te, ctx)
	case validators.ValidatorError:
		return renderValidatorError(te, ctx)
	case config.HintError:
		return renderHintError(te, ctx)
	case config.BpError:
		return renderBpError(te, ctx)
	case config.PosError:
		return renderPosError(te, ctx)
	default:
		return fmt.Sprintf("%s: %s", boldRed("Error"), err)
	}
}

func renderMultiError(errs config.Errors, ctx config.YamlCtx) string {
	var sb strings.Builder
	for _, e := range errs.Errors {
		sb.WriteString(renderError(e, ctx))
		sb.WriteString("\n")
	}
	return sb.String()
}

func renderValidatorError(err validators.ValidatorError, ctx config.YamlCtx) string {
	title := boldRed(fmt.Sprintf("validator %q failed:", err.Validator))
	return fmt.Sprintf("%s\n%v\n", title, renderError(err.Err, ctx))
}

func renderHintError(err config.HintError, ctx config.YamlCtx) string {
	return fmt.Sprintf("%s\n%s: %s", renderError(err.Err, ctx), boldYellow("Hint"), err.Hint)
}

func renderBpError(err config.BpError, ctx config.YamlCtx) string {
	if pos, ok := findPos(err.Path, ctx); ok {
		posErr := config.PosError{Pos: pos, Err: err.Err}
		return renderPosError(posErr, ctx)
	}
	return renderError(err.Err, ctx)
}

func renderPosError(err config.PosError, ctx config.YamlCtx) string {
	pos := err.Pos
	line := pos.Line - 1
	if line < 0 || line >= len(ctx.Lines) {
		return renderError(err, ctx)
	}

	pref := fmt.Sprintf("%d: ", pos.Line)
	arrow := " "
	if pos.Column > 0 {
		spaces := strings.Repeat(" ", len(pref)+pos.Column-1)
		arrow = spaces + "^"
	}

	return fmt.Sprintf("%s\n%s%s\n%s", renderError(err.Err, ctx), pref, ctx.Lines[line], arrow)
}
