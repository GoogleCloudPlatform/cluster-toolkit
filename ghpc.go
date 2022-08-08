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
package main

import (
	"embed"
	"hpc-toolkit/cmd"
	"hpc-toolkit/pkg/sourcereader"
	"os"
)

//go:embed modules community/modules
var moduleFS embed.FS

func main() {
	sourcereader.ModuleFS = moduleFS
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
