package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	relgithub "github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
)

// testPR is a pull request the fake GitHub server should serve.
type testPR struct {
	number int
	head   string
	state  string
	draft  bool
}

func (p testPR) listJSON() string {
	return fmt.Sprintf(`{"number":%d,"html_url":"https://github.com/acme/x/pull/%d","draft":%t,"head":{"ref":%q}}`,
		p.number, p.number, p.draft, p.head)
}

func (p testPR) detailJSON() string {
	return fmt.Sprintf(`{"number":%d,"html_url":"https://github.com/acme/x/pull/%d","draft":%t,`+
		`"mergeable":true,"mergeable_state":%q,"head":{"ref":%q}}`,
		p.number, p.number, p.draft, p.state, p.head)
}

// mergeMux serves what screening needs: the open pull requests of a repository
// and the merge state of each one. Every list call is counted so the tests can
// prove the review loop does not re-fetch.
func mergeMux(t *testing.T, repos map[string][]testPR, hits map[string]*int32) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	for name, prs := range repos {
		prs, counter := prs, hits[name]

		mux.HandleFunc("/repos/acme/"+name+"/pulls", func(w http.ResponseWriter, r *http.Request) {
			if counter != nil {
				atomic.AddInt32(counter, 1)
			}
			items := make([]string, 0, len(prs))
			for _, pr := range prs {
				items = append(items, pr.listJSON())
			}
			_, _ = w.Write([]byte("[" + strings.Join(items, ",") + "]"))
		})

		for _, pr := range prs {
			pr := pr
			mux.HandleFunc(fmt.Sprintf("/repos/acme/%s/pulls/%d", name, pr.number),
				func(w http.ResponseWriter, r *http.Request) {
					_, _ = w.Write([]byte(pr.detailJSON()))
				})
		}
	}
	return mux
}

func TestClassifyMergeState(t *testing.T) {
	tests := []struct {
		name string
		pr   relgithub.ReleasePR
		want mergeStatus
	}{
		{"clean", relgithub.ReleasePR{MergeableState: "clean"}, mergeReady},
		{"optional checks failing", relgithub.ReleasePR{MergeableState: "unstable"}, mergeReady},
		{"required checks or reviews", relgithub.ReleasePR{MergeableState: "blocked"}, mergeBlocked},
		{"base moved ahead", relgithub.ReleasePR{MergeableState: "behind"}, mergeBlocked},
		{"still being computed", relgithub.ReleasePR{MergeableState: "unknown"}, mergeBlocked},
		{"conflicts", relgithub.ReleasePR{MergeableState: "dirty"}, mergeConflict},
		{"draft state", relgithub.ReleasePR{MergeableState: "draft"}, mergeDraft},
		{"draft flag wins", relgithub.ReleasePR{MergeableState: "clean", Draft: true}, mergeDraft},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyMergeState(tt.pr); got != tt.want {
				t.Errorf("classifyMergeState(%+v) = %v, want %v", tt.pr, got, tt.want)
			}
		})
	}
}

func TestScreenForMergeClassifiesRepositories(t *testing.T) {
	repos := map[string][]testPR{
		"ready":   {{number: 91, head: "release/1.3.0", state: "clean"}},
		"blocked": {{number: 77, head: "release/2.5.0", state: "blocked"}},
		"none":    {},
		"unrelated": {
			{number: 5, head: "feature/login", state: "clean"},
		},
		"multiple": {
			{number: 12, head: "release/2.4.1", state: "clean"},
			{number: 13, head: "release/2.5.0", state: "clean"},
		},
	}

	client := newAutoSyncTestClient(t, mergeMux(t, repos, nil))

	tests := []struct {
		repo string
		want mergeStatus
	}{
		{"ready", mergeReady},
		{"blocked", mergeBlocked},
		{"none", mergeNoPR},
		{"unrelated", mergeNoPR},
		{"multiple", mergeMultiple},
	}

	for _, tt := range tests {
		t.Run(tt.repo, func(t *testing.T) {
			got := screenForMerge(context.Background(), client, tt.repo)
			if got.status != tt.want {
				t.Fatalf("status = %v, want %v (err: %v)", got.status, tt.want, got.err)
			}
		})
	}
}

func TestScreenForMergeKeepsEveryPullRequestWhenThereAreSeveral(t *testing.T) {
	client := newAutoSyncTestClient(t, mergeMux(t, map[string][]testPR{
		"svc": {
			{number: 12, head: "release/2.4.1", state: "clean"},
			{number: 13, head: "release/2.5.0", state: "clean"},
		},
	}, nil))

	got := screenForMerge(context.Background(), client, "svc")

	if got.pr.Number != 12 || len(got.others) != 1 || got.others[0].Number != 13 {
		t.Fatalf("result = %+v, want both pull requests kept for the report", got)
	}
	if note := got.note("master"); note != "2 open release PRs" {
		t.Errorf("note = %q, want it to count both", note)
	}
}

func TestScreenForMergeReportsFailures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := screenForMerge(context.Background(), client, "svc")
	if got.status != mergeFailed {
		t.Fatalf("status = %v, want mergeFailed", got.status)
	}
	if got.err == nil {
		t.Error("the failure must carry its error so the plan can show it")
	}
}

func TestApplySelectionMarksUntickedRows(t *testing.T) {
	results := []mergeResult{
		{repo: "alpha", status: mergeReady},
		{repo: "beta", status: mergeReady},
		{repo: "gamma", status: mergeBlocked},
	}

	got := applySelection(results, []string{"alpha"})

	if got[0].status != mergeReady {
		t.Errorf("alpha = %v, want it to stay ready", got[0].status)
	}
	if got[1].status != mergeUnselected {
		t.Errorf("beta = %v, want mergeUnselected", got[1].status)
	}
	if got[2].status != mergeBlocked {
		t.Errorf("gamma = %v, want its own reason to survive", got[2].status)
	}
}

// stubMergeLoop replaces the repository picker and the confirmation so the
// review loop can be driven end to end without a terminal.
func stubMergeLoop(t *testing.T, selections [][]string, answers []bool) {
	t.Helper()

	restoreSelect, restoreConfirm := selectRepos, confirmAction
	t.Cleanup(func() { selectRepos, confirmAction = restoreSelect, restoreConfirm })

	pass := 0
	selectRepos = func(repos []string, preset []string) ([]string, error) {
		out := selections[pass]
		pass++
		return out, nil
	}

	answered := 0
	confirmAction = func(title, description string, defaultValue bool) (bool, error) {
		got := answers[answered]
		answered++
		return got, nil
	}
}

func TestPlanMergeGoesBackWhenRejected(t *testing.T) {
	alpha, beta := int32(0), int32(0)
	client := newAutoSyncTestClient(t, mergeMux(t,
		map[string][]testPR{
			"alpha": {{number: 91, head: "release/1.3.0", state: "clean"}},
			"beta":  {{number: 44, head: "release/4.0.0", state: "clean"}},
		},
		map[string]*int32{"alpha": &alpha, "beta": &beta},
	))

	// First pass picks alpha and is rejected; the second adds beta and is approved.
	stubMergeLoop(t, [][]string{{"alpha"}, {"alpha", "beta"}}, []bool{false, true})

	scan := newMergeScan(client)
	items, err := planMerge(pickedMergePicker(context.Background(), scan, []string{"alpha", "beta"}),
		releasePlanProfile())
	if err != nil {
		t.Fatalf("planMerge returned %v", err)
	}

	if len(items) != 2 || items[0].repo != "alpha" || items[1].repo != "beta" {
		t.Fatalf("items = %+v, want the second, approved selection", items)
	}
	if alpha != 1 {
		t.Errorf("alpha was screened %d times, want 1 across both passes", alpha)
	}
	if beta != 1 {
		t.Errorf("beta was screened %d times, want 1", beta)
	}
}

func TestPlanMergeConfirmDefaultFollowsWarnings(t *testing.T) {
	tests := []struct {
		name  string
		state string
		want  bool
	}{
		{"clean pull request defaults to yes", "clean", true},
		{"failing checks default to no", "unstable", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newAutoSyncTestClient(t, mergeMux(t, map[string][]testPR{
				"alpha": {{number: 91, head: "release/1.3.0", state: tt.state}},
			}, nil))

			restoreSelect, restoreConfirm := selectRepos, confirmAction
			t.Cleanup(func() { selectRepos, confirmAction = restoreSelect, restoreConfirm })

			selectRepos = func(repos []string, preset []string) ([]string, error) {
				return []string{"alpha"}, nil
			}

			var gotDefault bool
			confirmAction = func(title, description string, defaultValue bool) (bool, error) {
				gotDefault = defaultValue
				return true, nil
			}

			scan := newMergeScan(client)
			if _, err := planMerge(pickedMergePicker(context.Background(), scan, []string{"alpha"}),
				releasePlanProfile()); err != nil {
				t.Fatalf("planMerge returned %v", err)
			}

			if gotDefault != tt.want {
				t.Errorf("confirmation default = %v, want %v", gotDefault, tt.want)
			}
		})
	}
}

func TestPlanMergeStopsWhenNothingCanBeMerged(t *testing.T) {
	client := newAutoSyncTestClient(t, mergeMux(t, map[string][]testPR{
		"alpha": {{number: 91, head: "release/1.3.0", state: "dirty"}},
	}, nil))

	restoreSelect, restoreConfirm := selectRepos, confirmAction
	t.Cleanup(func() { selectRepos, confirmAction = restoreSelect, restoreConfirm })

	selectRepos = func(repos []string, preset []string) ([]string, error) { return []string{"alpha"}, nil }
	confirmAction = func(title, description string, defaultValue bool) (bool, error) {
		t.Fatal("a plan with nothing to merge must not ask for confirmation")
		return false, nil
	}

	scan := newMergeScan(client)
	items, err := planMerge(pickedMergePicker(context.Background(), scan, []string{"alpha"}),
		releasePlanProfile())
	if err != nil {
		t.Fatalf("planMerge returned %v", err)
	}
	if items != nil {
		t.Errorf("items = %+v, want nothing", items)
	}
}

func TestPlanMergePropagatesAbort(t *testing.T) {
	client := newAutoSyncTestClient(t, http.NewServeMux())

	restore := selectRepos
	t.Cleanup(func() { selectRepos = restore })
	selectRepos = func(repos []string, preset []string) ([]string, error) { return nil, tui.ErrAborted }

	scan := newMergeScan(client)
	_, err := planMerge(pickedMergePicker(context.Background(), scan, []string{"alpha"}), releasePlanProfile())
	if !errors.Is(err, tui.ErrAborted) {
		t.Errorf("err = %v, want tui.ErrAborted", err)
	}
}

func TestAutoMergePickerOffersEveryReleasePullRequest(t *testing.T) {
	client := newAutoSyncTestClient(t, mergeMux(t, map[string][]testPR{
		"alpha": {{number: 91, head: "release/1.3.0", state: "clean"}},
		"beta":  {{number: 44, head: "release/4.0.0", state: "blocked"}},
		"gamma": {},
	}, nil))

	restore := confirmMergeRepos
	t.Cleanup(func() { confirmMergeRepos = restore })

	var offered []tui.RepoNote
	var gotPreset []string
	confirmMergeRepos = func(title string, items []tui.RepoNote, preset []string) ([]string, error) {
		offered, gotPreset = items, preset
		return preset, nil
	}

	scan := newMergeScan(client)
	pass, err := autoMergePicker(context.Background(), scan, []string{"alpha", "beta", "gamma"})(nil)
	if err != nil {
		t.Fatalf("picker returned %v", err)
	}

	if len(offered) != 1 || offered[0].Repo != "alpha" {
		t.Fatalf("offered = %+v, want only the mergeable pull request", offered)
	}
	if !strings.Contains(offered[0].Note, "#91") || !strings.Contains(offered[0].Note, "release/1.3.0") {
		t.Errorf("note = %q, want the pull request number and branch", offered[0].Note)
	}
	if len(gotPreset) != 1 || gotPreset[0] != "alpha" {
		t.Errorf("preset = %v, want the mergeable pull request ticked", gotPreset)
	}
	if len(pass.results) != 2 {
		t.Errorf("results = %+v, want the blocked repository kept and the empty one dropped", pass.results)
	}
}

func TestAutoMergePickerKeepsThePreviousAnswer(t *testing.T) {
	client := newAutoSyncTestClient(t, mergeMux(t, map[string][]testPR{
		"alpha": {{number: 91, head: "release/1.3.0", state: "clean"}},
		"beta":  {{number: 44, head: "release/4.0.0", state: "clean"}},
	}, nil))

	restore := confirmMergeRepos
	t.Cleanup(func() { confirmMergeRepos = restore })

	var gotPreset []string
	confirmMergeRepos = func(title string, items []tui.RepoNote, preset []string) ([]string, error) {
		gotPreset = preset
		return preset, nil
	}

	scan := newMergeScan(client)
	pick := autoMergePicker(context.Background(), scan, []string{"alpha", "beta"})

	if _, err := pick([]string{"beta"}); err != nil {
		t.Fatalf("picker returned %v", err)
	}
	if len(gotPreset) != 1 || gotPreset[0] != "beta" {
		t.Errorf("preset = %v, want the previous pass to survive going back", gotPreset)
	}
}

func TestAutoMergePickerReportsAnEmptyFleet(t *testing.T) {
	client := newAutoSyncTestClient(t, mergeMux(t, map[string][]testPR{"alpha": {}}, nil))

	restore := confirmMergeRepos
	t.Cleanup(func() { confirmMergeRepos = restore })
	confirmMergeRepos = func(title string, items []tui.RepoNote, preset []string) ([]string, error) {
		t.Fatal("an empty scan must not open the confirmation screen")
		return nil, nil
	}

	scan := newMergeScan(client)
	pass, err := autoMergePicker(context.Background(), scan, []string{"alpha"})(nil)
	if err != nil {
		t.Fatalf("picker returned %v", err)
	}
	if !strings.Contains(pass.reason, "No open release pull requests") {
		t.Errorf("reason = %q, want it to say nothing was found", pass.reason)
	}
}

func TestMergeScanCachesAndCountsFailures(t *testing.T) {
	hits := int32(0)

	mux := mergeMux(t, map[string][]testPR{
		"alpha": {{number: 91, head: "release/1.3.0", state: "clean"}},
	}, map[string]*int32{"alpha": &hits})
	mux.HandleFunc("/repos/acme/broken/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	scan := newMergeScan(newAutoSyncTestClient(t, mux))

	first := scan.screen(context.Background(), []string{"alpha", "broken"})
	second := scan.screen(context.Background(), []string{"alpha", "broken"})

	if len(first) != 2 || len(second) != 2 {
		t.Fatalf("screen returned %d and %d results, want one per repository", len(first), len(second))
	}
	if hits != 1 {
		t.Errorf("alpha was fetched %d times, want the second pass to come from the cache", hits)
	}
	if scan.failures() != 1 {
		t.Errorf("failures = %d, want the broken repository counted", scan.failures())
	}
	if err := mergeScanError(scan); err == nil {
		t.Error("a screening failure must not exit zero")
	}
}

func TestApplyMergesReportsFailures(t *testing.T) {
	mux := http.NewServeMux()
	var gotBody string
	mux.HandleFunc("/repos/acme/alpha/pulls/91/merge", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = w.Write([]byte(`{"merged":true,"sha":"abc123"}`))
	})
	mux.HandleFunc("/repos/acme/beta/pulls/44/merge", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"Squash merges are not allowed on this repository."}`))
	})

	client := newAutoSyncTestClient(t, mux)

	items := []mergeResult{
		{repo: "alpha", pr: relgithub.ReleasePR{Number: 91}},
		{repo: "beta", pr: relgithub.ReleasePR{Number: 44}},
	}

	if got := applyMerges(context.Background(), client, releasePlanProfile(), items, "squash"); got != 1 {
		t.Errorf("error count = %d, want 1", got)
	}
	if !strings.Contains(gotBody, `"merge_method":"squash"`) {
		t.Errorf("request body = %s, want the chosen method", gotBody)
	}
}

// resetMergeFlags restores the package-level flag state between tests.
func resetMergeFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		autoMerge, mergeYes, mergeDryRun, refreshMergeRepos = false, false, false, false
		mergeMethod = "squash"
		for _, name := range []string{"auto", "yes", "dry-run", "method", "refresh"} {
			if f := mergeCmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
}

func TestValidateMergeFlagsRejectsModifiersWithoutAuto(t *testing.T) {
	for _, name := range []string{"yes", "dry-run"} {
		t.Run(name, func(t *testing.T) {
			resetMergeFlags(t)

			autoMerge = false
			if err := mergeCmd.Flags().Set(name, "true"); err != nil {
				t.Fatalf("set %s: %v", name, err)
			}

			err := validateMergeFlags(mergeCmd)
			if err == nil {
				t.Fatal("err = nil, want a rejection")
			}
			if !strings.Contains(err.Error(), "--auto") {
				t.Errorf("err = %q, want it to mention --auto", err)
			}
		})
	}
}

func TestValidateMergeFlagsChecksTheMethod(t *testing.T) {
	tests := map[string]bool{
		"squash": true,
		"merge":  true,
		"rebase": true,
		"":       false,
		"SQUASH": false,
		"fast":   false,
	}

	for method, ok := range tests {
		t.Run(method, func(t *testing.T) {
			resetMergeFlags(t)
			mergeMethod = method

			err := validateMergeFlags(mergeCmd)
			if ok && err != nil {
				t.Fatalf("err = %v, want %q to be accepted", err, method)
			}
			if !ok {
				if err == nil {
					t.Fatalf("err = nil, want %q to be rejected", method)
				}
				if !strings.Contains(err.Error(), "squash, merge, rebase") {
					t.Errorf("err = %q, want it to list the valid methods", err)
				}
			}
		})
	}
}

func TestValidateMergeFlagsAcceptsAutoRun(t *testing.T) {
	resetMergeFlags(t)

	autoMerge = true
	if err := mergeCmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes: %v", err)
	}

	if err := validateMergeFlags(mergeCmd); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}
