package cmd

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
)

// detectStatus explains why a repository did or did not become an auto-sync
// candidate.
type detectStatus int

const (
	statusCandidate detectStatus = iota
	statusNotAhead
	statusStaleRelease
	statusNoRelease
	statusAlreadyOpen
	statusNoBranch
	statusFailed
)

// detectResult is the outcome of screening a single repository.
type detectResult struct {
	repo    string
	status  detectStatus
	sync    github.SyncState
	release github.ReleaseInfo
	prURL   string
	err     error
}

// clock is the time source, swapped out in tests.
var clock = time.Now

// note renders the annotation shown next to the repository in the confirmation
// screen.
func (r detectResult) note(now time.Time) string {
	if !r.release.Found() {
		return ""
	}
	return fmt.Sprintf("%s · %s ago", r.release.Tag, humanizeDuration(now.Sub(r.release.Published)))
}

// detectDeployedRepos screens every repository for a recent deploy that still
// needs a sync pull request. Screening runs in three short-circuiting stages so
// the expensive calls only happen for the few repositories that survive the
// cheap and highly selective "is base ahead of dev" filter.
func detectDeployedRepos(ctx context.Context, client *github.Client, repos []string, cutoff time.Time) []detectResult {
	results := make([]detectResult, len(repos))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 5)

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, name string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[idx] = classifyForAutoSync(ctx, client, name, cutoff)
		}(i, repo)
	}
	wg.Wait()

	return results
}

func classifyForAutoSync(ctx context.Context, client *github.Client, repo string, cutoff time.Time) detectResult {
	res := detectResult{repo: repo}

	state, err := client.CompareSyncState(ctx, repo)
	if err != nil {
		// A 404 means one of the two branches is absent, so the repository
		// simply does not take part in the base/dev workflow.
		if github.IsNotFound(err) {
			res.status = statusNoBranch
			return res
		}
		res.status, res.err = statusFailed, err
		return res
	}
	res.sync = state

	if !state.NeedsSync() {
		res.status = statusNotAhead
		return res
	}

	release, err := client.LatestRelease(ctx, repo)
	if err != nil {
		res.status, res.err = statusFailed, err
		return res
	}
	res.release = release

	// The open-PR check runs before the release window is applied, otherwise a
	// repository whose sync PR is already waiting for review would be reported
	// as actionable just because its release is old.
	prURL, err := client.FindOpenSyncPR(ctx, repo)
	if err != nil {
		res.status, res.err = statusFailed, err
		return res
	}
	if prURL != "" {
		res.status, res.prURL = statusAlreadyOpen, prURL
		return res
	}

	if !release.Found() {
		res.status = statusNoRelease
		return res
	}
	if release.Published.Before(cutoff) {
		res.status = statusStaleRelease
		return res
	}

	res.status = statusCandidate
	return res
}

// candidates returns the repositories that should get a sync pull request, in
// most-recently-released-first order.
func candidates(results []detectResult) []detectResult {
	var out []detectResult
	for _, r := range results {
		if r.status == statusCandidate {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].release.Published.After(out[j].release.Published)
	})
	return out
}

// countByStatus tallies the screening outcomes.
func countByStatus(results []detectResult) map[detectStatus]int {
	counts := make(map[detectStatus]int, len(results))
	for _, r := range results {
		counts[r.status]++
	}
	return counts
}

// humanizeDuration renders a coarse, human-friendly age such as "42m", "3h10m"
// or "12d".
func humanizeDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 48*time.Hour:
		h := int(d.Hours())
		if m := int(d.Minutes()) % 60; m > 0 {
			return fmt.Sprintf("%dh%dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	case d < 14*24*time.Hour:
		days := int(d.Hours()) / 24
		if h := int(d.Hours()) % 24; h > 0 {
			return fmt.Sprintf("%dd%dh", days, h)
		}
		return fmt.Sprintf("%dd", days)
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours())/24)
	default:
		days := int(d.Hours()) / 24
		years := days / 365
		if months := (days % 365) / 30; months > 0 {
			return fmt.Sprintf("%dy%dmo", years, months)
		}
		return fmt.Sprintf("%dy", years)
	}
}

// runAutoSync scans for repositories deployed inside the window and opens the
// sync pull requests they still need.
func runAutoSync(ctx context.Context, client *github.Client, profile *config.Profile, repoNames []string) error {
	now := clock()
	cutoff := now.Add(-autoSyncSince)
	window := humanizeDuration(autoSyncSince)

	fmt.Printf("Scanning %d repositories for releases in the last %s...\n\n", len(repoNames), window)

	results := detectDeployedRepos(ctx, client, repoNames, cutoff)

	fmt.Print(renderReport(buildReport(results, profile), profile, len(results), window))

	picked := candidates(results)
	if len(picked) == 0 {
		fmt.Println("\nNothing to sync. Exiting.")
		return errorsFrom(results)
	}

	if autoSyncDryRun {
		fmt.Printf("\n%s\n", sectionTitle(fmt.Sprintf("Dry run · %d sync %s would be created",
			len(picked), plural(len(picked), "PR", "PRs"))))
		for _, r := range picked {
			fmt.Printf("   %s %s\n", r.repo, dimStyle.Render("("+r.note(now)+")"))
		}
		return errorsFrom(results)
	}

	selected := make([]string, 0, len(picked))
	if autoSyncYes {
		for _, r := range picked {
			selected = append(selected, r.repo)
		}
	} else {
		items := make([]tui.RepoNote, 0, len(picked))
		for _, r := range picked {
			items = append(items, tui.RepoNote{Repo: r.repo, Note: r.note(now)})
		}

		chosen, err := tui.ConfirmRepos("Recently released · confirm sync PRs", items)
		if err != nil {
			if isAborted(err) {
				fmt.Println("\nOperation aborted by user. Exiting...")
				return nil
			}
			return err
		}
		selected = chosen
	}

	if len(selected) == 0 {
		fmt.Println("No repositories selected. Exiting.")
		return errorsFrom(results)
	}

	createSyncPRs(ctx, client, profile, selected, false)
	return errorsFrom(results)
}

// errorsFrom turns screening failures into a non-zero exit without hiding the
// work that did succeed.
func errorsFrom(results []detectResult) error {
	n := countByStatus(results)[statusFailed]
	if n == 0 {
		return nil
	}
	return fmt.Errorf("%d %s could not be checked", n, plural(n, "repository", "repositories"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
