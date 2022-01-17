package backend

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"testing"
	"time"

	. "gopkg.in/check.v1"
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
	dirName := fmt.Sprintf("ghpc_backend_test_%s", t.Format(time.RFC3339))
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

func (s *MySuite) TestGetBackendLocal(c *C) {
	backend := GetBackendLocal()
	c.Assert(backend, Equals, backends["local"])
}

func (s *MySuite) TestCreateDirectoryLocal(c *C) {
	backend := GetBackendLocal()

	// Try to create the exist directory
	err := backend.CreateDirectory(testDir)
	expErr := "The directory already exists: .*"
	c.Assert(err, ErrorMatches, expErr)

	directoryName := "dir_TestCreateDirectoryLocal"
	createdDir := path.Join(testDir, directoryName)
	err = backend.CreateDirectory(createdDir)
	c.Assert(err, IsNil)

	_, err = os.Stat(createdDir)
	c.Assert(err, IsNil)
}

func (s *MySuite) TestGetAbsSourcePath(c *C) {
	// Already abs path
	gotPath := getAbsSourcePath(testDir)
	c.Assert(gotPath, Equals, testDir)

	// Relative path
	relPath := "relative/path"
	cwd, err := os.Getwd()
	c.Assert(err, IsNil)
	gotPath = getAbsSourcePath(relPath)
	c.Assert(gotPath, Equals, path.Join(cwd, relPath))
}

func (s *MySuite) TestCopyFromPathLocal(c *C) {
	backend := GetBackendLocal()
	testSrcFilename := path.Join(testDir, "testSrc")
	str := []byte("TestCopyFromPathLocal")
	if err := os.WriteFile(testSrcFilename, str, 0755); err != nil {
		log.Fatalf("backend_test: failed to create %s: %v", testSrcFilename, err)
	}

	testDstFilename := path.Join(testDir, "testDst")
	backend.CopyFromPath(testSrcFilename, testDstFilename)

	src, err := ioutil.ReadFile(testSrcFilename)
	if err != nil {
		log.Fatalf("backend_test: failed to read %s: %v", testSrcFilename, err)
	}

	dst, err := ioutil.ReadFile(testDstFilename)
	if err != nil {
		log.Fatalf("backend_test: failed to read %s: %v", testDstFilename, err)
	}

	c.Assert(string(src), Equals, string(dst))
}

func (s *MySuite) TestMkdirWrapper(c *C) {
	// Test to create a directory causing a permission denied error
	testViolatedDir := "/TestViolatedDir"
	err := mkdirWrapper(testViolatedDir)
	expErr := "Failed to create the directory .*"
	c.Assert(err, ErrorMatches, expErr)

	// Test to create a directory
	testNewDir := path.Join(testDir, "testNewDir")
	err = mkdirWrapper(testNewDir)
	c.Assert(err, IsNil)
}
