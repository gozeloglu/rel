package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

// completeProfileNames provides shell completion for profile names. It only
// reads the local config file, so it never needs a token or network access, and
// it stays silent on errors to avoid polluting the shell.
func completeProfileNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Every command taking a profile name accepts exactly one.
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	current, _ := cfg.Current()

	var out []string
	for _, p := range cfg.Profiles {
		if !strings.HasPrefix(strings.ToLower(p.Name), strings.ToLower(toComplete)) {
			continue
		}

		description := p.Summary()
		if current != nil && current.Name == p.Name {
			description = "(active) " + description
		}
		// Tabs separate the value from its description in the protocol.
		out = append(out, p.Name+"\t"+strings.ReplaceAll(description, "\t", " "))
	}

	return out, cobra.ShellCompDirectiveNoFileComp
}

var completionCmd = &cobra.Command{
	Use:   "completion <bash|zsh|fish|powershell>",
	Short: "Generate a shell completion script",
	Long: `Generate a shell completion script for rel.

The quickest way to set this up is:

  rel completion install

which detects your shell and writes the script to the right place. To do it
manually, or to inspect the script first, print it instead:

  rel completion bash > /etc/bash_completion.d/rel
  rel completion zsh  > "${fpath[1]}/_rel"
  rel completion fish > ~/.config/fish/completions/rel.fish
  rel completion powershell | Out-String | Invoke-Expression

Once installed, profile names are completed for 'rel profile use',
'rel profile delete' and the '--profile' flag.`,
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		return generateCompletion(cmd.Root(), args[0], cmd.OutOrStdout())
	},
}

func generateCompletion(root *cobra.Command, shell string, w io.Writer) error {
	switch shell {
	case "bash":
		return root.GenBashCompletionV2(w, true)
	case "zsh":
		return root.GenZshCompletion(w)
	case "fish":
		return root.GenFishCompletion(w, true)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(w)
	default:
		return fmt.Errorf("unsupported shell %q", shell)
	}
}

var (
	installShell string
	installPrint bool
)

var completionInstallCmd = &cobra.Command{
	Use:               "install",
	Short:             "Install the completion script for your shell",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		shell := installShell
		if shell == "" {
			detected, err := detectShell()
			if err != nil {
				return err
			}
			shell = detected
			fmt.Printf("Detected shell: %s\n", shell)
		}

		if shell == "powershell" {
			fmt.Println("PowerShell cannot be installed automatically. Add this line to your $PROFILE:")
			fmt.Println("\n  rel completion powershell | Out-String | Invoke-Expression")
			return nil
		}

		target, followUp, err := completionTarget(shell)
		if err != nil {
			return err
		}

		if installPrint {
			fmt.Println(target)
			return nil
		}

		if _, err := os.Stat(target); err == nil {
			ok, err := tui.Confirm("Overwrite the existing completion script?", target, true)
			if err != nil {
				return abortAware(err, "Nothing was written.")
			}
			if !ok {
				fmt.Println("Nothing was written.")
				return nil
			}
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		var buf bytes.Buffer
		if err := generateCompletion(cmd.Root(), shell, &buf); err != nil {
			return err
		}
		if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil {
			return err
		}

		fmt.Printf("✅ Wrote %s completion to %s\n", shell, target)
		if followUp != "" {
			fmt.Printf("\n%s\n", followUp)
		}
		fmt.Println("\nRestart your shell (or open a new tab) to pick it up.")
		return nil
	},
}

// detectShell reads $SHELL and maps it to a supported shell name.
func detectShell() (string, error) {
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "bash", "zsh", "fish":
		return shell, nil
	default:
		return "", fmt.Errorf("could not detect your shell from $SHELL (%q); pass --shell bash|zsh|fish|powershell",
			os.Getenv("SHELL"))
	}
}

// completionTarget resolves where the script should be written, preferring a
// package manager directory when it already exists, and returns any extra step
// the user still has to perform.
func completionTarget(shell string) (target string, followUp string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}

	switch shell {
	case "zsh":
		if dir := brewDir("share/zsh/site-functions"); dir != "" {
			return filepath.Join(dir, "_rel"), "", nil
		}
		dir := filepath.Join(home, ".zsh", "completions")
		return filepath.Join(dir, "_rel"),
			fmt.Sprintf("Make sure this directory is in your fpath by adding to ~/.zshrc:\n"+
				"\n  fpath=(%s $fpath)\n  autoload -Uz compinit && compinit", dir), nil

	case "bash":
		if dir := brewDir("etc/bash_completion.d"); dir != "" {
			return filepath.Join(dir, "rel"), "", nil
		}
		return filepath.Join(home, ".local", "share", "bash-completion", "completions", "rel"),
			"This requires bash-completion v2. Install it with your package manager if TAB does nothing.", nil

	case "fish":
		return filepath.Join(home, ".config", "fish", "completions", "rel.fish"), "", nil

	default:
		return "", "", fmt.Errorf("unsupported shell %q", shell)
	}
}

// brewDir returns a Homebrew owned directory if Homebrew is installed and the
// directory already exists.
func brewDir(suffix string) string {
	brew, err := exec.LookPath("brew")
	if err != nil {
		return ""
	}

	out, err := exec.Command(brew, "--prefix").Output()
	if err != nil {
		return ""
	}

	dir := filepath.Join(strings.TrimSpace(string(out)), suffix)
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	return dir
}

func init() {
	// Replace cobra's generated command so we can document it and attach
	// 'install' to it.
	rootCmd.CompletionOptions.DisableDefaultCmd = true

	completionInstallCmd.Flags().StringVar(&installShell, "shell", "",
		"Shell to install for (bash, zsh, fish, powershell); defaults to $SHELL")
	completionInstallCmd.Flags().BoolVar(&installPrint, "print", false,
		"Only print the path the script would be written to")
	_ = completionInstallCmd.RegisterFlagCompletionFunc("shell",
		cobra.FixedCompletions([]string{"bash", "zsh", "fish", "powershell"}, cobra.ShellCompDirectiveNoFileComp))

	completionCmd.AddCommand(completionInstallCmd)
	rootCmd.AddCommand(completionCmd)
}
