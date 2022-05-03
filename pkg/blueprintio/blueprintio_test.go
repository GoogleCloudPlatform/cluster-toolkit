// Copyright 2022 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package blueprintio

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/spf13/afero"
	. "gopkg.in/check.v1"
)

const (
	testGitignoreTmpl = `
# Local .terraform directories
**/.terraform/*
`
	testGitignoreNewTmpl = `
# Local .terraform directories
**/.terraform/*

# .tfstate files
*.tfstate
*.tfstate.*
`
)

var testDir string

// Setup GoCheck
type MySuite struct{}

var _ = Suite(&MySuite{})

func Test(t *testing.T) {
	TestingT(t)
}

func setup() {
	t := time.Now()
	dirName := fmt.Sprintf("ghpc_blueprintio_test_%s", t.Format(time.RFC3339))
	dir, err := ioutil.TempDir("", dirName)
	if err != nil {
		log.Fatalf("reswriter_test: %v", err)
	}
	testDir = dir
}

func teardown() {
	os.RemoveAll(testDir)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	teardown()
	os.Exit(code)
}

func getTestFS() afero.IOFS {
	aferoFS := afero.NewMemMapFs()
	aferoFS.MkdirAll("pkg/reswriter", 0755)
	afero.WriteFile(
		aferoFS, "pkg/reswriter/blueprint.gitignore.tmpl", []byte(testGitignoreTmpl), 0644)
	afero.WriteFile(
		aferoFS, "pkg/reswriter/blueprint_new.gitignore.tmpl", []byte(testGitignoreNewTmpl), 0644)
	return afero.NewIOFS(aferoFS)
}

func (s *MySuite) TestGetBlueprintIOLocal(c *C) {
	blueprintio := GetBlueprintIOLocal()
	c.Assert(blueprintio, Equals, blueprintios["local"])
}
