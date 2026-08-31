package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Getir/rel/pkg/github"
	"github.com/Getir/rel/pkg/tui"
	"github.com/Getir/rel/pkg/utils"
	"github.com/spf13/cobra"
)

var releaseCmd = &cobra.Command{
	Use:   "release",
	Short: "Start the release process",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := github.NewClient()
		if err != nil {
			return err
		}

		fmt.Println("Fetching repositories...")
		repos, err := client.FetchRepos(ctx)
		if err != nil {
			return fmt.Errorf("failed to fetch repos: %w", err)
		}

		var repoNames []string
		for _, r := range repos {
			repoNames = append(repoNames, r.GetName())
		}

		selectedRepos, err := tui.SelectRepos(repoNames)
		if err != nil {
			return err
		}
		if len(selectedRepos) == 0 {
			fmt.Println("No repositories selected. Exiting.")
			return nil
		}

		var prURLs []string

		for _, repo := range selectedRepos {
			fmt.Printf("\nProcessing %s...\n", repo)
			tag, err := client.GetLatestReleaseTag(ctx, repo)
			if err != nil {
				fmt.Printf("❌ Failed to get latest tag for %s: %v\n", repo, err)
				continue
			}

			nextDefault := utils.BumpMinor(tag)
			version, err := tui.InputVersion(repo, nextDefault)
			if err != nil {
				return err
			}

			isAhead, err := client.CheckSyncStatus(ctx, repo)
			if err != nil {
				fmt.Printf("❌ Failed to check sync status for %s: %v\n", repo, err)
				continue
			}

			if isAhead {
				fmt.Printf("❌ Master is ahead of Dev. Sync missing for %s!\n", repo)
				continue
			}

			branchName, tagName := utils.GenerateBranchAndTag(version)
			fmt.Printf("Creating branch %s...\n", branchName)
			err = client.CreateReleaseBranch(ctx, repo, branchName)
			if err != nil {
				fmt.Printf("❌ Failed to create branch for %s: %v\n", repo, err)
				continue
			}

			fmt.Printf("Creating PR for %s...\n", tagName)
			prURL, err := client.CreateReleasePR(ctx, repo, branchName, tagName)
			if err != nil {
				fmt.Printf("❌ Failed to create PR for %s: %v\n", repo, err)
				continue
			}

			prURLs = append(prURLs, prURL)
			fmt.Printf("✅ Success: %s\n", prURL)
		}

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
			fmt.Printf("\n🎉 Release process completed! Wrote %s\n", filename)
		} else {
			fmt.Println("\nNo PRs were created.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(releaseCmd)
}
