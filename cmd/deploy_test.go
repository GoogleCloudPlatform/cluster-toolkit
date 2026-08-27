/*
Copyright 2026 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cmd

import (
	"context"
	"hpc-toolkit/pkg/config"
	"hpc-toolkit/pkg/shell"
	"os"

	"github.com/zclconf/go-cty/cty"
	. "gopkg.in/check.v1"
)

func (s *MySuite) TestDeployGroups(c *C) {
	var err error
	pathEnv := os.Getenv("PATH")
	os.Setenv("PATH", "")

	err = deployTerraformGroup(".", getArtifactsDir("."), shell.NeverApply, shell.TextOutput)
	c.Check(err, NotNil)

	err = deployPackerGroup(".", shell.NeverApply)
	c.Check(err, NotNil)

	os.Setenv("PATH", pathEnv)
}

func (s *MySuite) TestCreateGcsBucketsIfMissingEmptyBucket(c *C) {
	bp := config.Blueprint{
		Vars: config.NewDict(map[string]cty.Value{
			"project_id": cty.StringVal("my-project"),
		}),
		Groups: []config.Group{
			{
				Name: "group1",
				TerraformBackend: config.TerraformBackend{
					Type: "gcs",
					Configuration: config.NewDict(map[string]cty.Value{
						"bucket": cty.NullVal(cty.String),
					}),
				},
			},
		},
	}

	err := createGcsBucketsIfMissing(context.Background(), bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name cannot be empty or unknown")

	bp.Groups[0].TerraformBackend.Configuration = config.NewDict(map[string]cty.Value{
		"bucket": cty.StringVal(""),
	})
	err = createGcsBucketsIfMissing(context.Background(), bp)
	c.Assert(err, NotNil)
	c.Check(err.Error(), Equals, "GCS backend bucket name cannot be empty")
}
