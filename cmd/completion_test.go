package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gozeloglu/rel/pkg/config"
)

// completionHome points the config package at a throwaway directory. On macOS
// os.UserConfigDir() derives from HOME and ignores XDG_CONFIG_HOME, so both are
// set.
func completionHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	return dir
}

func writeProfiles(t *testing.T) {
	t.Helper()
	cfg := &config.Config{
		CurrentProfile: "acme-payments",
		Profiles: []*config.Profile{
			{
				Name:       "acme-payments",
				Owner:      "acme",
				OwnerType:  config.OwnerOrg,
				Team:       "payments",
				BaseBranch: "master",
				DevBranch:  "dev",
			},
			{
				Name:       "acme-billing",
				Owner:      "acme",
				OwnerType:  config.OwnerOrg,
				BaseBranch: "main",
				DevBranch:  "main",
			},
			{
				Name:       "personal",
				Owner:      "octocat",
				OwnerType:  config.OwnerUser,
				BaseBranch: "main",
				DevBranch:  "develop",
			},
		},
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
}

// runComplete drives cobra's hidden __complete command, which is exactly what
// the shell invokes on TAB.
func runComplete(t *testing.T, args ...string) (candidates []string, directive string) {
	t.Helper()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(append([]string{"__complete"}, args...))
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("__complete %v: %v", args, err)
	}

	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if strings.HasPrefix(line, ":") {
			directive = line
			continue
		}
		if line != "" && !strings.HasPrefix(line, "Completion ended") {
			candidates = append(candidates, line)
		}
	}
	return candidates, directive
}

func values(candidates []string) []string {
	out := make([]string, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, strings.SplitN(c, "\t", 2)[0])
	}
	return out
}

func TestCompleteProfileNamesListsAllProfiles(t *testing.T) {
	completionHome(t)
	writeProfiles(t)

	candidates, directive := runComplete(t, "profile", "use", "")

	got := values(candidates)
	want := []string{"acme-payments", "acme-billing", "personal"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if directive != ":4" {
		t.Fatalf("expected ShellCompDirectiveNoFileComp (:4), got %q", directive)
	}
}

func TestCompleteProfileNamesMarksActiveProfile(t *testing.T) {
	completionHome(t)
	writeProfiles(t)

	candidates, _ := runComplete(t, "profile", "use", "")

	for _, c := range candidates {
		parts := strings.SplitN(c, "\t", 2)
		if len(parts) != 2 || parts[1] == "" {
			t.Fatalf("candidate %q has no description", c)
		}
		active := strings.Contains(parts[1], "(active)")
		if parts[0] == "acme-payments" && !active {
			t.Fatalf("current profile must be marked active: %q", c)
		}
		if parts[0] != "acme-payments" && active {
			t.Fatalf("only the current profile may be marked active: %q", c)
		}
	}
}

func TestCompleteProfileNamesFiltersByPrefix(t *testing.T) {
	completionHome(t)
	writeProfiles(t)

	candidates, _ := runComplete(t, "profile", "delete", "acme-b")

	got := values(candidates)
	if len(got) != 1 || got[0] != "acme-billing" {
		t.Fatalf("expected only acme-billing, got %v", got)
	}
}

func TestCompleteProfileNamesStopsAfterFirstArgument(t *testing.T) {
	completionHome(t)
	writeProfiles(t)

	candidates, directive := runComplete(t, "profile", "use", "personal", "")

	if len(candidates) != 0 {
		t.Fatalf("expected no second argument, got %v", values(candidates))
	}
	if directive != ":4" {
		t.Fatalf("expected :4, got %q", directive)
	}
}

func TestCompleteProfileNamesIsSilentOnBrokenConfig(t *testing.T) {
	completionHome(t)

	p, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	candidates, directive := runComplete(t, "profile", "use", "")

	if len(candidates) != 0 {
		t.Fatalf("a broken config must not produce candidates, got %v", candidates)
	}
	if directive != ":4" {
		t.Fatalf("expected :4, got %q", directive)
	}
}

func TestCompleteProfileFlag(t *testing.T) {
	completionHome(t)
	writeProfiles(t)

	for _, name := range []string{"release", "sync", "merge"} {
		candidates, directive := runComplete(t, name, "--profile", "")

		got := values(candidates)
		if len(got) != 3 {
			t.Fatalf("%s --profile: expected 3 profiles, got %v", name, got)
		}
		if directive != ":4" {
			t.Fatalf("%s --profile: expected :4, got %q", name, directive)
		}
	}
}

func TestArglessCommandsDoNotCompleteFiles(t *testing.T) {
	completionHome(t)
	writeProfiles(t)

	commands := [][]string{
		{"init", ""},
		{"release", ""},
		{"sync", ""},
		{"merge", ""},
		{"profile", "list", ""},
		{"cache", "status", ""},
		{"cache", "clear", ""},
	}

	for _, args := range commands {
		candidates, directive := runComplete(t, args...)

		if len(candidates) != 0 {
			t.Fatalf("%v: expected no candidates, got %v", args, candidates)
		}
		if directive != ":4" {
			t.Fatalf("%v: expected ShellCompDirectiveNoFileComp (:4), got %q", args, directive)
		}
	}
}

func TestCompletionShellFlagCandidates(t *testing.T) {
	completionHome(t)

	candidates, directive := runComplete(t, "completion", "install", "--shell", "")

	got := values(candidates)
	want := []string{"bash", "zsh", "fish", "powershell"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if directive != ":4" {
		t.Fatalf("expected :4, got %q", directive)
	}
}

func TestCompleteMergeMethodCandidates(t *testing.T) {
	completionHome(t)

	candidates, directive := runComplete(t, "merge", "--method", "")

	got := values(candidates)
	want := []string{"squash", "merge", "rebase"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", got, want)
	}
	if directive != ":4" {
		t.Fatalf("expected :4, got %q", directive)
	}
}

func TestCompletionShellArguments(t *testing.T) {
	completionHome(t)

	candidates, _ := runComplete(t, "completion", "")

	got := values(candidates)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell", "install"} {
		found := false
		for _, c := range got {
			if c == shell {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected %q among %v", shell, got)
		}
	}
}

func TestCompletionTargetsStayInsideHome(t *testing.T) {
	home := completionHome(t)
	// Hide Homebrew so the user-owned fallback paths are exercised.
	t.Setenv("PATH", "")

	for _, shell := range []string{"bash", "zsh", "fish"} {
		target, _, err := completionTarget(shell)
		if err != nil {
			t.Fatalf("%s: %v", shell, err)
		}
		if !strings.HasPrefix(target, home+string(os.PathSeparator)) {
			t.Fatalf("%s: target %q escapes the home directory %q", shell, target, home)
		}
	}

	if _, _, err := completionTarget("tcsh"); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}

func TestDetectShell(t *testing.T) {
	cases := map[string]string{
		"/bin/zsh":            "zsh",
		"/usr/local/bin/fish": "fish",
		"/bin/bash":           "bash",
	}
	for value, want := range cases {
		t.Setenv("SHELL", value)
		got, err := detectShell()
		if err != nil {
			t.Fatalf("%s: %v", value, err)
		}
		if got != want {
			t.Fatalf("%s: got %q, want %q", value, got, want)
		}
	}

	t.Setenv("SHELL", "/usr/bin/tcsh")
	if _, err := detectShell(); err == nil {
		t.Fatal("expected an error for an unsupported shell")
	}
}
