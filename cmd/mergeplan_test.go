package cmd

import (
	"fmt"
	"strings"
	"testing"

	relgithub "github.com/gozeloglu/rel/pkg/github"
)

func readyResult(repo string, number int, head, state string) mergeResult {
	return mergeResult{
		repo:   repo,
		status: mergeReady,
		pr: relgithub.ReleasePR{
			Number:         number,
			Head:           head,
			URL:            "https://github.com/acme/" + repo + "/pull/1",
			MergeableState: state,
		},
	}
}

func TestRenderMergePlanShowsTheMethodAndEveryRow(t *testing.T) {
	items := []mergeResult{
		readyResult("payment-alpha", 91, "release/1.3.0", "clean"),
		readyResult("payment-core", 12, "release/2.4.1", "unstable"),
	}

	out := renderMergePlan(items, nil, releasePlanProfile(), "squash")

	for _, want := range []string{
		"Merge plan",
		"acme/payments",
		"→ master",
		"method squash",
		"payment-alpha",
		"#91",
		"release/1.3.0",
		"payment-core",
		"⚠ checks failing",
		"merged into master with squash",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan is missing %q:\n%s", want, out)
		}
	}
}

func TestRenderMergePlanGroupsWhatItLeavesOut(t *testing.T) {
	items := []mergeResult{readyResult("payment-alpha", 91, "release/1.3.0", "clean")}

	excluded := []mergeResult{
		{repo: "payment-delta", status: mergeBlocked, pr: relgithub.ReleasePR{
			Number: 77, Head: "release/2.5.0", MergeableState: "blocked",
			URL: "https://github.com/acme/payment-delta/pull/77",
		}},
		{repo: "payment-eps", status: mergeConflict, pr: relgithub.ReleasePR{
			Number: 8, Head: "release/1.1.0", MergeableState: "dirty",
			URL: "https://github.com/acme/payment-eps/pull/8",
		}},
		{repo: "payment-zeta", status: mergeNoPR},
		{repo: "payment-eta", status: mergeUnselected, pr: relgithub.ReleasePR{
			Number: 3, Head: "release/9.0.0", MergeableState: "clean",
		}},
	}

	out := renderMergePlan(items, excluded, releasePlanProfile(), "merge")

	for _, want := range []string{
		"4 repositories excluded",
		"BLOCKED · checks or reviews",
		"checks or reviews pending",
		"https://github.com/acme/payment-delta/pull/77",
		"CONFLICT",
		"conflicts with master",
		"NO OPEN RELEASE PR",
		"NOT SELECTED",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan is missing %q:\n%s", want, out)
		}
	}

	// The exclusions are ordered by how much they ask of the reader.
	blocked := strings.Index(out, "BLOCKED")
	conflict := strings.Index(out, "CONFLICT")
	unselected := strings.Index(out, "NOT SELECTED")
	if !(blocked < conflict && conflict < unselected) {
		t.Errorf("groups are out of order (%d, %d, %d):\n%s", blocked, conflict, unselected, out)
	}
}

func TestRenderMergePlanListsEveryPullRequestOfACrowdedRepository(t *testing.T) {
	excluded := []mergeResult{{
		repo:   "payment-alpha",
		status: mergeMultiple,
		pr:     relgithub.ReleasePR{Number: 12, Head: "release/2.4.1", URL: "https://github.com/acme/a/pull/12"},
		others: []relgithub.ReleasePR{{Number: 13, Head: "release/2.5.0", URL: "https://github.com/acme/a/pull/13"}},
	}}

	out := renderMergePlan(nil, excluded, releasePlanProfile(), "squash")

	for _, want := range []string{
		"nothing to merge",
		"MORE THAN ONE OPEN RELEASE PR",
		"2 open release PRs",
		"https://github.com/acme/a/pull/12",
		"https://github.com/acme/a/pull/13",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("plan is missing %q:\n%s", want, out)
		}
	}
}

func TestRenderMergePlanAlignsTheBranchColumn(t *testing.T) {
	items := []mergeResult{
		readyResult("a-very-long-repository-name", 7, "release/1.0.0", "clean"),
		readyResult("short", 1234, "release/10.20.30", "clean"),
	}

	out := renderMergePlan(items, nil, releasePlanProfile(), "squash")

	var columns []int
	for _, line := range strings.Split(out, "\n") {
		if idx := strings.Index(line, "release/"); idx >= 0 && !strings.Contains(line, "Merge plan") {
			columns = append(columns, len([]rune(line[:idx])))
		}
	}
	if len(columns) != 2 {
		t.Fatalf("expected two rows, got %d:\n%s", len(columns), out)
	}
	if columns[0] != columns[1] {
		t.Errorf("branch column is not aligned (%v):\n%s", columns, out)
	}
}

// The row numbers are how the reader counts the services at a glance, so they
// have to run 1..N and stay aligned when the count reaches two digits.
func TestRenderMergePlanNumbersItsRows(t *testing.T) {
	var items []mergeResult
	for i := 1; i <= 11; i++ {
		items = append(items, readyResult(fmt.Sprintf("payment-%02d", i), i, "release/1.0.0", "clean"))
	}

	out := renderMergePlan(items, nil, releasePlanProfile(), "squash")

	for i, want := range []string{" 1. payment-01", " 9. payment-09", "10. payment-10", "11. payment-11"} {
		if !strings.Contains(out, want) {
			t.Errorf("row %d is missing %q:\n%s", i, want, out)
		}
	}
	if strings.Contains(out, "12. ") {
		t.Errorf("numbering ran past the last row:\n%s", out)
	}
}

func TestRenderMergePlanNumbersEachGroupOfItsOwn(t *testing.T) {
	excluded := []mergeResult{
		{repo: "payment-delta", status: mergeBlocked, pr: relgithub.ReleasePR{
			Number: 77, Head: "release/2.5.0", MergeableState: "blocked",
			URL: "https://github.com/acme/payment-delta/pull/77",
		}},
		{repo: "payment-eps", status: mergeConflict, pr: relgithub.ReleasePR{
			Number: 8, Head: "release/1.1.0", MergeableState: "dirty",
			URL: "https://github.com/acme/payment-eps/pull/8",
		}},
	}

	out := renderMergePlan(nil, excluded, releasePlanProfile(), "squash")

	if !strings.Contains(out, "1. payment-delta") || !strings.Contains(out, "1. payment-eps") {
		t.Errorf("every group must count from one:\n%s", out)
	}

	// The follow-up link hangs under the repository name, not under the number.
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "└") {
			continue
		}
		if idx := strings.Index(line, "└"); idx < len("         1. ") {
			t.Errorf("detail line is not indented past the number: %q", line)
		}
	}
}

func TestMergePlanHasWarning(t *testing.T) {
	clean := []mergeResult{
		readyResult("alpha", 1, "release/1.0.0", "clean"),
		readyResult("beta", 2, "release/2.0.0", "clean"),
	}
	if mergePlanHasWarning(clean) {
		t.Error("a plan of clean pull requests must not warn")
	}

	if !mergePlanHasWarning(append(clean, readyResult("gamma", 3, "release/3.0.0", "unstable"))) {
		t.Error("a pull request with failing checks must flip the confirmation default")
	}
}

func TestMergeResultNote(t *testing.T) {
	tests := []struct {
		name string
		res  mergeResult
		want string
	}{
		{"clean", readyResult("a", 1, "release/1.0.0", "clean"), ""},
		{"unstable", readyResult("a", 1, "release/1.0.0", "unstable"), "⚠ checks failing"},
		{
			"behind",
			mergeResult{status: mergeBlocked, pr: relgithub.ReleasePR{MergeableState: "behind"}},
			"behind master",
		},
		{
			"undecided",
			mergeResult{status: mergeBlocked, pr: relgithub.ReleasePR{MergeableState: "unknown"}},
			"merge state unknown",
		},
		{"draft", mergeResult{status: mergeDraft}, "still a draft"},
		{"nothing open", mergeResult{status: mergeNoPR}, "nothing open"},
		{"left out", mergeResult{status: mergeUnselected}, "left out"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.res.note("master"); got != tt.want {
				t.Errorf("note = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitMergeResultsSortsWhatWillBeMerged(t *testing.T) {
	results := []mergeResult{
		readyResult("zeta", 1, "release/1.0.0", "clean"),
		{repo: "beta", status: mergeBlocked},
		readyResult("alpha", 2, "release/2.0.0", "clean"),
	}

	ready, excluded := splitMergeResults(results)

	if len(ready) != 2 || ready[0].repo != "alpha" || ready[1].repo != "zeta" {
		t.Fatalf("ready = %+v, want the mergeable rows in repository order", ready)
	}
	if len(excluded) != 1 || excluded[0].repo != "beta" {
		t.Fatalf("excluded = %+v, want the blocked row", excluded)
	}
}

func TestPRLabelFallsBackWhenThereIsNoPullRequest(t *testing.T) {
	empty := mergeResult{repo: "svc", status: mergeNoPR}
	if got := empty.prLabel(); got != "—" {
		t.Errorf("prLabel = %q, want an em dash", got)
	}
	if got := empty.headLabel(); got != "—" {
		t.Errorf("headLabel = %q, want an em dash", got)
	}
}
