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

package cmd

import (
	"encoding/json"
	"fmt"
	"hpc-toolkit/pkg/catalog"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"hpc-toolkit/pkg/sourcereader"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/spf13/cobra"
	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

var explainFlags = struct {
	format         string
	deploymentFile string
}{}

var explainCmd = &cobra.Command{
	Use:   "explain [module-or-blueprint-or-specs...]",
	Short: "Discover and inspect blueprints, embedded modules, and cluster specs.",
	Long: `Inspect Cluster Toolkit blueprints and modules, or find the best blueprint for given specs:
  1. ./gcluster explain
     Lists all available blueprint paths and top-level embedded modules.
  2. ./gcluster explain <blueprint.yaml | modules/...>
     Shows required GCP APIs, customizable variables, deployment stages, and ready-to-copy 'use:' snippets.
  3. ./gcluster explain <specs...> (e.g., './gcluster explain slurm a3u lustre' or './gcluster explain gke a3-megagpu lustre')
     Finds the exact or closest matching blueprint and generates snippets for any missing module specs.`,
	Args:         cobra.ArbitraryArgs,
	RunE:         runExplainCmd,
	SilenceUsage: true,
}

func init() {
	explainCmd.Flags().StringVarP(&explainFlags.format, "format", "f", "text",
		"Output format: text, yaml, or json")
	explainCmd.Flags().StringVarP(&explainFlags.deploymentFile, "deployment-file", "d", "",
		"Optional companion deployment YAML file when explaining a blueprint")
	rootCmd.AddCommand(explainCmd)
}

// ExplainIndexResult is the structured output for Mode 1 (0 args).
type ExplainIndexResult struct {
	Blueprints []ExplainBlueprintSummary `json:"blueprints" yaml:"blueprints"`
	Modules    map[string][]string       `json:"modules" yaml:"modules"`
	Hint       string                    `json:"hint" yaml:"hint"`
}

// ExplainBlueprintSummary is a compact entry in Mode 1.
type ExplainBlueprintSummary struct {
	Path           string   `json:"path" yaml:"path"`
	DeploymentPath string   `json:"deployment_path,omitempty" yaml:"deployment_path,omitempty"`
	Scheduler      string   `json:"scheduler" yaml:"scheduler"`
	MachineTypes   []string `json:"machine_types,omitempty" yaml:"machine_types,omitempty"`
	Storage        []string `json:"storage,omitempty" yaml:"storage,omitempty"`
}

// ExplainBlueprintResult is the structured output for Mode 2a (blueprint path).
type ExplainBlueprintResult struct {
	Path           string            `json:"path" yaml:"path"`
	DeploymentPath string            `json:"deployment_path,omitempty" yaml:"deployment_path,omitempty"`
	BlueprintName  string            `json:"blueprint_name" yaml:"blueprint_name"`
	RequiredAPIs   []string          `json:"required_apis" yaml:"required_apis"`
	GcloudCommand  string            `json:"gcloud_enable_command" yaml:"gcloud_enable_command"`
	Variables      []ExplainVariable `json:"variables" yaml:"variables"`
	Stages         []ExplainStage    `json:"stages" yaml:"stages"`
	DeployCommand  string            `json:"deploy_command" yaml:"deploy_command"`
}

// ExplainVariable describes a top-level blueprint or deployment variable.
type ExplainVariable struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"`
	Required bool   `json:"required" yaml:"required"`
	Default  string `json:"default,omitempty" yaml:"default,omitempty"`
}

// ExplainStage describes a deployment group and its modules in order.
type ExplainStage struct {
	Group   string               `json:"group" yaml:"group"`
	Modules []ExplainStageModule `json:"modules" yaml:"modules"`
}

// ExplainStageModule describes a module instance inside a deployment stage.
type ExplainStageModule struct {
	ID          string   `json:"id" yaml:"id"`
	Source      string   `json:"source" yaml:"source"`
	ShortSource string   `json:"short_source" yaml:"short_source"`
	Use         []string `json:"use,omitempty" yaml:"use,omitempty"`
}

// ExplainModuleResult is the structured output for Mode 2b (module path).
type ExplainModuleResult struct {
	Module         string               `json:"module" yaml:"module"`
	Kind           string               `json:"kind" yaml:"kind"`
	Deprecated     string               `json:"deprecated,omitempty" yaml:"deprecated,omitempty"`
	Alternative    string               `json:"alternative_module,omitempty" yaml:"alternative_module,omitempty"`
	RequiredAPIs   []string             `json:"required_apis" yaml:"required_apis"`
	GcloudCommand  string               `json:"gcloud_enable_command" yaml:"gcloud_enable_command"`
	Snippet        string               `json:"snippet" yaml:"snippet"`
	AutoInputs     []ExplainAutoInput   `json:"auto_injected_inputs" yaml:"auto_injected_inputs"`
	RequiredInputs []ExplainModuleInput `json:"required_inputs,omitempty" yaml:"required_inputs,omitempty"`
	OptionalInputs []ExplainModuleInput `json:"optional_inputs,omitempty" yaml:"optional_inputs,omitempty"`
	Outputs        []string             `json:"outputs,omitempty" yaml:"outputs,omitempty"`
}

// ExplainAutoInput describes an input filled automatically via use:, vars:, or inject_module_id.
type ExplainAutoInput struct {
	Inputs []string `json:"inputs" yaml:"inputs"`
	Source string   `json:"source" yaml:"source"`
}

// ExplainModuleInput describes a Terraform/Packer module variable.
type ExplainModuleInput struct {
	Name        string `json:"name" yaml:"name"`
	Type        string `json:"type" yaml:"type"`
	Default     string `json:"default,omitempty" yaml:"default,omitempty"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ExplainSpecMatchResult is the structured output for Mode 3 (spec matching).
type ExplainSpecMatchResult struct {
	ExactMatch     bool                   `json:"exact_match" yaml:"exact_match"`
	OtherExact     int                    `json:"other_exact_matches,omitempty" yaml:"other_exact_matches,omitempty"`
	RequestedSpecs []string               `json:"requested_specs" yaml:"requested_specs"`
	MatchedSpecs   []string               `json:"matched_specs" yaml:"matched_specs"`
	MissingSpecs   []ExplainMissingSpec   `json:"missing_specs,omitempty" yaml:"missing_specs,omitempty"`
	Blueprint      ExplainBlueprintResult `json:"blueprint" yaml:"blueprint"`
}

// ExplainMissingSpec describes a requested spec that was not in the closest blueprint.
type ExplainMissingSpec struct {
	Spec                string   `json:"spec" yaml:"spec"`
	Module              string   `json:"module,omitempty" yaml:"module,omitempty"`
	TargetGroup         string   `json:"target_group,omitempty" yaml:"target_group,omitempty"`
	Snippet             string   `json:"snippet,omitempty" yaml:"snippet,omitempty"`
	ReferenceBlueprints []string `json:"reference_blueprints,omitempty" yaml:"reference_blueprints,omitempty"`
}

var explainAutoGlobals = map[string]bool{
	"project_id":      true,
	"deployment_name": true,
	"region":          true,
	"zone":            true,
	"labels":          true,
}

var explainGenericOutputs = map[string]bool{
	"capacity_gb":      true,
	"gcp_service_list": true,
	"id":               true,
	"instance_name":    true,
	"name":             true,
	"self_link":        true,
}

func writef(w io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(w, format, args...)
}

func writeln(w io.Writer, s string) {
	_, _ = fmt.Fprintln(w, s)
}

func runExplainCmd(cmd *cobra.Command, args []string) error {
	format, err := parseExplainFormat(explainFlags.format)
	if err != nil {
		return err
	}

	repoRoot := findExplainRepoRoot()
	out := cmd.OutOrStdout()

	if len(args) == 0 {
		if err := rejectUnexpectedDeploymentFlag(); err != nil {
			return err
		}
		res, err := buildExplainIndex(repoRoot)
		if err != nil {
			return err
		}
		return renderExplainOutput(out, format, res, func(w io.Writer) {
			printExplainIndexText(w, res)
		})
	}

	if len(args) == 1 {
		if handled, err := tryExplainSingleTarget(out, format, repoRoot, strings.TrimSpace(args[0])); handled || err != nil {
			return err
		}
	}

	if err := rejectUnexpectedDeploymentFlag(); err != nil {
		return err
	}
	res, err := buildExplainSpecMatch(repoRoot, args)
	if err != nil {
		return err
	}
	return renderExplainOutput(out, format, res, func(w io.Writer) {
		printExplainSpecMatchText(w, res)
	})
}

func rejectUnexpectedDeploymentFlag() error {
	if strings.TrimSpace(explainFlags.deploymentFile) != "" {
		return fmt.Errorf("--deployment-file is only valid when explaining a blueprint file")
	}
	return nil
}

func parseExplainFormat(raw string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(raw))
	if format != "text" && format != "yaml" && format != "json" {
		return "", fmt.Errorf("invalid --format %q: must be one of text, yaml, json", raw)
	}
	return format, nil
}

func tryExplainSingleTarget(out io.Writer, format, repoRoot, target string) (bool, error) {
	bpPath, deplPath, ok, err := resolveBlueprintTarget(repoRoot, target, explainFlags.deploymentFile)
	if err != nil {
		return true, err
	}
	if ok {
		res, err := buildExplainBlueprint(repoRoot, bpPath, deplPath)
		if err != nil {
			return true, err
		}
		return true, renderExplainOutput(out, format, res, func(w io.Writer) {
			printExplainBlueprintText(w, res)
		})
	}

	if modPath, kind, ok := resolveModuleTarget(target); ok {
		if err := rejectUnexpectedDeploymentFlag(); err != nil {
			return true, err
		}
		res, err := buildExplainModule(modPath, kind)
		if err != nil {
			return true, err
		}
		return true, renderExplainOutput(out, format, res, func(w io.Writer) {
			printExplainModuleText(w, res)
		})
	}

	if looksLikePathTarget(target) {
		return true, fmt.Errorf("blueprint or module path %q not found; run './gcluster explain' to list all available blueprints and modules", target)
	}
	return false, nil
}

func looksLikePathTarget(target string) bool {
	return strings.Contains(target, "/") || strings.Contains(target, "\\") ||
		strings.HasSuffix(target, ".yaml") || strings.HasSuffix(target, ".yml")
}

func renderExplainOutput(w io.Writer, format string, data interface{}, textPrinter func(io.Writer)) error {
	switch format {
	case "json":
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	case "yaml":
		enc := yaml.NewEncoder(w)
		enc.SetIndent(2)
		if err := enc.Encode(data); err != nil {
			return err
		}
		return enc.Close()
	default:
		textPrinter(w)
		return nil
	}
}

func findExplainRepoRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := cwd
	for {
		if isExplainRepoRoot(dir) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return cwd
}

func isExplainRepoRoot(dir string) bool {
	if filepath.Base(dir) == "community" && hasExamplesAndModules(filepath.Dir(dir)) {
		return false
	}
	return hasExamplesAndModules(dir)
}

func hasExamplesAndModules(dir string) bool {
	for _, sub := range []string{"examples", "modules"} {
		if info, err := os.Stat(filepath.Join(dir, sub)); err != nil || !info.IsDir() {
			return false
		}
	}
	return true
}

type embeddedModEntry struct {
	Path     string
	Category string
	Name     string
	Kind     string
}

var (
	explainCacheMu        sync.Mutex
	explainBlueprintCache = make(map[string][]catalog.BlueprintEntry)
	explainModFSCacheKey  sourcereader.BaseFS
	explainModListCache   []embeddedModEntry
	explainProviderFSKey  sourcereader.BaseFS
	explainProviderCache  []candidateProvider
)

func scanBlueprintsCached(repoRoot string) ([]catalog.BlueprintEntry, error) {
	key := filepath.Clean(repoRoot)
	if abs, err := filepath.Abs(repoRoot); err == nil {
		key = abs
	}
	explainCacheMu.Lock()
	if cached, ok := explainBlueprintCache[key]; ok {
		explainCacheMu.Unlock()
		return slices.Clone(cached), nil
	}
	explainCacheMu.Unlock()

	bps, err := catalog.ScanBlueprints(repoRoot, nil)
	if err != nil {
		return nil, err
	}
	if len(bps) > 0 {
		explainCacheMu.Lock()
		explainBlueprintCache[key] = bps
		explainCacheMu.Unlock()
	}
	return bps, nil
}

func sameBaseFS(a, b sourcereader.BaseFS) bool {
	if a == nil || b == nil {
		return a == b
	}
	ra, rb := reflect.TypeOf(a), reflect.TypeOf(b)
	return ra == rb && ra.Comparable() && a == b
}

func listTopLevelEmbeddedModules(bfs sourcereader.BaseFS) []embeddedModEntry {
	if bfs == nil {
		return nil
	}
	explainCacheMu.Lock()
	if sameBaseFS(explainModFSCacheKey, bfs) && len(explainModListCache) > 0 {
		cached := slices.Clone(explainModListCache)
		explainCacheMu.Unlock()
		return cached
	}
	explainCacheMu.Unlock()

	var result []embeddedModEntry
	for _, root := range []string{"modules", "community/modules"} {
		result = append(result, scanEmbeddedModuleRoot(bfs, root)...)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Path < result[j].Path
	})
	if len(result) > 0 {
		explainCacheMu.Lock()
		explainModFSCacheKey = bfs
		explainModListCache = result
		explainCacheMu.Unlock()
	}
	return result
}

func scanEmbeddedModuleRoot(bfs sourcereader.BaseFS, root string) []embeddedModEntry {
	catEntries, err := bfs.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []embeddedModEntry
	for _, catDir := range catEntries {
		if !catDir.IsDir() || catDir.Name() == "internal" {
			continue
		}
		out = append(out, scanEmbeddedModuleCategory(bfs, path.Join(root, catDir.Name()), catDir.Name())...)
	}
	return out
}

func scanEmbeddedModuleCategory(bfs sourcereader.BaseFS, catPath, catName string) []embeddedModEntry {
	modEntries, err := bfs.ReadDir(catPath)
	if err != nil {
		return nil
	}
	var out []embeddedModEntry
	for _, modDir := range modEntries {
		if !modDir.IsDir() {
			continue
		}
		modPath := path.Join(catPath, modDir.Name())
		if kind := detectEmbeddedModuleKind(bfs, modPath); kind != "" {
			out = append(out, embeddedModEntry{
				Path:     modPath,
				Category: catName,
				Name:     modDir.Name(),
				Kind:     kind,
			})
		}
	}
	return out
}

func detectEmbeddedModuleKind(bfs sourcereader.BaseFS, modPath string) string {
	if bfs == nil {
		return ""
	}
	files, err := bfs.ReadDir(modPath)
	if err != nil {
		return ""
	}
	return classifyModuleDirEntries(files)
}

func detectLocalModuleKind(dirPath string) string {
	files, err := os.ReadDir(dirPath)
	if err != nil {
		return ""
	}
	return classifyModuleDirEntries(files)
}

func classifyModuleDirEntries(files []os.DirEntry) string {
	hasTF := false
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		if strings.HasSuffix(f.Name(), ".pkr.hcl") {
			return "packer"
		}
		if strings.HasSuffix(f.Name(), ".tf") {
			hasTF = true
		}
	}
	if hasTF {
		return "terraform"
	}
	return ""
}

func buildExplainIndex(repoRoot string) (ExplainIndexResult, error) {
	blueprints, err := scanBlueprintsCached(repoRoot)
	if err != nil {
		return ExplainIndexResult{}, fmt.Errorf("failed to scan blueprints: %w", err)
	}
	bpSummaries := make([]ExplainBlueprintSummary, 0, len(blueprints))
	for _, bp := range blueprints {
		bpSummaries = append(bpSummaries, ExplainBlueprintSummary{
			Path:           bp.Path,
			DeploymentPath: bp.DeploymentPath,
			Scheduler:      bp.Scheduler,
			MachineTypes:   bp.MachineTypes,
			Storage:        extractBlueprintStorageTags(bp),
		})
	}

	mods := listTopLevelEmbeddedModules(sourcereader.ModuleFS)
	groupedMods := make(map[string][]string)
	for _, m := range mods {
		groupedMods[m.Category] = append(groupedMods[m.Category], m.Path)
	}

	return ExplainIndexResult{
		Blueprints: bpSummaries,
		Modules:    groupedMods,
		Hint:       "Run './gcluster explain <path>' to inspect a blueprint/module, or './gcluster explain <specs...>' (e.g. 'slurm a3u lustre') to find the closest blueprint.",
	}, nil
}

var storageTagSuffixes = []struct {
	suffix string
	tag    string
}{
	{"/managed-lustre", "managed-lustre"},
	{"/filestore", "filestore"},
	{"/cloud-storage-bucket", "gcs-bucket"},
	{"/parallelstore", "parallelstore"},
	{"/netapp-storage-pool", "netapp"},
	{"/gke-storage", "gke-storage"},
	{"/pre-existing-network-storage", "network-storage"},
}

func extractBlueprintStorageTags(bp catalog.BlueprintEntry) []string {
	var tags []string
	for _, m := range bp.Modules {
		for _, rule := range storageTagSuffixes {
			if strings.HasSuffix(m.Source, rule.suffix) && !slices.Contains(tags, rule.tag) {
				tags = append(tags, rule.tag)
			}
		}
	}
	return tags
}

func printExplainIndexText(w io.Writer, res ExplainIndexResult) {
	writef(w, "Available Blueprints (%d):\n", len(res.Blueprints))
	if len(res.Blueprints) == 0 {
		writeln(w, "  (no Cluster Toolkit checkout found from current directory; blueprints are read from examples/ in a repo checkout)")
	} else {
		for _, bp := range res.Blueprints {
			printBlueprintSummaryLine(w, bp)
		}
	}

	writeln(w, "\nAvailable Embedded Modules:")
	cats := make([]string, 0, len(res.Modules))
	for c := range res.Modules {
		cats = append(cats, c)
	}
	sort.Strings(cats)
	for _, c := range cats {
		writef(w, "  [%s]\n", c)
		for _, p := range res.Modules[c] {
			writef(w, "    - %s\n", p)
		}
	}
	writef(w, "\nHint: %s\n", res.Hint)
}

func printBlueprintSummaryLine(w io.Writer, bp ExplainBlueprintSummary) {
	var meta []string
	if bp.Scheduler != "" && bp.Scheduler != "none" {
		meta = append(meta, "scheduler="+bp.Scheduler)
	}
	if len(bp.MachineTypes) > 0 {
		meta = append(meta, "machine="+strings.Join(bp.MachineTypes, ","))
	}
	if len(bp.Storage) > 0 {
		meta = append(meta, "storage="+strings.Join(bp.Storage, ","))
	}
	if len(meta) > 0 {
		writef(w, "  - %-68s [%s]\n", bp.Path, strings.Join(meta, " | "))
	} else {
		writef(w, "  - %s\n", bp.Path)
	}
}

func resolveBlueprintTarget(repoRoot, target, explicitDepl string) (string, string, bool, error) {
	candidates := []string{target}
	cleanRel := strings.TrimPrefix(sourcereader.ToSlash(filepath.Clean(target)), "./")
	if repoRoot != "" {
		candidates = append(candidates, filepath.Join(repoRoot, filepath.FromSlash(cleanRel)))
	}

	var firstErr error
	for _, cand := range candidates {
		bpPath, deplPath, ok, err := inspectCandidateBlueprintFile(cand, explicitDepl)
		if ok {
			return bpPath, deplPath, true, nil
		}
		if err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return "", "", false, firstErr
}

func inspectCandidateBlueprintFile(cand, explicitDepl string) (string, string, bool, error) {
	info, err := os.Stat(cand)
	if err != nil {
		return "", "", false, nil
	}
	if info.IsDir() {
		return inspectCandidateBlueprintDir(cand, explicitDepl)
	}
	if !strings.HasSuffix(cand, ".yaml") && !strings.HasSuffix(cand, ".yml") {
		return "", "", false, nil
	}
	if isDeploymentYAMLName(filepath.Base(cand)) {
		if strings.TrimSpace(explicitDepl) != "" {
			return "", "", false, fmt.Errorf("--deployment-file cannot be used when the target is already a deployment file")
		}
		if siblingBP := findSiblingBlueprintForDeployment(cand); siblingBP != "" {
			return siblingBP, cand, true, nil
		}
		return "", "", false, nil
	}
	bp, _, err := config.NewBlueprint(cand)
	if err != nil {
		return "", "", false, fmt.Errorf("failed to parse blueprint %s: %w", cand, err)
	}
	if len(bp.Groups) == 0 {
		return "", "", false, fmt.Errorf("blueprint %s has no deployment_groups", cand)
	}
	return cand, explicitDepl, true, nil
}

func inspectCandidateBlueprintDir(dir, explicitDepl string) (string, string, bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", false, nil
	}
	var matches, names []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isBlueprintCandidateYAML(name) {
			continue
		}
		full := filepath.Join(dir, name)
		if bp, _, err := config.NewBlueprint(full); err == nil && len(bp.Groups) > 0 {
			matches = append(matches, full)
			names = append(names, name)
		}
	}
	if len(matches) == 1 {
		return matches[0], explicitDepl, true, nil
	}
	if len(matches) > 1 {
		return "", "", false, fmt.Errorf("directory %q contains multiple blueprints (%s); specify one", dir, strings.Join(names, ", "))
	}
	return "", "", false, nil
}

func isBlueprintCandidateYAML(name string) bool {
	if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
		return false
	}
	return name != "metadata.yaml" && name != "metadata.display.yaml" && !isDeploymentYAMLName(name)
}

func isDeploymentYAMLName(base string) bool {
	return strings.HasSuffix(base, "-deployment.yaml") || base == "deployment.yaml"
}

func findSiblingBlueprintForDeployment(deplPath string) string {
	dir := filepath.Dir(deplPath)
	base := filepath.Base(deplPath)
	stem := strings.TrimSuffix(base, "-deployment.yaml")
	if stem != base {
		for _, candName := range []string{stem + "-blueprint.yaml", stem + ".yaml"} {
			cand := filepath.Join(dir, candName)
			if bp, _, err := config.NewBlueprint(cand); err == nil && len(bp.Groups) > 0 {
				return cand
			}
		}
	}
	return firstBlueprintInDir(dir, base)
}

func firstBlueprintInDir(dir, skipBase string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") || e.Name() == skipBase {
			continue
		}
		cand := filepath.Join(dir, e.Name())
		if bp, _, err := config.NewBlueprint(cand); err == nil && len(bp.Groups) > 0 {
			return cand
		}
	}
	return ""
}

func resolveModuleTarget(target string) (string, string, bool) {
	norm := strings.TrimSuffix(strings.TrimPrefix(sourcereader.ToSlash(filepath.Clean(target)), "./"), "/")

	if kind := detectEmbeddedModuleKind(sourcereader.ModuleFS, norm); kind != "" {
		return norm, kind, true
	}

	if modPath, kind, ok := matchEmbeddedModuleShortName(norm); ok {
		return modPath, kind, true
	}

	if info, err := os.Stat(target); err == nil && info.IsDir() {
		if kind := detectLocalModuleKind(target); kind != "" {
			localPath := target
			if !sourcereader.IsLocalPath(localPath) {
				localPath = "./" + sourcereader.ToSlash(localPath)
			}
			return localPath, kind, true
		}
	}
	return "", "", false
}

func matchEmbeddedModuleShortName(name string) (string, string, bool) {
	allMods := listTopLevelEmbeddedModules(sourcereader.ModuleFS)
	var matches []embeddedModEntry
	for _, m := range allMods {
		if m.Name == name {
			matches = append(matches, m)
		}
	}
	if len(matches) == 0 {
		return "", "", false
	}
	for _, m := range matches {
		if strings.HasPrefix(m.Path, "modules/") {
			return m.Path, m.Kind, true
		}
	}
	return matches[0].Path, matches[0].Kind, true
}

func buildExplainBlueprint(repoRoot, bpPath, explicitDeplPath string) (ExplainBlueprintResult, error) {
	bp, _, err := config.NewBlueprint(bpPath)
	if err != nil {
		return ExplainBlueprintResult{}, fmt.Errorf("failed to parse blueprint %s: %w", bpPath, err)
	}

	displayBPPath := displayRelPath(repoRoot, bpPath)
	displayDeplPath, ds, err := loadCompanionDeployment(repoRoot, bpPath, bp.BlueprintName, explicitDeplPath)
	if err != nil {
		return ExplainBlueprintResult{}, err
	}
	apis, stages := collectBlueprintAPIsAndStages(bp)

	gcloudCmd := ""
	if len(apis) > 0 {
		gcloudCmd = "gcloud services enable " + strings.Join(apis, " ")
	}

	varsList := mergeAndFormatBlueprintVars(bp.Vars, ds.Vars)
	deployCmd := fmt.Sprintf("./gcluster deploy %s", displayBPPath)
	if displayDeplPath != "" {
		deployCmd = fmt.Sprintf("./gcluster deploy -d %s %s", displayDeplPath, displayBPPath)
	}

	return ExplainBlueprintResult{
		Path:           displayBPPath,
		DeploymentPath: displayDeplPath,
		BlueprintName:  bp.BlueprintName,
		RequiredAPIs:   apis,
		GcloudCommand:  gcloudCmd,
		Variables:      varsList,
		Stages:         stages,
		DeployCommand:  deployCmd,
	}, nil
}

func loadCompanionDeployment(repoRoot, bpPath, blueprintName, explicitDeplPath string) (string, config.DeploymentSettings, error) {
	deplPath := strings.TrimSpace(explicitDeplPath)
	if deplPath == "" {
		deplPath = discoverCompanionDeploymentFile(bpPath, blueprintName)
	}
	if deplPath == "" {
		return "", config.DeploymentSettings{}, nil
	}
	displayDeplPath := displayRelPath(repoRoot, deplPath)
	parsedDS, _, err := config.NewDeploymentSettings(deplPath)
	if err != nil {
		return "", config.DeploymentSettings{}, fmt.Errorf("failed to load deployment file %s: %w", deplPath, err)
	}
	return displayDeplPath, parsedDS, nil
}

func collectBlueprintAPIsAndStages(bp config.Blueprint) ([]string, []ExplainStage) {
	servicesSet := make(map[string]bool)
	stages := make([]ExplainStage, 0, len(bp.Groups))

	for _, grp := range bp.Groups {
		stageMods := make([]ExplainStageModule, 0, len(grp.Modules))
		for _, mod := range grp.Modules {
			normSrc := strings.TrimPrefix(strings.TrimSpace(mod.Source), "./")
			for _, svc := range extractModuleRequiredServices(mod, normSrc) {
				servicesSet[svc] = true
			}
			useIDs := make([]string, 0, len(mod.Use))
			for _, u := range mod.Use {
				useIDs = append(useIDs, string(u))
			}
			stageMods = append(stageMods, ExplainStageModule{
				ID:          string(mod.ID),
				Source:      normSrc,
				ShortSource: path.Base(normSrc),
				Use:         useIDs,
			})
		}
		stages = append(stages, ExplainStage{
			Group:   string(grp.Name),
			Modules: stageMods,
		})
	}

	apis := make([]string, 0, len(servicesSet))
	for s := range servicesSet {
		apis = append(apis, s)
	}
	sort.Strings(apis)
	return apis, stages
}

func extractModuleRequiredServices(mod config.Module, normSrc string) []string {
	kind := mod.Kind.String()
	if kind == "" {
		kind = config.TerraformKind.String()
	}
	if kind != "terraform" && kind != "packer" {
		return nil
	}
	if detectEmbeddedModuleKind(sourcereader.ModuleFS, normSrc) != "" {
		if mi, err := modulereader.GetModuleInfo(normSrc, kind); err == nil {
			return nonEmptyStrings(mi.Metadata.Spec.Requirements.Services)
		}
	} else if sourcereader.IsLocalPath(mod.Source) {
		if mtd, err := modulereader.GetMetadata(mod.Source); err == nil {
			return nonEmptyStrings(mtd.Spec.Requirements.Services)
		}
	}
	return nil
}

func nonEmptyStrings(items []string) []string {
	out := make([]string, 0, len(items))
	for _, s := range items {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func displayRelPath(repoRoot, targetPath string) string {
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return sourcereader.ToSlash(targetPath)
	}
	if repoRoot != "" {
		if absRoot, err := filepath.Abs(repoRoot); err == nil {
			if rel, err := filepath.Rel(absRoot, absTarget); err == nil && !strings.HasPrefix(rel, "..") {
				return sourcereader.ToSlash(rel)
			}
		}
	}
	return sourcereader.ToSlash(targetPath)
}

func discoverCompanionDeploymentFile(bpPath, blueprintName string) string {
	dir := filepath.Dir(bpPath)
	base := filepath.Base(bpPath)
	stem := strings.TrimSuffix(strings.TrimSuffix(base, ".yaml"), ".yml")

	candidates := []string{filepath.Join(dir, stem+"-deployment.yaml")}
	if blueprintName != "" {
		candidates = append(candidates, filepath.Join(dir, blueprintName+"-deployment.yaml"))
	}
	for _, suffix := range []string{"-blueprint", "-advanced", "-infra", "-experiment"} {
		if trimmed := strings.TrimSuffix(stem, suffix); trimmed != stem {
			candidates = append(candidates, filepath.Join(dir, trimmed+"-deployment.yaml"))
		}
	}
	candidates = append(candidates, filepath.Join(dir, "deployment.yaml"))

	for _, cand := range candidates {
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	return ""
}

func mergeAndFormatBlueprintVars(bpVars, dsVars config.Dict) []ExplainVariable {
	merged := make(map[string]cty.Value)
	for k, v := range bpVars.Items() {
		merged[k] = v
	}
	// Mirror cmd/create.go:mergeDeploymentSettings: deployment values (including null) always win.
	for k, v := range dsVars.Items() {
		merged[k] = v
	}

	orderedKeys := orderBlueprintVarKeys(merged)
	out := make([]ExplainVariable, 0, len(orderedKeys))
	for _, k := range orderedKeys {
		out = append(out, inspectCtyVariable(k, merged[k]))
	}
	return out
}

func orderBlueprintVarKeys(merged map[string]cty.Value) []string {
	keys := make([]string, 0, len(merged))
	for k := range merged {
		if k != "labels" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	priorityOrder := []string{"project_id", "deployment_name", "region", "zone"}
	var orderedKeys []string
	for _, pk := range priorityOrder {
		if _, ok := merged[pk]; ok {
			orderedKeys = append(orderedKeys, pk)
		}
	}
	for _, k := range keys {
		if !slices.Contains(priorityOrder, k) {
			orderedKeys = append(orderedKeys, k)
		}
	}
	return orderedKeys
}

func inspectCtyVariable(name string, v cty.Value) ExplainVariable {
	if expr, isExpr := config.IsExpressionValue(v); isExpr {
		return inspectExpressionVariable(name, expr)
	}
	if !v.IsKnown() || v.IsNull() {
		return ExplainVariable{Name: name, Type: inferPlaceholderType(name), Required: true}
	}
	return inspectKnownCtyVariable(name, v)
}

func inspectExpressionVariable(name string, expr config.Expression) ExplainVariable {
	raw := strings.TrimSpace(string(hclwrite.Format(expr.Tokenize().Bytes())))
	if raw == "null" {
		// $(null) is the idiomatic "optional, unset by default" marker; validateVars accepts it.
		return ExplainVariable{Name: name, Type: "expression", Required: false, Default: "null"}
	}
	return ExplainVariable{Name: name, Type: "expression", Required: false, Default: "((" + raw + "))"}
}

func inspectKnownCtyVariable(name string, v cty.Value) ExplainVariable {
	ty := v.Type()
	switch {
	case ty == cty.String:
		return inspectStringVariable(name, v.AsString())
	case ty == cty.Number:
		return ExplainVariable{Name: name, Type: "number", Required: false, Default: formatCtyNumber(v)}
	case ty == cty.Bool:
		return ExplainVariable{Name: name, Type: "bool", Required: false, Default: fmt.Sprintf("%t", v.True())}
	case ty.IsTupleType() || ty.IsListType() || ty.IsSetType():
		return ExplainVariable{Name: name, Type: "list", Required: false, Default: fmt.Sprintf("[%d items]", v.LengthInt())}
	case ty.IsObjectType() || ty.IsMapType():
		return ExplainVariable{Name: name, Type: "map/object", Required: false, Default: "{...}"}
	default:
		return ExplainVariable{Name: name, Type: ty.FriendlyName(), Required: false}
	}
}

func inspectStringVariable(name, s string) ExplainVariable {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "<<") ||
		strings.HasPrefix(trimmed, "YOUR_") || strings.HasPrefix(trimmed, "<") {
		return ExplainVariable{Name: name, Type: "string", Required: true}
	}
	return ExplainVariable{Name: name, Type: "string", Required: false, Default: fmt.Sprintf("%q", s)}
}

func formatCtyNumber(v cty.Value) string {
	bf := v.AsBigFloat()
	if i, acc := bf.Int64(); acc == 0 {
		return fmt.Sprintf("%d", i)
	}
	f, _ := bf.Float64()
	return fmt.Sprintf("%g", f)
}

func inferPlaceholderType(name string) string {
	lower := strings.ToLower(name)
	for _, kw := range []string{"count", "size", "gb", "gib"} {
		if strings.Contains(lower, kw) {
			return "number"
		}
	}
	if strings.HasPrefix(lower, "enable_") || strings.HasSuffix(lower, "_enabled") {
		return "bool"
	}
	return "string"
}

func printExplainBlueprintText(w io.Writer, res ExplainBlueprintResult) {
	writef(w, "Blueprint: %s\n", res.Path)
	if res.DeploymentPath != "" {
		writef(w, "Companion Deployment File: %s\n", res.DeploymentPath)
	}

	writeln(w, "\n1. Enable Required GCP APIs:")
	if res.GcloudCommand != "" {
		writef(w, "   %s\n", res.GcloudCommand)
	} else {
		writeln(w, "   (no module service requirements declared)")
	}

	if res.DeploymentPath != "" {
		writef(w, "\n2. Customizable Variables (in %s):\n", path.Base(res.DeploymentPath))
	} else {
		writeln(w, "\n2. Customizable Variables (vars):")
	}
	for _, v := range res.Variables {
		if v.Required {
			writef(w, "   - %-26s (%s, required)\n", v.Name, v.Type)
		} else {
			writef(w, "   - %-26s (%s, default: %s)\n", v.Name, v.Type, v.Default)
		}
	}

	writeln(w, "\n3. Deployment Stages & Modules:")
	for i, st := range res.Stages {
		modParts := make([]string, 0, len(st.Modules))
		for _, m := range st.Modules {
			modParts = append(modParts, fmt.Sprintf("%s (%s)", m.ID, m.ShortSource))
		}
		writef(w, "   Stage %d [%s]: %s\n", i+1, st.Group, strings.Join(modParts, ", "))
	}

	writeln(w, "\n4. Deploy Command:")
	writef(w, "   %s\n", res.DeployCommand)
	if len(res.Stages) > 1 {
		stageNames := make([]string, 0, len(res.Stages))
		for _, st := range res.Stages {
			stageNames = append(stageNames, st.Group)
		}
		writef(w, "   (Optional staged deploy with --only: %s)\n", strings.Join(stageNames, " -> "))
	}
}

type upstreamCandidateGroup struct {
	PrimarySource string
	PrimaryID     string
	MatchedInputs []string
	Alternatives  []string
}

func buildExplainModule(modPath, kind string) (ExplainModuleResult, error) {
	mi, err := modulereader.GetModuleInfo(modPath, kind)
	if err != nil {
		return ExplainModuleResult{}, fmt.Errorf("failed to inspect module %s: %w", modPath, err)
	}

	apis := nonEmptyStrings(mi.Metadata.Spec.Requirements.Services)
	sort.Strings(apis)
	gcloudCmd := ""
	if len(apis) > 0 {
		gcloudCmd = "gcloud services enable " + strings.Join(apis, " ")
	}

	upstreamGroups, useProvidedInputs := findUpstreamCandidateGroups(modPath, mi)
	autoInputs := buildModuleAutoInputs(mi, upstreamGroups)
	reqInputs, optInputs := partitionModuleInputs(mi, useProvidedInputs)

	outputs := make([]string, 0, len(mi.Outputs))
	for _, out := range mi.Outputs {
		outputs = append(outputs, out.Name)
	}
	sort.Strings(outputs)

	snippet := renderModuleYAMLBlock(defaultModuleID(modPath), modPath, upstreamIDsFromGroups(upstreamGroups), reqInputs, optInputs)

	return ExplainModuleResult{
		Module:         modPath,
		Kind:           kind,
		Deprecated:     mi.Metadata.Ghpc.DeprecationDate,
		Alternative:    mi.Metadata.Ghpc.AlternativeModule,
		RequiredAPIs:   apis,
		GcloudCommand:  gcloudCmd,
		Snippet:        snippet,
		AutoInputs:     autoInputs,
		RequiredInputs: reqInputs,
		OptionalInputs: optInputs,
		Outputs:        outputs,
	}, nil
}

func buildModuleAutoInputs(mi modulereader.ModuleInfo, upstreamGroups []upstreamCandidateGroup) []ExplainAutoInput {
	var autoInputs []ExplainAutoInput
	for _, ug := range upstreamGroups {
		srcDesc := fmt.Sprintf("from use: [%s]", ug.PrimaryID)
		if len(ug.Alternatives) > 0 {
			srcDesc += fmt.Sprintf(" (alternatives: %s)", strings.Join(ug.Alternatives, ", "))
		}
		autoInputs = append(autoInputs, ExplainAutoInput{
			Inputs: ug.MatchedInputs,
			Source: srcDesc,
		})
	}

	var globalInjected []string
	for _, in := range mi.Inputs {
		if explainAutoGlobals[in.Name] {
			globalInjected = append(globalInjected, in.Name)
		}
	}
	sort.Strings(globalInjected)
	if len(globalInjected) > 0 {
		autoInputs = append(autoInputs, ExplainAutoInput{
			Inputs: globalInjected,
			Source: "from top-level vars",
		})
	}
	if injectedID := mi.Metadata.Ghpc.InjectModuleId; injectedID != "" {
		autoInputs = append(autoInputs, ExplainAutoInput{
			Inputs: []string{injectedID},
			Source: "from module id (ghpc.inject_module_id)",
		})
	}
	return autoInputs
}

func partitionModuleInputs(mi modulereader.ModuleInfo, useProvidedInputs map[string]bool) ([]ExplainModuleInput, []ExplainModuleInput) {
	injectedID := mi.Metadata.Ghpc.InjectModuleId
	var reqInputs []ExplainModuleInput
	var optInputs []ExplainModuleInput
	for _, in := range mi.Inputs {
		if explainAutoGlobals[in.Name] || in.Name == injectedID || useProvidedInputs[in.Name] || in.Name == "machine_configs" {
			continue
		}
		entry := ExplainModuleInput{
			Name:        in.Name,
			Type:        formatCtyType(in.Type),
			Default:     formatInterfaceDefault(in.Default),
			Description: firstSentence(in.Description),
		}
		if in.Required {
			reqInputs = append(reqInputs, entry)
		} else {
			optInputs = append(optInputs, entry)
		}
	}
	sort.Slice(reqInputs, func(i, j int) bool { return reqInputs[i].Name < reqInputs[j].Name })
	sort.Slice(optInputs, func(i, j int) bool { return optInputs[i].Name < optInputs[j].Name })
	return reqInputs, optInputs
}

type candidateProvider struct {
	source  string
	outputs map[string]bool
}

func findUpstreamCandidateGroups(targetSource string, targetInfo modulereader.ModuleInfo) ([]upstreamCandidateGroup, map[string]bool) {
	targetInputs, targetReqInputs := classifyTargetModuleInputs(targetInfo)
	providers := discoverCandidateProviders(targetSource)
	inputToProviders := matchProvidersToRequiredInputs(targetReqInputs, providers)
	primaryToInputs, primaryToAlts, useProvided := selectPrimaryProviders(inputToProviders, targetInputs, providers)
	return buildSortedUpstreamGroups(primaryToInputs, primaryToAlts), useProvided
}

func classifyTargetModuleInputs(targetInfo modulereader.ModuleInfo) (map[string]bool, map[string]bool) {
	targetInputs := make(map[string]bool, len(targetInfo.Inputs))
	targetReqInputs := make(map[string]bool)
	injectedID := targetInfo.Metadata.Ghpc.InjectModuleId
	for _, in := range targetInfo.Inputs {
		targetInputs[in.Name] = true
		if in.Required && !explainAutoGlobals[in.Name] && in.Name != injectedID {
			targetReqInputs[in.Name] = true
		}
	}
	return targetInputs, targetReqInputs
}

func discoverCandidateProviders(targetSource string) []candidateProvider {
	all := allCandidateProviders(sourcereader.ModuleFS)
	providers := make([]candidateProvider, 0, len(all))
	for _, p := range all {
		if p.source != targetSource {
			providers = append(providers, p)
		}
	}
	return providers
}

func allCandidateProviders(bfs sourcereader.BaseFS) []candidateProvider {
	explainCacheMu.Lock()
	if sameBaseFS(explainProviderFSKey, bfs) && len(explainProviderCache) > 0 {
		cached := slices.Clone(explainProviderCache)
		explainCacheMu.Unlock()
		return cached
	}
	explainCacheMu.Unlock()

	var providers []candidateProvider
	for _, m := range listTopLevelEmbeddedModules(bfs) {
		if m.Kind == "terraform" {
			if outs := extractProviderTrueOutputs(m); len(outs) > 0 {
				providers = append(providers, candidateProvider{source: m.Path, outputs: outs})
			}
		}
	}
	if len(providers) > 0 {
		explainCacheMu.Lock()
		explainProviderFSKey = bfs
		explainProviderCache = providers
		explainCacheMu.Unlock()
	}
	return providers
}

func extractProviderTrueOutputs(m embeddedModEntry) map[string]bool {
	info, err := modulereader.GetModuleInfo(m.Path, m.Kind)
	if err != nil || info.Metadata.Ghpc.DeprecationDate != "" {
		return nil
	}
	inMap := make(map[string]bool, len(info.Inputs))
	hasReq := false
	for _, in := range info.Inputs {
		inMap[in.Name] = true
		if in.Required && !explainAutoGlobals[in.Name] && in.Name != info.Metadata.Ghpc.InjectModuleId {
			hasReq = true
		}
	}
	outs := make(map[string]bool)
	for _, o := range info.Outputs {
		if !explainGenericOutputs[o.Name] && (!inMap[o.Name] || !hasReq) {
			outs[o.Name] = true
		}
	}
	return outs
}

func matchProvidersToRequiredInputs(targetReqInputs map[string]bool, providers []candidateProvider) map[string][]string {
	inputToProviders := make(map[string][]string)
	for reqIn := range targetReqInputs {
		for _, p := range providers {
			if p.outputs[reqIn] {
				inputToProviders[reqIn] = append(inputToProviders[reqIn], p.source)
			}
		}
	}
	return inputToProviders
}

func selectPrimaryProviders(
	inputToProviders map[string][]string,
	targetInputs map[string]bool,
	providers []candidateProvider,
) (map[string][]string, map[string]map[string]bool, map[string]bool) {
	primaryToInputs := make(map[string][]string)
	primaryToAlts := make(map[string]map[string]bool)
	useProvided := make(map[string]bool)

	for _, reqIn := range sortedRequiredInputKeys(inputToProviders) {
		cands := inputToProviders[reqIn]
		if len(cands) == 0 {
			continue
		}
		primary := chooseProviderForInput(cands, primaryToInputs)
		primaryToInputs[primary] = append(primaryToInputs[primary], reqIn)
		useProvided[reqIn] = true
		if primaryToAlts[primary] == nil {
			primaryToAlts[primary] = make(map[string]bool)
		}
		for _, c := range cands {
			if c != primary {
				primaryToAlts[primary][path.Base(c)] = true
			}
		}
	}

	for _, p := range providers {
		if _, isSelected := primaryToInputs[p.source]; !isSelected {
			continue
		}
		for outName := range p.outputs {
			if targetInputs[outName] && !useProvided[outName] && !explainAutoGlobals[outName] {
				primaryToInputs[p.source] = append(primaryToInputs[p.source], outName)
				useProvided[outName] = true
			}
		}
	}
	return primaryToInputs, primaryToAlts, useProvided
}

func sortedRequiredInputKeys(inputToProviders map[string][]string) []string {
	reqKeys := make([]string, 0, len(inputToProviders))
	for k := range inputToProviders {
		reqKeys = append(reqKeys, k)
	}
	sort.Slice(reqKeys, func(i, j int) bool {
		li, lj := len(inputToProviders[reqKeys[i]]), len(inputToProviders[reqKeys[j]])
		if li != lj {
			return li < lj
		}
		return reqKeys[i] < reqKeys[j]
	})
	return reqKeys
}

func chooseProviderForInput(cands []string, selected map[string][]string) string {
	var active []string
	for _, c := range cands {
		if _, ok := selected[c]; ok {
			active = append(active, c)
		}
	}
	if len(active) > 0 {
		return pickPreferredProvider(active)
	}
	return pickPreferredProvider(cands)
}

func buildSortedUpstreamGroups(primaryToInputs map[string][]string, primaryToAlts map[string]map[string]bool) []upstreamCandidateGroup {
	primaries := make([]string, 0, len(primaryToInputs))
	for p := range primaryToInputs {
		primaries = append(primaries, p)
	}
	sort.Slice(primaries, func(i, j int) bool {
		ri, rj := providerOrderRank(primaries[i]), providerOrderRank(primaries[j])
		if ri != rj {
			return ri < rj
		}
		return primaries[i] < primaries[j]
	})

	var groups []upstreamCandidateGroup
	for _, p := range primaries {
		inputs := primaryToInputs[p]
		sort.Strings(inputs)
		var alts []string
		for a := range primaryToAlts[p] {
			alts = append(alts, a)
		}
		sort.Strings(alts)
		groups = append(groups, upstreamCandidateGroup{
			PrimarySource: p,
			PrimaryID:     defaultModuleID(p),
			MatchedInputs: inputs,
			Alternatives:  alts,
		})
	}
	return groups
}

var preferredProviderOrder = []string{
	"modules/network/vpc",
	"modules/network/private-service-access",
	"modules/scheduler/gke-cluster",
	"modules/file-system/filestore",
	"modules/file-system/managed-lustre",
	"community/modules/compute/schedmd-slurm-gcp-v6-nodeset",
	"community/modules/compute/schedmd-slurm-gcp-v6-partition",
}

func providerOrderRank(source string) int {
	if idx := slices.Index(preferredProviderOrder, source); idx >= 0 {
		return idx
	}
	return len(preferredProviderOrder)
}

func pickPreferredProvider(candidates []string) string {
	for _, pref := range preferredProviderOrder {
		if slices.Contains(candidates, pref) {
			return pref
		}
	}
	for _, c := range candidates {
		if strings.HasPrefix(c, "modules/") && !strings.Contains(c, "pre-existing") {
			return c
		}
	}
	for _, c := range candidates {
		if strings.HasPrefix(c, "modules/") {
			return c
		}
	}
	return candidates[0]
}

func defaultModuleID(modPath string) string {
	base := path.Base(modPath)
	switch base {
	case "managed-lustre":
		return "lustre"
	case "cloud-storage-bucket":
		return "gcs_bucket"
	case "private-service-access":
		return "private_service_access"
	default:
		return strings.ReplaceAll(base, "-", "_")
	}
}

func upstreamIDsFromGroups(groups []upstreamCandidateGroup) []string {
	ids := make([]string, 0, len(groups))
	for _, g := range groups {
		ids = append(ids, g.PrimaryID)
	}
	return ids
}

func renderModuleYAMLBlock(id, source string, useIDs []string, reqInputs, optInputs []ExplainModuleInput) string {
	var sb strings.Builder
	writef(&sb, "- id: %s\n", id)
	writef(&sb, "  source: %s\n", source)
	if detectEmbeddedModuleKind(sourcereader.ModuleFS, source) == "packer" || detectLocalModuleKind(source) == "packer" {
		sb.WriteString("  kind: packer\n")
	}
	if len(useIDs) > 0 {
		writef(&sb, "  use: [%s]\n", strings.Join(useIDs, ", "))
	}

	settingsToRender := buildSampleSettings(source, reqInputs, optInputs)
	if len(settingsToRender) > 0 {
		sb.WriteString("  settings:\n")
		for _, kv := range settingsToRender {
			writef(&sb, "    %s: %s\n", kv[0], kv[1])
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func buildSampleSettings(source string, reqInputs, _ []ExplainModuleInput) [][2]string {
	var out [][2]string
	added := make(map[string]bool)

	switch {
	case strings.HasSuffix(source, "/managed-lustre"):
		out = append(out,
			[2]string{"name", "$(vars.deployment_name)-lustre"},
			[2]string{"size_gib", "36000"},
			[2]string{"local_mount", "/lustre"},
			[2]string{"remote_mount", "lustrefs"},
		)
		added["name"], added["size_gib"], added["local_mount"], added["remote_mount"] = true, true, true, true
	case strings.HasSuffix(source, "/filestore"):
		out = append(out, [2]string{"local_mount", "/shared"})
		added["local_mount"] = true
	case strings.HasSuffix(source, "/cloud-storage-bucket"):
		out = append(out, [2]string{"local_mount", "/gcs"}, [2]string{"random_suffix", "true"})
		added["local_mount"], added["random_suffix"] = true, true
	}

	for _, req := range reqInputs {
		if !added[req.Name] {
			out = append(out, [2]string{req.Name, sampleValueForInput(req.Name, req.Type)})
			added[req.Name] = true
		}
	}
	return out
}

var sampleByInputName = map[string]string{
	"remote_mount": "lustrefs",
	"local_mount":  "/shared",
	"machine_type": "n2-standard-8",
	"size_gib":     "36000",
}

var sampleByInputType = map[string]string{
	"number": "1",
	"bool":   "true",
	"list":   "[]",
	"map":    "{}",
	"object": "{}",
}

func sampleValueForInput(name, typ string) string {
	if v, ok := sampleByInputName[name]; ok {
		return v
	}
	if v, ok := sampleByInputType[typ]; ok {
		return v
	}
	return fmt.Sprintf("<%s>", name)
}

func formatCtyType(t cty.Type) string {
	if t == cty.NilType {
		return "any"
	}
	switch {
	case t == cty.String:
		return "string"
	case t == cty.Number:
		return "number"
	case t == cty.Bool:
		return "bool"
	case t.IsListType() || t.IsTupleType() || t.IsSetType():
		return "list"
	case t.IsMapType() || t.IsObjectType():
		return "object"
	default:
		return t.FriendlyName()
	}
}

func formatInterfaceDefault(v interface{}) string {
	if v == nil {
		return ""
	}
	if val, ok := v.(string); ok {
		return fmt.Sprintf("%q", val)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}

func firstSentence(desc string) string {
	s := strings.Join(strings.Fields(desc), " ")
	if idx := strings.Index(s, ". "); idx > 0 && idx < 120 {
		return s[:idx+1]
	}
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

func printExplainModuleText(w io.Writer, res ExplainModuleResult) {
	writef(w, "Module: %s (%s)\n", res.Module, res.Kind)
	if res.Deprecated != "" {
		writef(w, "WARNING: Deprecated (%s). Recommended replacement: %s\n", res.Deprecated, res.Alternative)
	}

	writeln(w, "Required GCP APIs (metadata.yaml):")
	if res.GcloudCommand != "" {
		writef(w, "  %s\n", res.GcloudCommand)
	} else {
		writeln(w, "  (none)")
	}

	writeln(w, "\nReady-to-Copy Blueprint Snippet (with `use:`):")
	for _, line := range strings.Split(res.Snippet, "\n") {
		writef(w, "  %s\n", line)
	}

	if len(res.AutoInputs) > 0 {
		writeln(w, "\nInputs Filled Automatically (do NOT set manually in settings):")
		for _, ai := range res.AutoInputs {
			writef(w, "  - %-34s <-- %s\n", strings.Join(ai.Inputs, ", "), ai.Source)
		}
	}

	if len(res.RequiredInputs) > 0 {
		writeln(w, "\nRequired Settings (variables.tf):")
		for _, in := range res.RequiredInputs {
			writef(w, "  - %-28s (%s) %s\n", in.Name, in.Type, in.Description)
		}
	}

	printOptionalModuleInputs(w, res.OptionalInputs)

	if len(res.Outputs) > 0 {
		writef(w, "\nModule Outputs (%d): %s\n", len(res.Outputs), strings.Join(res.Outputs, ", "))
	}
}

func printOptionalModuleInputs(w io.Writer, optInputs []ExplainModuleInput) {
	if len(optInputs) == 0 {
		return
	}
	names := make([]string, 0, len(optInputs))
	for _, in := range optInputs {
		names = append(names, in.Name)
	}
	writef(w, "\nOptional Settings (%d): %s\n", len(names), strings.Join(names, ", "))
}

type normalizedSpec struct {
	Raw          string
	Canonical    string
	Category     string // "scheduler", "hardware", "module", "keyword"
	ModuleSource string
	Aliases      []string
}

type specAliasDef struct {
	Canonical    string
	Category     string
	ModuleSource string
	Aliases      []string
	Tokens       []string
}

var knownSpecAliasTable = []specAliasDef{
	{Canonical: "slurm", Category: "scheduler", Aliases: []string{"slurm"}, Tokens: []string{"slurm"}},
	{Canonical: "gke", Category: "scheduler", Aliases: []string{"gke"}, Tokens: []string{"gke", "kubernetes", "k8s"}},
	{Canonical: "slurm-on-gke (slinky)", Category: "scheduler", Aliases: []string{"slinky"}, Tokens: []string{"slinky", "slurm-on-gke"}},
	{Canonical: "batch", Category: "scheduler", Aliases: []string{"batch"}, Tokens: []string{"batch", "gcp-batch"}},
	{Canonical: "htcondor", Category: "scheduler", Aliases: []string{"htcondor"}, Tokens: []string{"htcondor", "condor"}},
	{Canonical: "flux", Category: "scheduler", Aliases: []string{"flux"}, Tokens: []string{"flux"}},

	{Canonical: "a3-ultragpu-8g", Category: "hardware", Aliases: []string{"a3-ultragpu-8g", "a3-ultragpu", "a3ultra"}, Tokens: []string{"a3u", "a3-ultra", "a3ultra", "h200", "a3-ultragpu", "a3-ultragpu-8g"}},
	{Canonical: "a3-megagpu-8g", Category: "hardware", Aliases: []string{"a3-megagpu-8g", "a3-megagpu", "a3mega"}, Tokens: []string{"a3m", "a3-mega", "a3mega", "a3-megagpu", "a3-megagpu-8g"}},
	{Canonical: "a3-highgpu-8g", Category: "hardware", Aliases: []string{"a3-highgpu-8g", "a3-highgpu", "a3high"}, Tokens: []string{"a3h", "a3-high", "a3high", "h100", "a3-highgpu", "a3-highgpu-8g"}},
	{Canonical: "a4-highgpu-8g", Category: "hardware", Aliases: []string{"a4-highgpu-8g", "a4-highgpu", "a4high", "gke-a4"}, Tokens: []string{"a4", "a4h", "a4-high", "a4high", "b200", "a4-highgpu", "a4-highgpu-8g"}},
	{Canonical: "a4x-highgpu-4g", Category: "hardware", Aliases: []string{"a4x-highgpu-4g", "a4xhigh", "gke-a4x"}, Tokens: []string{"a4x", "a4xh", "a4x-high", "a4xhigh", "gb200", "a4x-highgpu", "a4x-highgpu-4g"}},
	{Canonical: "a4x-maxgpu-4g-metal", Category: "hardware", Aliases: []string{"a4x-maxgpu-4g-metal", "a4xmax-bm", "gke-a4x-max-bm"}, Tokens: []string{"a4xmax", "a4x-max", "a4x-max-bm", "a4xmax-bm", "gb300", "a4x-maxgpu", "a4x-maxgpu-4g-metal"}},
	{Canonical: "tpu7x-standard-4t", Category: "hardware", Aliases: []string{"tpu7x-standard-4t", "tpu-7x", "tpu7x"}, Tokens: []string{"tpu7x", "tpu-7x", "7x", "ironwood", "tpu7x-standard-4t"}},
	{Canonical: "tpu-v6e", Category: "hardware", Aliases: []string{"tpu-v6e", "v6e", "ct6e"}, Tokens: []string{"tpuv6e", "tpu-v6e", "v6e", "trillium"}},
	{Canonical: "tpu-v5p", Category: "hardware", Aliases: []string{"tpu-v5p", "v5p", "ct5p"}, Tokens: []string{"tpuv5p", "tpu-v5p", "v5p"}},
	{Canonical: "tpu-v5e", Category: "hardware", Aliases: []string{"tpu-v5e", "v5e", "ct5lp"}, Tokens: []string{"tpuv5e", "tpu-v5e", "v5e"}},
	{Canonical: "tpu-v4", Category: "hardware", Aliases: []string{"tpu-v4", "v4", "ct4p"}, Tokens: []string{"tpuv4", "tpu-v4", "v4"}},
	{Canonical: "tpu", Category: "hardware", Aliases: []string{"tpu", "tpu7x", "ct6e", "ct5p", "ct5lp", "ct4p"}, Tokens: []string{"tpu"}},
	{Canonical: "h4d", Category: "hardware", Aliases: []string{"h4d"}, Tokens: []string{"h4d"}},
	{Canonical: "h3", Category: "hardware", Aliases: []string{"h3"}, Tokens: []string{"h3"}},
	{Canonical: "c4a", Category: "hardware", Aliases: []string{"c4a"}, Tokens: []string{"c4a"}},
	{Canonical: "c3d", Category: "hardware", Aliases: []string{"c3d"}, Tokens: []string{"c3d"}},
	{Canonical: "c3", Category: "hardware", Aliases: []string{"c3"}, Tokens: []string{"c3"}},
	{Canonical: "c2d", Category: "hardware", Aliases: []string{"c2d"}, Tokens: []string{"c2d"}},
	{Canonical: "c2", Category: "hardware", Aliases: []string{"c2"}, Tokens: []string{"c2"}},
	{Canonical: "g2", Category: "hardware", Aliases: []string{"g2"}, Tokens: []string{"g2"}},
	{Canonical: "g4", Category: "hardware", Aliases: []string{"g4"}, Tokens: []string{"g4"}},
	{Canonical: "a2", Category: "hardware", Aliases: []string{"a2"}, Tokens: []string{"a2"}},
	{Canonical: "n2d", Category: "hardware", Aliases: []string{"n2d"}, Tokens: []string{"n2d"}},
	{Canonical: "n2", Category: "hardware", Aliases: []string{"n2"}, Tokens: []string{"n2"}},
	{Canonical: "e2", Category: "hardware", Aliases: []string{"e2"}, Tokens: []string{"e2"}},
	{Canonical: "t2a", Category: "hardware", Aliases: []string{"t2a"}, Tokens: []string{"t2a"}},

	{Canonical: "managed-lustre", Category: "module", ModuleSource: "modules/file-system/managed-lustre", Aliases: []string{"managed-lustre"}, Tokens: []string{"lustre", "managed-lustre"}},
	{Canonical: "filestore", Category: "module", ModuleSource: "modules/file-system/filestore", Aliases: []string{"filestore"}, Tokens: []string{"filestore", "nfs"}},
	{Canonical: "cloud-storage-bucket", Category: "module", ModuleSource: "modules/file-system/cloud-storage-bucket", Aliases: []string{"cloud-storage-bucket"}, Tokens: []string{"gcs", "gcs-bucket", "gcsfuse", "bucket", "cloud-storage", "cloud-storage-bucket"}},
	{Canonical: "parallelstore", Category: "module", ModuleSource: "modules/file-system/parallelstore", Aliases: []string{"parallelstore"}, Tokens: []string{"parallelstore", "daos"}},
	{Canonical: "netapp-storage-pool", Category: "module", ModuleSource: "modules/file-system/netapp-storage-pool", Aliases: []string{"netapp-storage-pool"}, Tokens: []string{"netapp", "netapp-storage-pool"}},
}

func parseSpecTokens(args []string) []normalizedSpec {
	var rawTokens []string
	for _, a := range args {
		for _, part := range strings.FieldsFunc(a, isSpecSeparator) {
			tok := strings.ToLower(strings.TrimSpace(part))
			if tok != "" && !slices.Contains(rawTokens, tok) {
				rawTokens = append(rawTokens, tok)
			}
		}
	}

	out := make([]normalizedSpec, 0, len(rawTokens))
	seenCanonical := make(map[string]bool, len(rawTokens))
	for _, tok := range rawTokens {
		sp := classifySpecToken(tok)
		if !seenCanonical[sp.Canonical] {
			seenCanonical[sp.Canonical] = true
			out = append(out, sp)
		}
	}
	return out
}

func isSpecSeparator(r rune) bool {
	return r == ' ' || r == ',' || r == ';' || r == '\t'
}

func classifySpecToken(tok string) normalizedSpec {
	for _, def := range knownSpecAliasTable {
		if slices.Contains(def.Tokens, tok) {
			return normalizedSpec{
				Raw:          tok,
				Canonical:    def.Canonical,
				Category:     def.Category,
				ModuleSource: def.ModuleSource,
				Aliases:      def.Aliases,
			}
		}
	}

	for _, m := range listTopLevelEmbeddedModules(sourcereader.ModuleFS) {
		if m.Name == tok {
			return normalizedSpec{Raw: tok, Canonical: m.Name, Category: "module", ModuleSource: m.Path, Aliases: []string{m.Name}}
		}
	}

	return normalizedSpec{Raw: tok, Canonical: tok, Category: "keyword", Aliases: []string{tok}}
}

func blueprintMatchesSpec(bp catalog.BlueprintEntry, spec normalizedSpec) bool {
	switch spec.Category {
	case "scheduler":
		return matchSchedulerSpec(bp, spec)
	case "module":
		return matchModuleSpec(bp, spec)
	case "hardware":
		return hardwareMatchStrength(bp, spec) > 0
	default:
		return matchKeywordSpec(bp, spec)
	}
}

func matchSchedulerSpec(bp catalog.BlueprintEntry, spec normalizedSpec) bool {
	if spec.Canonical == "slurm-on-gke (slinky)" {
		return strings.Contains(bp.Scheduler, "slinky") || strings.Contains(bp.Path, "slinky")
	}
	return bp.Scheduler == spec.Canonical
}

func matchModuleSpec(bp catalog.BlueprintEntry, spec normalizedSpec) bool {
	for _, m := range bp.Modules {
		if m.Source == spec.ModuleSource || path.Base(m.Source) == spec.Canonical {
			return true
		}
	}
	return false
}

func hardwareMatchStrength(bp catalog.BlueprintEntry, spec normalizedSpec) int {
	for _, mt := range bp.MachineTypes {
		if matchesHardwareMachineType(strings.ToLower(mt), spec.Aliases) {
			return 2
		}
	}
	lowerPath := strings.ToLower(bp.Path)
	lowerName := strings.ToLower(bp.BlueprintName)
	for _, al := range spec.Aliases {
		if containsSegment(lowerPath, al) || containsSegment(lowerName, al) {
			return 1
		}
	}
	return 0
}

func matchesHardwareMachineType(lowerMT string, aliases []string) bool {
	for _, al := range aliases {
		if lowerMT == al || strings.HasPrefix(lowerMT, al+"-") {
			return true
		}
	}
	return false
}

func matchKeywordSpec(bp catalog.BlueprintEntry, spec normalizedSpec) bool {
	if strings.Contains(strings.ToLower(bp.Path), spec.Canonical) ||
		strings.Contains(strings.ToLower(bp.BlueprintName), spec.Canonical) {
		return true
	}
	for _, mt := range bp.MachineTypes {
		if strings.Contains(strings.ToLower(mt), spec.Canonical) {
			return true
		}
	}
	for _, m := range bp.Modules {
		if strings.Contains(strings.ToLower(m.Source), spec.Canonical) ||
			strings.Contains(strings.ToLower(m.ID), spec.Canonical) {
			return true
		}
	}
	return false
}

func containsSegment(haystack, needle string) bool {
	if strings.Contains(haystack, "/"+needle+"/") || strings.Contains(haystack, "/"+needle+".") ||
		strings.Contains(haystack, "-"+needle+"-") || strings.HasSuffix(haystack, "-"+needle+".yaml") ||
		strings.HasSuffix(haystack, "-"+needle) {
		return true
	}
	if strings.Contains(haystack, "/"+needle+"-") || strings.HasPrefix(haystack, needle+"-") {
		return !strings.Contains(haystack, needle+"-max")
	}
	return false
}

type scoredBlueprintCandidate struct {
	entry        catalog.BlueprintEntry
	matchedCount int
	weightScore  int
	matchedSpecs []normalizedSpec
	missingSpecs []normalizedSpec
}

func buildExplainSpecMatch(repoRoot string, args []string) (ExplainSpecMatchResult, error) {
	specs := parseSpecTokens(args)
	if len(specs) == 0 {
		return ExplainSpecMatchResult{}, fmt.Errorf("no valid spec keywords provided")
	}

	blueprints, err := scanBlueprintsForSpecMatch(repoRoot)
	if err != nil {
		return ExplainSpecMatchResult{}, err
	}

	best, exactTotal := selectBestBlueprintCandidate(blueprints, specs)
	if best == nil {
		return ExplainSpecMatchResult{}, formatUnmatchedSpecsError(specs)
	}
	otherExact := 0
	if len(best.missingSpecs) == 0 && exactTotal > 1 {
		otherExact = exactTotal - 1
	}
	return assembleSpecMatchResult(repoRoot, specs, best, otherExact, blueprints)
}

func scanBlueprintsForSpecMatch(repoRoot string) ([]catalog.BlueprintEntry, error) {
	blueprints, err := scanBlueprintsCached(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to scan blueprints: %w", err)
	}
	if len(blueprints) == 0 {
		return nil, fmt.Errorf("no blueprints found under %s/examples; run './gcluster explain <module>' to inspect embedded modules", repoRoot)
	}
	return blueprints, nil
}

func selectBestBlueprintCandidate(blueprints []catalog.BlueprintEntry, specs []normalizedSpec) (*scoredBlueprintCandidate, int) {
	var best *scoredBlueprintCandidate
	exactCount := 0
	for _, bp := range blueprints {
		cand := scoreBlueprintAgainstSpecs(bp, specs)
		if cand.matchedCount == 0 {
			continue
		}
		if len(cand.missingSpecs) == 0 {
			exactCount++
		}
		if isBetterBlueprintCandidate(cand, best) {
			best = cand
		}
	}
	return best, exactCount
}

func scoreBlueprintAgainstSpecs(bp catalog.BlueprintEntry, specs []normalizedSpec) *scoredBlueprintCandidate {
	var matched []normalizedSpec
	var missing []normalizedSpec
	weight := 0
	for _, sp := range specs {
		if blueprintMatchesSpec(bp, sp) {
			matched = append(matched, sp)
			weight += specMatchWeight(bp, sp)
		} else {
			missing = append(missing, sp)
		}
	}
	if len(matched) > 0 {
		weight += blueprintTieBreakBonus(bp)
	}
	return &scoredBlueprintCandidate{
		entry:        bp,
		matchedCount: len(matched),
		weightScore:  weight,
		matchedSpecs: matched,
		missingSpecs: missing,
	}
}

func specMatchWeight(bp catalog.BlueprintEntry, sp normalizedSpec) int {
	base := specCategoryWeight(sp.Category)
	if sp.Category == "hardware" && hardwareMatchStrength(bp, sp) == 2 {
		return base + 5
	}
	return base
}

func specCategoryWeight(category string) int {
	switch category {
	case "scheduler":
		return 100
	case "hardware":
		return 90
	case "module":
		return 40
	default:
		return 30
	}
}

func blueprintTieBreakBonus(bp catalog.BlueprintEntry) int {
	bonus := 0
	if strings.HasPrefix(bp.Path, "examples/") {
		bonus += 5
	}
	if !strings.HasPrefix(bp.Path, "examples/gke-consumption-options/") {
		bonus += 2
	}
	if !strings.HasSuffix(bp.Path, "-vm.yaml") && !strings.HasSuffix(bp.Path, "-image.yaml") && bp.Scheduler != "none" {
		bonus += 3
	}
	if !strings.HasSuffix(bp.Path, "-advanced.yaml") {
		bonus += 1
	}
	return bonus
}

func isBetterBlueprintCandidate(cand, best *scoredBlueprintCandidate) bool {
	if best == nil || cand.matchedCount != best.matchedCount {
		return best == nil || cand.matchedCount > best.matchedCount
	}
	if cand.weightScore != best.weightScore {
		return cand.weightScore > best.weightScore
	}
	return cand.entry.Path < best.entry.Path
}

func formatUnmatchedSpecsError(specs []normalizedSpec) error {
	rawList := make([]string, 0, len(specs))
	for _, s := range specs {
		rawList = append(rawList, s.Raw)
	}
	return fmt.Errorf(
		"no blueprint matched specs [%s]; try keywords like 'slurm a3u lustre', 'gke a4', 'tpu 7x', or run './gcluster explain' to list all blueprints and modules",
		strings.Join(rawList, ", "))
}

func assembleSpecMatchResult(repoRoot string, specs []normalizedSpec, best *scoredBlueprintCandidate, otherExact int, allBlueprints []catalog.BlueprintEntry) (ExplainSpecMatchResult, error) {
	bpAbs := filepath.Join(repoRoot, filepath.FromSlash(best.entry.Path))
	deplAbs := ""
	if best.entry.DeploymentPath != "" {
		deplAbs = filepath.Join(repoRoot, filepath.FromSlash(best.entry.DeploymentPath))
	}
	bpExplain, err := buildExplainBlueprint(repoRoot, bpAbs, deplAbs)
	if err != nil {
		return ExplainSpecMatchResult{}, err
	}

	reqNames := make([]string, 0, len(specs))
	for _, s := range specs {
		reqNames = append(reqNames, s.Raw)
	}
	matchedNames := make([]string, 0, len(best.matchedSpecs))
	for _, s := range best.matchedSpecs {
		matchedNames = append(matchedNames, s.Canonical)
	}

	var missingOut []ExplainMissingSpec
	for _, ms := range best.missingSpecs {
		item := ExplainMissingSpec{Spec: ms.Raw, Module: ms.ModuleSource}
		if ms.ModuleSource != "" {
			item.Snippet = buildMissingModuleSnippetForBlueprint(ms.ModuleSource, best.entry)
			item.TargetGroup = pickTargetGroupForSnippet(bpExplain.Stages, item.Snippet)
			if detectEmbeddedModuleKind(sourcereader.ModuleFS, ms.ModuleSource) == "packer" {
				item.TargetGroup = "" // packer modules must be the only module in their own group
			}
			item.ReferenceBlueprints = referenceBlueprintsFor(allBlueprints, best.entry.Scheduler, ms.ModuleSource)
		}
		missingOut = append(missingOut, item)
	}

	return ExplainSpecMatchResult{
		ExactMatch:     len(best.missingSpecs) == 0,
		OtherExact:     otherExact,
		RequestedSpecs: reqNames,
		MatchedSpecs:   matchedNames,
		MissingSpecs:   missingOut,
		Blueprint:      bpExplain,
	}, nil
}

func pickTargetGroupForSnippet(stages []ExplainStage, snippet string) string {
	if len(stages) == 0 {
		return "primary"
	}
	deps := snippetExternalUseIDs(snippet)
	target := stages[0].Group
	matchedDep := false
	for _, st := range stages {
		for _, m := range st.Modules {
			if deps[m.ID] {
				target = st.Group
				matchedDep = true
			}
		}
	}
	if !matchedDep {
		for _, st := range stages {
			if slices.ContainsFunc(st.Modules, func(m ExplainStageModule) bool { return m.ShortSource == "vpc" }) {
				return st.Group
			}
		}
	}
	return target
}

func snippetExternalUseIDs(snippet string) map[string]bool {
	var blocks []struct {
		ID  string   `yaml:"id"`
		Use []string `yaml:"use"`
	}
	_ = yaml.Unmarshal([]byte(snippet), &blocks)
	defined, deps := map[string]bool{}, map[string]bool{}
	for _, b := range blocks {
		defined[b.ID] = true
	}
	for _, b := range blocks {
		for _, u := range b.Use {
			if !defined[u] {
				deps[u] = true
			}
		}
	}
	return deps
}

func referenceBlueprintsFor(all []catalog.BlueprintEntry, scheduler, modSource string) []string {
	var refs []string
	for _, bp := range all {
		if bp.Scheduler != scheduler {
			continue
		}
		for _, m := range bp.Modules {
			if m.Source == modSource {
				refs = append(refs, bp.Path)
				break
			}
		}
	}
	return refs
}

func buildMissingModuleSnippetForBlueprint(modSource string, bp catalog.BlueprintEntry) string {
	kind := detectEmbeddedModuleKind(sourcereader.ModuleFS, modSource)
	if kind == "" {
		kind = "terraform"
	}
	mi, err := modulereader.GetModuleInfo(modSource, kind)
	if err != nil {
		return fmt.Sprintf("# could not inspect %s: %v", modSource, err)
	}

	upstreamGroups, useProvided := findUpstreamCandidateGroups(modSource, mi)
	prereqBlocks, useIDs := resolveBlueprintUpstreamBlocks(bp, upstreamGroups)
	reqInputs, optInputs := partitionModuleInputs(mi, useProvided)
	mainBlock := renderModuleYAMLBlock(defaultModuleID(modSource), modSource, useIDs, reqInputs, optInputs)
	return strings.Join(append(prereqBlocks, mainBlock), "\n")
}

func resolveBlueprintUpstreamBlocks(bp catalog.BlueprintEntry, upstreamGroups []upstreamCandidateGroup) ([]string, []string) {
	var prereqBlocks []string
	var useIDs []string
	for _, ug := range upstreamGroups {
		if existingID := findMatchingModuleIDInBlueprint(bp, ug.PrimarySource, ug.MatchedInputs); existingID != "" {
			useIDs = append(useIDs, existingID)
			continue
		}
		prereqUse := resolvePrereqUpstreamUseIDs(bp, ug.PrimarySource)
		prereqBlocks = append(prereqBlocks, renderModuleYAMLBlock(ug.PrimaryID, ug.PrimarySource, prereqUse, nil, nil))
		useIDs = append(useIDs, ug.PrimaryID)
	}
	return prereqBlocks, useIDs
}

func resolvePrereqUpstreamUseIDs(bp catalog.BlueprintEntry, prereqSource string) []string {
	pInfo, err := modulereader.GetModuleInfo(prereqSource, "terraform")
	if err != nil {
		return nil
	}
	pGroups, _ := findUpstreamCandidateGroups(prereqSource, pInfo)
	var prereqUse []string
	for _, pg := range pGroups {
		if existingID := findMatchingModuleIDInBlueprint(bp, pg.PrimarySource, pg.MatchedInputs); existingID != "" {
			prereqUse = append(prereqUse, existingID)
		} else {
			prereqUse = append(prereqUse, pg.PrimaryID)
		}
	}
	return prereqUse
}

func findMatchingModuleIDInBlueprint(bp catalog.BlueprintEntry, preferredSource string, neededOutputs []string) string {
	useCount := make(map[string]int)
	for _, m := range bp.Modules {
		for _, u := range m.Use {
			useCount[u]++
		}
	}
	// Prefer the candidate consumed by the most modules (e.g. the cluster VPC over a
	// Packer image-builder VPC declared earlier); ties keep declaration order.
	pick := func(match func(catalog.ModuleSummary) bool) string {
		best, bestN := "", -1
		for _, m := range bp.Modules {
			if match(m) && useCount[m.ID] > bestN {
				best, bestN = m.ID, useCount[m.ID]
			}
		}
		return best
	}
	if id := pick(func(m catalog.ModuleSummary) bool { return m.Source == preferredSource }); id != "" {
		return id
	}
	return pick(func(m catalog.ModuleSummary) bool { return moduleOutputsAll(m.Source, neededOutputs) })
}

func moduleOutputsAll(source string, neededOutputs []string) bool {
	if len(neededOutputs) == 0 {
		return false
	}
	kind := detectEmbeddedModuleKind(sourcereader.ModuleFS, source)
	if kind == "" {
		return false
	}
	info, err := modulereader.GetModuleInfo(source, kind)
	if err != nil {
		return false
	}
	outMap := info.GetOutputsAsMap()
	for _, needed := range neededOutputs {
		if _, ok := outMap[needed]; !ok {
			return false
		}
	}
	return true
}

func printExplainSpecMatchText(w io.Writer, res ExplainSpecMatchResult) {
	if res.ExactMatch {
		writef(w, "Exact Blueprint Match (%d/%d specs matched: %s):\n",
			len(res.MatchedSpecs), len(res.RequestedSpecs), strings.Join(res.MatchedSpecs, ", "))
		if res.OtherExact > 0 {
			writef(w, "  Note: %d other blueprint(s) also matched all requested specs; add hardware or storage specs to narrow down.\n", res.OtherExact)
		}
		writeln(w, "")
		printExplainBlueprintText(w, res.Blueprint)
		return
	}

	writef(w, "Closest Blueprint Match (%d/%d specs matched: %s):\n",
		len(res.MatchedSpecs), len(res.RequestedSpecs), strings.Join(res.MatchedSpecs, ", "))
	writef(w, "  Blueprint: %s\n", res.Blueprint.Path)
	if res.Blueprint.DeploymentPath != "" {
		writef(w, "  Companion Deployment File: %s\n", res.Blueprint.DeploymentPath)
	}
	writef(w, "  Missing Spec: %s\n", strings.Join(formatMissingSpecLabels(res.MissingSpecs), ", "))

	printMissingSpecSnippets(w, res.MissingSpecs)

	writeln(w, "\nBase Blueprint Details:")
	printExplainBlueprintText(w, res.Blueprint)
}

func formatMissingSpecLabels(missing []ExplainMissingSpec) []string {
	labels := make([]string, 0, len(missing))
	for _, ms := range missing {
		if ms.Module != "" {
			labels = append(labels, fmt.Sprintf("%s (%s)", ms.Spec, ms.Module))
		} else {
			labels = append(labels, ms.Spec)
		}
	}
	return labels
}

func printMissingSpecSnippets(w io.Writer, missing []ExplainMissingSpec) {
	for _, ms := range missing {
		if ms.Snippet == "" {
			continue
		}
		if ms.TargetGroup == "" {
			writef(w, "\nTo add missing spec (%s -> %s), add it as a NEW deployment group (packer modules cannot share a group):\n",
				ms.Spec, ms.Module)
		} else {
			writef(w, "\nTo add missing spec (%s -> %s), insert into group [%s]:\n",
				ms.Spec, ms.Module, ms.TargetGroup)
		}
		for _, line := range strings.Split(ms.Snippet, "\n") {
			writef(w, "  %s\n", line)
		}
		if len(ms.ReferenceBlueprints) > 0 {
			writef(w, "  Note: Also wire downstream consumer/CSI/firewall settings for %s; see reference blueprints: %s\n",
				ms.Module, strings.Join(ms.ReferenceBlueprints, ", "))
		} else {
			writef(w, "  Note: Also add the new module ID to the 'use:' list of downstream compute/cluster modules that consume its outputs.\n")
		}
	}
}
