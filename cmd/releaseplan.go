package cmd

import (
	"fmt"
	"strings"

	"github.com/gozeloglu/rel/pkg/config"
	"golang.org/x/mod/semver"
)

// releaseItem is one line of the release plan: the repository, the tag it sits
// on today and the version the user just asked for.
type releaseItem struct {
	Repo       string
	CurrentTag string
	Version    string
}

// bumpKind labels how a version compares to the tag it replaces.
type bumpKind int

const (
	bumpMajor bumpKind = iota
	bumpMinor
	bumpPatch
	bumpFirst
	bumpNotNewer
	bumpInvalid
)

// warning reports whether the label is one the user should look at twice.
func (b bumpKind) warning() bool {
	return b == bumpNotNewer || b == bumpInvalid
}

// semverTag normalises a tag or version into the "vX.Y.Z" form the semver
// package expects, returning false when it is not a valid version at all.
func semverTag(v string) (string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", false
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	if !semver.IsValid(v) {
		return "", false
	}
	return v, true
}

// classifyBump describes the step from currentTag to version. The result is
// purely informational: nothing here blocks the release.
func classifyBump(currentTag, version string) bumpKind {
	next, ok := semverTag(version)
	if !ok {
		return bumpInvalid
	}

	current, ok := semverTag(currentTag)
	if !ok {
		// No usable previous tag, so this is the first release we can reason about.
		return bumpFirst
	}

	if semver.Compare(next, current) <= 0 {
		return bumpNotNewer
	}

	switch {
	case semver.Major(next) != semver.Major(current):
		return bumpMajor
	case semver.MajorMinor(next) != semver.MajorMinor(current):
		return bumpMinor
	default:
		return bumpPatch
	}
}

// label renders the bump for the plan report.
func (b bumpKind) label(currentTag string) string {
	switch b {
	case bumpMajor:
		return "major"
	case bumpMinor:
		return "minor"
	case bumpPatch:
		return "patch"
	case bumpFirst:
		return "first release"
	case bumpNotNewer:
		return fmt.Sprintf("⚠ not newer than %s", displayTag(currentTag))
	default:
		return "⚠ not valid semver"
	}
}

// displayTag shows a tag the way GitHub does, or an em dash when there is none.
// Input that is not a version at all is shown verbatim so a typo stays readable.
func displayTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return "—"
	}
	if v, ok := semverTag(tag); ok {
		return v
	}
	return tag
}

// planHasWarning reports whether any row needs a second look, which is what
// flips the confirmation default to "no".
func planHasWarning(items []releaseItem) bool {
	for _, it := range items {
		if classifyBump(it.CurrentTag, it.Version).warning() {
			return true
		}
	}
	return false
}

// renderReleasePlan builds the review screen shown before any branch or pull
// request is created. skipped lists repositories dropped earlier because their
// latest tag could not be read.
func renderReleasePlan(items []releaseItem, skipped []string, profile *config.Profile) string {
	var sb strings.Builder

	scope := profile.Owner
	if profile.Team != "" {
		scope += "/" + profile.Team
	}

	sb.WriteString("\n")
	sb.WriteString(sectionTitle(fmt.Sprintf("Release plan · %s · → %s", scope, profile.BaseBranch)))
	sb.WriteString("\n")
	sb.WriteString(ruleStyle.Render(strings.Repeat("─", reportWidth())))
	sb.WriteString("\n\n")

	repoWidth, fromWidth, toWidth := 0, 0, 0
	for _, it := range items {
		repoWidth = max(repoWidth, len([]rune(it.Repo)))
		fromWidth = max(fromWidth, len([]rune(displayTag(it.CurrentTag))))
		toWidth = max(toWidth, len([]rune(displayTag(it.Version))))
	}

	for _, it := range items {
		kind := classifyBump(it.CurrentTag, it.Version)

		style := dimStyle
		if kind.warning() {
			style = staleStyle
		}

		sb.WriteString("   ")
		sb.WriteString(repoStyle.Render(pad(it.Repo, repoWidth)))
		sb.WriteString("  ")
		sb.WriteString(dimStyle.Render(pad(displayTag(it.CurrentTag), fromWidth)))
		sb.WriteString(dimStyle.Render(" → "))
		sb.WriteString(goodStyle.Render(pad(displayTag(it.Version), toWidth)))
		sb.WriteString("   ")
		sb.WriteString(style.Render(kind.label(it.CurrentTag)))
		sb.WriteString("\n")
	}

	if len(skipped) > 0 {
		sb.WriteString("\n")
		sb.WriteString(fmt.Sprintf("   %s  %d %s skipped · latest tag could not be read\n",
			staleStyle.Render("⚠"), len(skipped),
			plural(len(skipped), "repository", "repositories")))
		sb.WriteString(wrapNames(skipped, "      ", reportWidth()))
	}

	sb.WriteString("\n")
	sb.WriteString(dimStyle.Render(fmt.Sprintf(
		"   each repository gets a release/<version> branch and a pull request into %s",
		profile.BaseBranch)))
	sb.WriteString("\n")

	return sb.String()
}
