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

package shell

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	tfjson "github.com/hashicorp/terraform-json"
	. "gopkg.in/check.v1"
)

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func (s *MySuite) TestFindTerraform(c *C) {
	if _, err := exec.LookPath("terraform"); err != nil {
		_, err := ConfigureTerraform(".")
		c.Assert(err, NotNil)
		c.Skip("terraform not found in PATH")
	}

	_, err := ConfigureTerraform(".")
	c.Assert(err, IsNil)

	// test failure when terraform cannot be found in PATH
	pathEnv := os.Getenv("PATH")
	os.Setenv("PATH", "")
	_, err = ConfigureTerraform(".")
	os.Setenv("PATH", pathEnv)
	c.Assert(err, NotNil)
}

func (s *MySuite) TestIsKubernetesUnreachableError(c *C) {
	c.Assert(IsKubernetesUnreachableError(nil), Equals, false)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("some other error")), Equals, false)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("dial tcp [::1]:80: connect: connection refused")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("Error: Kubernetes cluster unreachable: invalid configuration")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("failed to create kubernetes rest client for read")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("no configuration has been provided, try setting KUBERNETES_MASTER environment variable")), Equals, true)
}

func (s *MySuite) TestGetResourcesRecursively(c *C) {
	// Nil module
	c.Assert(getResourcesRecursively(nil), IsNil)

	// Leaf module
	leaf := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			{Address: "module.foo.kubernetes_service_account.main", Type: "kubernetes_service_account"},
			{Address: "module.foo.google_service_account.main", Type: "google_service_account"},
		},
	}
	c.Assert(getResourcesRecursively(leaf), DeepEquals, []*tfjson.StateResource{
		{Address: "module.foo.kubernetes_service_account.main", Type: "kubernetes_service_account"},
		{Address: "module.foo.google_service_account.main", Type: "google_service_account"},
	})

	// Nested modules
	root := &tfjson.StateModule{
		Resources: []*tfjson.StateResource{
			{Address: "google_compute_network.vpc", Type: "google_compute_network"},
		},
		ChildModules: []*tfjson.StateModule{
			leaf,
			{
				Resources: []*tfjson.StateResource{
					{Address: "module.bar.helm_release.apply_chart", Type: "helm_release"},
				},
			},
		},
	}
	c.Assert(getResourcesRecursively(root), DeepEquals, []*tfjson.StateResource{
		{Address: "google_compute_network.vpc", Type: "google_compute_network"},
		{Address: "module.foo.kubernetes_service_account.main", Type: "kubernetes_service_account"},
		{Address: "module.foo.google_service_account.main", Type: "google_service_account"},
		{Address: "module.bar.helm_release.apply_chart", Type: "helm_release"},
	})
}
