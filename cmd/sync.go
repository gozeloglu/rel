package cmd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

var (
	refreshSyncRepos bool
	autoSync         bool
	autoSyncSince    time.Duration
	autoSyncYes      bool
	autoSyncDryRun   bool
)

var syncCmd = &cobra.Command{
	Use:               "sync",
	Short:             "Open pull requests that merge the release branch back into the development branch",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateAutoSyncFlags(cmd); err != nil {
			return err
		}

		ctx := cmd.Context()
		client, profile, err := newClientForRun(cmd)
		if err != nil {
			return err
		}

		if profile.SingleBranch() {
			fmt.Printf("Profile '%s' uses a single branch ('%s'), so there is nothing to sync.\n",
				profile.Name, profile.BaseBranch)
			return nil
		}

		repoNames, err := fetchRepoNames(ctx, client, profile, refreshSyncRepos)
		if err != nil {
			return err
		}

		if autoSync {
			return runAutoSync(ctx, client, profile, repoNames)
		}

		selectedRepos, err := tui.SelectRepos(repoNames)
		if err != nil {
			if isAborted(err) {
				fmt.Println("\nOperation aborted by user. Exiting...")
				return nil
			}
			return err
		}
		if len(selectedRepos) == 0 {
			fmt.Println("No repositories selected. Exiting.")
			return nil
		}

		createSyncPRs(ctx, client, profile, selectedRepos, true)
		return nil
	},
}

// validateAutoSyncFlags rejects the auto-sync modifiers when --auto is absent,
// so they never silently do nothing.
func validateAutoSyncFlags(cmd *cobra.Command) error {
	if autoSync {
		if autoSyncSince <= 0 {
			return errors.New("--since must be a positive duration, for example --since 2h")
		}
		return nil
	}

	for _, name := range []string{"since", "yes", "dry-run"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s only applies together with --auto", name)
		}
	}
	return nil
}

func isAborted(err error) bool {
	return errors.Is(err, tui.ErrAborted)
}

// createSyncPRs opens a base-to-dev pull request for each repository. When
// verify is set the sync status is re-checked right before acting, which is
// what the interactive flow needs because the user picks repositories blind.
func createSyncPRs(ctx context.Context, client *github.Client, profile *config.Profile, repos []string, verify bool) {
	fmt.Println("\nStarting sync process concurrently...")

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)
	var errCount, skipCount int32

	var mu sync.Mutex
	var created []prLink

	for _, repo := range repos {
		wg.Add(1)
		go func(r string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if verify {
				isAhead, err := client.CheckSyncStatus(ctx, r)
				if err != nil {
					fmt.Printf("❌ [%s] Failed to check sync status: %v\n", r, err)
					atomic.AddInt32(&errCount, 1)
					return
				}
				if !isAhead {
					fmt.Printf("✅ [%s] '%s' is not ahead of '%s'. No sync needed.\n",
						r, profile.BaseBranch, profile.DevBranch)
					return
				}
			}

			prURL, err := client.CreateSyncPR(ctx, r)
			if err != nil {
				if errors.Is(err, github.ErrSyncPRExists) {
					fmt.Printf("⏭️  [%s] A sync PR is already open. Skipping.\n", r)
					atomic.AddInt32(&skipCount, 1)
					return
				}
				fmt.Printf("❌ [%s] Failed to create sync PR: %v\n", r, err)
				atomic.AddInt32(&errCount, 1)
				return
			}

			fmt.Printf("✅ [%s] Created Sync PR: %s\n", r, prURL)
			mu.Lock()
			created = append(created, prLink{Repo: r, URL: prURL})
			mu.Unlock()
		}(repo)
	}

	wg.Wait()

	sort.Slice(created, func(i, j int) bool { return created[i].Repo < created[j].Repo })

	if skipCount > 0 {
		fmt.Printf("\nSync completed. Created: %d, Skipped: %d, Errors: %d\n",
			len(created), skipCount, errCount)
	} else {
		fmt.Printf("\nSync completed. Success: %d, Errors: %d\n", len(created), errCount)
	}

	reportCreatedPRs(created)
}

func init() {
	syncCmd.Flags().BoolVar(&refreshSyncRepos, "refresh", false, "Bypass the repository cache and re-fetch from GitHub")
	syncCmd.Flags().BoolVar(&autoSync, "auto", false, "Detect repositories released recently and sync them without picking each one by hand")
	syncCmd.Flags().DurationVar(&autoSyncSince, "since", 2*time.Hour, "How far back to look for releases (requires --auto)")
	syncCmd.Flags().BoolVarP(&autoSyncYes, "yes", "y", false, "Skip the confirmation screen (requires --auto)")
	syncCmd.Flags().BoolVar(&autoSyncDryRun, "dry-run", false, "Report what would be synced without opening pull requests (requires --auto)")
	addOpenFlag(syncCmd)
	addProfileFlag(syncCmd)
	rootCmd.AddCommand(syncCmd)
}
