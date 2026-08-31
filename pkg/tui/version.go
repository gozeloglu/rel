package tui

import (
	"github.com/charmbracelet/huh"
)

// InputVersion shows a text input form to edit/confirm the version
func InputVersion(repo string, defaultVersion string) (string, error) {
	var version string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Next Version for " + repo).
				Description("Enter the version without 'v' prefix").
				Value(&version).
				Placeholder(defaultVersion),
		),
	)

	err := form.Run()
	
	if version == "" {
		version = defaultVersion
	}
	
	return version, err
}
