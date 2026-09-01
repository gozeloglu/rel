package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rel",
	Short: "rel is a TUI tool to automate GitHub release processes",
	Long: "A fast and beautiful TUI tool to automate periodic GitHub release processes\n" +
		"for any organization, team or personal account.\n\n" +
		"Run 'rel init' once to set up a profile, then use 'rel release', 'rel merge' and 'rel sync'.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
