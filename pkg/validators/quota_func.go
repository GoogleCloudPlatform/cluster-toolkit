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

package validators

import (
	"errors"
	"fmt"

	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

var reqCtyType = cty.ObjectWithOptionalAttrs(map[string]cty.Type{
	"metric":     cty.String,
	"required":   cty.Number,
	"dimensions": cty.Map(cty.String),
}, /*optional=*/ []string{"dimensions"})
var reqCtyRetType = function.StaticReturnType(reqCtyType)

func reqCtyFromStruct(s ResourceRequirement) cty.Value {
	dv := cty.NullVal(cty.Map(cty.String))
	if s.Dimensions != nil {
		dm := make(map[string]cty.Value)
		for k, v := range s.Dimensions {
			dm[k] = cty.StringVal(v)
		}
		dv = cty.MapVal(dm)
	}

	return cty.ObjectVal(map[string]cty.Value{
		"metric":     cty.StringVal(s.Metric),
		"required":   cty.NumberIntVal(s.Required),
		"dimensions": dv,
	})
}

// CPUQuotaFunc is a function that returns the quota required for a given CPU request.
var CPUQuotaFunc = function.New(&function.Spec{
	Description: `Returns quotas required for a given CPU request.`,
	Params: []function.Parameter{
		{Name: "machine_type", Type: cty.String},
		{Name: "count", Type: cty.Number},
	},
	Type: reqCtyRetType,
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		mt := args[0].AsString()
		cnt, acc := args[1].AsBigFloat().Int64()
		if acc != 0 || cnt < 0 { // TODO: also check for safety of int64 -> int conversion
			return cty.NilVal, function.NewArgError(1, errors.New("count must be a non-negative integer"))
		}
		req, err := cpuQuotaImpl(mt, int(cnt))
		if err != nil {
			return cty.NilVal, err
		}
		return reqCtyFromStruct(req), nil

	},
})

// Dummy implementation, supports only n2-standard-2.
func cpuQuotaImpl(mt string, count int) (ResourceRequirement, error) {
	if mt != "n2-standard-2" {
		return ResourceRequirement{}, fmt.Errorf("unsupported machine type %q", mt)
	}
	return ResourceRequirement{
		Metric:   "compute.googleapis.com/n2_cpus",
		Required: int64(count) * 2,
	}, nil
}
