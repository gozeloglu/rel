package tui

import (
	"github.com/charmbracelet/huh"
)

// SelectRepos shows a multi-select form to choose repositories
func SelectRepos(repos []string) ([]string, error) {
	var selected []string

	options := make([]huh.Option[string], len(repos))
	for i, r := range repos {
		options[i] = huh.NewOption(r, r)
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select Repositories").
				Options(options...).
				Value(&selected),
		),
	)

	err := form.Run()
	return selected, err
}
