// Copyright 2026 "Google LLC"
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package telemetry

import (
	"errors"
	"strings"
	"testing"
)

// TestNoOverEscapedPatterns guards against patterns being copied from the CI
// notebook's Python *source* (which contains \" and \n escapes) instead of the
// string's *value*.
func TestNoOverEscapedPatterns(t *testing.T) {
	for _, m := range substringErrMatchers {
		if strings.Contains(m.substring, `\`) {
			t.Errorf("base substring pattern contains a literal backslash: %q", m.substring)
		}
	}
	for _, m := range extraSubstringErrMatchers {
		if strings.Contains(m.substring, `\`) {
			t.Errorf("substring pattern contains a literal backslash and can never match: %q", m.substring)
		}
	}
	for _, m := range extraMultiSubstringErrMatchers {
		for _, s := range m.substrings {
			if strings.Contains(s, `\`) {
				t.Errorf("multi-substring pattern contains a literal backslash and can never match: %q", s)
			}
		}
	}
	for _, m := range extraRegexErrMatchers {
		if strings.Contains(m.pattern.String(), `\"`) {
			t.Errorf("regex pattern has redundant backslash before a quote: %q", m.pattern)
		}
	}
}

// TestEverySubstringPatternIsReachable feeds each pattern its own text and
// asserts the classifier returns that pattern's own category. A failure means
// the pattern is either unmatchable or shadowed by an earlier entry, and the
// label it was meant to produce will never appear in telemetry.
//
// Limitation: init() lowercases the substring tables in place, so the text fed
// in here is lowercase. Regexes are matched against the original-case message
// and nearly all of them contain capitals, so the regex table does not fire
// during this test. Shadowing of a substring pattern by an earlier regex is
// therefore NOT covered.
func TestEverySubstringPatternIsReachable(t *testing.T) {
	for _, m := range extraSubstringErrMatchers {
		got := getErrorType(errors.New(m.substring))
		if got == m.category {
			continue
		}
		t.Errorf("unreachable pattern %q\n  want category %s\n  got  category %s",
			m.substring, m.category, got)
	}
}

// TestEveryMultiSubstringPatternIsReachable does the same for the AND-matcher
// table, joining the required substrings into a single synthetic message.
func TestEveryMultiSubstringPatternIsReachable(t *testing.T) {
	for _, m := range extraMultiSubstringErrMatchers {
		got := getErrorType(errors.New(strings.Join(m.substrings, " ")))
		if got == m.category {
			continue
		}
		t.Errorf("unreachable multi-pattern %v\n  want category %s\n  got  category %s",
			m.substrings, m.category, got)
	}
}

func TestUpdatedRegexAndLocalExecOrdering(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want string
	}{
		{
			name: "unnamed GKE NodePool error",
			msg:  "Error: NodePool was created in the error state",
			want: ErrTypeGkeNodepoolError,
		},
		{
			name: "named GKE NodePool error",
			msg:  "Error: NodePool default-pool was created in the error state",
			want: ErrTypeGkeNodepoolError,
		},
		{
			name: "serial port output in progress inside local-exec provisioner",
			msg:  "Error: local-exec provisioner error\nCould not fetch serial port output: Cannot retrieve serial port output",
			want: ErrTypeSerialPortOutputInProgress,
		},
		{
			name: "kueue webhook unavailable inside local-exec provisioner",
			msg:  "Error: local-exec provisioner error\nInternal error occurred: no endpoints available for service \"kueue-webhook-service\"",
			want: ErrTypeKueueWebhookServiceUnavailable,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := getErrorType(errors.New(tc.msg)); got != tc.want {
				t.Errorf("getErrorType(%q) = %s, want %s", tc.msg, got, tc.want)
			}
		})
	}
}
