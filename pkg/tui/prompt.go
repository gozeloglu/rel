package tui

import (
	"errors"
	"strings"

	"github.com/charmbracelet/huh"
)

// InputText asks for a single line of text. An empty answer falls back to
// defaultValue, which is also shown as the placeholder.
func InputText(title, description, defaultValue string) (string, error) {
	var value string

	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c", "esc")

	input := huh.NewInput().
		Title(title).
		Description(description).
		Value(&value)
	if defaultValue != "" {
		input = input.Placeholder(defaultValue)
	}

	err := huh.NewForm(huh.NewGroup(input)).
		WithTheme(huh.ThemeCharm()).
		WithKeyMap(keyMap).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return "", ErrAborted
	}
	if err != nil {
		return "", err
	}

	if value = strings.TrimSpace(value); value == "" {
		value = defaultValue
	}
	return value, nil
}

// Confirm asks a yes/no question.
func Confirm(title, description string, defaultValue bool) (bool, error) {
	value := defaultValue

	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c", "esc")

	err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(title).
			Description(description).
			Value(&value),
	)).WithTheme(huh.ThemeCharm()).WithKeyMap(keyMap).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return false, ErrAborted
	}
	return value, err
}

// InputPrefilled asks for optional text with the current value already typed in
// so it can be edited or deleted. Unlike InputText an empty answer stays empty.
func InputPrefilled(title, description, initial string) (string, error) {
	value := initial

	keyMap := huh.NewDefaultKeyMap()
	keyMap.Quit.SetKeys("ctrl+c", "esc")

	err := huh.NewForm(huh.NewGroup(
		huh.NewInput().
			Title(title).
			Description(description).
			Value(&value),
	)).WithTheme(huh.ThemeCharm()).WithKeyMap(keyMap).Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return "", ErrAborted
	}
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(value), nil
}
