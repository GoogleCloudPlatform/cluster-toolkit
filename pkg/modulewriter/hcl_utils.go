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

	"hpc-toolkit/pkg/config"

	"github.com/hashicorp/hcl/v2/hclsyntax"
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

// WriteHclAttributes writes tfvars/pkvars.hcl files
func WriteHclAttributes(vars map[string]cty.Value, dst string) error {
	if err := createBaseFile(dst); err != nil {
		return fmt.Errorf("error creating variables file %v: %v", filepath.Base(dst), err)
	}

	hclFile := hclwrite.NewEmptyFile()
	hclBody := hclFile.Body()
	for _, k := range orderKeys(vars) {
		hclBody.AppendNewline()
		hclBody.SetAttributeValue(k, vars[k])
	}

	hclBytes := escapeLiteralVariables(hclFile.Bytes())
	hclBytes = escapeBlueprintVariables(hclBytes)
	err := appendHCLToFile(dst, hclBytes)
	if err != nil {
		return fmt.Errorf("error writing HCL to %v: %v", filepath.Base(dst), err)
	}
	return err
}

// TokensForValue is a modification of hclwrite.TokensForValue.
// The only difference in behavior is handling "HCL literal" strings.
func TokensForValue(val cty.Value) hclwrite.Tokens {
	// We need to handle both cases, until all "expression" users are moved to Expression
	if e, is := config.IsExpressionValue(val); is {
		return e.Tokenize()
	} else if s, is := config.IsYamlExpressionLiteral(val); is { // return it "as is"
		return hclwrite.TokensForIdentifier(s)
	}

	ty := val.Type()
	if ty.IsListType() || ty.IsSetType() || ty.IsTupleType() {
		tl := []hclwrite.Tokens{}
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			tl = append(tl, TokensForValue(v))
		}
		return hclwrite.TokensForTuple(tl)
	}
	if ty.IsMapType() || ty.IsObjectType() {
		tl := []hclwrite.ObjectAttrTokens{}
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			kt := hclwrite.TokensForIdentifier(k.AsString())
			if !hclsyntax.ValidIdentifier(k.AsString()) {
				kt = TokensForValue(k)
			}
			vt := TokensForValue(v)
			tl = append(tl, hclwrite.ObjectAttrTokens{Name: kt, Value: vt})
		}
		return hclwrite.TokensForObject(tl)

	}
	return hclwrite.TokensForValue(val) // rely on hclwrite implementation
}
