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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestIsA4x(t *testing.T) {
	tests := []struct {
		desc     string
		labels   map[string]string
		expected bool
	}{
		{
			desc:     "A4X Accelerator label",
			labels:   map[string]string{"cloud.google.com/gke-accelerator": "nvidia-gb200"},
			expected: true,
		},
		{
			desc:     "A4X Max Accelerator label",
			labels:   map[string]string{"cloud.google.com/gke-accelerator": "nvidia-gb300"},
			expected: true,
		},
		{
			desc:     "A4X Machine Family",
			labels:   map[string]string{"cloud.google.com/machine-family": "a4x"},
			expected: true,
		},
		{
			desc:     "A4X Max Machine Type",
			labels:   map[string]string{"cloud.google.com/machine-family": "a4x", "cloud.google.com/gke-accelerator": "nvidia-gb300"},
			expected: true,
		},
		{
			desc:     "B200 / A4 Machine Family",
			labels:   map[string]string{"cloud.google.com/machine-family": "a4", "cloud.google.com/gke-accelerator": "nvidia-b200"},
			expected: false,
		},
		{
			desc:     "No matching labels",
			labels:   map[string]string{"cloud.google.com/machine-family": "a3-megagpu-8g"},
			expected: false,
		},
		{
			desc:     "Empty Map",
			labels:   map[string]string{},
			expected: false,
		},
		{
			desc:     "Nil Node",
			labels:   nil,
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			var node *corev1.Node
			if tc.desc != "Nil Node" {
				node = &corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Labels: tc.labels,
					},
				}
			}
			if got := isA4x(node); got != tc.expected {
				t.Errorf("isA4x() = %v, expected %v", got, tc.expected)
			}
		})
	}
}

func TestParseNVLinkErrorLine(t *testing.T) {
	tests := []struct {
		line    string
		wantErr bool
	}{
		{"Link 0: FEC Errors - 0: 17941761", false},      // Safe ignore
		{"Link 0: Tx packets: 12345", false},             // Ignore non-error metric
		{"GPU 0: unhandled format", false},               // Unhandled syntax
		{"Link 0: Buffer overrun Errors: 1", true},       // Error condition!
		{"Link 1: Rx Errors: 50", true},                  // Error condition!
		{"Link 2: Link recovery failed events: 1", true}, // Error condition!
		{"Link 3: Effective Errors: 0", false},           // Count is 0, safe
		{"Link 0: Symbol Errors: 2", true},               // Error condition!
		{"Link 0: PLR Xmit Blocks: 1234567890", false},   // Safe ignore
	}

	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			err := parseNVLinkErrorLine("GPU X", tc.line)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseNVLinkErrorLine(%q) error = %v, wantErr %v", tc.line, err, tc.wantErr)
			}
		})
	}
}

func TestCheckBERThreshold(t *testing.T) {
	tests := []struct {
		desc    string
		tag     string
		val     string
		wantErr bool
	}{
		{"Effective BER safely under edge", "Effective BER", "1e-10", false},
		{"Effective BER exactly at edge", "Effective BER", "1e-9", false},
		{"Effective BER slightly over edge", "Effective BER", "1.1e-9", true},
		{"Effective BER significantly over", "Effective BER", "5e-6", true},

		{"Symbol BER safely under edge", "Symbol BER", "1e-26", false},
		{"Symbol BER exactly at edge", "Symbol BER", "1e-25", false},
		{"Symbol BER slightly over edge", "Symbol BER", "2e-25", true},

		{"Invalid float parsing", "Effective BER", "NaN", false}, // Fails parse, should ignore cleanly
		{"Unknown tag bypass", "Unknown BER", "1e-5", false},     // Ignored threshold type
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := checkBERThreshold("GPU X", tc.tag, tc.val, "dummy line")
			if (err != nil) != tc.wantErr {
				t.Errorf("checkBERThreshold(%q, %q) error = %v, wantErr %v", tc.tag, tc.val, err, tc.wantErr)
			}
		})
	}
}

func TestFormatGPUName(t *testing.T) {
	tests := []struct {
		desc     string
		line     string
		expected string
	}{
		{
			desc:     "Valid GPU and UUID",
			line:     "GPU 0: NVIDIA GB200 (UUID: GPU-4fa1a8a8-f788-6a1a-41be-dcb11f40a232)",
			expected: "GPU 0 (UUID: GPU-4fa1a8a8-f788-6a1a-41be-dcb11f40a232)",
		},
		{
			desc:     "Another valid GPU with different wording",
			line:     "GPU 1: NVIDIA H100 (UUID: GPU-12345678)",
			expected: "GPU 1 (UUID: GPU-12345678)",
		},
		{
			desc:     "Invalid line format without UUID",
			line:     "GPU 0: NVIDIA GB200",
			expected: "GPU 0: NVIDIA GB200", // Fallback to TrimSpace
		},
		{
			desc:     "Empty line",
			line:     "   ",
			expected: "", // Fallback to TrimSpace
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			got := formatGPUName(tc.line)
			if got != tc.expected {
				t.Errorf("formatGPUName(%q) = %q, expected %q", tc.line, got, tc.expected)
			}
		})
	}
}

func TestGetConditionStatus(t *testing.T) {
	unhealthyType := corev1.NodeConditionType(ConditionTypeGPUUnhealthy)

	tests := []struct {
		desc               string
		conditions         []corev1.NodeCondition
		wantHasIssue       bool
		wantConditionExist bool
		wantReason         string
	}{
		{
			desc:               "No conditions",
			conditions:         []corev1.NodeCondition{},
			wantHasIssue:       false,
			wantConditionExist: false,
			wantReason:         "",
		},
		{
			desc: "Irrelevant condition",
			conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			wantHasIssue:       false,
			wantConditionExist: false,
			wantReason:         "",
		},
		{
			desc: "GPU unhealthy condition present but false",
			conditions: []corev1.NodeCondition{
				{Type: unhealthyType, Status: corev1.ConditionFalse, Reason: "Resolved"},
			},
			wantHasIssue:       false,
			wantConditionExist: true,
			wantReason:         "Resolved",
		},
		{
			desc: "GPU unhealthy condition present and true",
			conditions: []corev1.NodeCondition{
				{Type: unhealthyType, Status: corev1.ConditionTrue, Reason: string(ReasonActiveTestFailed)},
			},
			wantHasIssue:       true,
			wantConditionExist: true,
			wantReason:         string(ReasonActiveTestFailed),
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			node := &corev1.Node{
				Status: corev1.NodeStatus{
					Conditions: tc.conditions,
				},
			}
			gotHasIssue, gotConditionExist, gotReason := getConditionStatus(node)

			if gotHasIssue != tc.wantHasIssue {
				t.Errorf("hasIssue = %v, want %v", gotHasIssue, tc.wantHasIssue)
			}
			if gotConditionExist != tc.wantConditionExist {
				t.Errorf("conditionExists = %v, want %v", gotConditionExist, tc.wantConditionExist)
			}
			if gotReason != tc.wantReason {
				t.Errorf("currentReason = %v, want %v", gotReason, tc.wantReason)
			}
		})
	}
}

func TestNvlinkStatusRegex(t *testing.T) {
	tests := []struct {
		desc  string
		line  string
		want  string
		match bool
	}{
		{
			desc:  "Exact match",
			line:  "Link 0: 50.0 GB/s",
			want:  "50.0",
			match: true,
		},
		{
			desc:  "Extra trailing whitespace",
			line:  "Link 2: 45.5 GB/s    ",
			want:  "45.5",
			match: true,
		},
		{
			desc:  "Trailing text",
			line:  "Link 3: 50.0 GB/s (active)",
			want:  "50.0",
			match: true,
		},
		{
			desc:  "Leading whitespace and trailing text",
			line:  "    Link 1: 50.0 GB/s with more words",
			want:  "50.0",
			match: true,
		},
		{
			desc:  "No match - incorrect format",
			line:  "Link 0: 50.0 MB/s",
			match: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			matches := nvlinkStatusRegex.FindStringSubmatch(tc.line)
			if tc.match {
				if len(matches) < 2 {
					t.Fatalf("expected match, got none for line: %q", tc.line)
				}
				got := matches[1]
				if got != tc.want {
					t.Errorf("got bandwidth %q, want %q", got, tc.want)
				}
			} else {
				if len(matches) > 0 {
					t.Errorf("expected no match, but got %v", matches)
				}
			}
		})
	}
}
