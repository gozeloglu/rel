package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	gh "github.com/google/go-github/v60/github"
	"github.com/gozeloglu/rel/pkg/config"
)

func testProfile() *config.Profile {
	return &config.Profile{
		Name:       "test",
		Owner:      "acme",
		OwnerType:  config.OwnerOrg,
		BaseBranch: "master",
		DevBranch:  "dev",
	}
}

// newTestClient wires a Client to a local test server so the API layer can be
// exercised without touching GitHub.
func newTestClient(t *testing.T, mux *http.ServeMux) *Client {
	t.Helper()

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	base, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}

	ghClient := gh.NewClient(nil)
	ghClient.BaseURL = base

	return &Client{GH: ghClient, Profile: testProfile()}
}

func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("parse time %q: %v", value, err)
	}
	return parsed
}

func TestLatestReleaseUsesNewestPublishedRelease(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v1.68.0","draft":false,"published_at":"2026-08-17T21:58:38Z"},
			{"tag_name":"v1.67.1","draft":false,"published_at":"2026-07-01T07:06:54Z"}
		]`))
	})

	client := newTestClient(t, mux)

	got, err := client.LatestRelease(context.Background(), "svc")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got.Tag != "v1.68.0" {
		t.Errorf("tag = %q, want v1.68.0", got.Tag)
	}
	if want := mustTime(t, "2026-08-17T21:58:38Z"); !got.Published.Equal(want) {
		t.Errorf("published = %v, want %v", got.Published, want)
	}
	if !got.Found() {
		t.Error("Found() = false, want true")
	}
}

func TestLatestReleaseSkipsDrafts(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name":"v2.0.0","draft":true,"published_at":"2026-08-20T10:00:00Z"},
			{"tag_name":"v1.68.0","draft":false,"published_at":"2026-08-17T21:58:38Z"}
		]`))
	})

	client := newTestClient(t, mux)

	got, err := client.LatestRelease(context.Background(), "svc")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got.Tag != "v1.68.0" {
		t.Errorf("tag = %q, want v1.68.0 (draft must be ignored)", got.Tag)
	}
}

func TestLatestReleaseFallsBackToNewestTag(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/acme/svc/tags", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"name":"v1.5.0","commit":{"sha":"abc123"}}]`))
	})
	mux.HandleFunc("/repos/acme/svc/git/commits/abc123", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"sha":"abc123","committer":{"date":"2026-08-18T09:30:00Z"}}`))
	})

	client := newTestClient(t, mux)

	got, err := client.LatestRelease(context.Background(), "svc")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got.Tag != "v1.5.0" {
		t.Errorf("tag = %q, want v1.5.0", got.Tag)
	}
	if want := mustTime(t, "2026-08-18T09:30:00Z"); !got.Published.Equal(want) {
		t.Errorf("published = %v, want %v", got.Published, want)
	}
}

func TestLatestReleaseWithoutAnySignal(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/releases", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	mux.HandleFunc("/repos/acme/svc/tags", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	client := newTestClient(t, mux)

	got, err := client.LatestRelease(context.Background(), "svc")
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}
	if got.Found() {
		t.Errorf("Found() = true, want false for %+v", got)
	}
}

func TestFindOpenSyncPRQueriesBaseToDev(t *testing.T) {
	var gotHead, gotBase, gotState string

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		gotHead = r.URL.Query().Get("head")
		gotBase = r.URL.Query().Get("base")
		gotState = r.URL.Query().Get("state")
		_, _ = w.Write([]byte(`[{"html_url":"https://github.com/acme/svc/pull/7"}]`))
	})

	client := newTestClient(t, mux)

	url, err := client.FindOpenSyncPR(context.Background(), "svc")
	if err != nil {
		t.Fatalf("FindOpenSyncPR: %v", err)
	}
	if url != "https://github.com/acme/svc/pull/7" {
		t.Errorf("url = %q, want the open PR URL", url)
	}
	if gotHead != "acme:master" {
		t.Errorf("head = %q, want acme:master", gotHead)
	}
	if gotBase != "dev" {
		t.Errorf("base = %q, want dev", gotBase)
	}
	if gotState != "open" {
		t.Errorf("state = %q, want open", gotState)
	}
}

func TestFindOpenSyncPRReturnsEmptyWhenNoneOpen(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})

	client := newTestClient(t, mux)

	url, err := client.FindOpenSyncPR(context.Background(), "svc")
	if err != nil {
		t.Fatalf("FindOpenSyncPR: %v", err)
	}
	if url != "" {
		t.Errorf("url = %q, want empty string", url)
	}
}

func TestFindOpenSyncPRSkipsSingleBranchProfiles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		t.Error("single-branch profiles must not hit the API")
	})

	client := newTestClient(t, mux)
	client.Profile.DevBranch = client.Profile.BaseBranch

	url, err := client.FindOpenSyncPR(context.Background(), "svc")
	if err != nil {
		t.Fatalf("FindOpenSyncPR: %v", err)
	}
	if url != "" {
		t.Errorf("url = %q, want empty string", url)
	}
}

func TestCreateSyncPRReportsExistingPullRequest(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"message":"Validation Failed",
			"errors":[{"resource":"PullRequest","message":"A pull request already exists for acme:master."}]
		}`))
	})

	client := newTestClient(t, mux)

	_, err := client.CreateSyncPR(context.Background(), "svc")
	if !errors.Is(err, ErrSyncPRExists) {
		t.Fatalf("err = %v, want ErrSyncPRExists", err)
	}
}

func TestCreateSyncPRPropagatesOtherErrors(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Resource not accessible by integration"}`))
	})

	client := newTestClient(t, mux)

	_, err := client.CreateSyncPR(context.Background(), "svc")
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, ErrSyncPRExists) {
		t.Fatalf("err = %v, must not be classified as an existing PR", err)
	}
}

func TestCreateSyncPRReturnsURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"html_url":"https://github.com/acme/svc/pull/9"}`))
	})

	client := newTestClient(t, mux)

	url, err := client.CreateSyncPR(context.Background(), "svc")
	if err != nil {
		t.Fatalf("CreateSyncPR: %v", err)
	}
	if url != "https://github.com/acme/svc/pull/9" {
		t.Errorf("url = %q, want the created PR URL", url)
	}
}

func TestIsAlreadyExistsErrIgnoresPlainErrors(t *testing.T) {
	if isAlreadyExistsErr(errors.New("a pull request already exists")) {
		t.Error("plain errors must not be classified as GitHub validation failures")
	}
}

func TestCompareSyncStateReadsDivergenceDetails(t *testing.T) {
	var gotPath string

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/dev...master", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{
			"ahead_by": 4,
			"behind_by": 67,
			"merge_base_commit": {"commit": {"committer": {"date": "2026-03-03T21:26:45Z"}}}
		}`))
	})

	client := newTestClient(t, mux)

	got, err := client.CompareSyncState(context.Background(), "svc")
	if err != nil {
		t.Fatalf("CompareSyncState: %v", err)
	}
	if got.AheadBy != 4 {
		t.Errorf("AheadBy = %d, want 4", got.AheadBy)
	}
	if got.BehindBy != 67 {
		t.Errorf("BehindBy = %d, want 67", got.BehindBy)
	}
	if want := mustTime(t, "2026-03-03T21:26:45Z"); !got.MergeBase.Equal(want) {
		t.Errorf("MergeBase = %v, want %v", got.MergeBase, want)
	}
	if !strings.HasSuffix(gotPath, "/compare/dev...master") {
		t.Errorf("path = %q, want the dev...master comparison direction", gotPath)
	}
	if !got.NeedsSync() {
		t.Error("NeedsSync() = false, want true when ahead_by > 0")
	}
}

// TestCheckSyncStatusBehaviourIsUnchanged locks the contract the release and
// sync flows rely on: true only when the base branch is ahead of dev.
func TestCheckSyncStatusBehaviourIsUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		aheadBy int
		want    bool
	}{
		{"base ahead", 3, true},
		{"identical branches", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/acme/svc/compare/dev...master", func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{"ahead_by":` + strconv.Itoa(tc.aheadBy) + `,"behind_by":9}`))
			})

			client := newTestClient(t, mux)

			got, err := client.CheckSyncStatus(context.Background(), "svc")
			if err != nil {
				t.Fatalf("CheckSyncStatus: %v", err)
			}
			if got != tc.want {
				t.Errorf("CheckSyncStatus = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCheckSyncStatusSkipsSingleBranchProfiles(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/compare/", func(w http.ResponseWriter, r *http.Request) {
		t.Error("single-branch profiles must not hit the compare API")
	})

	client := newTestClient(t, mux)
	client.Profile.DevBranch = client.Profile.BaseBranch

	got, err := client.CheckSyncStatus(context.Background(), "svc")
	if err != nil {
		t.Fatalf("CheckSyncStatus: %v", err)
	}
	if got {
		t.Error("CheckSyncStatus = true, want false for a single-branch profile")
	}
}

func TestFindOpenReleasePRsKeepsOnlyReleaseBranches(t *testing.T) {
	var gotQuery url.Values

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		_, _ = w.Write([]byte(`[
			{"number":91,"html_url":"https://github.com/acme/svc/pull/91","head":{"ref":"release/1.3.0"}},
			{"number":90,"html_url":"https://github.com/acme/svc/pull/90","head":{"ref":"feature/login"}},
			{"number":89,"html_url":"https://github.com/acme/svc/pull/89","head":{"ref":"releases/old"}},
			{"number":88,"html_url":"https://github.com/acme/svc/pull/88","head":{"ref":"release/1.4.0"},"draft":true}
		]`))
	})

	client := newTestClient(t, mux)

	prs, err := client.FindOpenReleasePRs(context.Background(), "svc")
	if err != nil {
		t.Fatalf("FindOpenReleasePRs: %v", err)
	}

	if len(prs) != 2 {
		t.Fatalf("got %d pull requests, want the two release branches: %+v", len(prs), prs)
	}
	if prs[0].Number != 91 || prs[0].Head != "release/1.3.0" {
		t.Errorf("first pull request = %+v, want #91 release/1.3.0", prs[0])
	}
	if !prs[1].Draft {
		t.Error("draft flag was dropped")
	}
	if got := gotQuery.Get("state"); got != "open" {
		t.Errorf("state = %q, want open", got)
	}
	if got := gotQuery.Get("base"); got != "master" {
		t.Errorf("base = %q, want the profile base branch", got)
	}
}

func TestFindOpenReleasePRsFollowsPagination(t *testing.T) {
	var server *httptest.Server

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			_, _ = w.Write([]byte(`[{"number":2,"head":{"ref":"release/2.0.0"}}]`))
			return
		}
		w.Header().Set("Link", `<`+server.URL+`/repos/acme/svc/pulls?page=2>; rel="next"`)
		_, _ = w.Write([]byte(`[{"number":1,"head":{"ref":"release/1.0.0"}}]`))
	})

	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)

	base, err := url.Parse(server.URL + "/")
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	ghClient := gh.NewClient(nil)
	ghClient.BaseURL = base
	client := &Client{GH: ghClient, Profile: testProfile()}

	prs, err := client.FindOpenReleasePRs(context.Background(), "svc")
	if err != nil {
		t.Fatalf("FindOpenReleasePRs: %v", err)
	}
	if len(prs) != 2 {
		t.Fatalf("got %d pull requests, want both pages: %+v", len(prs), prs)
	}
}

func TestPullRequestMergeabilityRetriesWhileUndecided(t *testing.T) {
	calls := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls/91", func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(`{"number":91,"mergeable_state":"unknown"}`))
			return
		}
		_, _ = w.Write([]byte(`{"number":91,"mergeable":true,"mergeable_state":"clean","head":{"ref":"release/1.3.0"}}`))
	})

	client := newTestClient(t, mux)

	restore := mergeabilityRetryDelay
	mergeabilityRetryDelay = time.Millisecond
	t.Cleanup(func() { mergeabilityRetryDelay = restore })

	pr, err := client.PullRequestMergeability(context.Background(), "svc", 91)
	if err != nil {
		t.Fatalf("PullRequestMergeability: %v", err)
	}
	if calls != 2 {
		t.Errorf("asked GitHub %d times, want a single retry after 'unknown'", calls)
	}
	if pr.MergeableState != "clean" || pr.Head != "release/1.3.0" {
		t.Errorf("pr = %+v, want the second, decided answer", pr)
	}
}

func TestPullRequestMergeabilityStopsAfterOneRetry(t *testing.T) {
	calls := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls/91", func(w http.ResponseWriter, r *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"number":91,"mergeable_state":"unknown"}`))
	})

	client := newTestClient(t, mux)

	restore := mergeabilityRetryDelay
	mergeabilityRetryDelay = time.Millisecond
	t.Cleanup(func() { mergeabilityRetryDelay = restore })

	pr, err := client.PullRequestMergeability(context.Background(), "svc", 91)
	if err != nil {
		t.Fatalf("PullRequestMergeability: %v", err)
	}
	if calls != 2 {
		t.Errorf("asked GitHub %d times, want it to give up after one retry", calls)
	}
	if pr.MergeableState != "unknown" {
		t.Errorf("state = %q, want the undecided answer to be reported as is", pr.MergeableState)
	}
}

func TestMergePRSendsTheRequestedMethod(t *testing.T) {
	var gotBody, gotMethod string

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls/91/merge", func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = w.Write([]byte(`{"merged":true,"sha":"abc123"}`))
	})

	client := newTestClient(t, mux)

	sha, err := client.MergePR(context.Background(), "svc", 91, "squash")
	if err != nil {
		t.Fatalf("MergePR: %v", err)
	}
	if sha != "abc123" {
		t.Errorf("sha = %q, want abc123", sha)
	}
	if gotMethod != http.MethodPut {
		t.Errorf("HTTP method = %q, want PUT", gotMethod)
	}
	if !strings.Contains(gotBody, `"merge_method":"squash"`) {
		t.Errorf("body = %s, want the squash method", gotBody)
	}
}

func TestMergePRClassifiesRefusals(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{
			name:   "merge method disabled",
			status: http.StatusMethodNotAllowed,
			body:   `{"message":"Squash merges are not allowed on this repository."}`,
			want:   ErrMergeNotAllowed,
		},
		{
			name:   "head moved",
			status: http.StatusConflict,
			body:   `{"message":"Head branch was modified. Review and try the merge again."}`,
			want:   ErrHeadChanged,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/repos/acme/svc/pulls/91/merge", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			client := newTestClient(t, mux)

			_, err := client.MergePR(context.Background(), "svc", 91, "squash")
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestMergePRReportsARefusedButSuccessfulResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/acme/svc/pulls/91/merge", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"merged":false,"message":"Pull Request is not mergeable"}`))
	})

	client := newTestClient(t, mux)

	if _, err := client.MergePR(context.Background(), "svc", 91, "merge"); !errors.Is(err, ErrMergeNotAllowed) {
		t.Fatalf("err = %v, want ErrMergeNotAllowed when GitHub answers merged:false", err)
	}
}
