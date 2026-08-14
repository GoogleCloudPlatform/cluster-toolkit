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

package config

import (
	"testing"

	"github.com/zclconf/go-cty/cty"
)

func TestExpandClusterAutoscaling_NoAutoscaling(t *testing.T) {
	mod := tMod("test-mod").build()
	bp := Blueprint{}
	err := ExpandClusterAutoscaling(bp, &mod)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestExpandClusterAutoscaling_GPU(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"gpus": {"a3-highgpu-8g": {"count": 8, "type": "nvidia-h100-80gb"}}}`)
	ca := cty.ObjectVal(map[string]cty.Value{
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type": cty.StringVal("a3-highgpu-8g"),
				"autoprovisioning_max_count":    cty.NumberIntVal(16),
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	if err != nil {
		t.Fatalf("ExpandClusterAutoscaling failed: %v", err)
	}

	newCa := mod.Settings.Get("cluster_autoscaling")
	caMap := newCa.AsValueMap()
	limits := caMap["limits"]
	it := limits.ElementIterator()
	if !it.Next() {
		t.Fatal("expected at least one limit element")
	}
	_, resVal := it.Element()
	resMap := resVal.AsValueMap()

	if resMap["autoprovisioning_resource_type"].AsString() != "nvidia-h100-80gb" {
		t.Errorf("expected resource type nvidia-h100-80gb, got %s", resMap["autoprovisioning_resource_type"].AsString())
	}

	f, _ := resMap["autoprovisioning_max_count"].AsBigFloat().Float64()
	if f != 16 {
		t.Errorf("expected max count 16, got %v", f)
	}
}

func TestExpandClusterAutoscaling_GPU_DefaultCount(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"gpus": {"a3-highgpu-8g": {"count": 8, "type": "nvidia-h100-80gb"}}}`)
	ca := cty.ObjectVal(map[string]cty.Value{
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type": cty.StringVal("a3-highgpu-8g"),
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	if err != nil {
		t.Fatalf("ExpandClusterAutoscaling failed: %v", err)
	}

	newCa := mod.Settings.Get("cluster_autoscaling")
	caMap := newCa.AsValueMap()
	limits := caMap["limits"]
	it := limits.ElementIterator()
	if !it.Next() {
		t.Fatal("expected at least one limit element")
	}
	_, resVal := it.Element()
	resMap := resVal.AsValueMap()

	f, _ := resMap["autoprovisioning_max_count"].AsBigFloat().Float64()
	if f != 1000 {
		t.Errorf("expected max count 1000, got %v", f)
	}
}

func TestExpandClusterAutoscaling_InvalidCount(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"gpus": {"a3-highgpu-8g": {"count": 8, "type": "nvidia-h100-80gb"}}}`)
	ca := cty.ObjectVal(map[string]cty.Value{
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type": cty.StringVal("a3-highgpu-8g"),
				"autoprovisioning_max_count":    cty.NumberIntVal(10), // Not a multiple of 8
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	if err == nil {
		t.Fatal("expected error for invalid count, got nil")
	}
}

func TestExpandClusterAutoscaling_ZeroAccelerators(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"cpus": {"n1-standard-4": {"count": 4}}}`)
	ca := cty.ObjectVal(map[string]cty.Value{
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type": cty.StringVal("n1-standard-4"),
				"autoprovisioning_max_count":    cty.NumberIntVal(10),
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	if err == nil {
		t.Fatal("expected error for zero accelerators, got nil")
	}
}

func TestExpandClusterAutoscaling_TPU(t *testing.T) {
	t.Setenv("GHPC_MOCK_MACHINE_CONFIG", `{"tpus": {"ct6e-standard-4t": {"count": 4}}}`)
	ca := cty.ObjectVal(map[string]cty.Value{
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type": cty.StringVal("ct6e-standard-4t"),
				"autoprovisioning_max_count":    cty.NumberIntVal(4),
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	if err != nil {
		t.Fatalf("ExpandClusterAutoscaling failed: %v", err)
	}

	newCa := mod.Settings.Get("cluster_autoscaling")
	caMap := newCa.AsValueMap()
	limits := caMap["limits"]
	it := limits.ElementIterator()
	if !it.Next() {
		t.Fatal("expected at least one limit element")
	}
	_, resVal := it.Element()
	resMap := resVal.AsValueMap()

	if resMap["autoprovisioning_resource_type"].AsString() != "ct6e-standard-4t" {
		t.Errorf("expected resource type ct6e-standard-4t, got %s", resMap["autoprovisioning_resource_type"].AsString())
	}
}
