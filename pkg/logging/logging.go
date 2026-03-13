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
	"fmt"
	"log"
	"os"
	"time"

	"github.com/fatih/color"
)

var (
	infolog      *log.Logger
	errorlog     *log.Logger
	fatallog     *log.Logger
	Exit         = os.Exit
	TsColor      = color.New(color.FgMagenta)
	WarningColor = color.New(color.FgYellow)
)

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

// Fatal prints message to stderr and ends the program
func Fatal(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	fatallog.Printf("%s: %s", formatTs(), msg)
	Exit(1)
}
