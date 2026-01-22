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

package modulereader

import "fmt"

// TFReader implements ModReader for terraform modules
type TFReader struct{}

// NewTFReader is a constructor for TFReader
func NewTFReader() TFReader {
	return TFReader{}
}

// GetInfo reads the ModuleInfo for a terraform module
func (r TFReader) GetInfo(source string) (ModuleInfo, error) {
	modInfo, err := getHCLInfo(source)
	if err != nil {
		return modInfo, fmt.Errorf(
			"failed to get info using tfconfig for terraform module at %s: %v",
			source, err)
	}
	return modInfo, nil
}
