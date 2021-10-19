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

package resreader

import "fmt"

// TFReader implements ResReader for terraform resources
type TFReader struct {
	allResInfo map[string]ResourceInfo
}

// SetInfo sets the resource info for a resource key'd by the source string
func (r TFReader) SetInfo(source string, resInfo ResourceInfo) {
	r.allResInfo[source] = resInfo
}

// GetInfo reads the ResourceInfo for a terraform module
func (r TFReader) GetInfo(source string) (ResourceInfo, error) {
	if resInfo, ok := r.allResInfo[source]; ok {
		return resInfo, nil
	}
	resInfo, err := getHCLInfo(source)
	if err != nil {
		return resInfo, fmt.Errorf(
			"failed to get info using tfconfig for terraform resource at %s: %v",
			source, err)
	}
	r.allResInfo[source] = resInfo
	return resInfo, nil
}
