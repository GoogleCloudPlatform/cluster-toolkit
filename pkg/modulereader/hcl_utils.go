// Copyright 2026 Google LLC
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

package modulereader

import (
	"errors"
	"fmt"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/sourcereader"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/zclconf/go-cty/cty"
)

// slashWrapFS intercepts calls from tfconfig and ensures all paths passed to
// the underlying io/fs.FS use forward slashes, as required by the io/fs.FS
// specification (which embed.FS strictly enforces). On Windows, tfconfig joins
// directory and filenames with filepath.Join, producing backslashes that cause
// embed.FS lookups to fail.
type slashWrapFS struct {
	tfconfig.FS
}

func (s slashWrapFS) Open(name string) (tfconfig.File, error) {
	return s.FS.Open(sourcereader.ToSlash(name))
}

func (s slashWrapFS) ReadFile(name string) ([]byte, error) {
	return s.FS.ReadFile(sourcereader.ToSlash(name))
}

func (s slashWrapFS) ReadDir(dirname string) ([]os.FileInfo, error) {
	return s.FS.ReadDir(sourcereader.ToSlash(dirname))
}

// LoadTFConfigModule loads a terraform module configuration using tfconfig.
// It supports both embedded modules via sourcereader.ModuleFS and local filesystem modules.
func LoadTFConfigModule(source string) (*tfconfig.Module, error) {
	if sourcereader.IsEmbeddedPath(source) && sourcereader.ModuleFS != nil {
		source = sourcereader.ToSlash(source)
		wrapFS := slashWrapFS{tfconfig.WrapFS(sourcereader.ModuleFS)}
		if !tfconfig.IsModuleDirOnFilesystem(wrapFS, source) {
			return nil, nil
		}
		mod, diags := tfconfig.LoadModuleFromFilesystem(wrapFS, source)
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to load module from filesystem at %s: %v", source, diags)
		}
		return mod, nil
	}

	fileInfo, err := os.Stat(source)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("source to module does not exist: %s", source)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read source of module: %s", source)
	}
	if !fileInfo.IsDir() {
		return nil, fmt.Errorf("source of module must be a directory: %s", source)
	}
	if !tfconfig.IsModuleDir(source) {
		return nil, nil
	}
	mod, diags := tfconfig.LoadModule(source)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to load module at %s: %v", source, diags)
	}
	return mod, nil
}

func getHCLInfo(source string) (ModuleInfo, error) {
	module, err := LoadTFConfigModule(source)
	if err != nil {
		return ModuleInfo{}, err
	}
	if module == nil {
		return ModuleInfo{}, fmt.Errorf("source is not a terraform module: %s", source)
	}

	ret := ModuleInfo{}
	var vars []VarInfo
	var outs []OutputInfo
	for _, v := range module.Variables {
		ty, err := GetCtyType(v.Type)
		if err != nil {
			return ModuleInfo{}, fmt.Errorf("failed to parse type of variable %q: %w", v.Name, err)
		}
		vInfo := VarInfo{
			Name:        v.Name,
			Type:        ty,
			Description: v.Description,
			Default:     v.Default,
			Required:    v.Required,
		}
		vars = append(vars, vInfo)
	}
	ret.Inputs = vars
	for _, v := range module.Outputs {
		oInfo := OutputInfo{
			Name:        v.Name,
			Description: v.Description,
		}
		outs = append(outs, oInfo)
	}
	ret.Outputs = outs
	return ret, nil
}

type hclVarMeta struct {
	deprecated    bool
	deprecatedMsg string
	validations   []string
}

// ApplyVarMeta returns a copy of mi with Deprecated, DeprecatedMsg, and
// Validations populated from the module's variable blocks. Only `gcluster
// explain` consumes these fields, keeping the extra HCL parse off the
// GetModuleInfo path used by create/expand/deploy and catalog-wide scans.
func ApplyVarMeta(source string, mi ModuleInfo) ModuleInfo {
	varMeta := extractHCLVarMeta(source)
	inputs := slices.Clone(mi.Inputs)
	for i := range inputs {
		meta := applyDescriptionDeprecation(inputs[i].Description, varMeta[inputs[i].Name])
		inputs[i].Deprecated = meta.deprecated
		inputs[i].DeprecatedMsg = meta.deprecatedMsg
		inputs[i].Validations = meta.validations
	}
	mi.Inputs = inputs
	return mi
}

func isVarFile(name string) bool {
	return strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".pkr.hcl")
}

func applyDescriptionDeprecation(desc string, meta hclVarMeta) hclVarMeta {
	trimmed := strings.TrimSpace(desc)
	if !meta.deprecated && strings.HasPrefix(strings.ToLower(trimmed), "deprecated") {
		meta.deprecated = true
	}
	if meta.deprecated && meta.deprecatedMsg == "" {
		meta.deprecatedMsg = strings.Join(strings.Fields(trimmed), " ")
	}
	return meta
}

func extractHCLVarMeta(source string) map[string]hclVarMeta {
	files := readModuleTFFiles(source)
	if len(files) == 0 {
		return nil
	}
	out := make(map[string]hclVarMeta)
	for _, tf := range files {
		parseFileVarMeta(tf.data, tf.name, out)
	}
	return out
}

type namedTFFile struct {
	name string
	data []byte
}

func readModuleTFFiles(source string) []namedTFFile {
	if sourcereader.IsEmbeddedPath(source) && sourcereader.ModuleFS != nil {
		return readEmbeddedTFFiles(sourcereader.ToSlash(source))
	}
	return readLocalTFFiles(source)
}

func readEmbeddedTFFiles(dir string) []namedTFFile {
	entries, err := sourcereader.ModuleFS.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []namedTFFile
	for _, e := range entries {
		if !e.IsDir() && isVarFile(e.Name()) {
			p := path.Join(dir, e.Name())
			if b, err := sourcereader.ModuleFS.ReadFile(p); err == nil {
				out = append(out, namedTFFile{name: e.Name(), data: b})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func readLocalTFFiles(dir string) []namedTFFile {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []namedTFFile
	for _, e := range entries {
		if !e.IsDir() && isVarFile(e.Name()) {
			p := filepath.Join(dir, e.Name())
			if b, err := os.ReadFile(p); err == nil {
				out = append(out, namedTFFile{name: e.Name(), data: b})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func parseFileVarMeta(src []byte, filename string, out map[string]hclVarMeta) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() || file == nil {
		return
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return
	}
	for _, block := range body.Blocks {
		if block.Type == "variable" && len(block.Labels) == 1 {
			name := block.Labels[0]
			if meta := parseVariableBlockMeta(src, name, block); meta.deprecated || len(meta.validations) > 0 {
				out[name] = meta
			}
		}
	}
}

func parseVariableBlockMeta(src []byte, varName string, block *hclsyntax.Block) hclVarMeta {
	var meta hclVarMeta
	for _, sub := range block.Body.Blocks {
		if sub.Type != "validation" {
			continue
		}
		cond := extractAttrSource(src, sub.Body.Attributes["condition"])
		errMsg := extractValidationErrorMessage(src, sub.Body.Attributes["error_message"])
		if isDeprecationCondition(varName, cond) {
			meta.deprecated = true
			if errMsg != "" {
				meta.deprecatedMsg = errMsg
			}
		} else if errMsg != "" && !slices.Contains(meta.validations, errMsg) {
			meta.validations = append(meta.validations, errMsg)
		}
	}
	return meta
}

func extractAttrSource(src []byte, attr *hclsyntax.Attribute) string {
	if attr == nil || attr.Expr == nil {
		return ""
	}
	r := attr.Expr.Range()
	if r.Start.Byte < 0 || r.End.Byte > len(src) || r.Start.Byte >= r.End.Byte {
		return ""
	}
	return string(src[r.Start.Byte:r.End.Byte])
}

func extractValidationErrorMessage(src []byte, attr *hclsyntax.Attribute) string {
	if attr == nil || attr.Expr == nil {
		return ""
	}
	if val, diags := attr.Expr.Value(nil); !diags.HasErrors() && val.Type() == cty.String && val.IsKnown() && !val.IsNull() {
		return strings.Join(strings.Fields(val.AsString()), " ")
	}
	raw := strings.Trim(extractAttrSource(src, attr), `"`)
	return strings.Join(strings.Fields(raw), " ")
}

func isDeprecationCondition(varName, cond string) bool {
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "\r", "").Replace(cond)
	ref := "var." + varName
	return compact == ref+"==null" ||
		compact == "null=="+ref ||
		compact == "length("+ref+")==0" ||
		compact == "0==length("+ref+")"
}

// Transforms Terraform type string into cty.Type
func GetCtyType(hclType string) (cty.Type, error) {
	if hclType == "" { // treat empty type as `any`
		// see https://developer.hashicorp.com/terraform/language/values/variables#type-constraints
		return cty.DynamicPseudoType, nil
	}
	expr, diags := hclsyntax.ParseExpression([]byte(hclType), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return cty.NilType, diags
	}

	switch hcl.ExprAsKeyword(expr) {
	case "list":
		return cty.List(cty.DynamicPseudoType), nil
	case "map":
		return cty.Map(cty.DynamicPseudoType), nil
	}

	typ, _, diags := typeexpr.TypeConstraintWithDefaults(expr)
	if diags.HasErrors() {
		return cty.NilType, diags
	}
	return typ, nil
}

// NormalizeType attempts to bring semantically equal types to equal string representation. E.g.:
//
//	 NormalizeType("object({count=number,kind=string})")
//		== NormalizeType("object({kind=string,count=number})").
//
// Additionally it removes any comments, whitespaces and line breaks.
//
// This method is fail-safe, if error arises passed type will be returned without changes.
func NormalizeType(hclType string) string {
	ctyType, err := GetCtyType(hclType)
	if err != nil {
		logging.Error("Failed to parse HCL type='%s', got %v", hclType, err)
		return hclType
	}
	return typeexpr.TypeString(ctyType)
}

// ReadHclAttributes reads cty.Values in from a .tfvars-style file
// it will error if any of the Values are not statically defined
func ReadHclAttributes(file string) (map[string]cty.Value, error) {
	f, diags := hclparse.NewParser().ParseHCLFile(file)
	if diags.HasErrors() {
		// work around ugly <nil> in error message missing d.Subject
		// https://github.com/hashicorp/hcl2/blob/fb75b3253c80b3bc7ca99c4bfa2ad6743841b1af/hcl/diagnostic.go#L76-L78
		if len(diags) == 1 {
			return nil, errors.New(diags[0].Detail)
		}
		return nil, diags
	}
	attrs, diags := f.Body.JustAttributes()
	if diags.HasErrors() {
		return nil, diags
	}

	a := make(map[string]cty.Value)
	for k, v := range attrs {
		ctyV, diags := v.Expr.Value(nil)
		if diags.HasErrors() {
			return nil, diags
		}
		a[k] = ctyV
	}

	return a, nil
}

// GetLocalDependencies returns a list of local module paths that the module at 'source' depends on.
// Paths returned are relative to the toolkit root if the module is embedded. For non-embedded modules,
// paths are resolved relative to the input source path.
func GetLocalDependencies(source string) ([]string, error) {
	module, err := LoadTFConfigModule(source)
	if err != nil {
		return nil, err
	}
	var isEmbedded = sourcereader.IsEmbeddedPath(source)

	if module == nil {
		return nil, nil // Return nil if it's not a Terraform module (e.g. Packer)
	}

	var dependencies []string
	for _, call := range module.ModuleCalls {
		if sourcereader.IsLocalPath(call.Source) {
			var resolvedPath string
			if isEmbedded {
				resolvedPath = path.Join(source, call.Source)
				if !sourcereader.IsEmbeddedPath(resolvedPath) {
					return nil, fmt.Errorf("module %s escapes embedded directory: %s", source, resolvedPath)
				}
			} else {
				resolvedPath = filepath.Clean(filepath.Join(source, call.Source))
			}
			dependencies = append(dependencies, resolvedPath)
		}
	}

	return dependencies, nil
}
