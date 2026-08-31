package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v60/github"
	"github.com/gozeloglu/rel/pkg/config"
)

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

// CheckSyncStatus reports whether the base branch is ahead of the dev branch.
func (c *Client) CheckSyncStatus(ctx context.Context, repo string) (bool, error) {
	if c.Profile.SingleBranch() {
		return false, nil
	}

	comp, _, err := c.GH.Repositories.CompareCommits(ctx, c.owner(), repo,
		c.Profile.DevBranch, c.Profile.BaseBranch, nil)
	if err != nil {
		return false, err
	}
	// If ahead_by > 0, the base branch is ahead of the dev branch.
	return comp.GetAheadBy() > 0, nil
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
		return "", err
	}
	return pr.GetHTMLURL(), nil
}
