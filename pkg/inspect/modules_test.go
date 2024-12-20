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

package inspect_test

import (
	"fmt"
	"hpc-toolkit/pkg/inspect"
	"hpc-toolkit/pkg/modulereader"
	"log"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2/ext/typeexpr"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/exp/slices"
)

type varInfo = modulereader.VarInfo
type predicate = func(modInfo) bool

type modInfo struct {
	modulereader.ModuleInfo
	inspect.SourceAndKind
}

func (m *modInfo) Input(name string) (varInfo, bool) {
	ind := slices.IndexFunc(m.Inputs, func(v varInfo) bool { return v.Name == name })
	if ind == -1 {
		return varInfo{}, false
	}
	return m.Inputs[ind], true
}

var allMods []modInfo = nil

// we can't use embedded FS here (defined in super-package).
// Craft local path.
func modPath(source string) string {
	return filepath.Join("../..", source)
}

func getModules() []modInfo {
	if allMods != nil {
		return allMods
	}
	sks, err := inspect.LocalModules()
	if err != nil {
		log.Fatal(err)
	}
	allMods = []modInfo{}
	for _, sk := range sks {
		if strings.Contains(sk.Source, "/internal/") {
			continue // skip internal modules
			// TODO: remove skipping internal modules
		}

		info, err := modulereader.GetModuleInfo(modPath(sk.Source), sk.Kind)
		if err != nil {
			log.Fatal(err)
		}
		allMods = append(allMods, modInfo{ModuleInfo: info, SourceAndKind: sk})
	}
	return allMods
}

func query(p predicate) []modInfo {
	ret := []modInfo{}
	for _, mod := range getModules() {
		if p(mod) {
			ret = append(ret, mod)
		}
	}
	return ret
}

func all(ps ...predicate) predicate {
	return func(mod modInfo) bool {
		for _, p := range ps {
			if !p(mod) {
				return false
			}
		}
		return true
	}
}

func hasInputField(name string) predicate {
	return func(mod modInfo) bool {
		return len(inspect.FindField(mod.Inputs, name)) > 0
	}
}

func queryInputFields(field string, t *testing.T) map[string]cty.Type {
	ret := map[string]cty.Type{}
	for _, mod := range notEmpty(query(hasInputField("additional_networks")), t) {
		for p, ty := range inspect.FindField(mod.Inputs, field) {
			ret[mod.Source+"@"+p] = ty
		}
	}
	return ret
}

// Fails test if slice is empty, returns not empty slice as is.
func notEmpty[E any](l []E, t *testing.T) []E {
	if len(l) == 0 {
		t.Fatal("Did not expect empty list")
	}
	return l
}

// Self-test checks that there are modules to inspect
func TestSanity(t *testing.T) {
	notEmpty(query(all()), t)
}

func TestLabelsType(t *testing.T) {
	want := "map(string)"
	for p, ty := range queryInputFields("labels", t) {
		got := typeexpr.TypeString(ty)
		if got != want {
			t.Errorf("%s has unexpected type expected, got:\n%#v\nwant:\n%#v", p, got, want)
		}
	}
}

func TestNetworkStorage(t *testing.T) {
	obj := modulereader.NormalizeType(`object({
		server_ip             = string
		remote_mount          = string
		local_mount           = string
		fs_type               = string
		mount_options         = string
		client_install_runner = map(string)
		mount_runner          = map(string)
	  })`)
	lst := modulereader.NormalizeType(fmt.Sprintf("list(%s)", obj))

	// short form (without runners) is used by Slurm6
	objShort := modulereader.NormalizeType(`object({
		server_ip             = string
		remote_mount          = string
		local_mount           = string
		fs_type               = string
		mount_options         = string
	})`)
	lstShort := modulereader.NormalizeType(fmt.Sprintf("list(%s)", objShort))

	for p, ty := range queryInputFields("network_storage", t) {
		got := typeexpr.TypeString(ty)
		if got != obj && got != lst && got != objShort && got != lstShort {
			t.Errorf("%s has unexpected type, got:\n%#v", p, got)
		}
	}
}

func TestAdditionalNetworks(t *testing.T) {
	want := modulereader.NormalizeType(`list(object({
		network            = string
		subnetwork         = string
		subnetwork_project = string
		network_ip         = string
		nic_type           = string
		stack_type         = string
		queue_count        = number
		access_config = list(object({
		  nat_ip       = string
		  network_tier = string
		}))
		ipv6_access_config = list(object({
		  network_tier = string
		}))
		alias_ip_range = list(object({
		  ip_cidr_range         = string
		  subnetwork_range_name = string
		}))
	  }))`)

	for p, ty := range queryInputFields("additional_networks", t) {
		got := typeexpr.TypeString(ty)
		if got != want {
			t.Errorf("%s has unexpected type expected, got:\n%#v\nwant:\n%#v", p, got, want)
		}
	}
}

func TestMetadataIsObtainable(t *testing.T) {
	// Test that `GetMetadata` does not fail. `GetMetadataSafe` falls back to legacy.
	for _, mod := range notEmpty(query(all()), t) {
		t.Run(mod.Source, func(t *testing.T) {
			_, err := modulereader.GetMetadata(modPath(mod.Source))
			if err != nil {
				t.Error(err)
			}
		})
	}
}

func TestMetadataHasServices(t *testing.T) {
	for _, mod := range notEmpty(query(all()), t) {
		t.Run(mod.Source, func(t *testing.T) {
			if mod.Metadata.Spec.Requirements.Services == nil {
				t.Error("metadata has no spec.requirements.services set")
			}
		})
	}
}

func TestMetadataInjectModuleId(t *testing.T) {
	for _, mod := range notEmpty(query(all()), t) {
		t.Run(mod.Source, func(t *testing.T) {
			gm := mod.Metadata.Ghpc
			if gm.InjectModuleId == "" {
				return
			}
			in, ok := mod.Input(gm.InjectModuleId)
			if !ok {
				t.Fatalf("has no input %q", gm.InjectModuleId)
			}
			if in.Type != cty.String {
				t.Errorf("%q type is not a string, but %q", gm.InjectModuleId, in.Type)
			}
		})
	}
}

func TestOutputForbiddenNames(t *testing.T) {
	nowhere := []string{}
	allowed := map[string][]string{
		// Global blueprint variables we don't want to get overwritten.
		"project_id":      nowhere,
		"labels":          nowhere,
		"region":          nowhere,
		"zone":            nowhere,
		"deployment_name": nowhere,
	}
	for _, mod := range query(all()) {
		t.Run(mod.Source, func(t *testing.T) {
			for _, out := range mod.Outputs {
				if where, ok := allowed[out.Name]; ok && !slices.Contains(where, mod.Source) {
					t.Errorf("forbidden name for output %q", out.Name)
				}
			}
		})
	}
}
