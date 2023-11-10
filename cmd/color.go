// Copyright 2023 Google LLC
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

package cmd

import (
	"os"

	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
	"github.com/spf13/pflag"
)

var noColorFlag bool = true

func init() {
	// Safety precaution in case `initColor` wasn't called.
	color.NoColor = true
}

func addColorFlag(flagset *pflag.FlagSet) {
	flagset.BoolVar(&noColorFlag, "no-color", true, "Disable colorized output.")
}

func initColor() {
	colorlessStdout := !isatty.IsTerminal(os.Stdout.Fd()) && !isatty.IsCygwinTerminal(os.Stdout.Fd())
	colorlessStderr := !isatty.IsTerminal(os.Stderr.Fd()) && !isatty.IsCygwinTerminal(os.Stderr.Fd())
	color.NoColor = noColorFlag || os.Getenv("TERM") == "dumb" || colorlessStdout || colorlessStderr
}

var boldRed = color.New(color.FgRed, color.Bold).SprintFunc()
var boldYellow = color.New(color.FgYellow, color.Bold).SprintFunc()
var boldGreen = color.New(color.FgGreen, color.Bold).SprintFunc()
