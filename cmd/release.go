package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/gozeloglu/rel/pkg/utils"
	"github.com/spf13/cobra"
)

var refreshReleaseRepos bool

// Injection points so the review loop can be driven without a terminal.
var (
	selectRepos   = tui.SelectReposWithPreset
	versionPrompt = tui.InputVersion
)

var releaseCmd = &cobra.Command{
	Use:               "release",
	Short:             "Start the release process",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		client, profile, err := newClientForRun(cmd)
		if err != nil {
			return err
		}

		repoNames, err := fetchRepoNames(ctx, client, profile, refreshReleaseRepos)
		if err != nil {
			return err
		}

		items, err := planRelease(ctx, client, profile, repoNames)
		if err != nil {
			if errors.Is(err, tui.ErrAborted) {
				fmt.Println("\nOperation aborted by user. Exiting...")
				return nil
			}
			return err
		}
		if len(items) == 0 {
			return nil
		}

		return applyRelease(ctx, client, profile, items)
	},
}

// planRelease runs the review loop: pick repositories, enter versions, look at
// the plan, and either confirm it or go back and change it. It returns the
// approved plan, or nothing when the user walked away.
func planRelease(ctx context.Context, client *github.Client, profile *config.Profile, repoNames []string) ([]releaseItem, error) {
	// Carried across passes so a second look is cheap: tags are never
	// re-fetched and previously typed versions come back as the defaults.
	tags := make(map[string]string)
	entered := make(map[string]string)
	var chosen []string

	for {
		selected, err := selectRepos(repoNames, chosen)
		if err != nil {
			return nil, err
		}
		if len(selected) == 0 {
			fmt.Println("No repositories selected. Exiting.")
			return nil, nil
		}
		chosen = selected

		fetchLatestTags(ctx, client, selected, tags)

		items, skipped, err := promptVersions(selected, tags, entered)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			fmt.Println("\nNo repositories left to release. Exiting.")
			return nil, nil
		}

		fmt.Print(renderReleasePlan(items, skipped, profile))

		question := fmt.Sprintf("Create %d release %s?",
			len(items), plural(len(items), "pull request", "pull requests"))

		// A flagged row must not be waved through by muscle memory.
		approved, err := confirmAction(question, "", !planHasWarning(items))
		if err != nil {
			return nil, err
		}
		if approved {
			return items, nil
		}

		fmt.Println("\n↩ Going back to the repository selection...")
	}
}

// fetchLatestTags fills in the tags that are still unknown, leaving anything
// already looked up in an earlier pass untouched.
func fetchLatestTags(ctx context.Context, client *github.Client, repos []string, tags map[string]string) {
	missing := make([]string, 0, len(repos))
	for _, r := range repos {
		if _, ok := tags[r]; !ok {
			missing = append(missing, r)
		}
	}
	if len(missing) == 0 {
		return
	}

	fmt.Println("\n⏳ Fetching latest tags concurrently...")

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 5)

	for _, repo := range missing {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			tag, err := client.GetLatestReleaseTag(ctx, r)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to get latest tag: %v\n", r, err)
				return
			}
			mu.Lock()
			tags[r] = tag
			mu.Unlock()
		}(repo)
	}
	wg.Wait()
}

// promptVersions asks for the next version of every repository whose latest tag
// is known, and reports the ones that had to be dropped.
func promptVersions(repos []string, tags map[string]string, entered map[string]string) ([]releaseItem, []string, error) {
	var items []releaseItem
	var skipped []string

	for _, repo := range repos {
		tag, ok := tags[repo]
		if !ok {
			skipped = append(skipped, repo)
			continue
		}

		if tag != "" {
			fmt.Printf("\n✅ [%s] Found latest tag: %s\n", repo, tag)
		} else {
			fmt.Printf("\nℹ️ [%s] No previous tag found, defaulting to 1.0.0\n", repo)
		}

		// Re-offer what was typed last time so only the wrong one needs fixing.
		def := entered[repo]
		if def == "" {
			def = utils.BumpMinor(tag)
		}

		version, err := versionPrompt(repo, def)
		if err != nil {
			return nil, nil, err
		}
		entered[repo] = version

		items = append(items, releaseItem{Repo: repo, CurrentTag: tag, Version: version})
	}
	return items, skipped, nil
}

// applyRelease creates the release branch and pull request for every approved
// repository.
func applyRelease(ctx context.Context, client *github.Client, profile *config.Profile, items []releaseItem) error {
	fmt.Println("\n⏳ Starting release processes concurrently...")

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 5)
	var created []createdPR
	var errCount int32

	for _, item := range items {
		wg.Add(1)
		go func(r, v string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			isAhead, err := client.CheckSyncStatus(ctx, r)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to check sync status: %v\n", r, err)
				atomic.AddInt32(&errCount, 1)
				return
			}

			if isAhead {
				fmt.Printf("❌ [%s] '%s' is ahead of '%s'. Sync missing!\n",
					r, profile.BaseBranch, profile.DevBranch)
				atomic.AddInt32(&errCount, 1)
				return
			}

			branchName, tagName := utils.GenerateBranchAndTag(v)
			err = client.CreateReleaseBranch(ctx, r, branchName)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to create branch: %v\n", r, err)
				atomic.AddInt32(&errCount, 1)
				return
			}

			prURL, err := client.CreateReleasePR(ctx, r, branchName, tagName)
			if err != nil {
				fmt.Printf("❌ [%s] Failed to create PR: %v\n", r, err)
				atomic.AddInt32(&errCount, 1)
				return
			}

			mu.Lock()
			created = append(created, createdPR{Repo: r, URL: prURL})
			mu.Unlock()
			fmt.Printf("✅ [%s] Created PR: %s\n", r, prURL)
		}(item.Repo, item.Version)
	}
	wg.Wait()

	if len(created) == 0 {
		fmt.Printf("\nNo PRs were created. Errors: %d\n", errCount)
		return nil
	}

	sort.Slice(created, func(i, j int) bool { return created[i].Repo < created[j].Repo })

	filename := fmt.Sprintf("release-notes-%s.md", time.Now().Format("2006-01-02-15-04"))
	f, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create release notes file: %w", err)
	}
	defer f.Close()

	f.WriteString("# Release Notes\n\n")
	for _, pr := range created {
		f.WriteString(fmt.Sprintf("- %s: %s\n", pr.Repo, pr.URL))
	}
	fmt.Printf("\n🎉 Release process completed! Wrote %s. Errors: %d\n", filename, errCount)

	reportCreatedPRs(created)

	return nil
}

func init() {
	releaseCmd.Flags().BoolVar(&refreshReleaseRepos, "refresh", false, "Bypass the repository cache and re-fetch from GitHub")
	addOpenFlag(releaseCmd)
	addProfileFlag(releaseCmd)
	rootCmd.AddCommand(releaseCmd)
}
