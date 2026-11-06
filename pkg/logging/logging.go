// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package logging

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/fatih/color"
)

const (
	successExitCode = 0
	failureExitCode = 1
)

var (
	captureMu    sync.Mutex
	infolog      *log.Logger
	errorlog     *log.Logger
	fatallog     *log.Logger
	FatalHook    func(exitCode int, err error) // FatalHook allows registering a callback to run before the program exits on a fatal error.
	Exit         = os.Exit
	TsColor      = color.New(color.FgMagenta)
	WarningColor = color.New(color.FgYellow)
)

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// CaptureForTesting redirects Info and Warn/Error/Fatal logger writers to race-safe in-memory buffers until restore is called.
func CaptureForTesting() (snapshot func() (infoOut, errOut string), restore func()) {
	captureMu.Lock()
	prevInfo := infolog.Writer()
	prevErr := errorlog.Writer()
	prevFatal := fatallog.Writer()
	infoBuf := &syncBuffer{}
	errBuf := &syncBuffer{}
	infolog.SetOutput(infoBuf)
	errorlog.SetOutput(errBuf)
	fatallog.SetOutput(errBuf)

	var once sync.Once
	return func() (string, string) {
			return infoBuf.String(), errBuf.String()
		}, func() {
			once.Do(func() {
				infolog.SetOutput(prevInfo)
				errorlog.SetOutput(prevErr)
				fatallog.SetOutput(prevFatal)
				captureMu.Unlock()
			})
		}
}

func init() {
	infolog = log.New(os.Stdout, "", 0)
	errorlog = log.New(os.Stderr, "", 0)
	fatallog = log.New(os.Stderr, "", 0)
}

// formatTs returns a timestamp
func formatTs() string {
	ts := time.Now().UTC().Format(time.RFC3339)
	return TsColor.Sprint(ts)
}

// Info prints info to stdout
func Info(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	infolog.Printf("%s: %s", formatTs(), msg)
}

// Warn prints message to stderr but does not end the program
func Warn(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	errorlog.Printf("%s: %s", formatTs(), WarningColor.Sprint("WARNING: "+msg))
}

// Error prints message to stderr but does not end the program
func Error(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	errorlog.Printf("%s: %s", formatTs(), msg)
}

// Fatal prints message to stderr and ends the program with exit code 1
func Fatal(f string, a ...any) {
	ExitWithCode(failureExitCode, f, a...)
}

// ExitWithCode ends the program with the specified exit code. It prints a message to stdout if exitCode is 0, or stderr otherwise.
func ExitWithCode(exitCode int, f string, a ...any) {
	defer Exit(exitCode)

	msg := fmt.Sprintf(f, a...)

	if exitCode == successExitCode {
		infolog.Printf("%s: %s", formatTs(), msg)
	} else {
		fatallog.Printf("%s: %s", formatTs(), msg)
	}

	// Execute the hook if it is registered
	if FatalHook != nil {
		FatalHook(exitCode, errors.New(msg))
	}
}
