package cmd

import (
	"context"
	"fmt"

	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync master to dev",
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

		for _, repo := range selectedRepos {
			fmt.Printf("\nChecking sync status for %s...\n", repo)
			
			isAhead, err := client.CheckSyncStatus(ctx, repo)
			if err != nil {
				fmt.Printf("❌ Failed to check sync status for %s: %v\n", repo, err)
				continue
			}

			if !isAhead {
				fmt.Printf("✅ Master is not ahead of Dev. No sync needed for %s.\n", repo)
				continue
			}

			fmt.Printf("Creating master to dev sync PR for %s...\n", repo)
			prURL, err := client.CreateSyncPR(ctx, repo)
			if err != nil {
				fmt.Printf("❌ Failed to create sync PR for %s: %v\n", repo, err)
				continue
			}

			fmt.Printf("✅ Success: %s\n", prURL)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
}
