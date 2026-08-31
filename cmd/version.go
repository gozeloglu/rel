package cmd

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
)

// Build information. The release pipeline injects these through ldflags; when
// the binary is built with plain 'go build' or 'go install' they stay at their
// zero values and buildInfoVersion() fills in whatever the Go toolchain knows.
var (
	buildVersion = ""
	buildCommit  = ""
	buildDate    = ""
	buildBy      = ""
)

// SetVersionInfo receives the values injected into package main by the release
// pipeline. Empty values are ignored so the fallbacks stay in place.
func SetVersionInfo(version, commit, date, builtBy string) {
	if version != "" {
		buildVersion = version
	}
	if commit != "" {
		buildCommit = commit
	}
	if date != "" {
		buildDate = date
	}
	if builtBy != "" {
		buildBy = builtBy
	}
	applyVersion()
}

// applyVersion refreshes what 'rel --version' prints. The report is rendered
// eagerly because cobra's version template cannot call arbitrary functions.
func applyVersion() {
	rootCmd.Version = Version()
	rootCmd.SetVersionTemplate(versionInfo())
}

// Version returns the release version, falling back to the module version
// recorded by 'go install' and finally to "dev" for local builds.
func Version() string {
	if v := normalizeVersion(buildVersion); v != "" {
		return v
	}
	if v := normalizeVersion(buildInfoVersion()); v != "" {
		return v
	}
	return "dev"
}

// normalizeVersion drops the placeholder values the Go toolchain and common
// build setups use for "no version here".
func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	switch v {
	case "", "(devel)", "unknown", "dev":
		return ""
	}
	return v
}

func buildInfoVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	return info.Main.Version
}

// buildSetting reads a VCS stamp that the Go toolchain embeds automatically
// when building inside a git work tree.
func buildSetting(key string) string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, s := range info.Settings {
		if s.Key == key {
			return s.Value
		}
	}
	return ""
}

func commitValue() string {
	if buildCommit != "" {
		return buildCommit
	}
	if rev := buildSetting("vcs.revision"); rev != "" {
		if buildSetting("vcs.modified") == "true" {
			return rev + " (dirty)"
		}
		return rev
	}
	return "unknown"
}

func dateValue() string {
	if buildDate != "" {
		return buildDate
	}
	if t := buildSetting("vcs.time"); t != "" {
		return t
	}
	return "unknown"
}

func builtByValue() string {
	if buildBy != "" {
		return buildBy
	}
	return "source"
}

// versionInfo renders the multi-line version report.
func versionInfo() string {
	var b strings.Builder
	fmt.Fprintf(&b, "rel %s\n", Version())
	fmt.Fprintf(&b, "commit:   %s\n", commitValue())
	fmt.Fprintf(&b, "built:    %s\n", dateValue())
	fmt.Fprintf(&b, "built by: %s\n", builtByValue())
	fmt.Fprintf(&b, "go:       %s\n", runtime.Version())
	fmt.Fprintf(&b, "platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	return b.String()
}

var versionCmd = &cobra.Command{
	Use:               "version",
	Short:             "Print the version of rel",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Fprint(cmd.OutOrStdout(), versionInfo())
	},
}

func init() {
	applyVersion()
	rootCmd.AddCommand(versionCmd)
}
