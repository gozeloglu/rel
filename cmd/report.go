package cmd

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/gozeloglu/rel/pkg/config"
)

var (
	headingStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	ruleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	repoStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	goodStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	staleStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	badStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// reportWidth is the column budget used to wrap long name lists. COLUMNS is
// honoured when the shell exports it, otherwise a readable default is used.
func reportWidth() int {
	if raw := os.Getenv("COLUMNS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 60 {
			return min(n, 120)
		}
	}
	return 92
}

func sectionTitle(text string) string {
	return headingStyle.Render(text)
}

// reportGroup is one block of the scan report.
type reportGroup struct {
	status  detectStatus
	icon    string
	label   string
	style   lipgloss.Style
	results []detectResult
	// detailed groups get an aligned table, the rest only a wrapped name list.
	detailed bool
}

// buildReport arranges the screening results into display groups, ordered from
// most to least actionable.
func buildReport(results []detectResult, profile *config.Profile) []reportGroup {
	byStatus := make(map[detectStatus][]detectResult)
	for _, r := range results {
		byStatus[r.status] = append(byStatus[r.status], r)
	}

	groups := []reportGroup{
		{status: statusCandidate, icon: "✔", label: "READY TO SYNC", style: goodStyle, detailed: true},
		{status: statusAlreadyOpen, icon: "⏭", label: "SYNC PR ALREADY OPEN", style: dimStyle, detailed: true},
		{status: statusStaleRelease, icon: "⏳", label: "OUT OF SYNC · RELEASED BEFORE WINDOW", style: staleStyle, detailed: true},
		{status: statusNoRelease, icon: "⚠", label: "OUT OF SYNC · NEVER RELEASED", style: staleStyle, detailed: true},
		{status: statusFailed, icon: "✖", label: "COULD NOT BE CHECKED", style: badStyle, detailed: true},
		{status: statusNotAhead, icon: "✓", label: "ALREADY IN SYNC", style: dimStyle},
		{status: statusNoBranch, icon: "−",
			label: fmt.Sprintf("NOT APPLICABLE · no '%s' or '%s' branch", profile.BaseBranch, profile.DevBranch),
			style: dimStyle},
	}

	out := make([]reportGroup, 0, len(groups))
	for _, g := range groups {
		rows := byStatus[g.status]
		if len(rows) == 0 {
			continue
		}
		g.results = sortGroup(g.status, rows)
		out = append(out, g)
	}
	return out
}

// sortGroup orders a group so the most useful row comes first: freshly released
// repositories at the top of the actionable list, and the longest-forgotten
// repositories at the top of every stale list.
func sortGroup(status detectStatus, rows []detectResult) []detectResult {
	sorted := make([]detectResult, len(rows))
	copy(sorted, rows)

	switch status {
	case statusCandidate:
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].release.Published.After(sorted[j].release.Published)
		})
	case statusAlreadyOpen, statusStaleRelease, statusNoRelease:
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].sync.MergeBase.Before(sorted[j].sync.MergeBase)
		})
	default:
		sort.SliceStable(sorted, func(i, j int) bool {
			return sorted[i].repo < sorted[j].repo
		})
	}
	return sorted
}

// renderReport turns the groups into the printable scan report.
func renderReport(groups []reportGroup, profile *config.Profile, total int, window string) string {
	var sb strings.Builder

	scope := profile.Owner
	if profile.Team != "" {
		scope += "/" + profile.Team
	}

	sb.WriteString(sectionTitle(fmt.Sprintf("Scan · %s · %s → %s · window %s",
		scope, profile.BaseBranch, profile.DevBranch, window)))
	sb.WriteString("\n")
	sb.WriteString(ruleStyle.Render(strings.Repeat("─", reportWidth())))
	sb.WriteString("\n")

	for _, g := range groups {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("  %s  %s · %d\n",
			g.style.Render(g.icon), g.style.Render(g.label), len(g.results)))

		if g.detailed {
			sb.WriteString(renderDetailRows(g))
			continue
		}
		sb.WriteString(wrapNames(namesOf(g.results), "     ", reportWidth()))
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  waiting  = commits on " + profile.BaseBranch +
		" not yet in " + profile.DevBranch))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render("  diverged = age of the newest commit both branches share"))
	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf("  scanned %d %s",
		total, plural(total, "repository", "repositories"))))
	sb.WriteString("\n")

	return sb.String()
}

// renderDetailRows builds the aligned table for one group. Padding is applied
// to plain text before styling, because ANSI escapes would corrupt the widths.
func renderDetailRows(g reportGroup) string {
	type row struct{ repo, waiting, diverged, release, extra string }

	rows := make([]row, 0, len(g.results))
	for _, r := range g.results {
		rows = append(rows, row{
			repo:     r.repo,
			waiting:  waitingText(r),
			diverged: divergedText(r),
			release:  releaseText(r),
			extra:    extraText(r),
		})
	}

	var wRepo, wWaiting, wDiverged int
	for _, r := range rows {
		wRepo = max(wRepo, len(r.repo))
		wWaiting = max(wWaiting, len(r.waiting))
		wDiverged = max(wDiverged, len(r.diverged))
	}

	var sb strings.Builder
	for _, r := range rows {
		sb.WriteString("     ")
		sb.WriteString(repoStyle.Render(pad(r.repo, wRepo)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render(pad(r.waiting, wWaiting)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render(pad(r.diverged, wDiverged)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render(r.release))
		sb.WriteString("\n")

		if r.extra != "" {
			sb.WriteString("       " + dimStyle.Render("└ "+r.extra) + "\n")
		}
	}
	return sb.String()
}

func waitingText(r detectResult) string {
	if r.status == statusFailed || r.status == statusNoBranch {
		return "—"
	}
	return fmt.Sprintf("%d %s", r.sync.AheadBy, plural(r.sync.AheadBy, "commit", "commits"))
}

func divergedText(r detectResult) string {
	if r.sync.MergeBase.IsZero() {
		return "—"
	}
	return "diverged " + humanizeDuration(clock().Sub(r.sync.MergeBase)) + " ago"
}

func releaseText(r detectResult) string {
	if !r.release.Found() {
		return "never released"
	}
	return fmt.Sprintf("%s · %s", r.release.Tag, humanizeDuration(clock().Sub(r.release.Published)))
}

func extraText(r detectResult) string {
	switch r.status {
	case statusAlreadyOpen:
		return r.prURL
	case statusFailed:
		if r.err != nil {
			return r.err.Error()
		}
	}
	return ""
}

func namesOf(results []detectResult) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.repo)
	}
	return names
}

// wrapNames renders a comma separated list that folds at the report width.
func wrapNames(names []string, indent string, width int) string {
	if len(names) == 0 {
		return ""
	}

	var sb strings.Builder
	line := indent

	for i, name := range names {
		piece := name
		if i < len(names)-1 {
			piece += ","
		}

		if len(line) > len(indent) && len(line)+1+len(piece) > width {
			sb.WriteString(dimStyle.Render(line) + "\n")
			line = indent
		}
		if line != indent {
			line += " "
		}
		line += piece
	}

	if strings.TrimSpace(line) != "" {
		sb.WriteString(dimStyle.Render(line) + "\n")
	}
	return sb.String()
}

func pad(s string, width int) string {
	if n := width - len(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}
