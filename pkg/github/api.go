package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/go-github/v60/github"
	"github.com/gozeloglu/rel/pkg/config"
)

// ErrSyncPRExists reports that the sync pull request the caller asked for is
// already open, so nothing was created.
var ErrSyncPRExists = errors.New("sync pull request already exists")

// FetchRepos lists the repositories the profile targets, applying its include
// and exclude filters. When a team is configured it is used first; if the team
// cannot be read (missing scope, wrong slug) it falls back to listing the whole
// owner so the tool stays usable.
func (c *Client) FetchRepos(ctx context.Context) ([]string, error) {
	p := c.Profile
	if p == nil {
		return nil, fmt.Errorf("no profile configured")
	}

	if p.Team != "" {
		repos, err := c.fetchTeamRepos(ctx, p)
		if err == nil {
			return repos, nil
		}
		fmt.Printf("⚠️  Team '%s' fetch failed (%v), falling back to all %s repositories...\n",
			p.Team, err, p.Owner)
	}

	return c.fetchOwnerRepos(ctx, p)
}

func (c *Client) fetchTeamRepos(ctx context.Context, p *config.Profile) ([]string, error) {
	fmt.Printf("Fetching repositories for team '%s/%s'...\n", p.Owner, p.Team)

	opts := &github.ListOptions{PerPage: 100}
	var names []string

	for {
		repos, resp, err := c.GH.Teams.ListTeamReposBySlug(ctx, p.Owner, p.Team, opts)
		if err != nil {
			return nil, err
		}

		for _, repo := range repos {
			if p.Matches(repo.GetName()) {
				names = append(names, repo.GetName())
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return names, nil
}

func (c *Client) fetchOwnerRepos(ctx context.Context, p *config.Profile) ([]string, error) {
	fmt.Printf("Fetching repositories for %s '%s'...\n", p.OwnerType, p.Owner)

	var names []string
	listOpts := github.ListOptions{PerPage: 100}

	for {
		var (
			repos []*github.Repository
			resp  *github.Response
			err   error
		)

		if p.OwnerType == config.OwnerUser {
			repos, resp, err = c.GH.Repositories.ListByUser(ctx, p.Owner,
				&github.RepositoryListByUserOptions{Type: "owner", ListOptions: listOpts})
		} else {
			repos, resp, err = c.GH.Repositories.ListByOrg(ctx, p.Owner,
				&github.RepositoryListByOrgOptions{Type: "all", ListOptions: listOpts})
		}
		if err != nil {
			return nil, err
		}

		for _, repo := range repos {
			if p.Matches(repo.GetName()) {
				names = append(names, repo.GetName())
			}
		}

		if resp.NextPage == 0 {
			break
		}
		listOpts.Page = resp.NextPage
	}

	return names, nil
}

// ReleaseInfo describes the most recent release marker of a repository. A zero
// value means the repository has never been released.
type ReleaseInfo struct {
	Tag       string
	Published time.Time
}

// Found reports whether a release marker was actually resolved.
func (r ReleaseInfo) Found() bool {
	return r.Tag != "" && !r.Published.IsZero()
}

// LatestRelease returns the newest published release of a repository. Draft
// releases are ignored because they are not visible to anyone but the author.
// Repositories that tag without publishing GitHub releases fall back to the
// newest tag, whose commit date is used as the publication time.
func (c *Client) LatestRelease(ctx context.Context, repo string) (ReleaseInfo, error) {
	releases, _, err := c.GH.Repositories.ListReleases(ctx, c.owner(), repo,
		&github.ListOptions{PerPage: 10})
	if err != nil {
		return ReleaseInfo{}, err
	}

	for _, rel := range releases {
		if rel.GetDraft() {
			continue
		}
		published := rel.GetPublishedAt().Time
		if published.IsZero() {
			published = rel.GetCreatedAt().Time
		}
		if published.IsZero() {
			continue
		}
		return ReleaseInfo{Tag: rel.GetTagName(), Published: published}, nil
	}

	return c.latestTagAsRelease(ctx, repo)
}

// latestTagAsRelease resolves the newest tag and dates it by its commit, so
// repositories that only push tags still produce a deploy signal.
func (c *Client) latestTagAsRelease(ctx context.Context, repo string) (ReleaseInfo, error) {
	tags, _, err := c.GH.Repositories.ListTags(ctx, c.owner(), repo,
		&github.ListOptions{PerPage: 1})
	if err != nil {
		return ReleaseInfo{}, err
	}
	if len(tags) == 0 {
		return ReleaseInfo{}, nil
	}

	tag := tags[0]
	sha := tag.GetCommit().GetSHA()
	if sha == "" {
		return ReleaseInfo{}, nil
	}

	commit, _, err := c.GH.Git.GetCommit(ctx, c.owner(), repo, sha)
	if err != nil {
		return ReleaseInfo{}, err
	}

	return ReleaseInfo{
		Tag:       tag.GetName(),
		Published: commit.GetCommitter().GetDate().Time,
	}, nil
}

// FindOpenSyncPR returns the URL of an already open base-to-dev pull request,
// or an empty string when there is none. It keeps repeated auto-sync runs from
// piling up duplicate pull requests.
func (c *Client) FindOpenSyncPR(ctx context.Context, repo string) (string, error) {
	if c.Profile.SingleBranch() {
		return "", nil
	}

	opts := &github.PullRequestListOptions{
		State:       "open",
		Head:        c.owner() + ":" + c.Profile.BaseBranch,
		Base:        c.Profile.DevBranch,
		ListOptions: github.ListOptions{PerPage: 1},
	}

	prs, _, err := c.GH.PullRequests.List(ctx, c.owner(), repo, opts)
	if err != nil {
		return "", err
	}
	if len(prs) == 0 {
		return "", nil
	}
	return prs[0].GetHTMLURL(), nil
}

// GetLatestReleaseTag returns the most recent tag of a repository.
func (c *Client) GetLatestReleaseTag(ctx context.Context, repo string) (string, error) {
	opts := &github.ListOptions{PerPage: 1}
	tags, _, err := c.GH.Repositories.ListTags(ctx, c.owner(), repo, opts)
	if err != nil {
		return "", err
	}
	if len(tags) == 0 {
		return "", nil // No tags
	}
	return tags[0].GetName(), nil
}

// SyncState describes how a repository's base and development branches relate.
type SyncState struct {
	// AheadBy counts commits the base branch has that the dev branch lacks.
	AheadBy int
	// BehindBy counts commits the dev branch has that the base branch lacks.
	BehindBy int
	// MergeBase is the date of the newest commit both branches share, which is
	// effectively the moment they were last in sync.
	MergeBase time.Time
}

// NeedsSync reports whether the base branch has commits the dev branch is missing.
func (s SyncState) NeedsSync() bool { return s.AheadBy > 0 }

// CompareSyncState compares the dev branch against the base branch. Everything
// it returns comes from a single compare call, so callers get the divergence
// details for free.
func (c *Client) CompareSyncState(ctx context.Context, repo string) (SyncState, error) {
	comp, _, err := c.GH.Repositories.CompareCommits(ctx, c.owner(), repo,
		c.Profile.DevBranch, c.Profile.BaseBranch, nil)
	if err != nil {
		return SyncState{}, err
	}

	return SyncState{
		AheadBy:   comp.GetAheadBy(),
		BehindBy:  comp.GetBehindBy(),
		MergeBase: comp.GetMergeBaseCommit().GetCommit().GetCommitter().GetDate().Time,
	}, nil
}

// CheckSyncStatus reports whether the base branch is ahead of the dev branch.
func (c *Client) CheckSyncStatus(ctx context.Context, repo string) (bool, error) {
	if c.Profile.SingleBranch() {
		return false, nil
	}

	state, err := c.CompareSyncState(ctx, repo)
	if err != nil {
		return false, err
	}
	// If ahead_by > 0, the base branch is ahead of the dev branch.
	return state.NeedsSync(), nil
}

// CreateReleaseBranch cuts a release branch from the dev branch.
func (c *Client) CreateReleaseBranch(ctx context.Context, repo, branchName string) error {
	source := c.Profile.DevBranch

	devBranch, _, err := c.GH.Repositories.GetBranch(ctx, c.owner(), repo, source, 1)
	if err != nil {
		return fmt.Errorf("failed to get %s branch: %w", source, err)
	}

	sha := devBranch.GetCommit().GetSHA()
	refName := "refs/heads/" + branchName

	ref := &github.Reference{
		Ref: &refName,
		Object: &github.GitObject{
			SHA: &sha,
		},
	}

	_, _, err = c.GH.Git.CreateRef(ctx, c.owner(), repo, ref)
	return err
}

// CreateReleasePR opens the release branch against the base branch.
func (c *Client) CreateReleasePR(ctx context.Context, repo, branchName, version string) (string, error) {
	title := fmt.Sprintf("Release %s", version) // e.g. v1.21.0
	head := branchName
	base := c.Profile.BaseBranch

	newPR := &github.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
	}

	pr, _, err := c.GH.PullRequests.Create(ctx, c.owner(), repo, newPR)
	if err != nil {
		return "", err
	}
	return pr.GetHTMLURL(), nil
}

// CreateSyncPR opens a PR that merges the base branch back into the dev branch.
func (c *Client) CreateSyncPR(ctx context.Context, repo string) (string, error) {
	head := c.Profile.BaseBranch
	base := c.Profile.DevBranch
	title := fmt.Sprintf("chore: %s to %s sync", head, base)

	newPR := &github.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
	}

	pr, _, err := c.GH.PullRequests.Create(ctx, c.owner(), repo, newPR)
	if err != nil {
		if isAlreadyExistsErr(err) {
			return "", ErrSyncPRExists
		}
		return "", err
	}
	return pr.GetHTMLURL(), nil
}

// IsNotFound reports whether an API call failed with 404. Auto-sync uses it to
// tell "this repository does not follow the base/dev model" apart from a real
// failure, since comparing against a branch that does not exist returns 404.
func IsNotFound(err error) bool {
	var errResp *github.ErrorResponse
	if !errors.As(err, &errResp) {
		return false
	}
	return errResp.Response != nil && errResp.Response.StatusCode == http.StatusNotFound
}

// isAlreadyExistsErr detects GitHub's 422 response for a pull request that
// duplicates an open one. GitHub reports it as a validation error rather than
// a conflict, so the message has to be inspected.
func isAlreadyExistsErr(err error) bool {
	var errResp *github.ErrorResponse
	if !errors.As(err, &errResp) {
		return false
	}
	if errResp.Response == nil || errResp.Response.StatusCode != http.StatusUnprocessableEntity {
		return false
	}
	for _, e := range errResp.Errors {
		if strings.Contains(strings.ToLower(e.Message), "already exists") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(errResp.Message), "already exists")
}
