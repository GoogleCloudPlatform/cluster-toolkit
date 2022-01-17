package backend

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"

	"github.com/otiai10/copy"
)

// Local writes blueprints to a local directory
type Local struct{}

func mkdirWrapper(directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return fmt.Errorf("Failed to create the directory %s: %v", directory, err)
	}

	return nil
}

func getAbsSourcePath(sourcePath string) string {
	if strings.HasPrefix(sourcePath, "/") { // Absolute Path Already
		return sourcePath
	}
	// Otherwise base it off of the CWD
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("backend: %v", err)
	}
	return path.Join(cwd, sourcePath)
}

// CreateDirectory creates the directory
func (b *Local) CreateDirectory(directory string) error {
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		return fmt.Errorf(
			"The directory already exists: %s", directory)
	}

	// Create directory
	return mkdirWrapper(directory)
}

// CopyFromPath copyes the source file to the destination file
func (b *Local) CopyFromPath(src string, dst string) error {
	absPath := getAbsSourcePath(src)
	return copy.Copy(absPath, dst)
}
