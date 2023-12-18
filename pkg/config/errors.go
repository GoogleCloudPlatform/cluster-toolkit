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

	"github.com/pkg/errors"
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

// HintError wraps another error to suggest other values
type HintError struct {
	Hint string
	Err  error
}

func (e HintError) Error() string {
	if len(e.Hint) > 0 {
		return fmt.Sprintf("%s - %s", e.Err, e.Hint)
	}
	return e.Err.Error()
}

func (e HintError) Unwrap() error {
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

// UnknownModuleError signifies a problem with the supplied module name.
type UnknownModuleError struct {
	ID ModuleID
}

func (e UnknownModuleError) Error() string {
	return fmt.Sprintf("invalid module id: \"%s\"", e.ID)
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

func (e *Errors) addDedup(err error) {
	msg := err.Error() // Do message comparison
	for _, e := range e.Errors {
		if msg == e.Error() {
			return
		}
	}
	e.Errors = append(e.Errors, err)
}

// Add adds an error to the Errors and returns itself
func (e *Errors) Add(err error) *Errors {
	if err == nil {
		return e
	}
	if multi, ok := err.(*Errors); ok {
		for _, c := range multi.Errors {
			e.addDedup(c)
		}
	} else {
		e.addDedup(err)
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

// Any returns true if there are any errors
func (e *Errors) Any() bool {
	return len(e.Errors) > 0
}

// Sentinel errors
var EmptyModuleID = errors.New("a module id cannot be empty")
var EmptyModuleSource = errors.New("a module source cannot be empty")
var InvalidModuleKind = errors.New("a module kind is invalid")
var UnknownModuleSetting = errors.New("a setting was added that is not found in the module")
var ModuleSettingWithPeriod = errors.New("a setting name contains a period, which is not supported; variable subfields cannot be set independently in a blueprint.")
var ModuleSettingInvalidChar = errors.New("a setting name must begin with a non-numeric character and all characters must be either letters, numbers, dashes ('-') or underscores ('_').")
var EmptyGroupName = errors.New("group name must be set for each deployment group")

// Error messages
const (
	errMsgFileLoadError    = string("failed to read the input yaml")
	errMsgYamlMarshalError = string("failed to export the configuration to a blueprint yaml file")
	errMsgYamlSaveError    = string("failed to write the expanded yaml")
	errMsgMissingSetting   = string("a required setting is missing from a module")
	errMsgInvalidVar       = string("invalid variable definition in")
	errMsgVarNotFound      = string("could not find source of variable")
	errMsgIntergroupOrder  = string("references to outputs from other groups must be to earlier groups")
	errMsgNoOutput         = string("output not found for a variable")
	errMsgCannotUsePacker  = string("Packer modules cannot be used by other modules")
	errMsgDuplicateGroup   = string("group names must be unique")
	errMsgDuplicateID      = string("module IDs must be unique")
	errMsgInvalidOutput    = string("requested output was not found in the module")
	errMsgValueNotString   = string("value was not of type string")
	errMsgValueEmptyString = string("value is an empty string")
	errMsgLabelNameReqs    = string("name must begin with a lowercase letter, can only contain lowercase letters, numeric characters, underscores and dashes, and must be between 1 and 63 characters long")
	errMsgLabelValueReqs   = string("value can only contain lowercase letters, numeric characters, underscores and dashes, and must be between 0 and 63 characters long")
)
