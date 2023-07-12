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
	"strings"
)

// BpError is an error wrapper to augment Path
type BpError struct {
	Path Path
	Err  error
}

func (e BpError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Err)
}

func (e BpError) Unwrap() error {
	return e.Err
}

// InvalidSettingError signifies a problem with the supplied setting name in a
// module definition.
type InvalidSettingError struct {
	cause string
}

func (err *InvalidSettingError) Error() string {
	return fmt.Sprintf("invalid setting provided to a module, cause: %v", err.cause)
}

// Errors is an error wrapper to combine multiple errors
type Errors struct {
	Errors []error
}

func (e Errors) Error() string {
	errs := make([]string, len(e.Errors))
	for i, err := range e.Errors {
		errs[i] = err.Error()
	}
	return fmt.Sprintf("%d errors encountered:\n:%s", len(e.Errors), strings.Join(errs, "\n"))
}

// OrNil returns nil if there are no errors, otherwise returns itself
func (e Errors) OrNil() error {
	switch len(e.Errors) {
	case 0:
		return nil
	case 1:
		return e.Errors[0]
	default:
		return e
	}
}

// Add adds an error to the Errors and returns itself
func (e *Errors) Add(err error) *Errors {
	if err == nil {
		return e
	}
	if multi, ok := err.(*Errors); ok {
		e.Errors = append(e.Errors, multi.Errors...)
	} else {
		e.Errors = append(e.Errors, err)
	}
	return e
}

// At is convenience method to conditionally add an error, if one is not nil,
// augmented with a supplied Path.
func (e *Errors) At(path Path, err error) *Errors {
	if err == nil {
		return e
	}
	return e.Add(BpError{Path: path, Err: err})
}
