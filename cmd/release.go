package cmd

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/gozeloglu/rel/pkg/utils"
	"github.com/spf13/cobra"
)

var refreshReleaseRepos bool

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

		selectedRepos, err := tui.SelectRepos(repoNames)
		if err != nil {
			if errors.Is(err, tui.ErrAborted) {
				fmt.Println("\nOperation aborted by user. Exiting...")
				return nil
			}
			return err
		}
		if len(selectedRepos) == 0 {
			fmt.Println("No repositories selected. Exiting.")
			return nil
		}

		fmt.Println("\n⏳ Fetching latest tags concurrently...")
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)
		tagsMap := make(map[string]string)
		var mapMutex sync.Mutex

		for _, repo := range selectedRepos {
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
				mapMutex.Lock()
				tagsMap[r] = tag
				mapMutex.Unlock()
			}(repo)
		}
		wg.Wait()

		// Phase 2: Sequential TUI for version prompts
		versionsToRelease := make(map[string]string)
		for _, repo := range selectedRepos {
			tag, ok := tagsMap[repo]
			if !ok {
				continue // skip failed repos
			}
			if tag != "" {
				fmt.Printf("\n✅ [%s] Found latest tag: %s\n", repo, tag)
			} else {
				fmt.Printf("\nℹ️ [%s] No previous tag found, defaulting to 1.0.0\n", repo)
			}

			nextDefault := utils.BumpMinor(tag)
			version, err := tui.InputVersion(repo, nextDefault)
			if err != nil {
				if errors.Is(err, tui.ErrAborted) {
					fmt.Println("\nOperation aborted by user. Exiting...")
					return nil
				}
				return err
			}
			versionsToRelease[repo] = version
		}

		// Phase 3: Concurrent Release Operations
		fmt.Println("\n⏳ Starting release processes concurrently...")
		var prURLs []string
		var prMutex sync.Mutex
		var errCount int32

		for repo, version := range versionsToRelease {
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

				prMutex.Lock()
				prURLs = append(prURLs, prURL)
				prMutex.Unlock()
				fmt.Printf("✅ [%s] Created PR: %s\n", r, prURL)
			}(repo, version)
		}
		wg.Wait()

		if len(prURLs) > 0 {
			filename := fmt.Sprintf("release-notes-%s.md", time.Now().Format("2006-01-02-15-04"))
			f, err := os.Create(filename)
			if err != nil {
				return fmt.Errorf("failed to create release notes file: %w", err)
			}
			defer f.Close()

			f.WriteString("# Release Notes\n\n")
			for _, url := range prURLs {
				f.WriteString(fmt.Sprintf("- %s\n", url))
			}
			fmt.Printf("\n🎉 Release process completed! Wrote %s. Errors: %d\n", filename, errCount)
		} else {
			fmt.Printf("\nNo PRs were created. Errors: %d\n", errCount)
		}

		return nil
	},
}

func init() {
	releaseCmd.Flags().BoolVar(&refreshReleaseRepos, "refresh", false, "Bypass the repository cache and re-fetch from GitHub")
	addProfileFlag(releaseCmd)
	rootCmd.AddCommand(releaseCmd)
}
