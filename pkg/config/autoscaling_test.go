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
	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *zeroSuite) TestExpandClusterAutoscaling_NoAutoscaling(c *C) {
	mod := tMod("test-mod").build()
	bp := Blueprint{}
	err := ExpandClusterAutoscaling(bp, &mod)
	c.Check(err, IsNil)
}

func (s *zeroSuite) TestExpandClusterAutoscaling_Disabled(c *C) {
	ca := cty.ObjectVal(map[string]cty.Value{
		"enabled": cty.False,
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}
	err := ExpandClusterAutoscaling(bp, &mod)
	c.Check(err, IsNil)
}

func (s *zeroSuite) TestExpandClusterAutoscaling_GPU(c *C) {
	ca := cty.ObjectVal(map[string]cty.Value{
		"enabled": cty.True,
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type":          cty.StringVal("a3-highgpu-8g"),
				"autoprovisioning_max_accelerator_count": cty.NumberIntVal(16),
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	c.Check(err, IsNil)

	newCa := mod.Settings.Get("cluster_autoscaling")
	caMap := newCa.AsValueMap()
	limits := caMap["limits"]
	it := limits.ElementIterator()
	c.Assert(it.Next(), Equals, true)
	_, resVal := it.Element()
	resMap := resVal.AsValueMap()

	c.Check(resMap["autoprovisioning_machine_type"].AsString(), Equals, "nvidia-h100-80gb")
	c.Check(resMap["autoprovisioning_max_accelerator_count"].AsBigFloat(), DeepEquals, cty.NumberIntVal(16).AsBigFloat())
}

func (s *zeroSuite) TestExpandClusterAutoscaling_GPU_DefaultCount(c *C) {
	ca := cty.ObjectVal(map[string]cty.Value{
		"enabled": cty.True,
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type": cty.StringVal("a3-highgpu-8g"),
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	c.Check(err, IsNil)

	newCa := mod.Settings.Get("cluster_autoscaling")
	caMap := newCa.AsValueMap()
	limits := caMap["limits"]
	it := limits.ElementIterator()
	c.Assert(it.Next(), Equals, true)
	_, resVal := it.Element()
	resMap := resVal.AsValueMap()

	c.Check(resMap["autoprovisioning_max_accelerator_count"].AsBigFloat(), DeepEquals, cty.NumberIntVal(8).AsBigFloat())
}

func (s *zeroSuite) TestExpandClusterAutoscaling_InvalidCount(c *C) {
	ca := cty.ObjectVal(map[string]cty.Value{
		"enabled": cty.True,
		"limits": cty.ListVal([]cty.Value{
			cty.ObjectVal(map[string]cty.Value{
				"autoprovisioning_machine_type":          cty.StringVal("a3-highgpu-8g"),
				"autoprovisioning_max_accelerator_count": cty.NumberIntVal(10), // Not a multiple of 8
			}),
		}),
	})
	mod := tMod("test-mod").set("cluster_autoscaling", ca).build()
	bp := Blueprint{}

	err := ExpandClusterAutoscaling(bp, &mod)
	c.Check(err, NotNil)
	c.Check(err.Error(), Matches, ".*must be a multiple.*")
}

func (s *zeroSuite) TestExtractAcceleratorCountAndType(c *C) {
	// GPU
	count, t := extractAcceleratorCountAndType("a3-highgpu-8g")
	c.Check(count, Equals, 8)
	c.Check(t, Equals, "nvidia-h100-80gb")

	count, t = extractAcceleratorCountAndType("a3-megagpu-8g")
	c.Check(count, Equals, 8)
	c.Check(t, Equals, "nvidia-h100-mega-80gb")

	count, t = extractAcceleratorCountAndType("a3-ultragpu-8g")
	c.Check(count, Equals, 8)
	c.Check(t, Equals, "nvidia-h200-141gb")

	// TPU
	count, t = extractAcceleratorCountAndType("ct4p-hightpu-4t")
	c.Check(count, Equals, 4)
	c.Check(t, Equals, "tpu-v4-podslice")

	count, t = extractAcceleratorCountAndType("ct5lp-hightpu-4t")
	c.Check(count, Equals, 4)
	c.Check(t, Equals, "tpu-v5-lite-podslice")

	// Unknown or non-accelerator
	count, t = extractAcceleratorCountAndType("n1-standard-4")
	c.Check(count, Equals, 0)
	c.Check(t, Equals, "")
}
