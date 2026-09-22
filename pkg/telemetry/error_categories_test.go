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

// knownShadowed lists patterns that are currently unreachable because an
// earlier, broader entry in the table matches first. Correcting these changes
// the label on telemetry that is already being collected, so they are handled
// separately from this test.
//
// This map must not grow. If a new entry is needed, the pattern being added is
// almost certainly redundant with one already in the table.
var knownShadowed = map[string]string{
	// pattern (as stored, lowercased) -> category actually returned today
	"startup-script timed out after":                                                         "OMNIA_TIMEOUT",
	"net/http: request canceled (client.timeout exceeded while awaiting headers)":            "API_POST_HEADERS_TIMEOUT",
	"not resumed by resumetimeout":                                                           "unknown_STARTUP_TIMEOUT_TPU",
	"does not currently have sufficient capacity for the requested resources":                "Stockout",
	"error 403: permission 'iam.serviceaccounts.get' denied on resource or it may not exist": "IAM_PERMISSION_DENIED",
}

// TestNoOverEscapedPatterns guards against patterns being copied from the CI
// notebook's Python *source* (which contains \" escapes) instead of the
// string's *value*. Such a pattern contains a literal backslash before each
// quote and can never match a real error message.
//
// The two table kinds need different checks. In a substring pattern, `\"` in
// the value is already wrong. In a regex, `\"` is a legitimate (if redundant)
// way to write a quote -- regexp/syntax treats an escaped punctuation
// character as itself -- so only a literal backslash followed by a quote
// indicates the copy-paste bug.
func TestNoOverEscapedPatterns(t *testing.T) {
	for _, m := range extraSubstringErrMatchers {
		if strings.Contains(m.substring, `\"`) {
			t.Errorf("substring pattern is over-escaped and can never match: %q", m.substring)
		}
	}
	for _, m := range extraMultiSubstringErrMatchers {
		for _, s := range m.substrings {
			if strings.Contains(s, `\"`) {
				t.Errorf("multi-substring pattern is over-escaped and can never match: %q", s)
			}
		}
	}
	for _, m := range extraRegexErrMatchers {
		if strings.Contains(m.pattern.String(), `\\"`) {
			t.Errorf("regex requires a literal backslash before a quote, which real logs do not contain: %q", m.pattern)
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
// therefore NOT covered, and knownShadowed below is a lower bound rather than
// a complete list.
func TestEverySubstringPatternIsReachable(t *testing.T) {
	for _, m := range extraSubstringErrMatchers {
		got := getErrorType(errors.New(m.substring))
		if got == m.category {
			continue
		}
		if want, ok := knownShadowed[m.substring]; ok && want == got {
			continue // documented above, tracked separately
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
		if want, ok := knownShadowed[strings.Join(m.substrings, " ")]; ok && want == got {
			continue
		}
		t.Errorf("unreachable multi-pattern %v\n  want category %s\n  got  category %s",
			m.substrings, m.category, got)
	}
}
