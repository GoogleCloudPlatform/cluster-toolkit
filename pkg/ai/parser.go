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
	"fmt"
	"regexp"
	"strings"
)

type Failure struct {
	File    string
	Line    int
	Message string
	Hook    string
}

func ParseFailures(output string) ([]Failure, bool) {
	var failures []Failure
	lines := strings.Split(output, "\n")
	modified := false
	commonErrorRegex := regexp.MustCompile(`^([^:\s]+):(\d+):(?:(\d+):)?\s*(.*)$`)

	hookHeaderRegex := regexp.MustCompile(`^(.+?)\.+Failed$`)

	currentHook := ""

	for _, line := range lines {
		if matches := hookHeaderRegex.FindStringSubmatch(line); len(matches) > 1 {
			currentHook = strings.TrimSpace(matches[1])
			continue
		}

		if matches := commonErrorRegex.FindStringSubmatch(line); len(matches) > 1 {
			msg := matches[4]
			if currentHook != "" {
				msg = "[" + currentHook + "] " + msg
			}
			lineNumber := 0
			if len(matches) > 2 {
				_, err := fmt.Sscanf(matches[2], "%d", &lineNumber)
				if err != nil {
					lineNumber = 0
				}
			}
			failures = append(failures, Failure{
				File:    matches[1],
				Line:    lineNumber,
				Message: msg,
				Hook:    currentHook,
			})
		}

		if strings.Contains(line, "files were modified by this hook") {
			modified = true
		}
	}

	return failures, modified
}
