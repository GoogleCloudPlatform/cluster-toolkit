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

package ai

import (
	"testing"
)

func TestParseFailures(t *testing.T) {
	output := `
Terraform fmt............................................................Failed
- hook id: terraform_fmt
- exit code: 3
- files were modified by this hook

modules/vpc/main.tf

GolangCI Lint............................................................Failed
- hook id: golangci-lint
- exit code: 1

pkg/shell/terraform.go:23:2: ineffectual assignment to err (ineffassign)
pkg/shell/terraform_test.go:10:5: unknown field (typecheck)

Check Yaml...............................................................Passed
`

	expected := []Failure{
		{
			File:    "pkg/shell/terraform.go",
			Line:    0, // regex doesn't parse line number to int yet, but string check
			Message: "[GolangCI Lint] ineffectual assignment to err (ineffassign)",
			Hook:    "GolangCI Lint",
		},
		{
			File:    "pkg/shell/terraform_test.go",
			Line:    0,
			Message: "[GolangCI Lint] unknown field (typecheck)",
			Hook:    "GolangCI Lint",
		},
	}

	failures, _ := ParseFailures(output)

	if len(failures) != len(expected) {
		t.Fatalf("Expected %d failures, got %d", len(expected), len(failures))
	}

	for i, f := range failures {
		if f.File != expected[i].File {
			t.Errorf("Failure %d: Expected File %s, got %s", i, expected[i].File, f.File)
		}
		if f.Message != expected[i].Message {
			t.Errorf("Failure %d: Expected Message %s, got %s", i, expected[i].Message, f.Message)
		}
		if f.Hook != expected[i].Hook {
			t.Errorf("Failure %d: Expected Hook %s, got %s", i, expected[i].Hook, f.Hook)
		}
	}

	outputModified := `
Terraform fmt............................................................Failed
- hook id: terraform_fmt
- exit code: 3
- files were modified by this hook

modules/vpc/main.tf
`
	_, modified := ParseFailures(outputModified)
	if !modified {
		t.Error("Expected modified=true, got false")
	}

	outputSuccess := `
Terraform fmt............................................................Passed
`
	failuresSuccess, modifiedSuccess := ParseFailures(outputSuccess)
	if len(failuresSuccess) != 0 {
		t.Errorf("Expected 0 failures, got %d", len(failuresSuccess))
	}
	if modifiedSuccess {
		t.Error("Expected modified=false, got true")
	}
}
