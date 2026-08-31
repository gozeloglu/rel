package tui

import (
	"github.com/charmbracelet/huh"
)

// newRepoForm builds the repository multi-select form writing into selected.
func newRepoForm(repos []string, selected *[]string) *huh.Form {
	options := make([]huh.Option[string], len(repos))
	for i, r := range repos {
		options[i] = huh.NewOption(r, r)
	}

	// Quit is bound to ctrl+c only, so that "esc" stays available to the
	// multi-select field itself: while filtering it stops the filter input and
	// then clears the filter, instead of tearing down the whole program.
	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c")

	return huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select Repositories ('/' filter, 'esc' cancel filter, 'ctrl+c' quit)").
				Options(options...).
				Filterable(true).
				Value(selected),
		),
	).WithTheme(huh.ThemeCharm()).WithKeyMap(keyMap)
}

// SelectRepos shows a multi-select form to choose repositories
func SelectRepos(repos []string) ([]string, error) {
	var selected []string

	err := newRepoForm(repos, &selected).Run()
	return selected, err
}
