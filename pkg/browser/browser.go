// Package browser opens URLs in the user's default web browser.
package browser

import (
	"errors"
	"fmt"
	"os/exec"
	"runtime"
)

// ErrUnsupported reports that no known way to launch a browser exists on this
// platform.
var ErrUnsupported = errors.New("opening a browser is not supported on this platform")

// command returns the launcher for the current platform.
func command(goos, url string) (string, []string, error) {
	switch goos {
	case "darwin":
		return "open", []string{url}, nil
	case "linux", "freebsd", "netbsd", "openbsd":
		return "xdg-open", []string{url}, nil
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}, nil
	default:
		return "", nil, ErrUnsupported
	}
}

// Open launches the default browser for a single URL.
func Open(url string) error {
	name, args, err := command(runtime.GOOS, url)
	if err != nil {
		return err
	}

	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%s is not available: %w", name, err)
	}

	return exec.Command(name, args...).Start()
}

// OpenAll opens every URL, continuing past failures so one broken launch does
// not hide the rest. It returns the first error encountered, if any.
func OpenAll(urls []string) error {
	var firstErr error
	for _, url := range urls {
		if err := Open(url); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
