// Copyright 2026 "Google LLC"
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

package config

// Performs "materialization" of the blueprint, which means:
// * evaluate Vars
// * evaluate TerraformBackens
// * partially evaluate `ghpc_stage` in module settings
// TODO:
// * perform substitution of IGC references with synthetic vars
// * perform evaluation of module settings for packer group
func (bp *Blueprint) Materialize() error {
	var err error
	if bp.Vars, err = bp.evalVars(); err != nil {
		return err
	}

	if err := bp.evalGhpcStage(); err != nil {
		return err
	}

	for ig := range bp.Groups {
		if err := materizalizeGroup(bp, &bp.Groups[ig]); err != nil {
			return err
		}
	}

	// TODO: perform validation of the blueprint here (instead of cmd.expandOrDie)

	return nil
}

func materizalizeGroup(bp *Blueprint, g *Group) error {
	var err error

	be := &g.TerraformBackend // evaluate TerrafomrBackend
	if be.Configuration, err = bp.EvalDict(be.Configuration); err != nil {
		return err
	}

	return nil
}
