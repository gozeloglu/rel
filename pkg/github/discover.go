package github

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/go-github/v60/github"
	"github.com/gozeloglu/rel/pkg/config"
)

// Common names teams use for the branch that release branches are cut from.
var devBranchCandidates = []string{"dev", "develop", "development"}

// ResolveOwner reports whether the given login is an organization or a user.
func (c *Client) ResolveOwner(ctx context.Context, owner string) (config.OwnerType, error) {
	if _, resp, err := c.GH.Organizations.Get(ctx, owner); err == nil {
		return config.OwnerOrg, nil
	} else if resp != nil && resp.StatusCode != http.StatusNotFound {
		return "", err
	}

	user, _, err := c.GH.Users.Get(ctx, owner)
	if err != nil {
		return "", fmt.Errorf("'%s' is not a known GitHub organization or user: %w", owner, err)
	}
	if user.GetType() == "Organization" {
		return config.OwnerOrg, nil
	}
	return config.OwnerUser, nil
}

// CheckTeam verifies that the token can list the repositories of a team.
func (c *Client) CheckTeam(ctx context.Context, owner, team string) error {
	_, _, err := c.GH.Teams.ListTeamReposBySlug(ctx, owner, team,
		&github.ListOptions{PerPage: 1})
	if err != nil {
		return fmt.Errorf("cannot read team '%s/%s': %w", owner, team, err)
	}
	return nil
}

// DetectBranches inspects a sample repository and suggests the base branch (its
// default branch) and the dev branch (the first well known dev-style branch
// that exists, otherwise the base branch).
func (c *Client) DetectBranches(ctx context.Context, repo string) (base string, dev string, err error) {
	r, _, err := c.GH.Repositories.Get(ctx, c.owner(), repo)
	if err != nil {
		return "", "", err
	}

	base = r.GetDefaultBranch()
	if base == "" {
		base = "main"
	}

	for _, candidate := range devBranchCandidates {
		if candidate == base {
			continue
		}

		_, resp, err := c.GH.Repositories.GetBranch(ctx, c.owner(), repo, candidate, 1)
		if err == nil {
			return base, candidate, nil
		}
		// A missing branch is expected; anything else (auth, rate limit) would
		// make the suggestion meaningless, so surface it.
		if resp == nil || resp.StatusCode != http.StatusNotFound {
			return "", "", err
		}
	}

	return base, base, nil
}
