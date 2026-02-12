/**
 * Copyright 2026 Google LLC
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

package shell

import (
	"bytes"
	"hpc-toolkit/pkg/logging"
	"io"
	"sync"
	"time"
)

type timestampWriter struct {
	writer      io.Writer
	startOfLine bool
	mu          sync.Mutex
}

func newTimestampWriter(writer io.Writer) io.Writer {
	return &timestampWriter{
		writer:      writer,
		startOfLine: true,
	}
}

func (w *timestampWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	var buf bytes.Buffer
	lastIdx := 0

	// Scan for newlines to ensure line-by-line timestamping
	for i, b := range p {
		if b == '\n' {
			w.writeSegment(&buf, p[lastIdx:i+1])
			w.startOfLine = true
			lastIdx = i + 1
		}
	}

	// Handle partial writes (e.g., progress bars)
	if lastIdx < len(p) {
		w.writeSegment(&buf, p[lastIdx:])
		w.startOfLine = false
	}

	// Single atomic write to the underlying writer
	nWritten, err := w.writer.Write(buf.Bytes())
	return nWritten, err
}

func (w *timestampWriter) writeSegment(buf *bytes.Buffer, p []byte) {
	if w.startOfLine {
		ts := time.Now().UTC().Format(time.RFC3339)
		coloredTs := logging.TsColor.Sprint(ts)
		buf.WriteString(coloredTs + " ")
	}
	buf.Write(p)
}
