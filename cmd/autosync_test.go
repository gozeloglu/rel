package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v60/github"
	"github.com/gozeloglu/rel/pkg/config"
	relgithub "github.com/gozeloglu/rel/pkg/github"
)

// newAutoSyncTestClient wires a client to a local test server using the same
// profile shape auto-sync runs against.
func newAutoSyncTestClient(t *testing.T, mux *http.ServeMux) *relgithub.Client {
	t.Helper()

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	base, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}

	ghClient := gh.NewClient(nil)
	ghClient.BaseURL = base

	return &relgithub.Client{
		GH: ghClient,
		Profile: &config.Profile{
			Name:       "test",
			Owner:      "acme",
			OwnerType:  config.OwnerOrg,
			BaseBranch: "master",
			DevBranch:  "dev",
		},
	}
}

func compareHandler(aheadBy int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ahead_by":` + strconv.Itoa(aheadBy) + `,"behind_by":0}`))
	}
}

func releasesHandler(tag string, published time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"tag_name":"` + tag + `","draft":false,"published_at":"` +
			published.UTC().Format(time.RFC3339) + `"}]`))
	}
}

func TestClassifyForAutoSyncCandidate(t *testing.T) {
	now := time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", compareHandler(3))
	mux.HandleFunc("/repos/acme/svc/releases", releasesHandler("v1.68.0", now.Add(-30*time.Minute)))
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", now.Add(-2*time.Hour))
	if got.status != statusCandidate {
		t.Fatalf("status = %v, want statusCandidate (err: %v)", got.status, got.err)
	}
	if got.release.Tag != "v1.68.0" {
		t.Errorf("release tag = %q, want v1.68.0", got.release.Tag)
	}
}

func TestClassifyForAutoSyncSkipsRepoAlreadyInSync(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", compareHandler(0))
	mux.HandleFunc("/repos/acme/svc/releases", func(w http.ResponseWriter, r *http.Request) {
		t.Error("releases must not be fetched once the repo is known to be in sync")
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", time.Now().Add(-2*time.Hour))
	if got.status != statusNotAhead {
		t.Fatalf("status = %v, want statusNotAhead", got.status)
	}
}

func TestClassifyForAutoSyncSkipsOldRelease(t *testing.T) {
	now := time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", compareHandler(3))
	mux.HandleFunc("/repos/acme/svc/releases", releasesHandler("v1.60.0", now.Add(-48*time.Hour)))
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", now.Add(-2*time.Hour))
	if got.status != statusStaleRelease {
		t.Fatalf("status = %v, want statusStaleRelease", got.status)
	}
	if got.release.Tag != "v1.60.0" {
		t.Errorf("release tag = %q, want it recorded even when stale", got.release.Tag)
	}
}

func TestClassifyForAutoSyncSkipsNeverReleased(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", compareHandler(3))
	mux.HandleFunc("/repos/acme/svc/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/acme/svc/tags", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", time.Now().Add(-2*time.Hour))
	if got.status != statusNoRelease {
		t.Fatalf("status = %v, want statusNoRelease", got.status)
	}
}

// TestClassifyForAutoSyncPrefersOpenPROverStaleRelease locks the ordering: a
// repository whose sync PR is already waiting must never be reported as
// actionable just because its release is old.
func TestClassifyForAutoSyncPrefersOpenPROverStaleRelease(t *testing.T) {
	now := time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", compareHandler(3))
	mux.HandleFunc("/repos/acme/svc/releases", releasesHandler("v1.60.0", now.Add(-400*time.Hour)))
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"html_url":"https://github.com/acme/svc/pull/3"}]`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", now.Add(-2*time.Hour))
	if got.status != statusAlreadyOpen {
		t.Fatalf("status = %v, want statusAlreadyOpen to win over a stale release", got.status)
	}
	if got.release.Tag != "v1.60.0" {
		t.Errorf("release tag = %q, want it still recorded for the report", got.release.Tag)
	}
}

func TestClassifyForAutoSyncSkipsWhenSyncPRAlreadyOpen(t *testing.T) {
	now := time.Now()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", compareHandler(3))
	mux.HandleFunc("/repos/acme/svc/releases", releasesHandler("v1.68.0", now.Add(-10*time.Minute)))
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"html_url":"https://github.com/acme/svc/pull/12"}]`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", now.Add(-2*time.Hour))
	if got.status != statusAlreadyOpen {
		t.Fatalf("status = %v, want statusAlreadyOpen", got.status)
	}
	if got.prURL != "https://github.com/acme/svc/pull/12" {
		t.Errorf("prURL = %q, want the existing PR URL", got.prURL)
	}
}

func TestClassifyForAutoSyncSkipsRepoWithoutDevBranch(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", time.Now().Add(-2*time.Hour))
	if got.status != statusNoBranch {
		t.Fatalf("status = %v, want statusNoBranch", got.status)
	}
	if got.err != nil {
		t.Errorf("err = %v, want nil so a missing branch is not treated as a failure", got.err)
	}
}

func TestClassifyForAutoSyncRecordsFailures(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"Server Error"}`))
	})

	client := newAutoSyncTestClient(t, mux)

	got := classifyForAutoSync(context.Background(), client, "svc", time.Now().Add(-2*time.Hour))
	if got.status != statusFailed {
		t.Fatalf("status = %v, want statusFailed", got.status)
	}
	if got.err == nil {
		t.Error("err = nil, want the underlying API error")
	}
}

func TestDetectDeployedReposKeepsInputOrder(t *testing.T) {
	now := time.Now()

	mux := http.NewServeMux()
	for _, name := range []string{"alpha", "beta", "gamma"} {
		mux.HandleFunc("/repos/acme/"+name+"/compare/dev...master", compareHandler(0))
	}

	client := newAutoSyncTestClient(t, mux)

	results := detectDeployedRepos(context.Background(), client,
		[]string{"alpha", "beta", "gamma"}, now.Add(-2*time.Hour))

	if len(results) != 3 {
		t.Fatalf("len(results) = %d, want 3", len(results))
	}
	for i, want := range []string{"alpha", "beta", "gamma"} {
		if results[i].repo != want {
			t.Errorf("results[%d].repo = %q, want %q", i, results[i].repo, want)
		}
	}
}

func TestCandidatesFiltersAndSortsByReleaseTime(t *testing.T) {
	now := time.Now()
	results := []detectResult{
		{repo: "old", status: statusCandidate, release: relgithub.ReleaseInfo{Tag: "v1", Published: now.Add(-90 * time.Minute)}},
		{repo: "skipped", status: statusNotAhead},
		{repo: "new", status: statusCandidate, release: relgithub.ReleaseInfo{Tag: "v2", Published: now.Add(-5 * time.Minute)}},
		{repo: "failed", status: statusFailed},
	}

	got := candidates(results)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].repo != "new" || got[1].repo != "old" {
		t.Errorf("order = [%s %s], want [new old]", got[0].repo, got[1].repo)
	}
}

func TestCountByStatus(t *testing.T) {
	results := []detectResult{
		{status: statusCandidate},
		{status: statusCandidate},
		{status: statusNotAhead},
		{status: statusFailed},
	}

	counts := countByStatus(results)
	if counts[statusCandidate] != 2 {
		t.Errorf("candidates = %d, want 2", counts[statusCandidate])
	}
	if counts[statusNotAhead] != 1 {
		t.Errorf("notAhead = %d, want 1", counts[statusNotAhead])
	}
	if counts[statusStaleRelease] != 0 {
		t.Errorf("staleRelease = %d, want 0", counts[statusStaleRelease])
	}
}

func TestErrorsFrom(t *testing.T) {
	if err := errorsFrom([]detectResult{{status: statusCandidate}}); err != nil {
		t.Errorf("err = %v, want nil when nothing failed", err)
	}

	err := errorsFrom([]detectResult{{status: statusFailed}, {status: statusFailed}})
	if err == nil {
		t.Fatal("err = nil, want a failure summary")
	}
	if !strings.Contains(err.Error(), "2 repositories") {
		t.Errorf("err = %q, want it to mention 2 repositories", err)
	}
}

func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{-time.Minute, "just now"},
		{30 * time.Second, "just now"},
		{time.Minute, "1m"},
		{42 * time.Minute, "42m"},
		{time.Hour, "1h"},
		{2 * time.Hour, "2h"},
		{3*time.Hour + 10*time.Minute, "3h10m"},
		{47 * time.Hour, "47h"},
		{48 * time.Hour, "2d"},
		{50 * time.Hour, "2d2h"},
		{278 * 24 * time.Hour, "278d"},
		{365 * 24 * time.Hour, "1y"},
		{400 * 24 * time.Hour, "1y1mo"},
	}

	for _, tc := range cases {
		if got := humanizeDuration(tc.in); got != tc.want {
			t.Errorf("humanizeDuration(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDetectResultNote(t *testing.T) {
	now := time.Now()

	withRelease := detectResult{release: relgithub.ReleaseInfo{Tag: "v1.68.0", Published: now.Add(-15 * time.Minute)}}
	if got, want := withRelease.note(now), "v1.68.0 · 15m ago"; got != want {
		t.Errorf("note = %q, want %q", got, want)
	}

	if got := (detectResult{}).note(now); got != "" {
		t.Errorf("note = %q, want empty when there is no release", got)
	}
}

// resetAutoSyncFlags restores the package-level flag state between tests.
func resetAutoSyncFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		autoSync, autoSyncYes, autoSyncDryRun = false, false, false
		autoSyncSince = 2 * time.Hour
		for _, name := range []string{"since", "yes", "dry-run", "auto"} {
			if f := syncCmd.Flags().Lookup(name); f != nil {
				f.Changed = false
			}
		}
	})
}

func TestValidateAutoSyncFlagsRejectsModifiersWithoutAuto(t *testing.T) {
	resetAutoSyncFlags(t)

	autoSync = false
	if err := syncCmd.Flags().Set("since", "6h"); err != nil {
		t.Fatalf("set since: %v", err)
	}

	err := validateAutoSyncFlags(syncCmd)
	if err == nil {
		t.Fatal("err = nil, want a rejection")
	}
	if !strings.Contains(err.Error(), "--auto") {
		t.Errorf("err = %q, want it to mention --auto", err)
	}
}

func TestValidateAutoSyncFlagsAllowsPlainSync(t *testing.T) {
	resetAutoSyncFlags(t)

	autoSync = false
	if err := validateAutoSyncFlags(syncCmd); err != nil {
		t.Errorf("err = %v, want nil for a plain sync run", err)
	}
}

func TestValidateAutoSyncFlagsRejectsNonPositiveWindow(t *testing.T) {
	resetAutoSyncFlags(t)

	autoSync = true
	autoSyncSince = 0

	if err := validateAutoSyncFlags(syncCmd); err == nil {
		t.Error("err = nil, want a rejection for a zero window")
	}
}

func TestValidateAutoSyncFlagsAcceptsAutoRun(t *testing.T) {
	resetAutoSyncFlags(t)

	autoSync = true
	autoSyncSince = 90 * time.Minute

	if err := validateAutoSyncFlags(syncCmd); err != nil {
		t.Errorf("err = %v, want nil", err)
	}
}
