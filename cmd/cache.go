package cmd

import (
	"fmt"
	"time"

	"github.com/gozeloglu/rel/pkg/cache"
	"github.com/spf13/cobra"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the local repository cache",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the cached repository list",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := cache.Clear(repoCacheKey); err != nil {
			return err
		}
		fmt.Println("✅ Repository cache cleared.")
		return nil
	},
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the cached repository list status",
	RunE: func(cmd *cobra.Command, args []string) error {
		age, ok := cache.Age(repoCacheKey)
		if !ok {
			fmt.Println("No repository cache found.")
			return nil
		}

		var repos []string
		cache.Load(repoCacheKey, 100*365*24*time.Hour, &repos)
		state := "expired"
		if age <= cache.DefaultTTL {
			state = "fresh"
		}
		fmt.Printf("Repository cache: %d repos, %s old (%s, TTL %s)\n",
			len(repos), age.Truncate(1e9), state, cache.DefaultTTL)
		return nil
	},
}

func init() {
	cacheCmd.AddCommand(cacheClearCmd, cacheStatusCmd)
	rootCmd.AddCommand(cacheCmd)
}
