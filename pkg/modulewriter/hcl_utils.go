// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modulewriter

import (
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func escapeBlueprintVariables(hclBytes []byte) []byte {
	// Convert \$(not.variable) to $(not.variable)
	re := regexp.MustCompile(`\\\\\$\(`)
	return re.ReplaceAll(hclBytes, []byte(`$(`))
}

func escapeLiteralVariables(hclBytes []byte) []byte {
	// Convert \((not.variable)) to ((not.variable))
	re := regexp.MustCompile(`\\\\\(\(`)
	return re.ReplaceAll(hclBytes, []byte(`((`))
}

func writeHclAttributes(vars map[string]cty.Value, dst string) error {
	if err := createBaseFile(dst); err != nil {
		return fmt.Errorf("error creating variables file %v: %v", filepath.Base(dst), err)
	}

	// Create hcl body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// for each variable
	for _, k := range orderKeys(vars) {
		// Write attribute
		hclBody.SetAttributeValue(k, vars[k])
	}

	// Write file
	hclBytes := escapeLiteralVariables(hclFile.Bytes())
	hclBytes = escapeBlueprintVariables(hclBytes)
	err := appendHCLToFile(dst, hclBytes)
	if err != nil {
		return fmt.Errorf("error writing HCL to %v: %v", filepath.Base(dst), err)
	}
	return err
}
