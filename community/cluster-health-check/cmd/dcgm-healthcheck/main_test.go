// Copyright 2026 Google LLC
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

package main

import (
	"reflect"
	"testing"
)

func TestGetSeverity(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{SeverityFatal, 4},
		{SeverityFailure, 3},
		{SeverityWarning, 2},
		{SeverityInfo, 1},
		{"unknown", 0},
		{"", 0},
	}

	for _, test := range tests {
		if got := getSeverity(test.input); got != test.expected {
			t.Errorf("getSeverity(%q) = %d, expected %d", test.input, got, test.expected)
		}
	}
}

func TestMergeStatus(t *testing.T) {
	tests := []struct {
		oldStatus string
		newStatus string
		expected  string
	}{
		{SeverityWarning, SeverityFatal, SeverityFatal},
		{SeverityFatal, SeverityWarning, SeverityFatal},
		{SeverityWarning, SeverityFailure, SeverityFailure},
		{SeverityFailure, SeverityWarning, SeverityFailure},
		{SeverityInfo, SeverityWarning, SeverityWarning},
		{SeverityWarning, SeverityInfo, SeverityWarning},
		{"unknown", SeverityInfo, SeverityInfo},
		{SeverityInfo, "unknown", SeverityInfo},
		{"", "", ""},
	}

	for _, test := range tests {
		if got := mergeStatus(test.oldStatus, test.newStatus); got != test.expected {
			t.Errorf("mergeStatus(%q, %q) = %q, expected %q", test.oldStatus, test.newStatus, got, test.expected)
		}
	}
}

func TestMergeMessage(t *testing.T) {
	tests := []struct {
		oldMsg   string
		newMsg   string
		expected string
	}{
		{"", "new message", "new message"},
		{"old message", "", "old message"},
		{"a | b", "b | c", "a | b | c"},
	}

	for _, test := range tests {
		if got := mergeMessage(test.oldMsg, test.newMsg); got != test.expected {
			t.Errorf("mergeMessage(%q, %q) = %q, expected %q", test.oldMsg, test.newMsg, got, test.expected)
		}
	}
}

func TestExtractOverflowMessages(t *testing.T) {
	tests := []struct {
		desc     string
		input    interface{}
		expected []string
	}{
		{
			desc: "nested overflow maps",
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"overflow": []interface{}{"msg1", "msg2"},
				},
				"b": map[string]interface{}{
					"c": map[string]interface{}{
						"overflow": []interface{}{"msg3"},
					},
				},
			},
			expected: []string{"msg1 msg2", "msg3"},
		},
	}

	for _, test := range tests {
		msgs := extractOverflowMessages(test.input)
		if !reflect.DeepEqual(msgs, test.expected) {
			t.Errorf("extractOverflowMessages(%s) = %v, expected %v", test.desc, msgs, test.expected)
		}
	}
}

func TestXidRegex(t *testing.T) {
	tests := []struct {
		input string
		xid   string
	}{
		{"NVRM: Xid (PCI:0000:01:00): 31, pid=1234, name=python", "31"},
		{"NVRM: sxid (PCI:0000:02:00): 10045, info", "10045"},
		{"xid: 13", "13"},
	}

	for _, test := range tests {
		matches := xidRegex.FindStringSubmatch(test.input)
		if len(matches) < 2 {
			t.Errorf("Expected match for %q", test.input)
		} else if matches[1] != test.xid {
			t.Errorf("Expected %q, got %q", test.xid, matches[1])
		}
	}
}

func TestParseFatalXids(t *testing.T) {
	tests := []struct {
		desc     string
		input    []byte
		expected map[int]struct{}
	}{
		{
			desc:  "valid and invalid xids",
			input: []byte(" 31 , 12, 13, invalid, 14 "),
			expected: map[int]struct{}{
				31: {},
				12: {},
				13: {},
				14: {},
			},
		},
	}

	for _, test := range tests {
		if got := parseFatalXids(test.input); !reflect.DeepEqual(got, test.expected) {
			t.Errorf("parseFatalXids(%s) = %v, expected %v", test.desc, got, test.expected)
		}
	}
}
