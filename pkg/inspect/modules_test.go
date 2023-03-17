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
	"hpc-toolkit/pkg/inspect"
	"hpc-toolkit/pkg/modulereader"
	"log"
	"path/filepath"
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

func notEmpty[E any](l []E, t *testing.T) []E {
	if l == nil || len(l) == 0 {
		t.Fatal("Did not expect empty list")
	}
	return l
}

// Self-test
func TestSanity(t *testing.T) {
	notEmpty(query(all()), t)
}

func TestLabelsType(t *testing.T) {
	for _, mod := range notEmpty(query(hasInput("labels")), t) {
		labels, _ := mod.Input("labels")
		if labels.Type != "map(string)" {
			t.Errorf("%s.labels has unexpected type %#v", mod.Source, labels.Type)
		}
	}
}
