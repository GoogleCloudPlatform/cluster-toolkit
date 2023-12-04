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

func (m *modInfo) Role() string {
	split := strings.Split(m.Source, "/")
	return split[len(split)-2]
}

var allMods []modInfo = nil

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
		info, err := modulereader.GetModuleInfo(filepath.Join("../..", sk.Source), sk.Kind)
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

func hasInput(name string) predicate {
	return func(mod modInfo) bool {
		_, ok := mod.Input(name)
		return ok
	}
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

func checkInputType(t *testing.T, mod modInfo, input string, expected string) {
	i, ok := mod.Input(input)
	if !ok {
		t.Errorf("%s does not have input %s", mod.Source, input)
	}
	expected = modulereader.NormalizeType(expected)
	got := modulereader.NormalizeType(i.Type)
	if expected != got {
		t.Errorf("%s %s has unexpected type expected:\n%#v\ngot:\n%#v",
			mod.Source, input, expected, got)
	}
}

func TestLabelsType(t *testing.T) {
	for _, mod := range notEmpty(query(hasInput("labels")), t) {
		checkInputType(t, mod, "labels", "map(string)")
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

	for _, mod := range notEmpty(query(hasInput("network_storage")), t) {
		i, _ := mod.Input("network_storage")
		got := modulereader.NormalizeType(i.Type)
		if got != obj && got != lst {
			t.Errorf("%s `network_storage` has unexpected type expected:\n%#v\nor\n%#v\ngot:\n%#v",
				mod.Source, obj, lst, got)
		}
	}
}
