package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gozeloglu/rel/pkg/cache"
	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/spf13/cobra"
)

// profileFlag holds the value of the shared --profile flag.
var profileFlag string

// addProfileFlag registers --profile on a command.
func addProfileFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&profileFlag, "profile", "",
		"Profile to use for this run (defaults to the active profile)")
}

// repoCacheKey scopes the repository cache to a profile. The fingerprint makes
// the cache miss automatically when the owner, team or filters change.
func repoCacheKey(p *config.Profile) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, p.Name)

	return fmt.Sprintf("repos-%s-%s", safe, p.Fingerprint())
}

// resolveProfile picks the profile for this run: the --profile flag first, then
// the active profile, and finally the setup wizard when nothing is configured.
func resolveProfile(cmd *cobra.Command) (*config.Profile, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}

	if profileFlag != "" {
		p, ok := cfg.Get(profileFlag)
		if !ok {
			return nil, fmt.Errorf("profile %q not found (run 'rel profile list' to see the available ones)", profileFlag)
		}
		return p, nil
	}

	p, err := cfg.Current()
	if errors.Is(err, config.ErrNoProfiles) {
		fmt.Println("No profile configured yet, starting the setup wizard...")
		return createProfile(cmd.Context(), cfg, nil)
	}
	if err != nil {
		return nil, err
	}

	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("profile %q is invalid: %w\nRun 'rel profile' to fix it", p.Name, err)
	}
	return p, nil
}

// newClientForRun resolves the profile and returns a client bound to it.
func newClientForRun(cmd *cobra.Command) (*github.Client, *config.Profile, error) {
	p, err := resolveProfile(cmd)
	if err != nil {
		return nil, nil, err
	}

	client, err := github.NewClient(p)
	if err != nil {
		return nil, nil, err
	}

	fmt.Printf("Profile: %s (%s)\n", p.Name, p.Summary())
	return client, p, nil
}

// fetchRepoNames returns the repository names for the profile, using a cached
// copy when it is still fresh (see cache.DefaultTTL).
func fetchRepoNames(ctx context.Context, client *github.Client, p *config.Profile, refresh bool) ([]string, error) {
	key := repoCacheKey(p)

	if !refresh {
		var cached []string
		if cache.Load(key, cache.DefaultTTL, &cached) && len(cached) > 0 {
			age, _ := cache.Age(key)
			fmt.Printf("Using cached repositories (%s old). Use --refresh to re-fetch.\n",
				age.Truncate(1e9))
			return cached, nil
		}
	}

	repoNames, err := client.FetchRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repos: %w", err)
	}
	if len(repoNames) == 0 {
		return nil, fmt.Errorf("no repositories matched profile %q (check its team and filters with 'rel profile')", p.Name)
	}

	if err := cache.Save(key, repoNames); err != nil {
		fmt.Printf("⚠️  Failed to cache repositories: %v\n", err)
	}

	return repoNames, nil
}
