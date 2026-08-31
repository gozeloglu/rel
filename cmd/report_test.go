package cmd

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gozeloglu/rel/pkg/config"
	relgithub "github.com/gozeloglu/rel/pkg/github"
)

// stubOpen replaces the browser and prompt hooks, returning a pointer to the
// URLs that were "opened".
func stubOpen(t *testing.T, tty bool, answer bool, answerErr error) *[]string {
	t.Helper()

	var opened []string

	origOpen, origTTY, origConfirm := openURLs, interactive, confirmAction
	origFlag := openPRs

	openURLs = func(urls []string) error {
		opened = append(opened, urls...)
		return nil
	}
	interactive = func() bool { return tty }
	confirmAction = func(string, string, bool) (bool, error) { return answer, answerErr }

	t.Cleanup(func() {
		openURLs, interactive, confirmAction = origOpen, origTTY, origConfirm
		openPRs = origFlag
	})

	return &opened
}

func samplePRs() []createdPR {
	return []createdPR{
		{Repo: "alpha", URL: "https://github.com/acme/alpha/pull/1"},
		{Repo: "beta", URL: "https://github.com/acme/beta/pull/2"},
	}
}

func TestMaybeOpenPRsWithFlagSkipsThePrompt(t *testing.T) {
	opened := stubOpen(t, false, false, nil)
	openPRs = true

	maybeOpenPRs(samplePRs())

	if len(*opened) != 2 {
		t.Fatalf("opened = %v, want both URLs even without a terminal", *opened)
	}
}

func TestMaybeOpenPRsWithoutTerminalStaysSilent(t *testing.T) {
	opened := stubOpen(t, false, true, nil)
	openPRs = false

	maybeOpenPRs(samplePRs())

	if len(*opened) != 0 {
		t.Fatalf("opened = %v, want nothing when there is no terminal", *opened)
	}
}

func TestMaybeOpenPRsPromptsOnTerminal(t *testing.T) {
	opened := stubOpen(t, true, true, nil)
	openPRs = false

	maybeOpenPRs(samplePRs())

	if len(*opened) != 2 {
		t.Fatalf("opened = %v, want both URLs after a yes answer", *opened)
	}
}

func TestMaybeOpenPRsRespectsNoAnswer(t *testing.T) {
	opened := stubOpen(t, true, false, nil)
	openPRs = false

	maybeOpenPRs(samplePRs())

	if len(*opened) != 0 {
		t.Fatalf("opened = %v, want nothing after a no answer", *opened)
	}
}

func TestMaybeOpenPRsIgnoresPromptErrors(t *testing.T) {
	opened := stubOpen(t, true, true, errors.New("aborted"))
	openPRs = false

	maybeOpenPRs(samplePRs())

	if len(*opened) != 0 {
		t.Fatalf("opened = %v, want nothing when the prompt fails", *opened)
	}
}

func TestMaybeOpenPRsWithNothingCreated(t *testing.T) {
	opened := stubOpen(t, true, true, nil)
	openPRs = true

	maybeOpenPRs(nil)

	if len(*opened) != 0 {
		t.Fatalf("opened = %v, want nothing when no PR was created", *opened)
	}
}

func reportProfile() *config.Profile {
	return &config.Profile{
		Name:       "test",
		Owner:      "acme",
		Team:       "payments",
		OwnerType:  config.OwnerOrg,
		BaseBranch: "master",
		DevBranch:  "dev",
	}
}

// freezeClock pins the report's time source so ages are deterministic.
func freezeClock(t *testing.T, now time.Time) {
	t.Helper()
	orig := clock
	clock = func() time.Time { return now }
	t.Cleanup(func() { clock = orig })
}

func TestBuildReportOmitsEmptyGroupsAndOrdersByActionability(t *testing.T) {
	results := []detectResult{
		{repo: "in-sync", status: statusNotAhead},
		{repo: "ready", status: statusCandidate},
		{repo: "stale", status: statusStaleRelease},
	}

	groups := buildReport(results, reportProfile())

	if len(groups) != 3 {
		t.Fatalf("len(groups) = %d, want 3 (empty groups must be dropped)", len(groups))
	}
	want := []detectStatus{statusCandidate, statusStaleRelease, statusNotAhead}
	for i, w := range want {
		if groups[i].status != w {
			t.Errorf("groups[%d].status = %v, want %v", i, groups[i].status, w)
		}
	}
}

func TestSortGroupPutsLongestForgottenFirst(t *testing.T) {
	now := time.Now()
	rows := []detectResult{
		{repo: "recent", sync: relgithub.SyncState{MergeBase: now.Add(-10 * 24 * time.Hour)}},
		{repo: "ancient", sync: relgithub.SyncState{MergeBase: now.Add(-800 * 24 * time.Hour)}},
		{repo: "middle", sync: relgithub.SyncState{MergeBase: now.Add(-100 * 24 * time.Hour)}},
	}

	got := sortGroup(statusStaleRelease, rows)

	for i, want := range []string{"ancient", "middle", "recent"} {
		if got[i].repo != want {
			t.Errorf("got[%d] = %q, want %q", i, got[i].repo, want)
		}
	}
}

func TestSortGroupPutsNewestReleaseFirstForCandidates(t *testing.T) {
	now := time.Now()
	rows := []detectResult{
		{repo: "older", release: relgithub.ReleaseInfo{Tag: "v1", Published: now.Add(-90 * time.Minute)}},
		{repo: "newer", release: relgithub.ReleaseInfo{Tag: "v2", Published: now.Add(-5 * time.Minute)}},
	}

	got := sortGroup(statusCandidate, rows)

	if got[0].repo != "newer" || got[1].repo != "older" {
		t.Errorf("order = [%s %s], want [newer older]", got[0].repo, got[1].repo)
	}
}

func TestRenderReportShowsDetailsAndNames(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	freezeClock(t, now)

	results := []detectResult{
		{
			repo:    "payment-alpha",
			status:  statusCandidate,
			sync:    relgithub.SyncState{AheadBy: 3, MergeBase: now.Add(-4 * time.Hour)},
			release: relgithub.ReleaseInfo{Tag: "v1.2.0", Published: now.Add(-12 * time.Minute)},
		},
		{
			repo:    "payment-beta",
			status:  statusAlreadyOpen,
			sync:    relgithub.SyncState{AheadBy: 1, MergeBase: now.Add(-48 * time.Hour)},
			release: relgithub.ReleaseInfo{Tag: "v3.0.1", Published: now.Add(-40 * time.Hour)},
			prURL:   "https://github.com/acme/payment-beta/pull/7",
		},
		{repo: "payment-gamma", status: statusNotAhead},
		{repo: "payment-delta", status: statusNoBranch},
	}

	out := renderReport(buildReport(results, reportProfile()), reportProfile(), len(results), "2h")

	for _, want := range []string{
		"acme/payments",
		"master → dev",
		"window 2h",
		"READY TO SYNC · 1",
		"payment-alpha",
		"3 commits",
		"diverged 4h ago",
		"v1.2.0 · 12m",
		"SYNC PR ALREADY OPEN · 1",
		"https://github.com/acme/payment-beta/pull/7",
		"ALREADY IN SYNC · 1",
		"NOT APPLICABLE",
		"payment-delta",
		"scanned 4 repositories",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("report is missing %q:\n%s", want, out)
		}
	}

	if strings.Contains(out, "payment-gamma") {
		t.Errorf("in-sync repositories must be summarised as a count only:\n%s", out)
	}

	if strings.Contains(out, "COULD NOT BE CHECKED") {
		t.Errorf("empty groups must not be rendered:\n%s", out)
	}
}

func TestRenderReportMarksNeverReleased(t *testing.T) {
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	freezeClock(t, now)

	results := []detectResult{{
		repo:   "payment-alpha",
		status: statusNoRelease,
		sync:   relgithub.SyncState{AheadBy: 1, MergeBase: now.Add(-72 * time.Hour)},
	}}

	out := renderReport(buildReport(results, reportProfile()), reportProfile(), 1, "2h")

	if !strings.Contains(out, "never released") {
		t.Errorf("report should mark repositories without a release:\n%s", out)
	}
	if !strings.Contains(out, "1 commit ") && !strings.Contains(out, "1 commit\n") {
		t.Errorf("singular commit wording is wrong:\n%s", out)
	}
}

func TestWrapNamesFoldsLongLists(t *testing.T) {
	names := []string{"alpha", "beta", "gamma", "delta", "epsilon"}

	out := wrapNames(names, "  ", 24)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected the list to fold, got:\n%s", out)
	}
	for _, line := range lines {
		if len(line) > 24 {
			t.Errorf("line %q is %d chars, want at most 24", line, len(line))
		}
	}
	joined := strings.Join(lines, " ")
	for _, name := range names {
		if !strings.Contains(joined, name) {
			t.Errorf("wrapped output lost %q:\n%s", name, out)
		}
	}
}

func TestWrapNamesWithNoNames(t *testing.T) {
	if got := wrapNames(nil, "  ", 40); got != "" {
		t.Errorf("wrapNames = %q, want empty string", got)
	}
}
