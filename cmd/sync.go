package cmd

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/charmbracelet/huh"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

var refreshSyncRepos bool

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync master to dev",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		client, err := github.NewClient()
		if err != nil {
			return err
		}

		repoNames, err := fetchRepoNames(ctx, client, refreshSyncRepos)
		if err != nil {
			return err
		}

		selectedRepos, err := tui.SelectRepos(repoNames)
		if err != nil {
			if errors.Is(err, huh.ErrUserAborted) {
				fmt.Println("\nOperation aborted by user. Exiting...")
				return nil
			}
			return err
		}
		if len(selectedRepos) == 0 {
			fmt.Println("No repositories selected. Exiting.")
			return nil
		}

		fmt.Println("Starting sync process concurrently...")
		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)
		var errCount, successCount int32

		for _, repo := range selectedRepos {
			wg.Add(1)
			go func(r string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				isAhead, err := client.CheckSyncStatus(ctx, r)
				if err != nil {
					fmt.Printf("❌ [%s] Failed to check sync status: %v\n", r, err)
					atomic.AddInt32(&errCount, 1)
					return
				}

				if !isAhead {
					fmt.Printf("✅ [%s] Master is not ahead of Dev. No sync needed.\n", r)
					return
				}

				prURL, err := client.CreateSyncPR(ctx, r)
				if err != nil {
					fmt.Printf("❌ [%s] Failed to create sync PR: %v\n", r, err)
					atomic.AddInt32(&errCount, 1)
					return
				}

				fmt.Printf("✅ [%s] Created Sync PR: %s\n", r, prURL)
				atomic.AddInt32(&successCount, 1)
			}(repo)
		}
		
		wg.Wait()
		fmt.Printf("\nSync completed. Success: %d, Errors: %d\n", successCount, errCount)

		return nil
	},
}

func init() {
	syncCmd.Flags().BoolVar(&refreshSyncRepos, "refresh", false, "Bypass the repository cache and re-fetch from GitHub")
	rootCmd.AddCommand(syncCmd)
}
