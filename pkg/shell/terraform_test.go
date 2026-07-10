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
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
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
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("kubernetes_namespace.ns: dial tcp 10.128.0.2:443: connect: connection refused")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("helm_release.release: dial tcp [::1]:80: connect: connection refused")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("kubernetes_namespace.ns: dial tcp 35.184.1.2:443: i/o timeout")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("kubernetes_namespace.ns: dial tcp controlplane.k8s.local:443: i/o timeout")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("kubernetes_namespace.ns: dial tcp: lookup controlplane.k8s.local: no such host")), Equals, true)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("Post \"https://storage.googleapis.com/... \": dial tcp 142.250.190.46:443: i/o timeout")), Equals, false)
	c.Assert(IsKubernetesUnreachableError(fmt.Errorf("google_compute_instance.vm: dial tcp 10.128.0.5:443: i/o timeout")), Equals, false)
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

type mockTerraformCLI struct {
	state   *tfjson.State
	removed []string
}

func (m *mockTerraformCLI) Show(ctx context.Context, opts ...tfexec.ShowOption) (*tfjson.State, error) {
	return m.state, nil
}

func (m *mockTerraformCLI) StateRm(ctx context.Context, address string, opts ...tfexec.StateRmCmdOption) error {
	m.removed = append(m.removed, address)
	return nil
}

func (s *MySuite) TestRemoveKubernetesResourcesFromState(c *C) {
	// Construct in-memory state with google and kubernetes/helm/kubectl resources
	state := &tfjson.State{
		Values: &tfjson.StateValues{
			RootModule: &tfjson.StateModule{
				Resources: []*tfjson.StateResource{
					{Address: "google_compute_network.vpc", Type: "google_compute_network"},
					{Address: "kubernetes_service_account.sa", Type: "kubernetes_service_account"},
				},
				ChildModules: []*tfjson.StateModule{
					{
						Resources: []*tfjson.StateResource{
							{Address: "module.foo.helm_release.nginx", Type: "helm_release"},
							{Address: "module.foo.kubectl_manifest.pod", Type: "kubectl_manifest"},
							{Address: "module.foo.google_service_account.sa", Type: "google_service_account"},
						},
					},
				},
			},
		},
	}

	mockTf := &mockTerraformCLI{state: state}

	err := RemoveKubernetesResourcesFromState(mockTf)
	c.Assert(err, IsNil)

	// Verify that StateRm was called exactly for the kubernetes, helm and kubectl resources
	expectedRemoved := []string{
		"kubernetes_service_account.sa",
		"module.foo.helm_release.nginx",
		"module.foo.kubectl_manifest.pod",
	}
	c.Assert(mockTf.removed, DeepEquals, expectedRemoved)
}
