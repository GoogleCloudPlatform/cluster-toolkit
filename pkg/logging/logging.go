// Copyright 2023 Google LLC
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
)

var (
	infolog  *log.Logger
	errorlog *log.Logger
	fatallog *log.Logger
	// Exit is a function that exits the program. It is overridden in tests.
	Exit = os.Exit
)

func init() {
	infolog = log.New(os.Stdout, "", 0)
	errorlog = log.New(os.Stderr, "", 0)
	fatallog = log.New(os.Stderr, "", 0)
}

// Info prints info to stdout
func Info(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	infolog.Println(msg)
}

// Error prints info to stderr but does not end the program
func Error(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	errorlog.Println(msg)
}

// Fatal prints info to stderr and ends the program
func Fatal(f string, a ...any) {
	msg := fmt.Sprintf(f, a...)
	fatallog.Println(msg)
	Exit(1)
}
