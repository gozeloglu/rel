package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/gozeloglu/rel/pkg/cache"
	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a new profile (organization/user, team, branches, filters)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		p, err := createProfile(cmd.Context(), cfg, nil)
		if err != nil {
			if errors.Is(err, tui.ErrAborted) {
				fmt.Println("\nSetup aborted. No profile was saved.")
				return nil
			}
			return err
		}

		fmt.Printf("\n✅ Profile '%s' saved and activated.\n   %s\n", p.Name, p.Summary())
		fmt.Println("\nRun 'rel release' or 'rel sync' to get started.")
		return nil
	},
}

// createProfile runs the wizard, stores the result and makes it active.
func createProfile(ctx context.Context, cfg *config.Config, base *config.Profile) (*config.Profile, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	p, err := runWizard(ctx, base)
	if err != nil {
		return nil, err
	}

	// Editing a profile under a new name should not leave a stale entry behind.
	if base != nil && base.Name != "" && base.Name != p.Name {
		_ = cfg.Delete(base.Name)
	}

	cfg.Add(p)
	if err := cfg.SetCurrent(p.Name); err != nil {
		return nil, err
	}
	if err := cfg.Save(); err != nil {
		return nil, err
	}

	// The repository list depends on owner/team/filters.
	_ = cache.Clear(repoCacheKey(p))
	return p, nil
}

func init() {
	rootCmd.AddCommand(initCmd)
}
