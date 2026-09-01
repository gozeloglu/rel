package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

var (
	refreshMergeRepos bool
	autoMerge         bool
	mergeMethod       string
	mergeYes          bool
	mergeDryRun       bool
)

// mergeMethods are the strategies GitHub accepts, squash first because that is
// how release branches are normally landed.
var mergeMethods = []string{"squash", "merge", "rebase"}

// confirmMergeRepos is an injection point so the review loop can be driven
// without a terminal.
var confirmMergeRepos = tui.ConfirmReposWithPreset

var mergeCmd = &cobra.Command{
	Use:               "merge",
	Short:             "Merge the open release pull requests in bulk",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := validateMergeFlags(cmd); err != nil {
			return err
		}

		ctx := cmd.Context()
		client, profile, err := newClientForRun(cmd)
		if err != nil {
			return err
		}

		repoNames, err := fetchRepoNames(ctx, client, profile, refreshMergeRepos)
		if err != nil {
			return err
		}

		return runMerge(ctx, client, profile, repoNames)
	},
}

// validateMergeFlags checks the merge method and rejects the auto-only
// modifiers when --auto is absent, so they never silently do nothing.
func validateMergeFlags(cmd *cobra.Command) error {
	known := false
	for _, m := range mergeMethods {
		if mergeMethod == m {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("--method must be one of %s, got %q",
			strings.Join(mergeMethods, ", "), mergeMethod)
	}

	if autoMerge {
		return nil
	}

	for _, name := range []string{"yes", "dry-run"} {
		if cmd.Flags().Changed(name) {
			return fmt.Errorf("--%s only applies together with --auto", name)
		}
	}
	return nil
}

func runMerge(ctx context.Context, client *github.Client, profile *config.Profile, repoNames []string) error {
	scan := newMergeScan(client)

	pick := pickedMergePicker(ctx, scan, repoNames)
	if autoMerge {
		pick = autoMergePicker(ctx, scan, repoNames)
	}

	items, err := planMerge(pick, profile)
	if err != nil {
		if isAborted(err) {
			fmt.Println("\nOperation aborted by user. Exiting...")
			return nil
		}
		return err
	}
	if len(items) == 0 {
		return mergeScanError(scan)
	}

	if errCount := applyMerges(ctx, client, profile, items, mergeMethod); errCount > 0 {
		return fmt.Errorf("%d %s could not be merged",
			errCount, plural(errCount, "pull request", "pull requests"))
	}
	return mergeScanError(scan)
}

// mergeSelection is what one pass of the review loop produced.
type mergeSelection struct {
	// selected are the repositories whose release pull request should be merged.
	selected []string
	// results is everything screened in this pass, including the rows that
	// cannot be merged, so the plan can explain what was left out.
	results []mergeResult
	// reason is set when there is nothing to review at all, and is printed
	// instead of the plan.
	reason string
}

// mergePicker produces one pass of the review loop. It is given the previous
// answer so a second look starts from what the user already decided.
type mergePicker func(chosen []string) (mergeSelection, error)

// planMerge runs the review loop: choose what to merge, look at the plan, and
// either confirm it or go back and change the selection. Nothing is merged
// until the plan is approved.
func planMerge(pick mergePicker, profile *config.Profile) ([]mergeResult, error) {
	var chosen []string

	for {
		pass, err := pick(chosen)
		if err != nil {
			return nil, err
		}
		if pass.reason != "" {
			fmt.Printf("\n%s\n", pass.reason)
			return nil, nil
		}
		chosen = pass.selected

		ready, excluded := splitMergeResults(applySelection(pass.results, pass.selected))

		fmt.Print(renderMergePlan(ready, excluded, profile, mergeMethod))

		if len(ready) == 0 {
			fmt.Println("\nNothing can be merged right now. Exiting.")
			return nil, nil
		}

		if mergeDryRun {
			fmt.Printf("\n%s\n", sectionTitle(fmt.Sprintf("Dry run · %d %s would be merged with %s",
				len(ready), plural(len(ready), "pull request", "pull requests"), mergeMethod)))
			return nil, nil
		}
		if mergeYes {
			return ready, nil
		}

		question := fmt.Sprintf("Merge %d %s with %s?",
			len(ready), plural(len(ready), "pull request", "pull requests"), mergeMethod)

		// A flagged row must not be waved through by muscle memory.
		approved, err := confirmAction(question, "", !mergePlanHasWarning(ready))
		if err != nil {
			return nil, err
		}
		if approved {
			return ready, nil
		}

		fmt.Println("\n↩ Going back to the repository selection...")
	}
}

// pickedMergePicker lets the user choose the repositories, then looks up the
// release pull request of each one.
func pickedMergePicker(ctx context.Context, scan *mergeScan, repoNames []string) mergePicker {
	return func(chosen []string) (mergeSelection, error) {
		selected, err := selectRepos(repoNames, chosen)
		if err != nil {
			return mergeSelection{}, err
		}
		if len(selected) == 0 {
			return mergeSelection{reason: "No repositories selected. Exiting."}, nil
		}

		return mergeSelection{selected: selected, results: scan.screen(ctx, selected)}, nil
	}
}

// autoMergePicker screens every repository once and offers the release pull
// requests it found with everything ticked, so the user only has to uncheck
// what should wait.
func autoMergePicker(ctx context.Context, scan *mergeScan, repoNames []string) mergePicker {
	return func(chosen []string) (mergeSelection, error) {
		// Repositories without an open release PR are the overwhelming
		// majority of a fleet and say nothing, so they never reach the plan.
		var found []mergeResult
		for _, r := range scan.screen(ctx, repoNames) {
			if r.status != mergeNoPR {
				found = append(found, r)
			}
		}
		if len(found) == 0 {
			return mergeSelection{reason: "No open release pull requests found. Exiting."}, nil
		}

		ready, _ := splitMergeResults(found)
		if len(ready) == 0 || mergeYes || mergeDryRun {
			return mergeSelection{selected: repoNamesOf(ready), results: found}, nil
		}

		preset := chosen
		if preset == nil {
			preset = repoNamesOf(ready)
		}

		items := make([]tui.RepoNote, 0, len(ready))
		for _, r := range ready {
			items = append(items, tui.RepoNote{
				Repo: r.repo,
				Note: fmt.Sprintf("%s  %s", r.prLabel(), r.headLabel()),
			})
		}

		selected, err := confirmMergeRepos("Open release pull requests · confirm merge", items, preset)
		if err != nil {
			return mergeSelection{}, err
		}
		if len(selected) == 0 {
			return mergeSelection{reason: "No pull requests selected. Exiting."}, nil
		}

		return mergeSelection{selected: selected, results: found}, nil
	}
}

// applySelection re-labels the pull requests the user left unticked, so the
// plan shows them as skipped instead of dropping them silently.
func applySelection(results []mergeResult, selected []string) []mergeResult {
	picked := make(map[string]bool, len(selected))
	for _, r := range selected {
		picked[r] = true
	}

	out := make([]mergeResult, 0, len(results))
	for _, r := range results {
		if r.status == mergeReady && !picked[r.repo] {
			r.status = mergeUnselected
		}
		out = append(out, r)
	}
	return out
}

func repoNamesOf(results []mergeResult) []string {
	out := make([]string, 0, len(results))
	for _, r := range results {
		out = append(out, r.repo)
	}
	return out
}

// mergeScan screens repositories for an open release pull request and caches
// every answer, so going back and forth in the review loop never re-asks
// GitHub about a repository it already knows.
type mergeScan struct {
	client  *github.Client
	mu      sync.Mutex
	results map[string]mergeResult
}

func newMergeScan(client *github.Client) *mergeScan {
	return &mergeScan{client: client, results: make(map[string]mergeResult)}
}

// screen returns one result per repository, in the order they were asked for.
func (s *mergeScan) screen(ctx context.Context, repos []string) []mergeResult {
	var missing []string
	for _, r := range repos {
		if _, ok := s.results[r]; !ok {
			missing = append(missing, r)
		}
	}

	if len(missing) > 0 {
		fmt.Printf("\n⏳ Checking %d %s for open release pull requests...\n",
			len(missing), plural(len(missing), "repository", "repositories"))

		var wg sync.WaitGroup
		sem := make(chan struct{}, 5)

		for _, repo := range missing {
			wg.Add(1)
			go func(name string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				res := screenForMerge(ctx, s.client, name)

				s.mu.Lock()
				s.results[name] = res
				s.mu.Unlock()
			}(repo)
		}
		wg.Wait()
	}

	out := make([]mergeResult, 0, len(repos))
	for _, r := range repos {
		out = append(out, s.results[r])
	}
	return out
}

// failures counts the repositories whose screening did not complete, so a run
// that hid something behind an API error cannot exit zero.
func (s *mergeScan) failures() int {
	n := 0
	for _, r := range s.results {
		if r.status == mergeFailed {
			n++
		}
	}
	return n
}

func mergeScanError(scan *mergeScan) error {
	n := scan.failures()
	if n == 0 {
		return nil
	}
	return fmt.Errorf("%d %s could not be checked", n, plural(n, "repository", "repositories"))
}

// screenForMerge resolves the release pull request of one repository and how
// GitHub feels about merging it.
func screenForMerge(ctx context.Context, client *github.Client, repo string) mergeResult {
	res := mergeResult{repo: repo}

	prs, err := client.FindOpenReleasePRs(ctx, repo)
	if err != nil {
		res.status, res.err = mergeFailed, err
		return res
	}

	switch len(prs) {
	case 0:
		res.status = mergeNoPR
		return res
	case 1:
	default:
		// Which of them is "the" release is not something to guess at two in
		// the morning, so the repository is handed back to the user.
		res.status, res.pr, res.others = mergeMultiple, prs[0], prs[1:]
		return res
	}

	// The list endpoint does not report mergeability, so it takes a second call.
	pr, err := client.PullRequestMergeability(ctx, repo, prs[0].Number)
	if err != nil {
		res.status, res.pr, res.err = mergeFailed, prs[0], err
		return res
	}

	res.pr = pr
	res.status = classifyMergeState(pr)
	return res
}

// applyMerges merges every approved pull request and returns how many failed.
func applyMerges(ctx context.Context, client *github.Client, profile *config.Profile,
	items []mergeResult, method string) int {
	fmt.Printf("\n⏳ Merging %d %s concurrently...\n",
		len(items), plural(len(items), "pull request", "pull requests"))

	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 5)
	var merged []string
	var errCount int32

	for _, item := range items {
		wg.Add(1)
		go func(repo string, number int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if _, err := client.MergePR(ctx, repo, number, method); err != nil {
				fmt.Printf("❌ [%s] Failed to merge #%d: %v\n", repo, number, err)
				atomic.AddInt32(&errCount, 1)
				return
			}

			fmt.Printf("✅ [%s] Merged #%d (%s)\n", repo, number, method)
			mu.Lock()
			merged = append(merged, repo)
			mu.Unlock()
		}(item.repo, item.pr.Number)
	}
	wg.Wait()

	sort.Strings(merged)
	fmt.Printf("\nMerge completed. Merged: %d, Errors: %d\n", len(merged), errCount)

	// Merging the release branch is what puts the base branch ahead, which is
	// exactly what sync exists to clean up.
	if len(merged) > 0 && !profile.SingleBranch() {
		fmt.Println(dimStyle.Render("Next: 'rel sync --auto' opens the sync pull requests."))
	}

	return int(errCount)
}

func init() {
	mergeCmd.Flags().BoolVar(&refreshMergeRepos, "refresh", false, "Bypass the repository cache and re-fetch from GitHub")
	mergeCmd.Flags().BoolVar(&autoMerge, "auto", false, "Find every open release pull request instead of picking repositories by hand")
	mergeCmd.Flags().StringVar(&mergeMethod, "method", "squash", "How to merge the pull requests (squash, merge, rebase)")
	mergeCmd.Flags().BoolVarP(&mergeYes, "yes", "y", false, "Skip the confirmation screen (requires --auto)")
	mergeCmd.Flags().BoolVar(&mergeDryRun, "dry-run", false, "Report what would be merged without merging anything (requires --auto)")
	_ = mergeCmd.RegisterFlagCompletionFunc("method",
		cobra.FixedCompletions(mergeMethods, cobra.ShellCompDirectiveNoFileComp))
	addProfileFlag(mergeCmd)
	rootCmd.AddCommand(mergeCmd)
}
