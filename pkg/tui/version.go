package tui

// InputVersion shows a text input to edit/confirm the next version of a repo.
func InputVersion(repo string, defaultVersion string) (string, error) {
	return InputText(
		"Next Version for "+repo,
		"Enter the version without 'v' prefix",
		defaultVersion,
	)
}
