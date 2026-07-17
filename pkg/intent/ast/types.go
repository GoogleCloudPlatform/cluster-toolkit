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

// Package ast defines the abstract syntax tree data structures for the v2 intent compiler.
package ast

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// UnmarshalClusterConfig parses YAML bytes into a ClusterConfig, ignoring unknown keys.
//
// This is the lenient parser and exists for best-effort readers such as telemetry, which
// must still be able to identify a run as v2 even when the config is malformed enough
// that the compiler will reject it. Anything that actually compiles the config must use
// UnmarshalClusterConfigStrict instead.
func UnmarshalClusterConfig(data []byte) (ClusterConfig, error) {
	return unmarshalClusterConfig(data, false)
}

// UnmarshalClusterConfigStrict parses YAML bytes into a ClusterConfig and rejects any key
// that is not part of the schema.
//
// Without this, a misspelled or obsolete top-level key (`addons:`, `compute_archetype:`)
// is silently discarded: the section never reaches the compiler, the cluster is built
// without it, and gcluster exits 0. A silently smaller cluster is far worse than a failed
// compile, so unknown keys are a hard error.
func UnmarshalClusterConfigStrict(data []byte) (ClusterConfig, error) {
	return unmarshalClusterConfig(data, true)
}

func cleanYAMLError(err error) string {
	msg := err.Error()
	msg = strings.ReplaceAll(msg, "yaml: unmarshal errors:\n  ", "")
	msg = strings.ReplaceAll(msg, "in type ast.ClusterConfig", "in cluster config (allowed top-level keys: config_base, vars, compute_archetypes, features, overlays)")
	msg = strings.ReplaceAll(msg, "in type ast.ArchetypeReference", "in compute_archetypes entry (allowed keys: name, machine_type, settings)")
	msg = strings.ReplaceAll(msg, "in type ast.FeatureReference", "in features/overlays entry (allowed keys: name, type, attach_to, settings)")
	msg = strings.ReplaceAll(msg, "in type ast.rawConfigBase", "in config_base (allowed keys: type, settings)")
	return msg
}

func unmarshalClusterConfig(data []byte, strict bool) (ClusterConfig, error) {
	var cfg ClusterConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(strict)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return ClusterConfig{}, nil
		}
		if strict {
			return ClusterConfig{}, fmt.Errorf("invalid cluster config: %s", cleanYAMLError(err))
		}
		return ClusterConfig{}, fmt.Errorf("syntax error in cluster config: %s", cleanYAMLError(err))
	}
	for i := range cfg.Features {
		if cfg.Features[i].Type == "" {
			cfg.Features[i].Type = cfg.Features[i].Name
		}
	}
	for i := range cfg.Overlays {
		if cfg.Overlays[i].Type == "" {
			cfg.Overlays[i].Type = cfg.Overlays[i].Name
		}
	}
	if len(cfg.ConfigBase.Settings) > 0 && strings.TrimSpace(cfg.ConfigBase.Type) == "" {
		return ClusterConfig{}, fmt.Errorf("invalid cluster config: config_base mapping must specify a non-empty 'type' field")
	}
	return cfg, nil
}

// UnmarshalClusterConfigFile reads a YAML file and parses it into a ClusterConfig.
//
// This is the compiler's entry point, so it parses strictly.
func UnmarshalClusterConfigFile(path string) (ClusterConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ClusterConfig{}, fmt.Errorf("failed to read cluster config file: %w", err)
	}
	return UnmarshalClusterConfigStrict(data)
}

// ClusterConfig represents Tier 1 User Intent (v2/cluster-configs/*.yaml).
type ClusterConfig struct {
	ConfigBase        ConfigBaseReference  `yaml:"config_base"`        // Scalar ("slurm") OR map ({ type: slurm, settings: {...} })
	Vars              map[string]any       `yaml:"vars"`               // Global deployment variables
	ComputeArchetypes []ArchetypeReference `yaml:"compute_archetypes"` // Compute pools
	Features          []FeatureReference   `yaml:"features,omitempty"` // Infrastructure add-ons
	Overlays          []FeatureReference   `yaml:"overlays,omitempty"` // Workload presets & customer modifiers (uses exact same FeatureReference struct)
}

// ConfigBaseReference unmarshals from EITHER `config_base: slurm`
// OR `config_base: { type: slurm, settings: { ... } }`.
type ConfigBaseReference struct {
	Type     string         `yaml:"type,omitempty"`     // "slurm", "gke", "jbvm", or "batch"
	Settings map[string]any `yaml:"settings,omitempty"` // Routed across config-base modules (slurm_controller, slurm_login, cluster)
}

// UnmarshalYAML allows ConfigBaseReference to be specified as either a scalar string
// (`config_base: slurm`) or a mapping (`config_base: { type: slurm, settings: {...} }`).
func (cb *ConfigBaseReference) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		cb.Type = value.Value
		cb.Settings = nil
		return nil
	case yaml.MappingNode:
		type rawConfigBase struct {
			Type     string         `yaml:"type,omitempty"`
			Settings map[string]any `yaml:"settings,omitempty"`
		}
		var raw rawConfigBase
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		if err := enc.Encode(value); err != nil {
			return fmt.Errorf("invalid config_base: %w", err)
		}
		_ = enc.Close()
		dec := yaml.NewDecoder(&buf)
		dec.KnownFields(true)
		if err := dec.Decode(&raw); err != nil {
			return fmt.Errorf("invalid config_base: %w", err)
		}
		cb.Type = raw.Type
		cb.Settings = raw.Settings
		return nil
	default:
		return fmt.Errorf("`config_base` must be a string (e.g. `config_base: slurm`) or a mapping (`type:` and `settings:`)")
	}
}

// MarshalYAML serializes ConfigBaseReference as a scalar string when Settings is empty,
// or as a mapping when Settings is non-empty.
func (cb ConfigBaseReference) MarshalYAML() (any, error) {
	if len(cb.Settings) == 0 {
		return cb.Type, nil
	}
	return struct {
		Type     string         `yaml:"type"`
		Settings map[string]any `yaml:"settings,omitempty"`
	}{
		Type:     cb.Type,
		Settings: cb.Settings,
	}, nil
}

// ArchetypeReference defines a compute pool request in ClusterConfig.
type ArchetypeReference struct {
	Name        string         `yaml:"name"`                   // Unique pool name (replaces {name})
	MachineType string         `yaml:"machine_type,omitempty"` // Matches ComputeArchetypeManifest.Name or GCE machine type
	Settings    map[string]any `yaml:"settings,omitempty"`     // Single settings map! Routed to {name}_nodeset, {name}_partition, {name}_startup, or pool-scoped vars
}

// FeatureReference is the SINGLE struct shared by both `features:` and `overlays:` in ClusterConfig!
type FeatureReference struct {
	Name     string         `yaml:"name"`                // Unique feature/overlay instance name (replaces {name})
	Type     string         `yaml:"type,omitempty"`      // Matches FeatureManifest.Name or OverlayManifest.Name (defaults to Name if omitted)
	AttachTo []string       `yaml:"attach_to,omitempty"` // Target pool names (["train-pool"])
	Settings map[string]any `yaml:"settings,omitempty"`  // Single settings map! Routed to target modules / vars
}

// BlueprintBlock is the shared structural block across ConfigBaseManifest,
// ComputeArchetypeManifest, and FeatureManifest.
type BlueprintBlock struct {
	Vars             map[string]any     `yaml:"vars,omitempty"`              // Default variables offered by this block
	DeploymentGroups []DeploymentGroup  `yaml:"deployment_groups,omitempty"` // Ordered module groups
	Assertions       []FeatureAssertion `yaml:"assertions,omitempty"`        // Optional CEL pre-flight checks
}

// ConfigBaseManifest represents Tier 2 Config Base skeleton specifications (v2/config-base/*.yaml).
type ConfigBaseManifest struct {
	Name           string            `yaml:"name"` // "slurm", "gke", "jbvm", "batch"
	Description    string            `yaml:"description,omitempty"`
	Aliases        map[string]string `yaml:"aliases,omitempty"`
	BlueprintBlock `yaml:",inline"`
}

// ComputeArchetypeManifest represents Tier 3 Hardware specifications (v2/compute-archetypes/*.yaml).
type ComputeArchetypeManifest struct {
	Name             string                    `yaml:"name"`                        // e.g. "a3-ultragpu-8g", "cpu"
	Description      string                    `yaml:"description,omitempty"`       // Human-readable hardware archetype summary
	MatchPatterns    []string                  `yaml:"match_patterns,omitempty"`    // e.g. ["ct6e-standard-*", "*"]
	ExcludedPrefixes []string                  `yaml:"excluded_prefixes,omitempty"` // Prefixes blocked from cpu fallback
	BlueprintBlock   `yaml:",inline"`          // Shared Vars, DeploymentGroups, Assertions
	ConfigBase       map[string]BlueprintBlock `yaml:"config_base"` // Per-orchestrator ("slurm", "gke", "jbvm") BlueprintBlock
}

// FeatureAssertion represents a CEL rule validation step to run before expanding a feature.
type FeatureAssertion struct {
	Rule    string `yaml:"rule"`
	Message string `yaml:"message"`
}

// FeatureManifest represents Tier 4 Composable Feature specifications (v2/features/*.yaml).
type FeatureManifest struct {
	Name           string                    `yaml:"name"` // e.g. "filestore", "managed-lustre"
	Description    string                    `yaml:"description,omitempty"`
	BlueprintBlock `yaml:",inline"`          // Root Vars, DeploymentGroups, Assertions
	ConfigBase     map[string]BlueprintBlock `yaml:"config_base,omitempty"`
}

// OverlayManifest represents Tier 5 Solution Payload & Composite Customer Profile specifications (v2/overlays/*.yaml).
//
// Shares the core schema of ClusterConfig (`config_base`, `vars`, `compute_archetypes`, `features`)
// with two structural differences:
//  1. All ClusterConfig sections are optional (`omitempty`) since an overlay is strictly additive in Phase 1.
//  2. `overlays:` is omitted so overlays cannot nest other overlays or form cyclic dependencies.
type OverlayManifest struct {
	Name              string               `yaml:"name"`
	Description       string               `yaml:"description,omitempty"`
	ConfigBase        ConfigBaseReference  `yaml:"config_base,omitempty"`        // Target config_base ("gke", "slurm", "jbvm", "batch")
	Vars              map[string]any       `yaml:"vars,omitempty"`               // Additive deployment variables (or fills nil placeholders)
	ComputeArchetypes []ArchetypeReference `yaml:"compute_archetypes,omitempty"` // Additive compute pools
	Features          []FeatureReference   `yaml:"features,omitempty"`           // Additive features
}

// DeploymentGroup represents an execution group of modules within an AST specification.
type DeploymentGroup struct {
	Group   string       `yaml:"group"`             // "cluster-env", "cluster", or "auto"
	Modules []ModuleSpec `yaml:"modules,omitempty"` // Single unified list of modules
}

// UnmarshalYAML decodes DeploymentGroup and marks compute pool modules (`{name}_nodeset`,
// `{name}_nodepool`, `{name}_pool`, `{name}_instance`) with `IsComputePool: true`.
func (g *DeploymentGroup) UnmarshalYAML(value *yaml.Node) error {
	type rawDeploymentGroup struct {
		Group   string       `yaml:"group"`
		Modules []ModuleSpec `yaml:"modules,omitempty"`
	}
	var raw rawDeploymentGroup
	if err := decodeStrictNode(value, &raw); err != nil {
		return err
	}
	g.Group = raw.Group
	g.Modules = make([]ModuleSpec, 0, len(raw.Modules))
	for _, m := range raw.Modules {
		if isComputePoolModuleSpec(m) {
			m.IsComputePool = true
		}
		g.Modules = append(g.Modules, m)
	}
	return nil
}

func isComputePoolModuleSpec(m ModuleSpec) bool {
	if !strings.Contains(m.ID, "{name}") || m.Settings == nil {
		return false
	}
	_, hasMachineType := m.Settings["machine_type"]
	return hasMachineType
}

func decodeStrictNode(value *yaml.Node, target any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(value); err != nil {
		return err
	}
	_ = enc.Close()
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	return dec.Decode(target)
}

// Provenance tracks user-facing YAML section & instance name for compiler diagnostics.
type Provenance struct {
	Section      string // e.g. "config_base", "compute_archetypes[0]", "features[1]", "overlays[0]"
	InstanceName string // e.g. "train-pool", "scratchfs", "gpu-check"
	CatalogType  string // e.g. "slurm", "a3-ultragpu-8g", "filestore", "run-nvidia-smi"
}

// ModuleSpec defines a modular building block or hardware stanza within the AST.
type ModuleSpec struct {
	ID        string         `yaml:"id"`
	Source    string         `yaml:"source,omitempty"`
	Kind      string         `yaml:"kind,omitempty"`
	Condition string         `yaml:"condition,omitempty"`
	Export    bool           `yaml:"export,omitempty"`
	Outputs   []any          `yaml:"outputs,omitempty"`
	Use       []string       `yaml:"use,omitempty"`
	Settings  map[string]any `yaml:"settings,omitempty"`

	// Compiler-internal metadata (not serialized to YAML)
	Origin            Provenance `yaml:"-"`
	FeatureInstanceID string     `yaml:"-"`
	Tier              int        `yaml:"-"`
	PoolName          string     `yaml:"-"`
	AttachTo          []string   `yaml:"-"`
	IsComputePool     bool       `yaml:"-"`
}
