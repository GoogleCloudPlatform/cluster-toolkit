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

	"github.com/hashicorp/hcl/v2"
	"gopkg.in/yaml.v3"
)

// Pos represents position in blueprint file.
// Pos{} is used to represent unknown position.
type Pos struct {
	Line int
	Col  int
}

// PosError is an error wrapper with position in blueprint file.
type PosError struct {
	Pos Pos
	Err error
}

// Unwrap returns wrapped error.
func (e PosError) Unwrap() error { return e.Err }

func (e PosError) Error() string {
	var msg string
	switch te := e.Err.(type) {
	case hcl.Diagnostics: // special case for hcl.Diagnostic to not expose HCL position
		if len(te) > 0 {
			d := te[0]
			msg = fmt.Sprintf("%s; %s", d.Summary, d.Detail)
		} else {
			msg = te.Error()
		}
	default:
		msg = te.Error()
	}
	return fmt.Sprintf("line %d, col %d: %s", e.Pos.Line, e.Pos.Col, msg)
}

func wrapHclDiagnostics(p Pos, diags hcl.Diagnostics) PosError {
	if len(diags) > 0 {
		hp := diags[0].Subject.Start
		p = Pos{Line: p.Line + hp.Line, Col: p.Col + hp.Column}
	}
	return PosError{p, diags}
}

// AsPosError wraps error with position.
func AsPosError(p Pos, err error) PosError {
	switch e := err.(type) {
	case PosError:
		return e // Do not wrap PosError, preserve original position.
	case hcl.Diagnostics:
		return wrapHclDiagnostics(p, e)
	default:
		return PosError{p, err}
	}
}

// YamlError wraps error with position from yaml.Node.
func YamlError(n *yaml.Node, err error) PosError {
	return AsPosError(Pos{Line: n.Line, Col: n.Column}, err)
}

// VarFormatError is returned when variable has invalid format.
type VarFormatError struct {
	Got string
}

func (e VarFormatError) Error() string {
	return fmt.Sprintf("invalid variable format, expected $(vars.var_name) or $(module_id.output_name), got: %q", e.Got)
}
