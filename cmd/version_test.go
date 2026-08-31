package cmd

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
)

// restoreVersion snapshots the package level build variables so a test can
// override them without leaking into the rest of the suite.
func restoreVersion(t *testing.T) {
	t.Helper()
	v, c, d, b := buildVersion, buildCommit, buildDate, buildBy
	t.Cleanup(func() {
		buildVersion, buildCommit, buildDate, buildBy = v, c, d, b
		applyVersion()
	})
}

func TestVersionFallsBackToBuildInfo(t *testing.T) {
	restoreVersion(t)
	buildVersion = ""

	// Under 'go test' the main module version is "(devel)", which must be
	// treated as "no version" rather than printed verbatim.
	if got := Version(); got != "dev" {
		t.Fatalf("expected the dev fallback, got %q", got)
	}
}

func TestVersionPrefersInjectedValue(t *testing.T) {
	restoreVersion(t)
	SetVersionInfo("1.2.3", "abcdef", "2026-08-31T12:00:00Z", "goreleaser")

	if got := Version(); got != "1.2.3" {
		t.Fatalf("got %q, want 1.2.3", got)
	}
	if rootCmd.Version != "1.2.3" {
		t.Fatalf("rootCmd.Version not refreshed: %q", rootCmd.Version)
	}
}

func TestSetVersionInfoIgnoresEmptyValues(t *testing.T) {
	restoreVersion(t)
	SetVersionInfo("1.2.3", "abcdef", "2026-08-31T12:00:00Z", "goreleaser")
	SetVersionInfo("", "", "", "")

	if got := Version(); got != "1.2.3" {
		t.Fatalf("empty values must not clear the version, got %q", got)
	}
	if commitValue() != "abcdef" {
		t.Fatalf("empty values must not clear the commit, got %q", commitValue())
	}
}

func TestNormalizeVersionDropsPlaceholders(t *testing.T) {
	for _, v := range []string{"", "  ", "(devel)", "unknown", "dev"} {
		if got := normalizeVersion(v); got != "" {
			t.Fatalf("%q should normalize to empty, got %q", v, got)
		}
	}
	if got := normalizeVersion(" v1.0.0 "); got != "v1.0.0" {
		t.Fatalf("got %q, want v1.0.0", got)
	}
}

func TestVersionInfoReportsPlatform(t *testing.T) {
	restoreVersion(t)
	SetVersionInfo("1.2.3", "abcdef", "2026-08-31T12:00:00Z", "goreleaser")

	out := versionInfo()
	for _, want := range []string{
		"rel 1.2.3",
		"abcdef",
		"2026-08-31T12:00:00Z",
		"goreleaser",
		runtime.Version(),
		runtime.GOOS + "/" + runtime.GOARCH,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("version output missing %q:\n%s", want, out)
		}
	}
}

func TestVersionCommandOutput(t *testing.T) {
	restoreVersion(t)
	SetVersionInfo("9.9.9", "cafebabe", "2026-01-01T00:00:00Z", "goreleaser")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"version"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "rel 9.9.9") {
		t.Fatalf("unexpected output:\n%s", buf.String())
	}
}

func TestVersionFlagUsesTheSameReport(t *testing.T) {
	restoreVersion(t)
	SetVersionInfo("9.9.9", "cafebabe", "2026-01-01T00:00:00Z", "goreleaser")

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"--version"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	})

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "rel 9.9.9") || !strings.Contains(got, "platform:") {
		t.Fatalf("--version should print the full report, got:\n%s", got)
	}
}
