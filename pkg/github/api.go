package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v60/github"
)

func (c *Client) FetchRepos(ctx context.Context) ([]*github.Repository, error) {
	fmt.Println("Fetching repositories for team 'payment-integrations'...")
	opts := &github.ListOptions{PerPage: 100}

	var allRepos []*github.Repository
	teamFetchSuccess := false

	for {
		repos, resp, err := c.GH.Teams.ListTeamReposBySlug(ctx, "Getir", "payment-integrations", opts)
		if err != nil {
			// Fallback to org-wide fetch
			fmt.Printf("Team fetch failed (%v), falling back to org repos with 'payment-' prefix...\n", err)
			break
		}
		teamFetchSuccess = true

		for _, repo := range repos {
			if !strings.HasSuffix(repo.GetName(), "-manifests") {
				allRepos = append(allRepos, repo)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	if teamFetchSuccess {
		return allRepos, nil
	}

	orgOpts := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		repos, resp, err := c.GH.Repositories.ListByOrg(ctx, "Getir", orgOpts)
		if err != nil {
			return nil, err
		}

		for _, repo := range repos {
			if strings.HasPrefix(repo.GetName(), "payment-") && !strings.HasSuffix(repo.GetName(), "-manifests") {
				allRepos = append(allRepos, repo)
			}
		}

		if resp.NextPage == 0 {
			break
		}
		orgOpts.Page = resp.NextPage
	}

	return allRepos, nil
}

func (c *Client) GetLatestReleaseTag(ctx context.Context, repo string) (string, error) {
	opts := &github.ListOptions{PerPage: 1}
	tags, _, err := c.GH.Repositories.ListTags(ctx, "Getir", repo, opts)
	if err != nil {
		return "", err
	}
	if len(tags) == 0 {
		return "", nil // No tags
	}
	return tags[0].GetName(), nil
}

func (c *Client) CheckSyncStatus(ctx context.Context, repo string) (bool, error) {
	comp, _, err := c.GH.Repositories.CompareCommits(ctx, "Getir", repo, "dev", "master", nil)
	if err != nil {
		return false, err
	}
	// If ahead_by > 0, master is ahead of dev.
	return comp.GetAheadBy() > 0, nil
}

func (c *Client) CreateReleaseBranch(ctx context.Context, repo, branchName string) error {
	// Get dev branch SHA
	devBranch, _, err := c.GH.Repositories.GetBranch(ctx, "Getir", repo, "dev", 1)
	if err != nil {
		return fmt.Errorf("failed to get dev branch: %w", err)
	}
	
	sha := devBranch.GetCommit().GetSHA()
	refName := "refs/heads/" + branchName
	
	ref := &github.Reference{
		Ref: &refName,
		Object: &github.GitObject{
			SHA: &sha,
		},
	}
	
	_, _, err = c.GH.Git.CreateRef(ctx, "Getir", repo, ref)
	return err
}

func (c *Client) CreateReleasePR(ctx context.Context, repo, branchName, version string) (string, error) {
	title := fmt.Sprintf("Release %s", version) // e.g. v1.21.0
	head := branchName
	base := "master"
	
	newPR := &github.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
	}
	
	pr, _, err := c.GH.PullRequests.Create(ctx, "Getir", repo, newPR)
	if err != nil {
		return "", err
	}
	return pr.GetHTMLURL(), nil
}

func (c *Client) CreateSyncPR(ctx context.Context, repo string) (string, error) {
	title := "chore: master to dev sync"
	head := "master"
	base := "dev"
	
	newPR := &github.NewPullRequest{
		Title: &title,
		Head:  &head,
		Base:  &base,
	}
	
	pr, _, err := c.GH.PullRequests.Create(ctx, "Getir", repo, newPR)
	if err != nil {
		return "", err
	}
	return pr.GetHTMLURL(), nil
}
