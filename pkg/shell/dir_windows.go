//go:build windows

/**
 * Copyright 2026 Google LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package shell

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// fixLongPath extends paths >= 248 characters with the \\?\ prefix (or \\?\UNC\
// for network shares) to bypass the legacy Windows MAX_PATH limitation.
func fixLongPath(path string) string {
	if len(path) < 248 || strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\\.\`) {
		return path
	}
	if strings.HasPrefix(path, `\\`) {
		return `\\?\UNC\` + path[2:]
	}
	return `\\?\` + path
}

func isDirWritable(path string) bool {
	randSuffix, err := RandomString(8)
	if err != nil {
		return false
	}
	probePath := filepath.Join(path, ".gcluster_probe_"+randSuffix)
	fullPath, err := filepath.Abs(probePath)
	if err != nil {
		return false
	}
	path16, err := windows.UTF16PtrFromString(fixLongPath(fullPath))
	if err != nil {
		return false
	}
	handle, err := windows.CreateFile(
		path16,
		windows.FILE_GENERIC_WRITE|windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_TEMPORARY|windows.FILE_FLAG_DELETE_ON_CLOSE,
		0,
	)
	if err != nil {
		return false
	}
	_ = windows.CloseHandle(handle)
	return true
}
