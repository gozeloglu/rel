package cmd

import (
	"context"
	"fmt"

	"github.com/gozeloglu/rel/pkg/cache"
	"github.com/gozeloglu/rel/pkg/github"
)

const repoCacheKey = "repos"

// fetchRepoNames returns the repository names, using a cached copy when it is
// still fresh (see cache.DefaultTTL). Pass refresh=true to bypass the cache.
func fetchRepoNames(ctx context.Context, client *github.Client, refresh bool) ([]string, error) {
	if !refresh {
		var cached []string
		if cache.Load(repoCacheKey, cache.DefaultTTL, &cached) && len(cached) > 0 {
			age, _ := cache.Age(repoCacheKey)
			fmt.Printf("Using cached repositories (%s old). Use --refresh to re-fetch.\n", age.Truncate(1e9))
			return cached, nil
		}
	}

	fmt.Println("Fetching repositories...")
	repos, err := client.FetchRepos(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch repos: %w", err)
	}

	repoNames := make([]string, 0, len(repos))
	for _, r := range repos {
		repoNames = append(repoNames, r.GetName())
	}

	if len(repoNames) > 0 {
		if err := cache.Save(repoCacheKey, repoNames); err != nil {
			fmt.Printf("⚠️  Failed to cache repositories: %v\n", err)
		}
	}

	return repoNames, nil
}
