// Copyright 2026 Google LLC
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

// Package catalog generates the machine-readable blueprint and module wiring
// catalog (references/catalog.yaml) and updates the summary table in
// examples/README.md.
package catalog

import (
	"bytes"
	"fmt"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/modulereader"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/zclconf/go-cty/cty"
	"gopkg.in/yaml.v3"
)

const (
	// BeginCatalogMarker marks the start of the auto-generated table in examples/README.md.
	BeginCatalogMarker = "<!-- BEGIN_BLUEPRINT_CATALOG -->"
	// EndCatalogMarker marks the end of the auto-generated table in examples/README.md.
	EndCatalogMarker = "<!-- END_BLUEPRINT_CATALOG -->"
)

// autoInjectedGlobals are top-level vars automatically passed by gcluster to
// every module that declares them as inputs.
var autoInjectedGlobals = map[string]bool{
	"project_id":      true,
	"deployment_name": true,
	"region":          true,
	"zone":            true,
	"labels":          true,
}

// genericOutputNames are output names that are too generic for blind
// name-based output-to-input matching unless backed by an observed blueprint use edge.
var genericOutputNames = map[string]bool{
	"id":            true,
	"instance_name": true,
	"name":          true,
	"self_link":     true,
}

// integrationInputNames are optional module inputs that commonly wire modules
// together via `use:` in Cluster Toolkit blueprints.
var integrationInputNames = map[string]bool{
	"network_storage":           true,
	"partitions":                true,
	"nodeset":                   true,
	"nodeset_dyn":               true,
	"nodeset_tpu":               true,
	"login_nodes":               true,
	"runners":                   true,
	"startup_script":            true,
	"compute_startup_script":    true,
	"controller_startup_script": true,
}

// Catalog is the top-level structure serialized to references/catalog.yaml.
type Catalog struct {
	Blueprints   []BlueprintEntry             `yaml:"blueprints"`
	ModuleWiring map[string]ModuleWiringEntry `yaml:"module_wiring"`
}

// BlueprintEntry describes a single blueprint template in examples/ or community/examples/.
type BlueprintEntry struct {
	Path           string                 `yaml:"path"`
	DeploymentPath string                 `yaml:"deployment_path,omitempty"`
	BlueprintName  string                 `yaml:"blueprint_name"`
	Scheduler      string                 `yaml:"scheduler"`
	Description    string                 `yaml:"description,omitempty"`
	MachineTypes   []string               `yaml:"machine_types,omitempty,flow"`
	Vars           map[string]interface{} `yaml:"vars,omitempty"`
	Modules        []ModuleSummary        `yaml:"modules"`
}

// ModuleSummary describes one module block inside a blueprint.
type ModuleSummary struct {
	ID       string   `yaml:"id"`
	Source   string   `yaml:"source"`
	Use      []string `yaml:"use,omitempty,flow"`
	Settings []string `yaml:"settings,omitempty,flow"`
}

// ModuleWiringEntry describes inputs, recommended upstream providers, and
// downstream consumers for a Terraform or Packer module.
type ModuleWiringEntry struct {
	RequiredInputs []string `yaml:"required_inputs,omitempty,flow"`
	RecommendedUse []string `yaml:"recommended_use,omitempty"`
	ConsumedBy     []string `yaml:"consumed_by,omitempty"`
}

// Generate scans the repository at repoRoot, writes the YAML catalog to
// catalogOut, and updates the summary table in readmePath.
func Generate(repoRoot, catalogOut, readmePath string) error {
	cat, err := BuildCatalog(repoRoot)
	if err != nil {
		return err
	}
	yamlBytes, err := MarshalCatalog(cat)
	if err != nil {
		return err
	}
	if err := os.WriteFile(catalogOut, yamlBytes, 0644); err != nil {
		return fmt.Errorf("writing %s: %w", catalogOut, err)
	}
	if readmePath != "" {
		if err := UpdateExamplesReadme(readmePath, cat.Blueprints); err != nil {
			return err
		}
	}
	return nil
}

// BuildCatalog scans blueprints and modules under repoRoot and returns a populated Catalog.
func BuildCatalog(repoRoot string) (Catalog, error) {
	readmeDescriptions := parseReadmeDescriptions(filepath.Join(repoRoot, "examples", "README.md"))
	blueprints, err := ScanBlueprints(repoRoot, readmeDescriptions)
	if err != nil {
		return Catalog{}, err
	}
	wiring, err := ScanModuleWiring(repoRoot, blueprints)
	if err != nil {
		return Catalog{}, err
	}
	return Catalog{
		Blueprints:   blueprints,
		ModuleWiring: wiring,
	}, nil
}

// ScanBlueprints discovers and parses all valid Cluster Toolkit blueprints in
// examples/ and community/examples/.
func ScanBlueprints(repoRoot string, readmeDescs map[string]string) ([]BlueprintEntry, error) {
	searchRoots := []string{"examples", filepath.Join("community", "examples")}
	tracked := gitTrackedFiles(repoRoot, searchRoots...)
	var entries []BlueprintEntry

	for _, relRoot := range searchRoots {
		absRoot := filepath.Join(repoRoot, relRoot)
		found, err := scanBlueprintDir(repoRoot, absRoot, readmeDescs, tracked)
		if err != nil {
			return nil, err
		}
		entries = append(entries, found...)
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

func scanBlueprintDir(repoRoot, absRoot string, readmeDescs map[string]string, tracked map[string]bool) ([]BlueprintEntry, error) {
	if _, err := os.Stat(absRoot); os.IsNotExist(err) {
		return nil, nil
	}
	var entries []BlueprintEntry
	err := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() || !isCandidateBlueprintFile(path) || !isTracked(repoRoot, path, tracked) {
			return walkErr
		}
		entry, ok := parseBlueprintEntry(repoRoot, path, readmeDescs, tracked)
		if ok {
			entries = append(entries, entry)
		}
		return nil
	})
	return entries, err
}

func gitTrackedFiles(repoRoot string, roots ...string) map[string]bool {
	args := append([]string{"-C", repoRoot, "ls-files", "-z", "--"}, roots...)
	out, err := exec.Command("git", args...).Output()
	if err != nil {
		return nil
	}
	set := make(map[string]bool)
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			set[p] = true
		}
	}
	return set
}

func isTracked(repoRoot, absPath string, tracked map[string]bool) bool {
	if tracked == nil {
		return true
	}
	rel, err := filepath.Rel(repoRoot, absPath)
	return err == nil && tracked[filepath.ToSlash(rel)]
}

func isCandidateBlueprintFile(path string) bool {
	if !strings.HasSuffix(path, ".yaml") {
		return false
	}
	base := filepath.Base(path)
	return !strings.HasSuffix(base, "-deployment.yaml") && base != "deployment.yaml"
}

func parseBlueprintEntry(repoRoot, absPath string, readmeDescs map[string]string, tracked map[string]bool) (BlueprintEntry, bool) {
	bp, _, err := config.NewBlueprint(absPath)
	if err != nil || len(bp.Groups) == 0 || bp.AIAssisted {
		return BlueprintEntry{}, false
	}

	relPath, err := filepath.Rel(repoRoot, absPath)
	if err != nil {
		return BlueprintEntry{}, false
	}
	relPath = filepath.ToSlash(relPath)

	varsMap := dictToPlainMap(bp.Vars)
	if varsMap == nil {
		varsMap = make(map[string]interface{})
	}
	deployRel := findCompanionDeployment(repoRoot, absPath, bp.BlueprintName, varsMap, tracked)
	modules := extractModuleSummaries(bp)
	scheduler := inferScheduler(modules)
	machineTypes := extractMachineTypes(bp, varsMap)
	desc := lookupDescription(repoRoot, absPath, relPath, readmeDescs)
	if len(varsMap) == 0 {
		varsMap = nil
	}

	return BlueprintEntry{
		Path:           relPath,
		DeploymentPath: deployRel,
		BlueprintName:  bp.BlueprintName,
		Scheduler:      scheduler,
		Description:    desc,
		MachineTypes:   machineTypes,
		Vars:           varsMap,
		Modules:        modules,
	}, true
}

// findCompanionDeployment pairs a blueprint with a git-tracked deployment file
// in the same directory and merges the deployment file's vars into varsMap for
// keys that are missing or null in the blueprint. It returns the repo-relative
// path of the matched deployment file, or "" when none matches.
func findCompanionDeployment(repoRoot, bpAbsPath, blueprintName string, varsMap map[string]interface{}, tracked map[string]bool) string {
	dir := filepath.Dir(bpAbsPath)
	candidates := trackedDeploymentFiles(repoRoot, dir, tracked)
	match := matchDeploymentFile(filepath.Base(bpAbsPath), blueprintName, candidates, isExampleSubfolder(repoRoot, dir))
	if match == "" {
		return ""
	}
	cand := filepath.Join(dir, match)
	if ds, _, err := config.NewDeploymentSettings(cand); err == nil {
		for k, v := range dictToPlainMap(ds.Vars) {
			if _, exists := varsMap[k]; !exists || varsMap[k] == nil {
				varsMap[k] = v
			}
		}
	}
	rel, err := filepath.Rel(repoRoot, cand)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// trackedDeploymentFiles returns the sorted basenames of git-tracked
// `*-deployment.yaml` / `deployment.yaml` files directly inside dir.
func trackedDeploymentFiles(repoRoot, dir string, tracked map[string]bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !isDeploymentFileName(name) || !isTracked(repoRoot, filepath.Join(dir, name), tracked) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func isDeploymentFileName(name string) bool {
	return name == "deployment.yaml" || strings.HasSuffix(name, "-deployment.yaml")
}

// isExampleSubfolder reports whether dir is strictly below examples/ or
// community/examples/ (not one of those roots, and not elsewhere in the repo).
func isExampleSubfolder(repoRoot, dir string) bool {
	rel, err := filepath.Rel(repoRoot, dir)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	return strings.HasPrefix(rel, "examples/") || strings.HasPrefix(rel, "community/examples/")
}

// matchDeploymentFile selects the deployment file for a blueprint from the
// candidates (basenames) in the same directory using three deterministic steps:
//  1. exact match: <stem>-deployment.yaml or <blueprint_name>-deployment.yaml
//  2. longest prefix match: <prefix>-deployment.yaml where <stem> starts with
//     "<prefix>-" or <prefix> starts with "<stem>-"
//  3. single-file fallback: the only candidate when dir is an examples subfolder
func matchDeploymentFile(baseName, blueprintName string, candidates []string, inSubfolder bool) string {
	if len(candidates) == 0 {
		return ""
	}
	stem := strings.TrimSuffix(baseName, ".yaml")
	for _, exact := range []string{stem + "-deployment.yaml", blueprintName + "-deployment.yaml"} {
		if exact != "-deployment.yaml" && slices.Contains(candidates, exact) {
			return exact
		}
	}
	if best := longestPrefixDeployment(stem, candidates); best != "" {
		return best
	}
	if inSubfolder && len(candidates) == 1 {
		return candidates[0]
	}
	return ""
}

func longestPrefixDeployment(stem string, candidates []string) string {
	best, bestLen := "", 0
	for _, cand := range candidates {
		prefix := strings.TrimSuffix(cand, "-deployment.yaml")
		if prefix == cand || prefix == "" {
			continue
		}
		if (strings.HasPrefix(stem, prefix+"-") || strings.HasPrefix(prefix, stem+"-")) && len(prefix) > bestLen {
			best, bestLen = cand, len(prefix)
		}
	}
	return best
}

func extractModuleSummaries(bp config.Blueprint) []ModuleSummary {
	var out []ModuleSummary
	for _, grp := range bp.Groups {
		for _, mod := range grp.Modules {
			useList := make([]string, 0, len(mod.Use))
			for _, u := range mod.Use {
				useList = append(useList, string(u))
			}
			settingsKeys := mod.Settings.Keys()
			sort.Strings(settingsKeys)
			out = append(out, ModuleSummary{
				ID:       string(mod.ID),
				Source:   normalizeModuleSource(mod.Source),
				Use:      useList,
				Settings: settingsKeys,
			})
		}
	}
	return out
}

func normalizeModuleSource(src string) string {
	return strings.TrimPrefix(strings.TrimSpace(src), "./")
}

var schedulerMatchers = []struct {
	substr    string
	scheduler string
}{
	{"slinky", "slurm-on-gke (slinky)"},
	{"slurm", "slurm"},
	{"gke-cluster", "gke"},
	{"batch", "batch"},
	{"htcondor", "htcondor"},
	{"flux", "flux"},
}

func inferScheduler(modules []ModuleSummary) string {
	for _, rule := range schedulerMatchers {
		for _, m := range modules {
			if strings.Contains(m.Source, rule.substr) {
				return rule.scheduler
			}
		}
	}
	return "none"
}

var varRefRegex = regexp.MustCompile(`^\$\(vars?\.([a-zA-Z0-9_]+)\)$`)

// controlPlaneModuleSources are module sources whose machine_type describes a
// control-plane or helper VM (login, controller, image builder) rather than
// workload compute. They are excluded from the catalog's machine_types.
var controlPlaneModuleSources = []string{
	"schedmd-slurm-gcp-v6-login",
	"schedmd-slurm-gcp-v6-controller",
	"schedmd-slurm-gcp-v5-login",
	"schedmd-slurm-gcp-v5-controller",
	"modules/packer/",
}

// controlPlaneMachineTypeVars are blueprint vars that size control-plane or
// helper VMs. They stay in vars (so deployment.yaml can override them) but are
// excluded from the catalog's machine_types.
var controlPlaneMachineTypeVars = map[string]bool{
	"system_node_pool_machine_type": true,
	"login_machine_type":            true,
	"controller_machine_type":       true,
	"image_build_machine_type":      true,
}

func isControlPlaneModule(source string) bool {
	for _, s := range controlPlaneModuleSources {
		if strings.Contains(source, s) {
			return true
		}
	}
	return false
}

// extractMachineTypes returns the workload compute machine types advertised by
// a blueprint: machine_type settings on non-control-plane modules plus any
// *machine_type var that is not a control-plane var.
func extractMachineTypes(bp config.Blueprint, varsMap map[string]interface{}) []string {
	var result []string
	addMT := func(val string) {
		resolved := resolveMachineTypeString(val, varsMap)
		if resolved != "" && !slices.Contains(result, resolved) {
			result = append(result, resolved)
		}
	}

	for _, grp := range bp.Groups {
		for _, mod := range grp.Modules {
			if isControlPlaneModule(mod.Source) {
				continue
			}
			plainSettings := dictToPlainMap(mod.Settings)
			if s, ok := plainSettings["machine_type"].(string); ok {
				addMT(s)
			}
		}
	}
	varKeys := make([]string, 0, len(varsMap))
	for k := range varsMap {
		varKeys = append(varKeys, k)
	}
	sort.Strings(varKeys)
	for _, k := range varKeys {
		if strings.HasSuffix(k, "machine_type") && !controlPlaneMachineTypeVars[k] {
			if s, ok := varsMap[k].(string); ok {
				addMT(s)
			}
		}
	}
	return result
}

func resolveMachineTypeString(raw string, varsMap map[string]interface{}) string {
	raw = strings.TrimSpace(raw)
	if m := varRefRegex.FindStringSubmatch(raw); len(m) == 2 {
		if v, ok := varsMap[m[1]].(string); ok {
			raw = strings.TrimSpace(v)
		} else {
			return ""
		}
	}
	if raw == "" || strings.ContainsAny(raw, "$(){}") {
		return ""
	}
	return raw
}

func dictToPlainMap(d config.Dict) map[string]interface{} {
	items := d.Items()
	if len(items) == 0 {
		return nil
	}
	res := make(map[string]interface{}, len(items))
	for k, v := range items {
		res[k] = ctyToPlainValue(v)
	}
	return res
}

func ctyToPlainValue(v cty.Value) interface{} {
	if expr, isExpr := config.IsExpressionValue(v); isExpr {
		return formatBlueprintExpression(expr)
	}
	if !v.IsKnown() || v.IsNull() {
		return nil
	}
	return ctyKnownToPlain(v)
}

func formatBlueprintExpression(expr config.Expression) interface{} {
	raw := strings.TrimSpace(string(expr.Tokenize().Bytes()))
	if raw == "null" {
		return nil
	}
	raw = strings.ReplaceAll(raw, "var.", "vars.")
	return "$(" + raw + ")"
}

func ctyKnownToPlain(v cty.Value) interface{} {
	ty := v.Type()
	switch {
	case ty == cty.String:
		return cleanPlaceholderString(v.AsString())
	case ty == cty.Bool:
		return v.True()
	case ty == cty.Number:
		return ctyNumberToPlain(v)
	case ty.IsTupleType() || ty.IsListType() || ty.IsSetType():
		return ctySequenceToPlain(v)
	case ty.IsObjectType() || ty.IsMapType():
		return ctyMappingToPlain(v)
	default:
		return nil
	}
}

func cleanPlaceholderString(s string) interface{} {
	trimmed := strings.TrimSpace(s)
	if strings.HasPrefix(trimmed, "##") || strings.HasPrefix(trimmed, "<<") {
		return nil
	}
	return s
}

func ctyNumberToPlain(v cty.Value) interface{} {
	bf := v.AsBigFloat()
	if i, acc := bf.Int64(); acc == 0 {
		return i
	}
	f, _ := bf.Float64()
	return f
}

func ctySequenceToPlain(v cty.Value) []interface{} {
	out := make([]interface{}, 0, v.LengthInt())
	it := v.ElementIterator()
	for it.Next() {
		_, elem := it.Element()
		out = append(out, ctyToPlainValue(elem))
	}
	return out
}

func ctyMappingToPlain(v cty.Value) map[string]interface{} {
	out := make(map[string]interface{}, v.LengthInt())
	it := v.ElementIterator()
	for it.Next() {
		k, elem := it.Element()
		out[k.AsString()] = ctyToPlainValue(elem)
	}
	return out
}

var (
	headingRegex    = regexp.MustCompile(`^###\s+\[([^\]]+)\]`)
	mdLinkRegex     = regexp.MustCompile(`\[([^\]]+)\]\([^\)]+\)|\[([^\]]+)\]\[[^\]]*\]|\[([^\]]+)\]`)
	linkRefDefRegex = regexp.MustCompile(`^\[[^\]]+\]:\s*`)
	spaceRunRegex   = regexp.MustCompile(`\s+`)
)

func parseReadmeDescriptions(readmePath string) map[string]string {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return map[string]string{}
	}
	res := make(map[string]string)
	lines := strings.Split(string(data), "\n")
	var currentKey string
	var paraLines []string

	flush := func() {
		if currentKey != "" && len(paraLines) > 0 {
			res[currentKey] = cleanDescriptionText(strings.Join(paraLines, " "))
		}
		currentKey = ""
		paraLines = nil
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if m := headingRegex.FindStringSubmatch(trimmed); len(m) == 2 {
			flush()
			currentKey = strings.TrimSpace(m[1])
			continue
		}
		if currentKey == "" {
			continue
		}
		if shouldStopParagraph(trimmed, len(paraLines) > 0) {
			flush()
			continue
		}
		if isParagraphLine(trimmed) {
			paraLines = append(paraLines, trimmed)
		}
	}
	flush()
	return res
}

func shouldStopParagraph(trimmed string, hasLines bool) bool {
	if strings.HasPrefix(trimmed, "### ") || (hasLines && strings.HasPrefix(trimmed, "#")) {
		return true
	}
	return trimmed == "" && hasLines
}

func isParagraphLine(trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || linkRefDefRegex.MatchString(trimmed) || strings.HasPrefix(trimmed, "!") {
		return false
	}
	return !strings.HasPrefix(trimmed, "<!--") && !strings.HasPrefix(trimmed, ">") && !strings.HasPrefix(trimmed, "```")
}

func cleanDescriptionText(raw string) string {
	s := mdLinkRegex.ReplaceAllStringFunc(raw, func(match string) string {
		sub := mdLinkRegex.FindStringSubmatch(match)
		for i := 1; i < len(sub); i++ {
			if sub[i] != "" {
				return sub[i]
			}
		}
		return match
	})
	s = strings.ReplaceAll(s, "`", "")
	s = strings.ReplaceAll(s, "**", "")
	s = spaceRunRegex.ReplaceAllString(strings.TrimSpace(s), " ")
	runes := []rune(s)
	if len(runes) > 240 {
		cut := string(runes[:237])
		if idx := strings.LastIndex(cut, " "); idx > 0 {
			cut = cut[:idx]
		}
		s = strings.TrimRight(cut, ".,;:- ") + "..."
	}
	return s
}

func lookupDescription(repoRoot, absPath, relPath string, readmeDescs map[string]string) string {
	base := filepath.Base(absPath)
	stem := strings.TrimSuffix(base, ".yaml")
	parentDir := filepath.Base(filepath.Dir(absPath))

	if !strings.HasPrefix(relPath, "examples/gke-consumption-options/") {
		for _, key := range []string{base, stem, parentDir, parentDir + ".yaml"} {
			if d := readmeDescs[key]; d != "" {
				return d
			}
		}
	}
	if parentDir != "examples" {
		if d := extractSiblingReadmeDescription(filepath.Dir(absPath), base); d != "" {
			return d
		}
	}
	if d := extractBlueprintHeaderComment(absPath); d != "" {
		return d
	}
	return fmt.Sprintf("Cluster Toolkit blueprint (%s).", relPath)
}

func extractSiblingReadmeDescription(dir, base string) string {
	data, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		return ""
	}
	text := string(data)
	if d := extractHeadingParagraphForBlueprint(text, base); d != "" {
		return d
	}
	others := otherBlueprintYAMLsInDir(dir, base)
	if d := extractFirstParagraphIfUnclaimed(text, others); d != "" {
		return d
	}
	return extractSectionParagraphMentioning(strings.Split(text, "\n"), base)
}

func extractHeadingParagraphForBlueprint(mdText, base string) string {
	lines := strings.Split(mdText, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		level := markdownHeadingLevel(trimmed)
		if level == 0 || !strings.Contains(trimmed, base) {
			continue
		}
		section := sliceUntilSameOrOuterHeading(lines[i+1:], level)
		if d := extractImmediateParagraph(section); d != "" {
			return d
		}
		if d := extractSectionParagraphMentioning(section, base); d != "" {
			return d
		}
	}
	return ""
}

func markdownHeadingLevel(trimmed string) int {
	n := 0
	for n < len(trimmed) && trimmed[n] == '#' {
		n++
	}
	if n > 0 && n < len(trimmed) && trimmed[n] == ' ' {
		return n
	}
	return 0
}

func sliceUntilSameOrOuterHeading(lines []string, level int) []string {
	for i, line := range lines {
		if l := markdownHeadingLevel(strings.TrimSpace(line)); l > 0 && l <= level {
			return lines[:i]
		}
	}
	return lines
}

func extractImmediateParagraph(lines []string) string {
	var para []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if shouldStopParagraph(trimmed, len(para) > 0) || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "```") {
			break
		}
		if isParagraphLine(trimmed) {
			para = append(para, trimmed)
		}
	}
	return cleanDescriptionText(strings.Join(para, " "))
}

func extractSectionParagraphMentioning(lines []string, base string) string {
	var para []string
	inCode := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCode = !inCode
			para = nil
			continue
		}
		if inCode {
			continue
		}
		if isParagraphBoundary(trimmed, len(para) > 0) {
			if d := matchBlueprintParagraph(para, base); d != "" {
				return d
			}
			para = nil
		}
		if isParagraphLine(trimmed) {
			para = append(para, trimmed)
		}
	}
	return matchBlueprintParagraph(para, base)
}

func isParagraphBoundary(trimmed string, hasLines bool) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return true
	}
	return hasLines && strings.HasPrefix(trimmed, "- ")
}

func matchBlueprintParagraph(para []string, base string) string {
	if len(para) == 0 {
		return ""
	}
	joined := strings.TrimSpace(strings.TrimPrefix(strings.Join(para, " "), "- "))
	if !strings.Contains(joined, base) || strings.HasSuffix(joined, ":") {
		return ""
	}
	return cleanDescriptionText(joined)
}

func extractFirstParagraphIfUnclaimed(mdText string, otherYAMLs []string) string {
	var para []string
	for _, line := range strings.Split(mdText, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" && len(para) > 0 {
			break
		}
		if strings.HasPrefix(trimmed, "#") {
			if len(para) > 0 || mentionsAnyString(trimmed, otherYAMLs) {
				break
			}
			continue
		}
		if isParagraphLine(trimmed) {
			para = append(para, trimmed)
		}
	}
	raw := strings.Join(para, " ")
	if raw == "" || mentionsAnyString(raw, otherYAMLs) {
		return ""
	}
	return cleanDescriptionText(raw)
}

func otherBlueprintYAMLsInDir(dir, base string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || name == base || !isCandidateBlueprintFile(name) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err == nil && strings.Contains(string(data), "deployment_groups:") {
			out = append(out, name)
		}
	}
	return out
}

func mentionsAnyString(text string, substrs []string) bool {
	for _, s := range substrs {
		if strings.Contains(text, s) {
			return true
		}
	}
	return false
}

func extractBlueprintHeaderComment(yamlPath string) string {
	data, err := os.ReadFile(yamlPath)
	if err != nil {
		return ""
	}
	var comments []string
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || trimmed == "---" {
			if len(comments) > 0 {
				break
			}
			continue
		}
		if !strings.HasPrefix(trimmed, "#") {
			break
		}
		c := strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		if !isLicenseCommentLine(c) && c != "" {
			comments = append(comments, c)
		}
	}
	return cleanDescriptionText(strings.Join(comments, " "))
}

func isLicenseCommentLine(c string) bool {
	lower := strings.ToLower(c)
	for _, kw := range []string{"copyright", "licensed under", "you may not use", "you may obtain a copy", "apache license", "http://www.apache.org", "https://www.apache.org", "unless required", "distributed under", "without warranties", "see the license", "limitations under"} {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

type parsedModule struct {
	source         string
	requiredInputs []string
	allInputs      map[string]bool
	trueOutputs    map[string]bool
}

// ScanModuleWiring inspects all modules under modules/ and community/modules/,
// filters out auto-injected global variables and echo outputs, and computes
// recommended_use and consumed_by wiring.
func ScanModuleWiring(repoRoot string, blueprints []BlueprintEntry) (map[string]ModuleWiringEntry, error) {
	mods, err := discoverAllModules(repoRoot)
	if err != nil {
		return nil, err
	}
	observedProviderToConsumer := collectObservedUseEdges(blueprints)
	return buildModuleWiringMap(mods, observedProviderToConsumer), nil
}

func discoverAllModules(repoRoot string) ([]parsedModule, error) {
	roots := []string{"modules", filepath.Join("community", "modules")}
	var result []parsedModule

	for _, relRoot := range roots {
		absRoot := filepath.Join(repoRoot, relRoot)
		entries, err := os.ReadDir(absRoot)
		if err != nil {
			continue
		}
		for _, catDir := range entries {
			if !catDir.IsDir() || catDir.Name() == "internal" {
				continue
			}
			found := scanModuleCategory(repoRoot, filepath.Join(absRoot, catDir.Name()))
			result = append(result, found...)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].source < result[j].source
	})
	return result, nil
}

func scanModuleCategory(repoRoot, catAbsPath string) []parsedModule {
	modDirs, err := os.ReadDir(catAbsPath)
	if err != nil {
		return nil
	}
	var out []parsedModule
	for _, d := range modDirs {
		if !d.IsDir() {
			continue
		}
		modAbs := filepath.Join(catAbsPath, d.Name())
		if pm, ok := inspectModuleDir(repoRoot, modAbs); ok {
			out = append(out, pm)
		}
	}
	return out
}

func inspectModuleDir(repoRoot, modAbs string) (parsedModule, bool) {
	kind := detectModuleKind(modAbs)
	if kind == "" {
		return parsedModule{}, false
	}
	rel, err := filepath.Rel(repoRoot, modAbs)
	if err != nil {
		return parsedModule{}, false
	}
	relSource := filepath.ToSlash(rel)
	info, err := modulereader.GetModuleInfo(relSource, kind)
	if err != nil {
		return parsedModule{}, false
	}
	return buildParsedModule(relSource, info), true
}

func detectModuleKind(modAbs string) string {
	files, err := os.ReadDir(modAbs)
	if err != nil {
		return ""
	}
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

func buildParsedModule(source string, info modulereader.ModuleInfo) parsedModule {
	allInputs := make(map[string]bool, len(info.Inputs))
	injectedID := info.Metadata.Ghpc.InjectModuleId
	var reqInputs []string
	for _, in := range info.Inputs {
		allInputs[in.Name] = true
		if in.Required && !autoInjectedGlobals[in.Name] && in.Name != injectedID {
			reqInputs = append(reqInputs, in.Name)
		}
	}
	sort.Strings(reqInputs)

	// Echo variable suppression: exclude outputs that share a name with an input
	// on the same module, unless the module is a root provider (no required inputs)
	// whose outputs also accept optional name-override inputs.
	trueOutputs := make(map[string]bool, len(info.Outputs))
	for _, out := range info.Outputs {
		if !allInputs[out.Name] && !genericOutputNames[out.Name] {
			trueOutputs[out.Name] = true
		}
	}
	if len(reqInputs) == 0 {
		for _, out := range info.Outputs {
			if !genericOutputNames[out.Name] && !integrationInputNames[out.Name] {
				trueOutputs[out.Name] = true
			}
		}
	}
	return parsedModule{
		source:         source,
		requiredInputs: reqInputs,
		allInputs:      allInputs,
		trueOutputs:    trueOutputs,
	}
}

func collectObservedUseEdges(blueprints []BlueprintEntry) map[string]map[string]bool {
	edges := make(map[string]map[string]bool)
	for _, bp := range blueprints {
		idToSource := make(map[string]string, len(bp.Modules))
		for _, m := range bp.Modules {
			idToSource[m.ID] = m.Source
		}
		for _, consumer := range bp.Modules {
			for _, usedID := range consumer.Use {
				providerSrc := idToSource[usedID]
				if providerSrc == "" || providerSrc == consumer.Source {
					continue
				}
				if edges[providerSrc] == nil {
					edges[providerSrc] = make(map[string]bool)
				}
				edges[providerSrc][consumer.Source] = true
			}
		}
	}
	return edges
}

func buildModuleWiringMap(mods []parsedModule, observed map[string]map[string]bool) map[string]ModuleWiringEntry {
	knownSources := make(map[string]bool, len(mods))
	for _, m := range mods {
		knownSources[m.source] = true
	}
	wiring := make(map[string]ModuleWiringEntry, len(mods))
	for _, m := range mods {
		recommendedUse := findRecommendedUse(m, mods)
		consumedBy := findConsumedBy(m, mods, observed, knownSources)
		wiring[m.source] = ModuleWiringEntry{
			RequiredInputs: m.requiredInputs,
			RecommendedUse: recommendedUse,
			ConsumedBy:     consumedBy,
		}
	}
	return wiring
}

func findRecommendedUse(consumer parsedModule, allMods []parsedModule) []string {
	providers := make(map[string]bool)
	for _, reqIn := range consumer.requiredInputs {
		for _, cand := range allMods {
			if cand.source != consumer.source && cand.trueOutputs[reqIn] {
				providers[cand.source] = true
			}
		}
	}
	return sortedKeys(providers)
}

func findConsumedBy(provider parsedModule, allMods []parsedModule, observed map[string]map[string]bool, knownSources map[string]bool) []string {
	consumers := make(map[string]bool)
	for outName := range provider.trueOutputs {
		for _, cand := range allMods {
			if cand.source == provider.source {
				continue
			}
			if slices.Contains(cand.requiredInputs, outName) || (integrationInputNames[outName] && cand.allInputs[outName]) {
				consumers[cand.source] = true
			}
		}
	}
	for consSrc := range observed[provider.source] {
		if consSrc != provider.source && knownSources[consSrc] {
			consumers[consSrc] = true
		}
	}
	return sortedKeys(consumers)
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

const catalogHeader = `# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Auto-generated by tools/catalog-gen. DO NOT EDIT MANUALLY.
# Regenerate with: go run ./tools/catalog-gen

---
`

// MarshalCatalog serializes the Catalog to YAML formatted to satisfy the
// repository's .yamllint configuration (indent-sequences: false).
func MarshalCatalog(cat Catalog) ([]byte, error) {
	var rawBuf bytes.Buffer
	enc := yaml.NewEncoder(&rawBuf)
	enc.SetIndent(2)
	if err := enc.Encode(&cat); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(rawBuf.Bytes(), &doc); err != nil {
		return nil, err
	}

	var out bytes.Buffer
	out.WriteString(catalogHeader)
	if len(doc.Content) > 0 {
		renderMappingNode(&out, doc.Content[0], 0)
	}
	return out.Bytes(), nil
}

func renderMappingNode(buf *bytes.Buffer, node *yaml.Node, indent int) {
	prefix := strings.Repeat(" ", indent)
	for i := 0; i+1 < len(node.Content); i += 2 {
		kNode := node.Content[i]
		vNode := node.Content[i+1]
		writeKeyAndValue(buf, prefix, indent, kNode, vNode)
	}
}

func writeKeyAndValue(buf *bytes.Buffer, prefix string, indent int, kNode, vNode *yaml.Node) {
	keyStr := formatScalarNode(kNode)
	if vNode.Style == yaml.FlowStyle || vNode.Kind == yaml.ScalarNode {
		valStr := renderInlineNode(vNode)
		if valStr == "" {
			fmt.Fprintf(buf, "%s%s:\n", prefix, keyStr)
		} else {
			fmt.Fprintf(buf, "%s%s: %s\n", prefix, keyStr, valStr)
		}
		return
	}
	if vNode.Kind == yaml.SequenceNode {
		if len(vNode.Content) == 0 {
			fmt.Fprintf(buf, "%s%s: []\n", prefix, keyStr)
			return
		}
		fmt.Fprintf(buf, "%s%s:\n", prefix, keyStr)
		// indent-sequences: false keeps '-' at the same indent as the parent key
		renderSequenceNode(buf, vNode, indent)
		return
	}
	if vNode.Kind == yaml.MappingNode {
		if len(vNode.Content) == 0 {
			fmt.Fprintf(buf, "%s%s: {}\n", prefix, keyStr)
			return
		}
		fmt.Fprintf(buf, "%s%s:\n", prefix, keyStr)
		renderMappingNode(buf, vNode, indent+2)
	}
}

func renderSequenceNode(buf *bytes.Buffer, seq *yaml.Node, indent int) {
	prefix := strings.Repeat(" ", indent)
	for _, item := range seq.Content {
		if item.Kind == yaml.MappingNode && item.Style != yaml.FlowStyle && len(item.Content) >= 2 {
			firstKey := formatScalarNode(item.Content[0])
			firstVal := renderInlineNode(item.Content[1])
			fmt.Fprintf(buf, "%s- %s: %s\n", prefix, firstKey, firstVal)
			subMap := &yaml.Node{Kind: yaml.MappingNode, Content: item.Content[2:]}
			renderMappingNode(buf, subMap, indent+2)
			continue
		}
		fmt.Fprintf(buf, "%s- %s\n", prefix, renderInlineNode(item))
	}
}

func renderInlineNode(n *yaml.Node) string {
	switch n.Kind {
	case yaml.ScalarNode:
		return formatScalarNode(n)
	case yaml.SequenceNode:
		parts := make([]string, 0, len(n.Content))
		for _, c := range n.Content {
			parts = append(parts, renderInlineNode(c))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case yaml.MappingNode:
		parts := make([]string, 0, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			parts = append(parts, fmt.Sprintf("%s: %s", formatScalarNode(n.Content[i]), renderInlineNode(n.Content[i+1])))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	default:
		return ""
	}
}

func formatScalarNode(n *yaml.Node) string {
	if n.Tag == "!!null" || n.Value == "null" && n.Style == 0 {
		return "null"
	}
	if n.Tag == "!!bool" || n.Tag == "!!int" || n.Tag == "!!float" {
		return n.Value
	}
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	single := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   n.Tag,
		Value: n.Value,
	}
	if needsQuotedYAMLString(n.Value) {
		single.Style = yaml.DoubleQuotedStyle
	}
	_ = enc.Encode(single)
	_ = enc.Close()
	return strings.TrimSpace(b.String())
}

func needsQuotedYAMLString(s string) bool {
	if s == "" || s == "true" || s == "false" || s == "True" || s == "False" || s == "null" || s == "~" {
		return true
	}
	if strings.ContainsAny(s, "\n\r\t") || strings.HasSuffix(s, ".") || strings.ContainsAny(s, ":#{}[]&*!|>'\"%@`,?") {
		return true
	}
	return strings.TrimSpace(s) != s
}

// UpdateExamplesReadme replaces the block between BeginCatalogMarker and
// EndCatalogMarker in readmePath with a Markdown summary table of blueprints.
func UpdateExamplesReadme(readmePath string, blueprints []BlueprintEntry) error {
	content, err := os.ReadFile(readmePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", readmePath, err)
	}
	text := string(content)
	startIdx := strings.Index(text, BeginCatalogMarker)
	endIdx := strings.Index(text, EndCatalogMarker)
	if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
		return fmt.Errorf("markers %s ... %s not found in %s", BeginCatalogMarker, EndCatalogMarker, readmePath)
	}

	table := renderBlueprintMarkdownTable(blueprints)
	updated := text[:startIdx+len(BeginCatalogMarker)] + "\n\n" + table + "\n" + text[endIdx:]
	return os.WriteFile(readmePath, []byte(updated), 0644)
}

func renderBlueprintMarkdownTable(blueprints []BlueprintEntry) string {
	var sb strings.Builder
	sb.WriteString("| Blueprint | Scheduler | Machine Types | Description |\n")
	sb.WriteString("| :--- | :--- | :--- | :--- |\n")
	for _, bp := range blueprints {
		linkTarget := readmeRelativeLink(bp.Path)
		mt := "-"
		if len(bp.MachineTypes) > 0 {
			mt = strings.Join(bp.MachineTypes, ", ")
		}
		desc := strings.ReplaceAll(bp.Description, "|", "\\|")
		fmt.Fprintf(&sb, "| [%s](%s) | %s | %s | %s |\n", bp.Path, linkTarget, bp.Scheduler, mt, desc)
	}
	return sb.String()
}

func readmeRelativeLink(bpPath string) string {
	if strings.HasPrefix(bpPath, "examples/") {
		return "./" + strings.TrimPrefix(bpPath, "examples/")
	}
	return "../" + bpPath
}
