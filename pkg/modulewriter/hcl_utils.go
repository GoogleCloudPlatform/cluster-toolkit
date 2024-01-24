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
	"hpc-toolkit/pkg/config"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
)

// WriteHclAttributes writes tfvars/pkvars.hcl files
func WriteHclAttributes(vars map[string]cty.Value, dst string) error {
	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()
	for _, k := range orderKeys(vars) {
		hclBody.AppendNewline()
		toks := config.TokensForValue(vars[k])
		hclBody.SetAttributeRaw(k, toks)
	}
	return writeHclFile(dst, hclFile)
}
