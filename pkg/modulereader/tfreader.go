/**
 * Copyright 2021 Google LLC
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
type TFReader struct {
	allModInfo map[string]ModuleInfo
}

// SetInfo sets the module info for a module key'd by the source string
func (r TFReader) SetInfo(source string, modInfo ModuleInfo) {
	r.allModInfo[source] = modInfo
}

// GetInfo reads the ModuleInfo for a terraform module
func (r TFReader) GetInfo(source string) (ModuleInfo, error) {
	if modInfo, ok := r.allModInfo[source]; ok {
		return modInfo, nil
	}
	modInfo, err := getHCLInfo(source)
	if err != nil {
		return modInfo, fmt.Errorf(
			"failed to get info using tfconfig for terraform module at %s: %v",
			source, err)
	}
	r.allModInfo[source] = modInfo
	return modInfo, nil
}
