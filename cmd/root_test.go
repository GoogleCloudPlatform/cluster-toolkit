/*
Copyright 2022 Google LLC

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
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/storage/memory"
	. "gopkg.in/check.v1"
)

type MySuite struct{}

var _ = Suite(&MySuite{})

// git hash variables for testing
var initialGitHash = "8fc4768edbef9b3f115a41eaf2a5740d41758cff"
var oldGitHashFromMain = "b0a5f6f1ef6298ccda812e9c332be7a195e1f117"
var randomGitHash = "a975c295ddeab5b1a5323df92f61c4cc9fc88207"
var gitCommitInfo = "v1.0.0-393-gb8106eb"
var gitTagVersion = "v1.0.0"
var gitBranch = "main"

func Test(t *testing.T) {
	TestingT(t)
}

func TestMain(m *testing.M) {
	setup()
	code := m.Run()
	os.Exit(code)
}

func setup() {

}

// mockInjectedGitVariables()
func mockInjectedGitVariables() {
	GitTagVersion = gitTagVersion
	GitBranch = gitBranch
	GitCommitInfo = gitCommitInfo
	GitCommitHash = oldGitHashFromMain
	GitInitialHash = initialGitHash
}

/* Tests */
// root.go
func (s *MySuite) TestHpcToolkitRepo(c *C) {
	mockInjectedGitVariables()
	_, execPath, _, _ := runtime.Caller(0)
	workDir := filepath.Dir(execPath)
	repoDir := filepath.Dir(filepath.Dir(execPath))

	// find repo when workdir is in ./cmd subdir of repo
	_, got, err := hpcToolkitRepo()
	c.Assert(got, Equals, workDir)
	c.Assert(err, IsNil)

	// find repo when workdir is root of repo
	os.Chdir(repoDir)
	_, got, err = hpcToolkitRepo()
	c.Assert(got, Equals, repoDir)
	c.Assert(err, IsNil)

	// Try to find repo when workdir is outside root of repo. Normal execution
	// returns binary site in. Empty value expected during tests since test
	// binary is built in a test dir that is not in a git repository
	err = os.Chdir(filepath.Dir(repoDir))
	_, got, err = hpcToolkitRepo()
	c.Assert(err, Equals, git.ErrRepositoryNotExists)
	c.Assert(got, Equals, "")
}

func (s *MySuite) TestIsHpcToolkitRepo(c *C) {
	// sub directory of an hpc-toolkit git repository
	_, callDir, _, _ := runtime.Caller(0)
	repoDir := filepath.Dir(filepath.Dir(callDir))
	repo, _ := git.PlainOpen(repoDir)
	got := isHpcToolkitRepo(*repo)
	c.Assert(got, Equals, true)

	// temporary git repo is not a hpc-toolkit repo
	storer := memory.NewStorage()
	fs := memfs.New()
	testRepo, _ := git.Init(storer, fs)
	initTestRepo(*testRepo, fs)
	got = isHpcToolkitRepo(*testRepo)
	c.Assert(got, Equals, false)
}

func (s *MySuite) TestCheckGitHashMismatch(c *C) {
	_, callDir, _, _ := runtime.Caller(0)
	repoDir := filepath.Dir(filepath.Dir(callDir))
	workDir := filepath.Dir(callDir)
	repo, _ := git.PlainOpen(repoDir)
	head, _ := repo.Head()
	hash := head.Hash().String()
	branch := head.Name().Short()

	// verify current working directory git hash against hash of v1.0.0
	mockInjectedGitVariables()
	mismatch, b, h, dir := checkGitHashMismatch()
	c.Assert(mismatch, Equals, true)
	c.Assert(dir, Equals, workDir)
	c.Assert(b, Equals, branch)
	c.Assert(h, Equals, hash)

	// verify current working directory git hash against random initial hash
	mockInjectedGitVariables()
	GitInitialHash = randomGitHash
	mismatch, b, h, dir = checkGitHashMismatch()
	c.Assert(mismatch, Equals, false)
	c.Assert(b, Equals, "")
	c.Assert(h, Equals, "")

	// verify current working directory git hash against a incorrect random hash
	mockInjectedGitVariables()
	GitCommitHash = randomGitHash
	mismatch, b, h, dir = checkGitHashMismatch()
	c.Assert(mismatch, Equals, true)
	c.Assert(dir, Equals, workDir)
	c.Assert(b, Equals, branch)
	c.Assert(h, Equals, hash)

	// Binary contains no git information
	GitTagVersion = ""
	GitBranch = ""
	GitCommitInfo = ""
	GitCommitHash = ""
	GitInitialHash = ""
	mismatch, b, h, dir = checkGitHashMismatch()
	c.Assert(mismatch, Equals, false)
	c.Assert(b, Equals, "")
	c.Assert(h, Equals, "")
	c.Assert(dir, Equals, "")
}

// initTestRepo initializes an in-memory test repository and commits a test
// file to it
func initTestRepo(r git.Repository, fs billy.Filesystem) {
	filePath := "test.txt"
	testFile, err := fs.Create(filePath)
	if err != nil {
		return
	}

	testFile.Write([]byte("Test file"))
	testFile.Close()

	w, err := r.Worktree()
	w.Add(filePath)

	w.Commit("Initial commit", &git.CommitOptions{})
}
