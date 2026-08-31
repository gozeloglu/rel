package cmd

import (
	"errors"
	"fmt"

	"github.com/gozeloglu/rel/pkg/cache"
	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/tui"
	"github.com/spf13/cobra"
)

var profileCmd = &cobra.Command{
	Use:               "profile",
	Short:             "Switch between and manage profiles",
	ValidArgsFunction: cobra.NoFileCompletions,
	Long: "Opens a picker to choose the active profile.\n" +
		"Use 'n' to create a new profile, 'e' to edit and 'd' to delete the highlighted one.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}

		if len(cfg.Profiles) == 0 {
			fmt.Println("No profiles yet, starting the setup wizard...")
			p, err := createProfile(cmd.Context(), cfg, nil)
			if err != nil {
				return abortAware(err, "Setup aborted. No profile was saved.")
			}
			fmt.Printf("\n✅ Profile '%s' saved and activated.\n", p.Name)
			return nil
		}

		return runProfilePicker(cmd, cfg)
	},
}

func runProfilePicker(cmd *cobra.Command, cfg *config.Config) error {
	for {
		current, _ := cfg.Current()

		items := make([]tui.PickerItem, 0, len(cfg.Profiles))
		for _, p := range cfg.Profiles {
			label := p.Name
			if current != nil && p.Name == current.Name {
				label += "  (active)"
			}
			items = append(items, tui.PickerItem{Label: label, Description: p.Summary()})
		}

		res, err := tui.Pick("Profiles", "Choose the profile to use", items, true)
		if err != nil {
			return abortAware(err, "No changes made.")
		}

		// The "new" shortcut works even when the filter hides every profile,
		// so there is not always a highlighted entry.
		var selected *config.Profile
		if res.Index >= 0 && res.Index < len(cfg.Profiles) {
			selected = cfg.Profiles[res.Index]
		}
		if selected == nil && res.Action != tui.PickNew {
			continue
		}

		switch res.Action {
		case tui.PickSelected:
			if err := cfg.SetCurrent(selected.Name); err != nil {
				return err
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Printf("✅ Now using profile '%s'\n   %s\n", selected.Name, selected.Summary())
			return nil

		case tui.PickNew:
			p, err := createProfile(cmd.Context(), cfg, nil)
			if err != nil {
				if errors.Is(err, tui.ErrAborted) {
					continue
				}
				return err
			}
			fmt.Printf("✅ Profile '%s' saved and activated.\n", p.Name)
			return nil

		case tui.PickEdit:
			p, err := createProfile(cmd.Context(), cfg, selected)
			if err != nil {
				if errors.Is(err, tui.ErrAborted) {
					continue
				}
				return err
			}
			fmt.Printf("✅ Profile '%s' updated.\n   %s\n", p.Name, p.Summary())
			return nil

		case tui.PickDelete:
			ok, err := tui.Confirm(fmt.Sprintf("Delete profile '%s'?", selected.Name),
				selected.Summary(), false)
			if err != nil {
				return abortAware(err, "No changes made.")
			}
			if !ok {
				continue
			}
			if err := deleteProfile(cfg, selected.Name); err != nil {
				return err
			}
			fmt.Printf("🗑️  Deleted profile '%s'\n", selected.Name)
			if len(cfg.Profiles) == 0 {
				return nil
			}
		}
	}
}

func deleteProfile(cfg *config.Config, name string) error {
	p, ok := cfg.Get(name)
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	_ = cache.Clear(repoCacheKey(p))

	if err := cfg.Delete(name); err != nil {
		return err
	}
	return cfg.Save()
}

var profileListCmd = &cobra.Command{
	Use:               "list",
	Short:             "List saved profiles",
	Args:              cobra.NoArgs,
	ValidArgsFunction: cobra.NoFileCompletions,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if len(cfg.Profiles) == 0 {
			fmt.Println("No profiles yet. Run 'rel init' to create one.")
			return nil
		}

		current, _ := cfg.Current()
		for _, p := range cfg.Profiles {
			marker := " "
			if current != nil && p.Name == current.Name {
				marker = "*"
			}
			fmt.Printf("%s %-20s %s\n", marker, p.Name, p.Summary())
		}
		return nil
	},
}

var profileUseCmd = &cobra.Command{
	Use:               "use <name>",
	Short:             "Set the active profile",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := cfg.SetCurrent(args[0]); err != nil {
			return err
		}
		if err := cfg.Save(); err != nil {
			return err
		}

		p, _ := cfg.Current()
		fmt.Printf("✅ Now using profile '%s'\n   %s\n", p.Name, p.Summary())
		return nil
	},
}

var profileDeleteCmd = &cobra.Command{
	Use:               "delete <name>",
	Short:             "Delete a profile",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: completeProfileNames,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if err := deleteProfile(cfg, args[0]); err != nil {
			return err
		}
		fmt.Printf("🗑️  Deleted profile '%s'\n", args[0])
		return nil
	},
}

// abortAware turns a user abort into a friendly message instead of an error.
func abortAware(err error, message string) error {
	if errors.Is(err, tui.ErrAborted) {
		fmt.Println("\n" + message)
		return nil
	}
	return err
}

func init() {
	profileCmd.AddCommand(profileListCmd, profileUseCmd, profileDeleteCmd)
	rootCmd.AddCommand(profileCmd)
}
