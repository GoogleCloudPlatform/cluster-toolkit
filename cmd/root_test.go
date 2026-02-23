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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	. "gopkg.in/check.v1"
)

type MySuite struct{}

var _ = Suite(&MySuite{})

var randomGitHash = "a975c295ddeab5b1a5323df92f61c4cc9fc88207"

func Test(t *testing.T) {
	TestingT(t)
}

func TestMain(m *testing.M) {
	wd, _ := os.Getwd()
	code := m.Run()
	os.Chdir(wd)
	os.Exit(code)
}

/* Tests */
// root.go
func (s *MySuite) TestHpcToolkitRepo(c *C) {
	path := c.MkDir()
	repo, initHash, err := initTestRepo(path)
	if err != nil {
		c.Fatal(err)
	}
	GitInitialHash = initHash.String()
	head, _ := repo.Head()

	{ // CWD is repo root
		if err = os.Chdir(path); err != nil {
			c.Fatal(err)
		}
		r, dir, err := hpcToolkitRepo()
		c.Assert(err, IsNil)
		checkPathsEqual(c, dir, path)
		h, _ := r.Head()
		c.Check(h.Hash(), Equals, head.Hash())
	}

	{ // CWD is subdir in repo root
		subDir := filepath.Join(path, "subdir")
		if err = os.MkdirAll(subDir, os.ModePerm); err != nil {
			c.Fatal(err)
		}
		if err = os.Chdir(subDir); err != nil {
			c.Fatal(err)
		}
		r, dir, err := hpcToolkitRepo()
		c.Assert(err, IsNil)
		checkPathsEqual(c, dir, subDir)
		h, _ := r.Head()
		c.Check(h.Hash(), Equals, head.Hash())
	}

	{ // CWD is root of sub repo in repo root
		subRepo := filepath.Join(path, "subrepo")
		if err = os.MkdirAll(subRepo, os.ModePerm); err != nil {
			c.Fatal(err)
		}
		_, _, err = initTestRepo(subRepo)
		if err != nil {
			c.Fatal(err)
		}
		if err = os.Chdir(subRepo); err != nil {
			c.Fatal(err)
		}
		r, dir, err := hpcToolkitRepo()
		c.Assert(err, IsNil)
		checkPathsEqual(c, dir, path)
		h, _ := r.Head()
		c.Check(h.Hash(), Equals, head.Hash())
	}

	{ // CWD is parent of repo root, hope it's not repo itself.
		os.Chdir(filepath.Dir(path))
		_, _, err = hpcToolkitRepo()
		c.Check(err, Equals, git.ErrRepositoryNotExists)
	}
}

func (s *MySuite) TestIsHpcToolkitRepo(c *C) {
	repo, initHash, err := initTestRepo(c.MkDir())
	if err != nil {
		c.Fatal(err)
	}

	// Doesn't match
	GitInitialHash = randomGitHash
	c.Check(isHpcToolkitRepo(*repo), Equals, false)

	// Matches
	GitInitialHash = initHash.String()
	c.Check(isHpcToolkitRepo(*repo), Equals, true)
}

func (s *MySuite) TestCheckGitHashMismatch(c *C) {
	path := c.MkDir()
	repo, init, err := initTestRepo(path)
	if err != nil {
		c.Fatal(err)
	}
	if err = os.Chdir(path); err != nil {
		c.Fatal(err)
	}

	head, _ := repo.Head()
	hash := head.Hash().String()
	branch := head.Name().Short()

	{ // Matches
		GitInitialHash = init.String()
		GitCommitHash = hash
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, false)
		c.Check(b, Equals, "")
		c.Check(h, Equals, "")
		c.Check(dir, Equals, "")
	}

	{ // Baked commit hash doesn't match (present in repo, but not HEAD)
		GitInitialHash = init.String()
		GitCommitHash = init.String()
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, true)
		c.Check(b, Equals, branch)
		c.Check(h, Equals, hash)
		checkPathsEqual(c, dir, path)
	}

	{ // Baked commit hash doesn't match (not present in repo)
		GitInitialHash = init.String()
		GitCommitHash = randomGitHash
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, true)
		c.Check(b, Equals, branch)
		c.Check(h, Equals, hash)
		checkPathsEqual(c, dir, path)
	}

	{ // Not a right repo, initial hash doesn't match
		GitInitialHash = randomGitHash
		GitCommitHash = randomGitHash
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, false)
		c.Check(b, Equals, "")
		c.Check(h, Equals, "")
		c.Check(dir, Equals, "")
	}

	{ // Binary contains no git information
		GitTagVersion = ""
		GitBranch = ""
		GitCommitInfo = ""
		GitCommitHash = ""
		GitInitialHash = ""
		mismatch, b, h, dir := checkGitHashMismatch()
		c.Check(mismatch, Equals, false)
		c.Check(b, Equals, "")
		c.Check(h, Equals, "")
		c.Check(dir, Equals, "")
	}
}

func checkPathsEqual(c *C, a, b string) {
	a, err := filepath.EvalSymlinks(a)
	if err != nil {
		c.Fatal(err)
	}
	b, err = filepath.EvalSymlinks(b)
	if err != nil {
		c.Fatal(err)
	}
	c.Check(a, Equals, b)
}

// Creates a Git repo at `path`, performs multiple commits.
func initTestRepo(path string) (repo *git.Repository, initHash plumbing.Hash, err error) {
	fs := osfs.New(path)
	storer := filesystem.NewStorage(fs, cache.NewObjectLRUDefault())

	repo, err = git.Init(storer, fs)
	if err != nil {
		return
	}
	w, err := repo.Worktree()
	if err != nil {
		return
	}

	commit := func(s string) (hash plumbing.Hash, err error) {
		n := "test_" + s + ".txt"
		f, err := fs.Create(n)
		if err != nil {
			return
		}
		// Set unique content to avoid hash collision
		f.Write([]byte(s + " @ " + path))
		f.Close()
		w.Add(n)
		hash, err = w.Commit(s, &git.CommitOptions{
			Author: &object.Signature{
				Name:  "T T",
				Email: "t@t.io",
				When:  time.Now(),
			},
		})
		return
	}

	initHash, err = commit("Init")
	if err != nil {
		return
	}
	_, err = commit("Last")
	return
}
