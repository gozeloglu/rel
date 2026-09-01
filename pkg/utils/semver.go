package utils

import (
	"fmt"
	"strings"

	"golang.org/x/mod/semver"
)

// ReleaseBranchPrefix is what every branch cut by the release flow starts with.
// It is also how an open release pull request is recognised later.
const ReleaseBranchPrefix = "release/"

// BumpMinor takes a tag string (e.g. "v1.21.0" or "1.21.0"), bumps the minor version,
// and returns the default version bump without 'v' prefix.
func BumpMinor(tag string) string {
	if tag == "" {
		return "1.0.0"
	}

	vTag := tag
	if !strings.HasPrefix(tag, "v") {
		vTag = "v" + tag
	}

	if !semver.IsValid(vTag) {
		return "1.0.0"
	}

	major := semver.Major(vTag)

	v := strings.TrimPrefix(vTag, major+".")
	parts := strings.Split(v, ".")
	var minor int
	if len(parts) > 0 {
		fmt.Sscanf(parts[0], "%d", &minor)
	}

	return fmt.Sprintf("%s.%d.0", strings.TrimPrefix(major, "v"), minor+1)
}

// GenerateBranchAndTag generates branch name and tag name from a given version (e.g., "1.22.0" or "v1.22.0")
func GenerateBranchAndTag(version string) (string, string) {
	cleanVersion := strings.TrimPrefix(version, "v")
	return ReleaseBranchPrefix + cleanVersion, "v" + cleanVersion
}
