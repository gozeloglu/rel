package tui

import (
	"errors"

	"github.com/charmbracelet/huh"
)

// InputVersion shows a text input form to edit/confirm the version
func InputVersion(repo string, defaultVersion string) (string, error) {
	var version string

	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c", "esc")

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Next Version for " + repo).
				Description("Enter the version without 'v' prefix").
				Value(&version).
				Placeholder(defaultVersion),
		),
	).WithTheme(huh.ThemeCharm()).WithKeyMap(keyMap)

	err := form.Run()
	if errors.Is(err, huh.ErrUserAborted) {
		err = ErrAborted
	}

	if version == "" {
		version = defaultVersion
	}

	return version, err
}
