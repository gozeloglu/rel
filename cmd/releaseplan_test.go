package cmd

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/tui"
)

func TestClassifyBump(t *testing.T) {
	tests := []struct {
		name    string
		current string
		next    string
		want    bumpKind
	}{
		{"major", "v1.2.0", "2.0.0", bumpMajor},
		{"minor", "v1.2.0", "1.3.0", bumpMinor},
		{"patch", "v1.2.0", "1.2.1", bumpPatch},
		{"no previous tag", "", "1.0.0", bumpFirst},
		{"unparsable previous tag", "release-2020", "1.0.0", bumpFirst},
		{"same version", "v1.2.0", "1.2.0", bumpNotNewer},
		{"downgrade", "v2.4.0", "2.3.0", bumpNotNewer},
		{"major downgrade", "v3.0.0", "1.9.9", bumpNotNewer},
		{"garbage version", "v1.2.0", "not-a-version", bumpInvalid},
		{"empty version", "v1.2.0", "", bumpInvalid},
		{"typo drops a segment", "v1.2.0", "1.60", bumpMinor},
		{"v prefix is accepted", "v1.2.0", "v1.3.0", bumpMinor},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyBump(tt.current, tt.next); got != tt.want {
				t.Errorf("classifyBump(%q, %q) = %v, want %v",
					tt.current, tt.next, got, tt.want)
			}
		})
	}
}

func TestBumpKindWarning(t *testing.T) {
	safe := []bumpKind{bumpMajor, bumpMinor, bumpPatch, bumpFirst}
	for _, k := range safe {
		if k.warning() {
			t.Errorf("bumpKind %v must not be a warning", k)
		}
	}
	for _, k := range []bumpKind{bumpNotNewer, bumpInvalid} {
		if !k.warning() {
			t.Errorf("bumpKind %v must be a warning", k)
		}
	}
}

func TestPlanHasWarning(t *testing.T) {
	clean := []releaseItem{
		{Repo: "a", CurrentTag: "v1.0.0", Version: "1.1.0"},
		{Repo: "b", CurrentTag: "", Version: "1.0.0"},
	}
	if planHasWarning(clean) {
		t.Error("a plan with only sane bumps must not warn")
	}

	dirty := append(clean, releaseItem{Repo: "c", CurrentTag: "v2.0.0", Version: "1.0.0"})
	if !planHasWarning(dirty) {
		t.Error("a downgrade must flip the confirmation default")
	}
}

func TestDisplayTag(t *testing.T) {
	tests := map[string]string{
		"":            "—",
		"   ":         "—",
		"v1.2.0":      "v1.2.0",
		"1.2.0":       "v1.2.0",
		"1.2.0-a":     "v1.2.0-a",
		"oops":        "oops",
		"release-202": "release-202",
	}
	for in, want := range tests {
		if got := displayTag(in); got != want {
			t.Errorf("displayTag(%q) = %q, want %q", in, got, want)
		}
	}
}

func releasePlanProfile() *config.Profile {
	return &config.Profile{
		Owner:      "acme",
		Team:       "payments",
		BaseBranch: "master",
		DevBranch:  "dev",
	}
}

func TestRenderReleasePlan(t *testing.T) {
	items := []releaseItem{
		{Repo: "payment-alpha", CurrentTag: "v1.2.0", Version: "1.3.0"},
		{Repo: "payment-beta", CurrentTag: "v3.0.1", Version: "4.0.0"},
		{Repo: "payment-core", CurrentTag: "v2.4.0", Version: "2.4.1"},
		{Repo: "payment-gamma", CurrentTag: "", Version: "1.0.0"},
		{Repo: "payment-delta", CurrentTag: "v2.4.0", Version: "2.3.0"},
	}

	out := renderReleasePlan(items, []string{"payment-epsilon"}, releasePlanProfile())

	for _, want := range []string{
		"Release plan",
		"acme/payments",
		"→ master",
		"payment-alpha",
		"v1.2.0",
		"v1.3.0",
		"minor",
		"major",
		"patch",
		"first release",
		"not newer than v2.4.0",
		"1 repository skipped",
		"payment-epsilon",
		"release/<version>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("release plan is missing %q:\n%s", want, out)
		}
	}
}

func TestRenderReleasePlanShowsNoTagAsDash(t *testing.T) {
	items := []releaseItem{{Repo: "fresh-service", CurrentTag: "", Version: "1.0.0"}}

	out := renderReleasePlan(items, nil, releasePlanProfile())

	if !strings.Contains(out, "—") {
		t.Errorf("a repository without a tag must show an em dash:\n%s", out)
	}
	if strings.Contains(out, "skipped") {
		t.Errorf("no repository was skipped, the block must be omitted:\n%s", out)
	}
}

func TestRenderReleasePlanAlignsColumns(t *testing.T) {
	items := []releaseItem{
		{Repo: "a-very-long-repository-name", CurrentTag: "v1.2.0", Version: "1.3.0"},
		{Repo: "short", CurrentTag: "v10.20.30", Version: "10.20.31"},
		// The em dash is multi-byte, so byte-based padding would misalign it.
		{Repo: "brand-new", CurrentTag: "", Version: "1.0.0"},
	}

	out := renderReleasePlan(items, nil, releasePlanProfile())

	var arrows []int
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "→") && !strings.Contains(line, "Release plan") {
			// Compare display columns, not byte offsets: the em dash is multi-byte.
			arrows = append(arrows, len([]rune(line[:strings.Index(line, "→")])))
		}
	}
	if len(arrows) != 3 {
		t.Fatalf("expected three version rows, got %d:\n%s", len(arrows), out)
	}
	for _, got := range arrows {
		if got != arrows[0] {
			t.Fatalf("arrows are not aligned (%v):\n%s", arrows, out)
		}
	}
}

func TestPromptVersionsSkipsUnknownTags(t *testing.T) {
	restore := versionPrompt
	versionPrompt = func(repo, def string) (string, error) { return def, nil }
	t.Cleanup(func() { versionPrompt = restore })

	tags := map[string]string{"known": "v1.2.0"}
	entered := map[string]string{}

	items, skipped, err := promptVersions([]string{"known", "unknown"}, tags, entered)
	if err != nil {
		t.Fatalf("promptVersions returned %v", err)
	}

	if len(items) != 1 || items[0].Repo != "known" {
		t.Fatalf("items = %+v, want only the repository with a known tag", items)
	}
	if items[0].Version != "1.3.0" {
		t.Errorf("version = %q, want the minor bump default", items[0].Version)
	}
	if len(skipped) != 1 || skipped[0] != "unknown" {
		t.Errorf("skipped = %v, want [unknown]", skipped)
	}
}

func TestPromptVersionsReusesPreviousAnswers(t *testing.T) {
	restore := versionPrompt
	var offered []string
	versionPrompt = func(repo, def string) (string, error) {
		offered = append(offered, def)
		return def, nil
	}
	t.Cleanup(func() { versionPrompt = restore })

	tags := map[string]string{"svc": "v1.2.0"}
	entered := map[string]string{"svc": "2.0.0"}

	items, _, err := promptVersions([]string{"svc"}, tags, entered)
	if err != nil {
		t.Fatalf("promptVersions returned %v", err)
	}

	if len(offered) != 1 || offered[0] != "2.0.0" {
		t.Errorf("offered defaults = %v, want the previously typed version", offered)
	}
	if items[0].Version != "2.0.0" {
		t.Errorf("version = %q, want 2.0.0", items[0].Version)
	}
}

func TestPromptVersionsRecordsAnswersForTheNextPass(t *testing.T) {
	restore := versionPrompt
	versionPrompt = func(repo, def string) (string, error) { return "9.9.9", nil }
	t.Cleanup(func() { versionPrompt = restore })

	entered := map[string]string{}
	if _, _, err := promptVersions([]string{"svc"}, map[string]string{"svc": "v1.0.0"}, entered); err != nil {
		t.Fatalf("promptVersions returned %v", err)
	}

	if entered["svc"] != "9.9.9" {
		t.Errorf("entered[svc] = %q, want the answer remembered for a second pass", entered["svc"])
	}
}

// stubReviewLoop replaces the two terminal prompts and the confirmation so the
// review loop can be driven end to end.
func stubReviewLoop(t *testing.T, selections [][]string, versions map[string]string, answers []bool) *int {
	t.Helper()

	restoreSelect, restoreVersion, restoreConfirm := selectRepos, versionPrompt, confirmAction
	t.Cleanup(func() {
		selectRepos, versionPrompt, confirmAction = restoreSelect, restoreVersion, restoreConfirm
	})

	pass := 0
	selectRepos = func(repos []string, preset []string) ([]string, error) {
		out := selections[pass]
		pass++
		return out, nil
	}
	versionPrompt = func(repo, def string) (string, error) {
		if v, ok := versions[repo]; ok {
			return v, nil
		}
		return def, nil
	}

	answered := 0
	confirmAction = func(title, description string, defaultValue bool) (bool, error) {
		got := answers[answered]
		answered++
		return got, nil
	}

	return &pass
}

func tagsHandler(t *testing.T, hits map[string]*int32) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	for repo, counter := range hits {
		c := counter
		mux.HandleFunc("/repos/acme/"+repo+"/tags", func(w http.ResponseWriter, r *http.Request) {
			atomic.AddInt32(c, 1)
			_, _ = w.Write([]byte(`[{"name":"v1.2.0"}]`))
		})
	}
	return mux
}

func TestPlanReleaseGoesBackWhenRejected(t *testing.T) {
	alpha, beta := int32(0), int32(0)
	client := newAutoSyncTestClient(t, tagsHandler(t, map[string]*int32{
		"alpha": &alpha,
		"beta":  &beta,
	}))

	// First pass picks alpha and is rejected; the second adds beta and is approved.
	stubReviewLoop(t,
		[][]string{{"alpha"}, {"alpha", "beta"}},
		nil,
		[]bool{false, true},
	)

	items, err := planRelease(context.Background(), client, releasePlanProfile(), []string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("planRelease returned %v", err)
	}

	if len(items) != 2 || items[0].Repo != "alpha" || items[1].Repo != "beta" {
		t.Fatalf("items = %+v, want the second, approved selection", items)
	}
	if alpha != 1 {
		t.Errorf("alpha tag was fetched %d times, want 1 across both passes", alpha)
	}
	if beta != 1 {
		t.Errorf("beta tag was fetched %d times, want 1", beta)
	}
}

func TestPlanReleaseKeepsVersionsBetweenPasses(t *testing.T) {
	alpha := int32(0)
	client := newAutoSyncTestClient(t, tagsHandler(t, map[string]*int32{"alpha": &alpha}))

	restoreSelect, restoreVersion, restoreConfirm := selectRepos, versionPrompt, confirmAction
	t.Cleanup(func() {
		selectRepos, versionPrompt, confirmAction = restoreSelect, restoreVersion, restoreConfirm
	})

	selectRepos = func(repos []string, preset []string) ([]string, error) {
		return []string{"alpha"}, nil
	}

	var offered []string
	versionPrompt = func(repo, def string) (string, error) {
		offered = append(offered, def)
		return "7.0.0", nil
	}

	answers := []bool{false, true}
	confirmAction = func(title, description string, defaultValue bool) (bool, error) {
		got := answers[0]
		answers = answers[1:]
		return got, nil
	}

	if _, err := planRelease(context.Background(), client, releasePlanProfile(), []string{"alpha"}); err != nil {
		t.Fatalf("planRelease returned %v", err)
	}

	if len(offered) != 2 {
		t.Fatalf("version was asked %d times, want 2", len(offered))
	}
	if offered[0] != "1.3.0" {
		t.Errorf("first default = %q, want the minor bump", offered[0])
	}
	if offered[1] != "7.0.0" {
		t.Errorf("second default = %q, want the answer from the first pass", offered[1])
	}
}

func TestPlanReleaseConfirmDefaultFollowsWarnings(t *testing.T) {
	tests := []struct {
		name    string
		version string
		want    bool
	}{
		{"sane bump defaults to yes", "1.3.0", true},
		{"downgrade defaults to no", "1.0.0", false},
		{"invalid version defaults to no", "oops", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := int32(0)
			client := newAutoSyncTestClient(t, tagsHandler(t, map[string]*int32{"alpha": &hits}))

			restoreSelect, restoreVersion, restoreConfirm := selectRepos, versionPrompt, confirmAction
			t.Cleanup(func() {
				selectRepos, versionPrompt, confirmAction = restoreSelect, restoreVersion, restoreConfirm
			})

			selectRepos = func(repos []string, preset []string) ([]string, error) {
				return []string{"alpha"}, nil
			}
			versionPrompt = func(repo, def string) (string, error) { return tt.version, nil }

			var gotDefault bool
			confirmAction = func(title, description string, defaultValue bool) (bool, error) {
				gotDefault = defaultValue
				return true, nil
			}

			if _, err := planRelease(context.Background(), client, releasePlanProfile(), []string{"alpha"}); err != nil {
				t.Fatalf("planRelease returned %v", err)
			}

			if gotDefault != tt.want {
				t.Errorf("confirmation default = %v, want %v", gotDefault, tt.want)
			}
		})
	}
}

func TestPlanReleaseStopsWhenNothingSelected(t *testing.T) {
	client := newAutoSyncTestClient(t, http.NewServeMux())

	restoreSelect := selectRepos
	t.Cleanup(func() { selectRepos = restoreSelect })
	selectRepos = func(repos []string, preset []string) ([]string, error) { return nil, nil }

	items, err := planRelease(context.Background(), client, releasePlanProfile(), []string{"alpha"})
	if err != nil {
		t.Fatalf("planRelease returned %v", err)
	}
	if items != nil {
		t.Errorf("items = %+v, want nothing when the selection is empty", items)
	}
}

func TestPlanReleasePropagatesAbort(t *testing.T) {
	client := newAutoSyncTestClient(t, http.NewServeMux())

	restoreSelect := selectRepos
	t.Cleanup(func() { selectRepos = restoreSelect })
	selectRepos = func(repos []string, preset []string) ([]string, error) {
		return nil, tui.ErrAborted
	}

	if _, err := planRelease(context.Background(), client, releasePlanProfile(), []string{"alpha"}); !errors.Is(err, tui.ErrAborted) {
		t.Errorf("err = %v, want tui.ErrAborted", err)
	}
}
