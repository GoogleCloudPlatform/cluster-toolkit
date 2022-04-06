// Copyright 2021 Google LLC
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

package reswriter

import (
	"fmt"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

func writeHclAttributes(vars map[string]cty.Value, dst string) error {
	if err := createBaseFile(dst); err != nil {
		return fmt.Errorf("error creating variables file %v: %v", filepath.Base(dst), err)
	}

	// Create hcl body
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()

	// for each variable
	for k, v := range vars {
		// Write attribute
		hclBody.SetAttributeValue(k, v)
	}

	// Write file
	err := appendHCLToFile(dst, hclFile.Bytes())
	if err != nil {
		return fmt.Errorf("error writing HCL to %v: %v", filepath.Base(dst), err)
	}
	return err
}
