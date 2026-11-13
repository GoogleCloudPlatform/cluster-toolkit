/**
* Copyright 2026 Google LLC
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*
*      http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
 */

// Package modulewriter writes modules to a deployment directory
package modulewriter

import (
	"crypto/md5"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/deploymentio"
	"hpc-toolkit/pkg/logging"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/go-getter"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/otiai10/copy"
	"github.com/zclconf/go-cty/cty"
)

// strings that get re-used throughout this package and others
const (
	HiddenGhpcDirName        = ".ghpc"
	ArtifactsDirName         = "artifacts"
	ExpandedBlueprintName    = "expanded_blueprint.yaml"
	prevGroupDirName         = "previous_deployment_groups"
	gitignoreTemplate        = "deployment.gitignore.tmpl"
	agentNoticeTemplate      = "deployment.GEMINI.md.tmpl"
	agentNoticeFilename      = "GEMINI.md"
	artifactsWarningFilename = "DO_NOT_MODIFY_THIS_DIRECTORY"
)

func HiddenGhpcDir(deplDir string) string {
	return filepath.Join(filepath.Clean(deplDir), HiddenGhpcDirName)
}

func ArtifactsDir(deplDir string) string {
	return filepath.Join(HiddenGhpcDir(deplDir), ArtifactsDirName)
}

// ModuleWriter interface for writing modules to a deployment
type ModuleWriter interface {
	writeGroup(
		bp config.Blueprint,
		grpIdx int,
		groupPath string,
		instructionsFile io.Writer,
	) error
	restoreState(deploymentDir string) error
	kind() config.ModuleKind
}

var kinds = map[config.ModuleKind]ModuleWriter{
	config.TerraformKind: new(TFWriter),
	config.PackerKind:    new(PackerWriter),
}

//go:embed *.tmpl
var templatesFS embed.FS

// WriteDeployment writes a deployment directory using modules defined the environment blueprint.
func WriteDeployment(bp config.Blueprint, deploymentDir string) error {
	expanded := bp.Clone() // clone to avoid modifying the original blueprint

	// TODO: probably not a right place to do "materialize". Consider bubbling it up.
	if err := bp.Materialize(); err != nil {
		return err
	}

	if err := validateModuleIgnores(bp); err != nil {
		return err
	}

	if err := prepDepDir(deploymentDir); err != nil {
		return err
	}

	if err := stageFiles(bp, deploymentDir); err != nil {
		return err
	}

	instructions, err := os.Create(InstructionsPath(deploymentDir))
	if err != nil {
		return err
	}
	defer instructions.Close()
	fmt.Fprintln(instructions, "Advanced Deployment Instructions")
	fmt.Fprintln(instructions, "================================")

	if err := writeAgentNotice(deploymentDir); err != nil {
		return err
	}

	if err := copySharedEmbeddedModules(bp, deploymentDir); err != nil {
		return err
	}

	for ig := range bp.Groups {
		if err := writeGroup(deploymentDir, bp, ig, instructions); err != nil {
			return err
		}
	}

	writeDestroyInstructions(instructions, bp, deploymentDir)

	if err := writeExpandedBlueprint(deploymentDir, expanded); err != nil {
		return err
	}

	for _, writer := range kinds {
		if err := writer.restoreState(deploymentDir); err != nil {
			return fmt.Errorf("error trying to restore terraform state: %w", err)
		}
	}
	return nil
}

func copySharedEmbeddedModules(bp config.Blueprint, deploymentDir string) error {
	var allSources []string
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			if mod.Kind == config.TerraformKind {
				allSources = append(allSources, mod.Source)
			}
		}
	}
	resolvedDeps, err := modulereader.ResolveDependencies(allSources)
	if err != nil {
		return fmt.Errorf("failed to resolve dependencies: %w", err)
	}

	for _, dep := range resolvedDeps {
		if sourcereader.IsEmbeddedPath(dep) {
			dst := filepath.Join(deploymentDir, config.SharedModulesDirName, "embedded", dep)
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return err
			}
			if err := copyModuleSource(dep, dst); err != nil {
				return err
			}
		}
	}

	return writeSharedEmbeddedOverrides(bp, deploymentDir)
}

func validateTargetExistence(source string, reconfigIgnores map[string][]string, discovered map[string][]string) error {
	for target := range reconfigIgnores {
		if strings.Contains(target, ".") {
			parts := strings.SplitN(target, ".", 2)
			rType, rName := parts[0], parts[1]
			names, ok := discovered[rType]
			if !ok || !slices.Contains(names, rName) {
				return formatTargetNotFoundError(target, source, discovered)
			}
		} else {
			names, ok := discovered[target]
			if !ok || len(names) == 0 {
				return formatTargetNotFoundError(target, source, discovered)
			}
		}
	}
	return nil
}

func validateSingleModuleIgnores(mod config.Module) error {
	if len(mod.ReconfigureIgnores) == 0 {
		return nil
	}
	if mod.Kind != config.TerraformKind {
		return fmt.Errorf("module %q specifies reconfigure_ignores, which is only supported for terraform modules (got kind %q)", mod.ID, mod.Kind)
	}
	if sourcereader.IsRemotePath(mod.Source) {
		return fmt.Errorf("module %q specifies reconfigure_ignores, but uses a remote source %q: "+
			"reconfigure_ignores is currently only supported on local and embedded modules", mod.ID, mod.Source)
	}
	discovered, err := findModuleResources(mod.Source)
	if err != nil {
		return fmt.Errorf("failed to discover resources in module %q (%s): %w", mod.ID, mod.Source, err)
	}
	if err := validateTargetExistence(mod.Source, mod.ReconfigureIgnores, discovered); err != nil {
		return err
	}

	existingIgnores, err := parseExistingModuleIgnores(mod.Source)
	if err != nil {
		return fmt.Errorf("failed to parse existing lifecycle ignores in module %q (%s): %w", mod.ID, mod.Source, err)
	}

	for target, ignores := range mod.ReconfigureIgnores {
		var targetNames []string
		var rType string
		if strings.Contains(target, ".") {
			parts := strings.SplitN(target, ".", 2)
			rType = parts[0]
			targetNames = []string{parts[1]}
		} else {
			rType = target
			targetNames = discovered[rType]
		}
		if err := validateTargetIgnoreEntries(mod.Source, target, ignores, targetNames, rType, existingIgnores); err != nil {
			return err
		}
	}
	return nil
}

func validateModuleIgnores(bp config.Blueprint) error {
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			if err := validateSingleModuleIgnores(mod); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeSharedEmbeddedOverrides(bp config.Blueprint, deploymentDir string) error {
	for _, g := range bp.Groups {
		for _, mod := range g.Modules {
			if mod.Kind == config.TerraformKind && sourcereader.IsEmbeddedPath(mod.Source) && len(mod.ReconfigureIgnores) > 0 {
				instanceSrc := fmt.Sprintf("%s-%s", mod.Source, mod.ID)
				dst := filepath.Join(deploymentDir, config.SharedModulesDirName, "embedded", instanceSrc)
				if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
					return err
				}
				if err := copyModuleSource(mod.Source, dst); err != nil {
					return err
				}
				if err := writeModuleReconfigureOverrides(dst, mod.ReconfigureIgnores); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func stageFiles(bp config.Blueprint, deplPath string) error {
	staged := bp.StagedFiles()
	if len(staged) == 0 {
		return nil
	}

	// create staging directory
	if err := os.MkdirAll(filepath.Join(deplPath, config.StagingDir), 0700); err != nil {
		return err
	}

	errs := config.Errors{}
	for _, f := range staged {
		// TODO: attribute error to the position in the blueprint
		errs.Add(stageFile(deplPath, f))
	}
	return errs.OrNil()
}

func doesExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func stageFile(deplPath string, f config.StagedFile) error {
	// RelDst is relative to group folders ("../.ghpc/staged"),
	// prepend any_group_dir to be "eaten" by ".."
	dst := filepath.Join(deplPath, "any_group_dir", f.RelDst)
	dstExists, err := doesExists(dst)
	if err != nil {
		return err
	}

	srcExists, err := doesExists(f.AbsSrc)
	if err != nil {
		return err
	}

	if !srcExists && !dstExists {
		return fmt.Errorf("file for staging %s does not exists", f.AbsSrc)
	}
	if !srcExists && dstExists {
		// We implement this relaxation for cases where user does not have access to the original blueprint,
		// and does re-creation using expanded blueprint.
		logging.Error("WARNING: file %s does not exists, proceeding by using previously staged copy", f.AbsSrc)
		return nil
	}
	return copy.Copy(f.AbsSrc, dst)
}

func writeGroup(deplPath string, bp config.Blueprint, gIdx int, instructions io.Writer) error {
	g := bp.Groups[gIdx]
	gPath, err := createGroupDir(deplPath, g)
	if err != nil {
		return err
	}

	if err := copyGroupSources(gPath, g); err != nil {
		return err
	}

	writer, ok := kinds[g.Kind()]
	if !ok {
		return fmt.Errorf("invalid kind in deployment group %q, got %q", g.Name, g.Kind())
	}

	if err := writer.writeGroup(bp, gIdx, gPath, instructions); err != nil {
		return fmt.Errorf("error writing deployment group %s: %w", g.Name, err)
	}
	return nil
}

// InstructionsPath returns the path to the instructions file for a deployment
func InstructionsPath(deploymentDir string) string {
	return filepath.Join(deploymentDir, "instructions.txt")
}

// AgentNoticePath returns the path to the agent notice file for a deployment.
func AgentNoticePath(deploymentDir string) string {
	return filepath.Join(deploymentDir, agentNoticeFilename)
}

// writeAgentNotice writes GEMINI.md into the deployment folder. Gemini CLI loads a
// folder's GEMINI.md when it opens a file there, so the "do not edit" rule reaches
// the agent together with the file. Rewritten on every create so it never goes stale.
func writeAgentNotice(deploymentDir string) error {
	raw, err := templatesFS.ReadFile(agentNoticeTemplate)
	if err != nil {
		return err
	}
	absDir, err := filepath.Abs(deploymentDir)
	if err != nil {
		return err
	}
	// Plain string replace avoids importing text/template, which triggers YAML security scans on this file.
	content := strings.ReplaceAll(string(raw), "{{.Folder}}", filepath.Base(absDir))
	return os.WriteFile(AgentNoticePath(deploymentDir), []byte(content), 0644)
}

func createGroupDir(deplPath string, g config.Group) (string, error) {
	gPath := filepath.Join(deplPath, string(g.Name))
	// Create the deployment group directory if not already created.
	if _, err := os.Stat(gPath); errors.Is(err, os.ErrNotExist) {
		if err := os.Mkdir(gPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create directory at %s for deployment group %s: err=%w", gPath, g.Name, err)
		}
	}
	return gPath, nil
}

// DeploymentSource returns module source within deployment group
// Rules are following:
//   - remote source
//     = terraform => <mod.Source>
//     = packer    => <mod.ID>/<package_subdir>
//   - packer
//     => <mod.ID>
//   - embedded (source starts with "modules" or "community/modules")
//     => ./modules/embedded/<mod.Source>
//   - other
//     => ./modules/<basename(mod.Source)>-<hash(abs(mod.Source))>
func DeploymentSource(mod config.Module) (string, error) {
	switch mod.Kind {
	case config.TerraformKind:
		return tfDeploymentSource(mod)
	case config.PackerKind:
		return packerDeploymentSource(mod), nil
	default:
		return "", fmt.Errorf("unexpected module kind %#v", mod.Kind)
	}
}

// TODO: attribute error to Blueprint position
func tfDeploymentSource(mod config.Module) (string, error) {
	switch {
	case sourcereader.IsEmbeddedPath(mod.Source):
		srcPath := mod.Source
		if len(mod.ReconfigureIgnores) > 0 {
			srcPath = fmt.Sprintf("%s-%s", mod.Source, mod.ID)
		}
		return "../" + config.SharedModulesDirName + "/" + filepath.ToSlash(filepath.Join("embedded", srcPath)), nil
	case sourcereader.IsLocalPath(mod.Source):
		clean := filepath.Clean(mod.Source)
		base := filepath.Base(clean)
		if len(mod.ReconfigureIgnores) > 0 {
			return fmt.Sprintf("./modules/%s-%s-%s", base, mod.ID, shortHash(clean)), nil
		}
		return fmt.Sprintf("./modules/%s-%s", base, shortHash(clean)), nil
	default:
		return mod.Source, nil
	}
}

func packerDeploymentSource(mod config.Module) string {
	if sourcereader.IsRemotePath(mod.Source) {
		_, subDir := getter.SourceDirSubdir(mod.Source)
		return filepath.Join(string(mod.ID), subDir)
	}
	return string(mod.ID)
}

// Returns first 4 characters of md5 sum in hex form
func shortHash(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])[:4]
}

func copyModuleSource(src, dst string) error {
	_, err := os.Stat(dst)
	if err == nil {
		return nil // Already exists
	}
	if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check if module exists at %s: %w", dst, err)
	}

	reader := sourcereader.Factory(src)
	if err := reader.GetModule(src, dst); err != nil {
		return fmt.Errorf("failed to get module from %s to %s: %w", src, dst, err)
	}

	// remove .git directory if one exists; we do not want submodule
	// git history in deployment directory
	if err := os.RemoveAll(filepath.Join(dst, ".git")); err != nil {
		return err
	}
	return nil
}

func findModuleResources(source string) (map[string][]string, error) {
	resources := make(map[string][]string)
	mod, err := modulereader.LoadTFConfigModule(source)
	if err != nil {
		return nil, fmt.Errorf("failed to load terraform module at %s: %w", source, err)
	}
	if mod == nil {
		return nil, fmt.Errorf("source is not a terraform module: %s", source)
	}

	for _, res := range mod.ManagedResources {
		resources[res.Type] = append(resources[res.Type], res.Name)
	}
	return resources, nil
}

type tfLifecycleOverride struct {
	IgnoreChanges []string `json:"ignore_changes"`
}

type tfResourceOverride struct {
	Lifecycle tfLifecycleOverride `json:"lifecycle"`
}

type tfOverrideFile struct {
	Resource map[string]map[string]tfResourceOverride `json:"resource"`
}

var (
	quoteIndexRe = regexp.MustCompile(`\[\s*['"]([^'"]*)['"]\s*\]`)
	intIndexRe   = regexp.MustCompile(`\[\s*([0-9]+)\s*\]`)
	validAttrRe  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\[\s*([0-9]+|'[^']+'|"[^"]+")\s*\]|\.[a-zA-Z_][a-zA-Z0-9_]*)*$`)
)

func normalizeIgnoreAttribute(attr string) string {
	attr = quoteIndexRe.ReplaceAllString(attr, `["$1"]`)
	return intIndexRe.ReplaceAllString(attr, `[$1]`)
}

func parseIgnoreEntry(item string) (string, bool, error) {
	trimmed := strings.TrimSpace(item)
	if trimmed == "" {
		return "", false, fmt.Errorf("entry cannot be empty or whitespace only")
	}

	isSub := false
	rawAttr := trimmed
	if strings.HasPrefix(trimmed, "-") {
		isSub = true
		rawAttr = strings.TrimSpace(strings.TrimPrefix(trimmed, "-"))
		if rawAttr == "" {
			return "", false, fmt.Errorf("subtraction entry %q must specify an attribute name after '-'", item)
		}
	}

	normAttr := normalizeIgnoreAttribute(rawAttr)
	if normAttr == "all" {
		return "", false, fmt.Errorf("specifies %q; ignoring \"all\" changes is not supported, please specify individual attribute names", item)
	}

	if !validAttrRe.MatchString(normAttr) {
		return "", false, fmt.Errorf("entry %q is not a valid attribute name", item)
	}

	return normAttr, isSub, nil
}

func mergeIgnores(existing, additions []string) ([]string, error) {
	seen := make(map[string]bool)
	removeSet := make(map[string]bool)
	var adds []string

	for _, item := range additions {
		norm, isSub, err := parseIgnoreEntry(item)
		if err != nil {
			return nil, err
		}
		if isSub {
			removeSet[norm] = true
		} else {
			adds = append(adds, norm)
		}
	}

	var merged []string
	for _, item := range existing {
		norm := normalizeIgnoreAttribute(item)
		if removeSet[norm] {
			continue
		}
		if !seen[norm] {
			seen[norm] = true
			merged = append(merged, norm)
		}
	}
	for _, norm := range adds {
		if !seen[norm] {
			seen[norm] = true
			merged = append(merged, norm)
		}
	}
	return merged, nil
}

func validateTargetSubtraction(source, target, attr string, targetNames []string, rType string, existing map[string]map[string][]string) error {
	var available []string
	found := false

	for _, name := range targetNames {
		for _, ign := range existing[rType][name] {
			norm := normalizeIgnoreAttribute(ign)
			available = append(available, norm)
			if norm == attr {
				found = true
			}
		}
	}

	if found {
		return nil
	}

	slices.Sort(available)
	available = slices.Compact(available)

	hint := moduleSourceURLHint(source)
	if len(available) == 0 {
		return fmt.Errorf("cannot subtract %q in reconfigure_ignores target %q: target resource has no lifecycle.ignore_changes defined in module %s%s", attr, target, source, hint)
	}

	return fmt.Errorf("cannot subtract %q in reconfigure_ignores target %q: attribute is not in the module's lifecycle.ignore_changes (available module ignores: [%s])%s",
		attr, target, strings.Join(available, ", "), hint)
}

func validateTargetAllAndEmpty(source, target, rType string, targetNames []string, ignores []string, existing map[string]map[string][]string) error {
	hint := moduleSourceURLHint(source)
	for _, rName := range targetNames {
		current := existing[rType][rName]
		if slices.Contains(current, "all") {
			for _, item := range ignores {
				if _, isSub, _ := parseIgnoreEntry(item); isSub {
					return fmt.Errorf("cannot subtract in reconfigure_ignores target %q: resource defines \"lifecycle { ignore_changes = all }\" in module %s; selective subtraction is not supported on resources that ignore all changes%s",
						target, source, hint)
				}
			}
			continue
		}
		if len(current) > 0 {
			final, err := mergeIgnores(current, ignores)
			if err != nil {
				return fmt.Errorf("in module %s reconfigure_ignores target %q: %w", source, target, err)
			}
			if len(final) == 0 {
				return fmt.Errorf("cannot subtract all attributes from reconfigure_ignores target %q in module %s: Terraform's override loader cannot clear base module ignores with an empty list; at least one attribute must remain ignored%s",
					target, source, hint)
			}
		}
	}
	return nil
}

func validateTargetIgnoreEntries(source, target string, ignores []string, targetNames []string, rType string, existing map[string]map[string][]string) error {
	if err := validateTargetAllAndEmpty(source, target, rType, targetNames, ignores, existing); err != nil {
		return err
	}

	seenAdd := make(map[string]bool)
	seenSub := make(map[string]bool)

	for _, item := range ignores {
		norm, isSub, err := parseIgnoreEntry(item)
		if err != nil {
			return fmt.Errorf("in module %s reconfigure_ignores target %q: %w", source, target, err)
		}
		if isSub {
			if seenAdd[norm] {
				return fmt.Errorf("in module %s reconfigure_ignores target %q: cannot both add and subtract attribute %q", source, target, norm)
			}
			seenSub[norm] = true
			if err := validateTargetSubtraction(source, target, norm, targetNames, rType, existing); err != nil {
				return err
			}
		} else {
			if seenSub[norm] {
				return fmt.Errorf("in module %s reconfigure_ignores target %q: cannot both add and subtract attribute %q", source, target, norm)
			}
			seenAdd[norm] = true
		}
	}
	return nil
}

func isTFSourceFile(e fs.DirEntry) bool {
	return !e.IsDir() && strings.HasSuffix(e.Name(), ".tf") && !strings.HasSuffix(e.Name(), "override.tf")
}

func listModuleTFFiles(source string) ([]string, func(string) ([]byte, error), error) {
	if sourcereader.IsEmbeddedPath(source) && sourcereader.ModuleFS != nil {
		slashSource := sourcereader.ToSlash(source)
		entries, err := sourcereader.ModuleFS.ReadDir(slashSource)
		if err == nil {
			var tfFiles []string
			for _, e := range entries {
				if isTFSourceFile(e) {
					tfFiles = append(tfFiles, e.Name())
				}
			}
			return tfFiles, func(name string) ([]byte, error) {
				return sourcereader.ModuleFS.ReadFile(path.Join(slashSource, name))
			}, nil
		}
	}

	entries, err := os.ReadDir(source)
	if err != nil {
		return nil, nil, err
	}
	var tfFiles []string
	for _, e := range entries {
		if isTFSourceFile(e) {
			tfFiles = append(tfFiles, e.Name())
		}
	}
	return tfFiles, func(name string) ([]byte, error) {
		return os.ReadFile(filepath.Join(source, name))
	}, nil
}

func parseExistingModuleIgnores(source string) (map[string]map[string][]string, error) {
	existing := make(map[string]map[string][]string)
	tfFiles, readFn, err := listModuleTFFiles(source)
	if err != nil {
		return nil, err
	}

	parser := hclparse.NewParser()
	for _, fileName := range tfFiles {
		bytes, err := readFn(fileName)
		if err != nil {
			continue
		}
		f, diags := parser.ParseHCL(bytes, fileName)
		if diags.HasErrors() || f == nil {
			continue
		}
		body, ok := f.Body.(*hclsyntax.Body)
		if !ok {
			continue
		}
		for _, block := range body.Blocks {
			extractResourceIgnores(block, f.Bytes, existing)
		}
	}
	return existing, nil
}

func parseLifecycleAttribute(attr *hclsyntax.Attribute, src []byte) []string {
	if scope, ok := attr.Expr.(*hclsyntax.ScopeTraversalExpr); ok && scope.Traversal.RootName() == "all" {
		return []string{"all"}
	}
	tuple, ok := attr.Expr.(*hclsyntax.TupleConsExpr)
	if !ok {
		return nil
	}
	var list []string
	for _, elem := range tuple.Exprs {
		list = append(list, string(elem.Range().SliceBytes(src)))
	}
	return list
}

func extractResourceIgnores(block *hclsyntax.Block, src []byte, existing map[string]map[string][]string) {
	if block.Type != "resource" || len(block.Labels) < 2 {
		return
	}
	rType, rName := block.Labels[0], block.Labels[1]
	for _, sub := range block.Body.Blocks {
		if sub.Type != "lifecycle" {
			continue
		}
		attr, ok := sub.Body.Attributes["ignore_changes"]
		if !ok {
			continue
		}
		list := parseLifecycleAttribute(attr, src)
		if len(list) > 0 {
			if existing[rType] == nil {
				existing[rType] = make(map[string][]string)
			}
			existing[rType][rName] = list
		}
	}
}

func moduleSourceURLHint(source string) string {
	if sourcereader.IsEmbeddedPath(source) {
		return fmt.Sprintf("\nInspect module: https://github.com/GoogleCloudPlatform/cluster-toolkit/tree/main/%s", source)
	}
	return ""
}

func formatTargetNotFoundError(target, moduleDir string, discovered map[string][]string) error {
	var available []string
	for rType, names := range discovered {
		for _, rName := range names {
			available = append(available, fmt.Sprintf("%s.%s", rType, rName))
		}
	}
	sort.Strings(available)
	hint := moduleSourceURLHint(moduleDir)
	if len(available) > 0 {
		return fmt.Errorf("reconfigure_ignores target %q was not found in module %s. Available resources in this module: %s%s",
			target, moduleDir, strings.Join(available, ", "), hint)
	}
	return fmt.Errorf("reconfigure_ignores target %q was not found in module %s%s", target, moduleDir, hint)
}

// GetModuleDefaultIgnores returns a map of resource target to its default lifecycle.ignore_changes.
func GetModuleDefaultIgnores(source string) (map[string][]string, error) {
	existing, err := parseExistingModuleIgnores(source)
	if err != nil {
		return nil, err
	}
	result := make(map[string][]string)
	for rType, resMap := range existing {
		var allTypeIgnores []string
		for _, ignores := range resMap {
			for _, ign := range ignores {
				allTypeIgnores = append(allTypeIgnores, normalizeIgnoreAttribute(ign))
			}
		}
		slices.Sort(allTypeIgnores)
		allTypeIgnores = slices.Compact(allTypeIgnores)
		if len(allTypeIgnores) > 0 {
			result[rType] = allTypeIgnores
		}
	}
	return result, nil
}

func setResourceOverride(override tfOverrideFile, existingIgnores map[string]map[string][]string, rType, rName string, ignores []string, moduleDir string) error {
	if slices.Contains(existingIgnores[rType][rName], "all") {
		return nil
	}
	current := existingIgnores[rType][rName]
	if prev, ok := override.Resource[rType][rName]; ok {
		current = prev.Lifecycle.IgnoreChanges
	}
	finalIgnores, err := mergeIgnores(current, ignores)
	if err != nil {
		return err
	}
	if len(finalIgnores) == 0 {
		if len(existingIgnores[rType][rName]) > 0 {
			hint := moduleSourceURLHint(moduleDir)
			return fmt.Errorf("cannot subtract all attributes from reconfigure_ignores target %q in module %s: Terraform's override loader cannot clear base module ignores with an empty list; at least one attribute must remain ignored%s",
				fmt.Sprintf("%s.%s", rType, rName), moduleDir, hint)
		}
		return nil
	}
	if override.Resource[rType] == nil {
		override.Resource[rType] = make(map[string]tfResourceOverride)
	}
	sort.Strings(finalIgnores)
	override.Resource[rType][rName] = tfResourceOverride{
		Lifecycle: tfLifecycleOverride{IgnoreChanges: finalIgnores},
	}
	return nil
}

func applyTargetOverrides(override tfOverrideFile, discovered map[string][]string, existingIgnores map[string]map[string][]string, target string, ignores []string, moduleDir string) error {
	if len(ignores) == 0 {
		return nil
	}

	var targetNames []string
	var rType string
	if strings.Contains(target, ".") {
		parts := strings.SplitN(target, ".", 2)
		rType = parts[0]
		rName := parts[1]
		names, ok := discovered[rType]
		if !ok || !slices.Contains(names, rName) {
			return formatTargetNotFoundError(target, moduleDir, discovered)
		}
		targetNames = []string{rName}
	} else {
		rType = target
		names, ok := discovered[rType]
		if !ok || len(names) == 0 {
			return formatTargetNotFoundError(target, moduleDir, discovered)
		}
		targetNames = names
	}

	if err := validateTargetIgnoreEntries(moduleDir, target, ignores, targetNames, rType, existingIgnores); err != nil {
		return err
	}

	for _, rName := range targetNames {
		if err := setResourceOverride(override, existingIgnores, rType, rName, ignores, moduleDir); err != nil {
			return err
		}
	}
	return nil
}

func writeModuleReconfigureOverrides(moduleDir string, reconfigIgnores map[string][]string) error {
	if len(reconfigIgnores) == 0 {
		return nil
	}

	discovered, err := findModuleResources(moduleDir)
	if err != nil {
		return fmt.Errorf("failed to discover resources in module at %s: %w", moduleDir, err)
	}

	existingIgnores, err := parseExistingModuleIgnores(moduleDir)
	if err != nil {
		return fmt.Errorf("failed to parse existing lifecycle ignores in module at %s: %w", moduleDir, err)
	}

	override := tfOverrideFile{
		Resource: make(map[string]map[string]tfResourceOverride),
	}

	targets := make([]string, 0, len(reconfigIgnores))
	for target := range reconfigIgnores {
		targets = append(targets, target)
	}
	sort.Strings(targets)

	for _, target := range targets {
		ignores := reconfigIgnores[target]
		if err := applyTargetOverrides(override, discovered, existingIgnores, target, ignores, moduleDir); err != nil {
			return err
		}
	}

	if len(override.Resource) == 0 {
		return nil
	}

	data, err := json.MarshalIndent(override, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal reconfigure_ignores override json: %w", err)
	}

	overridePath := filepath.Join(moduleDir, "override.tf.json")
	if err := os.WriteFile(overridePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", overridePath, err)
	}

	logging.Info("generated %s with reconfigured lifecycle ignores", overridePath)
	return nil
}

func copySingleNonEmbeddedModule(gPath string, mod *config.Module) error {
	if mod.Kind == config.TerraformKind {
		if sourcereader.IsEmbeddedPath(mod.Source) || sourcereader.IsRemotePath(mod.Source) {
			return nil
		}
	}

	var src, dst string
	if sourcereader.IsRemotePath(mod.Source) && mod.Kind == config.PackerKind {
		src, _ = getter.SourceDirSubdir(mod.Source)
		dst = filepath.Join(gPath, string(mod.ID))
	} else {
		deplSource, err := DeploymentSource(*mod)
		if err != nil {
			return err
		}
		src = mod.Source
		dst = filepath.Join(gPath, deplSource)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	if err := copyModuleSource(src, dst); err != nil {
		return err
	}
	if mod.Kind == config.TerraformKind {
		if err := writeModuleReconfigureOverrides(dst, mod.ReconfigureIgnores); err != nil {
			return err
		}
	}
	return nil
}

func copyNonEmbeddedModules(gPath string, modules []config.Module) error {
	for iMod := range modules {
		if err := copySingleNonEmbeddedModule(gPath, &modules[iMod]); err != nil {
			return err
		}
	}
	return nil
}

func copyGroupSources(gPath string, g config.Group) error {
	return copyNonEmbeddedModules(gPath, g.Modules)
}

// Prepares a deployment directory to be written to.
func prepDepDir(depDir string) error {
	deploymentio := deploymentio.GetDeploymentioLocal()
	ghpcDir := HiddenGhpcDir(depDir)

	// create deployment directory
	if err := deploymentio.CreateDirectory(depDir); err != nil {
		// Confirm we have a previously written deployment dir before overwriting.
		if _, err := os.Stat(ghpcDir); os.IsNotExist(err) {
			return fmt.Errorf("while trying to update the deployment directory at %s, the '.ghpc/' dir could not be found", depDir)
		}
	} else {
		if err := deploymentio.CreateDirectory(ghpcDir); err != nil {
			return fmt.Errorf("failed to create directory at %s: err=%w", ghpcDir, err)
		}

		gitignoreFile := filepath.Join(depDir, ".gitignore")
		if err := deploymentio.CopyFromFS(templatesFS, gitignoreTemplate, gitignoreFile); err != nil {
			return fmt.Errorf("failed to copy template.gitignore file to %s: err=%w", gitignoreFile, err)
		}
	}

	if err := prepArtifactsDir(ArtifactsDir(depDir)); err != nil {
		return err
	}

	// remove any existing backups of deployment group
	prevGroupDir := filepath.Join(ghpcDir, prevGroupDirName)
	os.RemoveAll(prevGroupDir)
	if err := os.MkdirAll(prevGroupDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory to save previous deployment groups at %s: %w", prevGroupDir, err)
	}

	// create new backup of deployment group directory
	files, err := os.ReadDir(depDir)
	if err != nil {
		return fmt.Errorf("error trying to read directories in %s, %w", depDir, err)
	}
	for _, f := range files {
		if !f.IsDir() || f.Name() == HiddenGhpcDirName {
			continue
		}
		src := filepath.Join(depDir, f.Name())
		dest := filepath.Join(prevGroupDir, f.Name())
		if err := os.Rename(src, dest); err != nil {
			return fmt.Errorf("error while moving previous deployment groups: failed on %s: %w", f.Name(), err)
		}
	}
	return nil
}

func prepArtifactsDir(artifactsDir string) error {
	// cleanup previous artifacts on every write
	if err := os.RemoveAll(artifactsDir); err != nil {
		return fmt.Errorf(
			"error while removing the artifacts directory at %s; %s", artifactsDir, err.Error())
	}

	if err := os.MkdirAll(artifactsDir, 0700); err != nil {
		return err
	}

	artifactsWarningFile := filepath.Join(artifactsDir, artifactsWarningFilename)
	f, err := os.Create(artifactsWarningFile)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.WriteString(artifactsWarning)
	return err
}

func writeExpandedBlueprint(depDir string, bp config.Blueprint) error {
	return bp.Export(filepath.Join(ArtifactsDir(depDir), ExpandedBlueprintName))
}

func writeDestroyInstructions(w io.Writer, bp config.Blueprint, deploymentDir string) {
	packerManifests := []string{}
	gcsBuckets := []string{}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Destroying infrastructure when no longer needed")
	fmt.Fprintln(w, "===============================================")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Automated")
	fmt.Fprintln(w, "---------")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "gcluster destroy %s\n", deploymentDir)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Advanced / Manual")
	fmt.Fprintln(w, "-----------------")
	fmt.Fprintln(w, "Infrastructure should be destroyed in reverse order of creation:")
	fmt.Fprintln(w)
	for grpIdx := len(bp.Groups) - 1; grpIdx >= 0; grpIdx-- {
		grp := bp.Groups[grpIdx]
		grpPath := filepath.Join(deploymentDir, string(grp.Name))
		if grp.Kind() == config.TerraformKind {
			fmt.Fprintf(w, "terraform -chdir=%s destroy\n", grpPath)
		}
		if grp.Kind() == config.PackerKind {
			packerManifests = append(packerManifests, filepath.Join(grpPath, string(grp.Modules[0].ID), "packer-manifest.json"))
		}
	}

	gcsBuckets, _ = GetUniqueGcsBuckets(bp)

	WritePackerDestroyInstructions(w, packerManifests)
	WriteGcsDestroyInstructions(w, gcsBuckets)
}

// WritePackerDestroyInstructions prints our best effort guidance to the user on
// deleting images produced by Packer; must improve definition of Packer outputs
func WritePackerDestroyInstructions(w io.Writer, manifests []string) {
	if len(manifests) == 0 {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Please browse to the Cloud Console to remove VM images produced by Packer.\n")
	fmt.Fprintln(w, "If this file is present, the names of images can be read from it:")
	fmt.Fprintln(w)
	for _, manifest := range manifests {
		fmt.Fprintln(w, manifest)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "https://console.cloud.google.com/compute/images")
}

// WriteGcsDestroyInstructions prints our best effort guidance to the user on
// deleting GCS buckets used for Terraform state.
func WriteGcsDestroyInstructions(w io.Writer, buckets []string) {
	if len(buckets) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "The following buckets were used for Terraform state and will not be automatically deleted by gcluster destroy:")
	_, _ = fmt.Fprintln(w)
	for _, bucket := range buckets {
		_, _ = fmt.Fprintln(w, bucket)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Please browse to the Cloud Console to remove them:")
	_, _ = fmt.Fprintln(w, "https://console.cloud.google.com/storage/browser")
}

// GetUniqueGcsBuckets returns a list of unique GCS buckets used for Terraform state.
func GetUniqueGcsBuckets(bp config.Blueprint) ([]string, error) {
	seenBuckets := make(map[string]bool)
	var buckets []string

	for _, g := range bp.Groups {
		if g.TerraformBackend.Type != "gcs" || !g.TerraformBackend.Configuration.Has("bucket") {
			continue
		}
		evaluatedConfig, err := bp.EvalDict(g.TerraformBackend.Configuration)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate terraform backend configuration for group %q: %w", g.Name, err)
		}
		bucketVal := evaluatedConfig.Get("bucket")
		if bucketVal.IsNull() || !bucketVal.IsKnown() || bucketVal.Type() != cty.String {
			return nil, fmt.Errorf("GCS backend bucket name for group %q cannot be empty or unknown", g.Name)
		}
		bucketName := bucketVal.AsString()
		if bucketName == "" {
			return nil, fmt.Errorf("GCS backend bucket name for group %q cannot be empty", g.Name)
		}
		if seenBuckets[bucketName] {
			continue
		}
		seenBuckets[bucketName] = true
		buckets = append(buckets, bucketName)
	}

	return buckets, nil
}
