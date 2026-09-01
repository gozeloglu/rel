package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
)

// mergeStatus explains why a repository did or did not end up in the merge set.
type mergeStatus int

const (
	mergeReady mergeStatus = iota
	mergeBlocked
	mergeConflict
	mergeDraft
	mergeMultiple
	mergeNoPR
	mergeUnselected
	mergeFailed
)

// mergeResult is the outcome of screening one repository for an open release
// pull request.
type mergeResult struct {
	repo   string
	status mergeStatus
	pr     github.ReleasePR
	// others holds the remaining pull requests when a repository has more than
	// one open release PR, a case rel refuses to guess its way out of.
	others []github.ReleasePR
	err    error
}

// classifyMergeState maps GitHub's mergeable_state onto the plan groups.
// "unstable" only means an optional check is red, which GitHub still merges, so
// the row stays actionable and carries a warning instead.
func classifyMergeState(pr github.ReleasePR) mergeStatus {
	if pr.Draft {
		return mergeDraft
	}

	switch pr.MergeableState {
	case "clean", "unstable", "has_hooks":
		return mergeReady
	case "dirty":
		return mergeConflict
	case "draft":
		return mergeDraft
	default:
		// blocked, behind and unknown all mean GitHub would refuse right now.
		return mergeBlocked
	}
}

// note renders the short annotation shown next to a row.
func (r mergeResult) note(baseBranch string) string {
	switch r.status {
	case mergeReady:
		if r.pr.MergeableState == "unstable" {
			return "⚠ checks failing"
		}
		return ""
	case mergeBlocked:
		switch r.pr.MergeableState {
		case "behind":
			return "behind " + baseBranch
		case "unknown", "":
			return "merge state unknown"
		default:
			return "checks or reviews pending"
		}
	case mergeConflict:
		return "conflicts with " + baseBranch
	case mergeDraft:
		return "still a draft"
	case mergeMultiple:
		return fmt.Sprintf("%d open release PRs", len(r.others)+1)
	case mergeNoPR:
		return "nothing open"
	case mergeUnselected:
		return "left out"
	default:
		return "could not be checked"
	}
}

// details are the indented follow-up lines of a row: where to go and look, or
// what went wrong.
func (r mergeResult) details() []string {
	switch r.status {
	case mergeBlocked, mergeConflict, mergeDraft:
		return []string{r.pr.URL}
	case mergeMultiple:
		urls := []string{r.pr.URL}
		for _, pr := range r.others {
			urls = append(urls, pr.URL)
		}
		return urls
	case mergeFailed:
		if r.err != nil {
			return []string{r.err.Error()}
		}
	}
	return nil
}

// prLabel is the "#91" column, or an em dash when there is no pull request.
func (r mergeResult) prLabel() string {
	if r.pr.Number == 0 {
		return "—"
	}
	return "#" + strconv.Itoa(r.pr.Number)
}

// headLabel is the release branch of the pull request.
func (r mergeResult) headLabel() string {
	if r.pr.Head == "" {
		return "—"
	}
	return r.pr.Head
}

// mergeGroup is one block of excluded rows.
type mergeGroup struct {
	icon    string
	label   string
	style   lipgloss.Style
	results []mergeResult
}

// excludedGroups arranges everything that will not be merged, most actionable
// first. Ready rows are not grouped because they are the plan itself.
func excludedGroups(results []mergeResult) []mergeGroup {
	byStatus := make(map[mergeStatus][]mergeResult)
	for _, r := range results {
		if r.status == mergeReady {
			continue
		}
		byStatus[r.status] = append(byStatus[r.status], r)
	}

	order := []struct {
		status mergeStatus
		icon   string
		label  string
		style  lipgloss.Style
	}{
		{mergeBlocked, "⏭", "BLOCKED · checks or reviews", staleStyle},
		{mergeConflict, "✖", "CONFLICT", badStyle},
		{mergeDraft, "◻", "DRAFT", dimStyle},
		{mergeMultiple, "⚠", "MORE THAN ONE OPEN RELEASE PR", staleStyle},
		{mergeFailed, "✖", "COULD NOT BE CHECKED", badStyle},
		{mergeNoPR, "−", "NO OPEN RELEASE PR", dimStyle},
		{mergeUnselected, "−", "NOT SELECTED", dimStyle},
	}

	out := make([]mergeGroup, 0, len(order))
	for _, g := range order {
		rows := byStatus[g.status]
		if len(rows) == 0 {
			continue
		}
		sort.SliceStable(rows, func(i, j int) bool { return rows[i].repo < rows[j].repo })
		out = append(out, mergeGroup{icon: g.icon, label: g.label, style: g.style, results: rows})
	}
	return out
}

// splitMergeResults separates what will be merged from what will not, both in
// repository order so the plan reads the same way twice.
func splitMergeResults(results []mergeResult) (ready, excluded []mergeResult) {
	for _, r := range results {
		if r.status == mergeReady {
			ready = append(ready, r)
			continue
		}
		excluded = append(excluded, r)
	}
	sort.SliceStable(ready, func(i, j int) bool { return ready[i].repo < ready[j].repo })
	return ready, excluded
}

// mergePlanHasWarning reports whether a row in the merge set needs a second
// look, which is what flips the confirmation default to "no".
func mergePlanHasWarning(items []mergeResult) bool {
	for _, it := range items {
		if it.pr.MergeableState == "unstable" {
			return true
		}
	}
	return false
}

// renderMergePlan builds the review screen shown before anything is merged.
// The merge method is part of the header because it is the one detail that
// cannot be seen anywhere else on the screen.
func renderMergePlan(items, excluded []mergeResult, profile *config.Profile, method string) string {
	var sb strings.Builder

	scope := profile.Owner
	if profile.Team != "" {
		scope += "/" + profile.Team
	}

	sb.WriteString("\n")
	sb.WriteString(sectionTitle(fmt.Sprintf("Merge plan · %s · → %s · method %s",
		scope, profile.BaseBranch, method)))
	sb.WriteString("\n")
	sb.WriteString(ruleStyle.Render(strings.Repeat("─", reportWidth())))
	sb.WriteString("\n\n")

	if len(items) == 0 {
		sb.WriteString(dimStyle.Render("   nothing to merge\n"))
	}
	sb.WriteString(renderMergeRows(items, profile.BaseBranch, "   "))

	if len(excluded) > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("   %s  %d %s excluded\n",
			staleStyle.Render("⚠"), len(excluded),
			plural(len(excluded), "repository", "repositories")))

		for _, g := range excludedGroups(excluded) {
			sb.WriteString(fmt.Sprintf("\n      %s  %s · %d\n",
				g.style.Render(g.icon), g.style.Render(g.label), len(g.results)))
			// Rows line up with the group label, not with its icon.
			sb.WriteString(renderMergeRows(g.results, profile.BaseBranch, "         "))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf(
		"   each pull request is merged into %s with %s", profile.BaseBranch, method)))
	sb.WriteString("\n")

	return sb.String()
}

// renderMergeRows builds an aligned table. Padding is applied to plain text
// before styling, because ANSI escapes would corrupt the widths.
func renderMergeRows(results []mergeResult, baseBranch, indent string) string {
	var wRepo, wPR, wHead int
	for _, r := range results {
		wRepo = max(wRepo, len([]rune(r.repo)))
		wPR = max(wPR, len([]rune(r.prLabel())))
		wHead = max(wHead, len([]rune(r.headLabel())))
	}

	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(indent)
		sb.WriteString(repoStyle.Render(pad(r.repo, wRepo)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render(pad(r.prLabel(), wPR)))
		sb.WriteString("  ")
		sb.WriteString(goodStyle.Render(pad(r.headLabel(), wHead)))

		if note := r.note(baseBranch); note != "" {
			style := dimStyle
			if r.status == mergeReady {
				style = staleStyle
			}
			sb.WriteString("  " + style.Render(note))
		}
		sb.WriteString("\n")

		for _, line := range r.details() {
			sb.WriteString(indent + "  " + dimStyle.Render("└ "+line) + "\n")
		}
	}
	return sb.String()
}
