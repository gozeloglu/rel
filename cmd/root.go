package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "rel",
	Short: "rel is a TUI tool to automate Getir GitHub release processes",
	Long:  `A fast and beautiful TUI tool to automate periodic GitHub release processes for the Getir organization.`,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
