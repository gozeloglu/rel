package cmd

import (
	"fmt"
	"time"

	"github.com/gozeloglu/rel/pkg/cache"
	"github.com/gozeloglu/rel/pkg/config"
	"github.com/spf13/cobra"
)

var cacheAll bool

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the cached repository lists",
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear the cached repository list of the active profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		if cacheAll {
			n, err := cache.ClearAll()
			if err != nil {
				return err
			}
			fmt.Printf("✅ Cleared %d cache entries.\n", n)
			return nil
		}

		p, err := activeProfile()
		if err != nil {
			return err
		}
		if err := cache.Clear(repoCacheKey(p)); err != nil {
			return err
		}
		fmt.Printf("✅ Repository cache cleared for profile '%s'.\n", p.Name)
		return nil
	},
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show the cached repository list status",
	RunE: func(cmd *cobra.Command, args []string) error {
		p, err := activeProfile()
		if err != nil {
			return err
		}

		age, ok := cache.Age(repoCacheKey(p))
		if !ok {
			fmt.Printf("No repository cache for profile '%s'.\n", p.Name)
			return nil
		}

		var repos []string
		cache.Load(repoCacheKey(p), 100*365*24*time.Hour, &repos)

		state := "expired"
		if age <= cache.DefaultTTL {
			state = "fresh"
		}
		fmt.Printf("Profile '%s': %d repos, %s old (%s, TTL %s)\n",
			p.Name, len(repos), age.Truncate(time.Second), state, cache.DefaultTTL)
		return nil
	},
}

// activeProfile returns the configured profile without starting the wizard.
func activeProfile() (*config.Profile, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	p, err := cfg.Current()
	if err != nil {
		return nil, fmt.Errorf("%w — run 'rel init' first", err)
	}
	return p, nil
}

func init() {
	cacheClearCmd.Flags().BoolVar(&cacheAll, "all", false, "Clear the cache of every profile")
	cacheCmd.AddCommand(cacheClearCmd, cacheStatusCmd)
	rootCmd.AddCommand(cacheCmd)
}
