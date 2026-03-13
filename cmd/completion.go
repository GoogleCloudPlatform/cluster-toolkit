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

package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(completionCmd)
}

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate completion script",
	Long: fmt.Sprintf(`
Bash:
	# To load the %[1]s completion code into the current shell
	$ source <(%[1]s completion bash)
	
	# To load completions for each session, execute once:
	$ mkdir -p ~/.config/%[1]s
	$ %[1]s completion bash > ~/.config/%[1]s/completion.bash.inc
	$ printf "
		# %[1]s shell completion
		source '$HOME/.config/%[1]s/completion.bash.inc'
		" >> $HOME/.bash_profile
	$ source $HOME/.bash_profile
	
Zsh:
	# To load the %[1]s completion code into the current shell
	$ source <(%[1]s completion zsh)

	# To load completions for each session, execute once:
	$ %[1]s completion zsh > "${fpath[1]}/_%[1]s"
	$ source ~/.zshrc

fish:
	# To load the %[1]s completion code into the current shell
	$ %[1]s completion fish | source

	# To load completions for each session, execute once:
	$ %[1]s completion fish > ~/.config/fish/completions/%[1]s.fish
	$ source ~/.config/fish/completions/%[1]s.fish

PowerShell:
	# To load the %[1]s completion code into the current shell
	PS> %[1]s completion powershell | Out-String | Invoke-Expression

	# To load completions for each session, execute once:
	PS> %[1]s completion powershell >> $PROFILE
	# and source this file from your PowerShell profile.
`, rootCmd.Name()),
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	Run:                   runCompletionCmd,
}

func runCompletionCmd(cmd *cobra.Command, args []string) {
	switch args[0] {
	case "bash":
		cmd.Root().GenBashCompletion(os.Stdout)
	case "zsh":
		cmd.Root().GenZshCompletion(os.Stdout)
	case "fish":
		cmd.Root().GenFishCompletion(os.Stdout, true)
	case "powershell":
		cmd.Root().GenPowerShellCompletionWithDesc(os.Stdout)
	}
}
