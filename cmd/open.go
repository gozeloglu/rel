package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/gozeloglu/rel/pkg/browser"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

// openPRs is set by the shared --open flag.
var openPRs bool

// prLink pairs a repository with one of its pull requests.
type prLink struct {
	Repo string
	URL  string
}

// addOpenFlag registers the shared --open flag on a command.
func addOpenFlag(cmd *cobra.Command) {
	cmd.Flags().BoolVar(&openPRs, "open", false,
		"Open the resulting pull requests in your browser without asking")
}

// isInteractive reports whether both ends of the terminal are attached, which
// is what a confirmation prompt needs. Piped or CI runs answer false so they
// never block.
func isInteractive() bool {
	return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func isCharDevice(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// reportCreatedPRs prints every pull request that was opened, then offers to
// launch them in a browser.
func reportCreatedPRs(created []prLink) {
	reportPRs(fmt.Sprintf("Created %d %s",
		len(created), plural(len(created), "pull request", "pull requests")), created)
}

// reportPRs prints a numbered list of pull requests under a title, then offers
// to launch them in a browser.
func reportPRs(title string, prs []prLink) {
	if len(prs) == 0 {
		return
	}

	fmt.Printf("\n%s\n", sectionTitle(title))

	width, index := 0, len(strconv.Itoa(len(prs)))
	for _, pr := range prs {
		if n := len(pr.Repo); n > width {
			width = n
		}
	}
	for i, pr := range prs {
		fmt.Printf("   %*d. %-*s  %s\n", index, i+1, width, pr.Repo, pr.URL)
	}

	maybeOpenPRs(prs)
}

// Injection points so the open decision can be tested without a real browser.
var (
	openURLs      = browser.OpenAll
	interactive   = isInteractive
	confirmAction = tui.Confirm
)

// maybeOpenPRs applies the --open flag, falling back to a prompt when the
// command is running in a real terminal.
func maybeOpenPRs(created []prLink) {
	if len(created) == 0 {
		return
	}

	if !openPRs {
		if !interactive() {
			return
		}

		question := fmt.Sprintf("Open %d %s in your browser?",
			len(created), plural(len(created), "pull request", "pull requests"))

		want, err := confirmAction(question, "", false)
		if err != nil || !want {
			return
		}
	}

	urls := make([]string, 0, len(created))
	for _, pr := range created {
		urls = append(urls, pr.URL)
	}

	if err := openURLs(urls); err != nil {
		fmt.Printf("⚠️  Could not open the browser: %v\n", err)
	}
}
