package resutils

import (
	"io/fs"
	"os"
	"path"
	"strings"
)

// IsLocalPath checks if a source path is a local FS path
func IsLocalPath(source string) bool {
	return strings.HasPrefix(source, "./") ||
		strings.HasPrefix(source, "../") ||
		strings.HasPrefix(source, "/")
}

// IsEmbeddedPath checks if a source path points to an embedded resources
func IsEmbeddedPath(source string) bool {
	return strings.HasPrefix(source, "resources/")
}

// BaseFS is an extension of the io.fs interface with the functionality needed
// in CopyDirFromResources. Works with embed.FS and afero.FS
type BaseFS interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
}

// CopyDirFromResources copies an FS directory to a local path
func CopyDirFromResources(fs BaseFS, source string, dest string) error {
	dirEntries, err := fs.ReadDir(source)
	if err != nil {
		return err
	}
	for _, dirEntry := range dirEntries {
		entryName := dirEntry.Name()
		entrySource := path.Join(source, entryName)
		entryDest := path.Join(dest, entryName)
		if dirEntry.IsDir() {
			if err := os.Mkdir(entryDest, 0755); err != nil {
				return err
			}
			if err = CopyDirFromResources(fs, entrySource, entryDest); err != nil {
				return err
			}
		} else {
			fileBytes, err := fs.ReadFile(entrySource)
			if err != nil {
				return err
			}
			copyFile, err := os.Create(entryDest)
			if err != nil {
				return nil
			}
			if _, err = copyFile.Write(fileBytes); err != nil {
				return nil
			}
		}
	}
	return nil
}
