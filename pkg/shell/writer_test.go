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

package shell

import (
	"bytes"
	"regexp"

	. "gopkg.in/check.v1"
)

func (s *MySuite) TestTimestampWriter(c *C) {
	var buf bytes.Buffer
	w := newTimestampWriter(&buf)

	testCases := []string{
		"line 1\n",
		"line 2 start...",
		"...line 2 end\n",
		"line 3",
	}

	for _, tc := range testCases {
		_, err := w.Write([]byte(tc))
		c.Assert(err, IsNil)
	}

	output := buf.String()

	// matches timestamp format: 2026-02-11T08:27:38Z
	pattern := regexp.MustCompile(`\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z`)
	matches := pattern.FindAllString(output, -1)

	// Since we wrote 3 lines, we expect 3 matches
	c.Assert(len(matches), Equals, 3)
}
