package github

import (
	"errors"
	"os"

	"github.com/google/go-github/v60/github"
	"github.com/gozeloglu/rel/pkg/config"
)

// Client wraps the GitHub API for a single rel profile.
type Client struct {
	GH      *github.Client
	Profile *config.Profile
}

// NewClient builds an authenticated client bound to the given profile. The
// profile may be nil for calls that only need to inspect an owner (for example
// the setup wizard).
func NewClient(profile *config.Profile) (*Client, error) {
	token := os.Getenv("GH_TOKEN")
	if token == "" {
		return nil, errors.New("GH_TOKEN environment variable is not set\n" +
			"Create a token with the 'repo' scope (plus 'read:org' when using a team) and export it:\n" +
			"  export GH_TOKEN=\"your_github_token_here\"")
	}

	return &Client{
		GH:      github.NewClient(nil).WithAuthToken(token),
		Profile: profile,
	}, nil
}

func (c *Client) owner() string {
	if c.Profile == nil {
		return ""
	}
	return c.Profile.Owner
}
