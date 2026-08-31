package cmd

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gozeloglu/rel/pkg/config"
	"github.com/gozeloglu/rel/pkg/github"
	"github.com/gozeloglu/rel/pkg/tui"
)

// runWizard walks the user through creating (or editing) a profile. When base
// is non-nil its values are offered as defaults.
func runWizard(ctx context.Context, base *config.Profile) (*config.Profile, error) {
	client, err := github.NewClient(nil)
	if err != nil {
		return nil, err
	}

	p := &config.Profile{}
	if base != nil {
		copied := *base
		p = &copied
	}

	if err := askOwner(ctx, client, p); err != nil {
		return nil, err
	}
	if err := askTeam(ctx, client, p); err != nil {
		return nil, err
	}
	if err := askFilters(p); err != nil {
		return nil, err
	}
	if err := askBranches(ctx, client, p); err != nil {
		return nil, err
	}
	if err := askName(p); err != nil {
		return nil, err
	}

	if err := p.Validate(); err != nil {
		return nil, err
	}
	return p, nil
}

func askOwner(ctx context.Context, client *github.Client, p *config.Profile) error {
	// Keep the last attempt around so a failed lookup does not force the user
	// to retype everything.
	last := p.Owner

	for {
		owner, err := tui.InputText(
			"GitHub organization or username",
			"Example: 'my-company' for an organization, or your own username",
			last)
		if err != nil {
			return err
		}
		if owner == "" {
			fmt.Println("⚠️  An organization or username is required.")
			continue
		}
		last = owner

		fmt.Printf("Looking up '%s'...\n", owner)
		ownerType, err := client.ResolveOwner(ctx, owner)
		if err != nil {
			fmt.Printf("❌ %v\n", err)
			continue
		}

		p.Owner, p.OwnerType = owner, ownerType
		fmt.Printf("✅ Found %s '%s'\n", ownerType, owner)
		return nil
	}
}

func askTeam(ctx context.Context, client *github.Client, p *config.Profile) error {
	if p.OwnerType != config.OwnerOrg {
		// Personal accounts have no teams.
		p.Team = ""
		return nil
	}

	last := p.Team

	for {
		team, err := tui.InputPrefilled(
			"Team slug (optional)",
			"Limit repositories to one team. Leave empty to use every repository of the organization.",
			last)
		if err != nil {
			return err
		}
		if team == "" {
			p.Team = ""
			return nil
		}
		last = team

		fmt.Printf("Checking team '%s/%s'...\n", p.Owner, team)
		if err := client.CheckTeam(ctx, p.Owner, team); err != nil {
			fmt.Printf("❌ %v\n", err)

			keep, cErr := tui.Confirm("Continue without a team?",
				"Answer 'no' to type a different team slug.", true)
			if cErr != nil {
				return cErr
			}
			if keep {
				p.Team = ""
				return nil
			}
			continue
		}

		p.Team = team
		fmt.Printf("✅ Team '%s' is reachable\n", team)
		return nil
	}
}

func askFilters(p *config.Profile) error {
	include, err := tui.InputPrefilled(
		"Repository name filter (optional)",
		"Comma separated globs to include, e.g. 'payment-*,billing-*'. Empty means every repository.",
		strings.Join(p.Include, ","))
	if err != nil {
		return err
	}

	exclude, err := tui.InputPrefilled(
		"Repositories to exclude (optional)",
		"Comma separated globs, e.g. '*-manifests,*-archive'.",
		strings.Join(p.Exclude, ","))
	if err != nil {
		return err
	}

	p.Include = config.ParsePatterns(include)
	p.Exclude = config.ParsePatterns(exclude)
	return nil
}

func askBranches(ctx context.Context, client *github.Client, p *config.Profile) error {
	baseDefault, devDefault := p.BaseBranch, p.DevBranch

	// Probe a repository so we can suggest real branch names.
	if baseDefault == "" || devDefault == "" {
		client.Profile = p

		fmt.Println("Detecting branch names...")
		repos, err := client.FetchRepos(ctx)
		if err != nil || len(repos) == 0 {
			if err != nil {
				fmt.Printf("⚠️  Could not list repositories (%v)\n", err)
			} else {
				fmt.Println("⚠️  No repositories matched the filters.")
			}
		} else if base, dev, err := client.DetectBranches(ctx, repos[0]); err != nil {
			fmt.Printf("⚠️  Could not inspect '%s' (%v)\n", repos[0], err)
		} else {
			fmt.Printf("✅ Detected from '%s': base '%s', dev '%s'\n", repos[0], base, dev)
			baseDefault, devDefault = base, dev
		}
	}

	if baseDefault == "" {
		baseDefault = "main"
	}
	if devDefault == "" {
		devDefault = baseDefault
	}

	base, err := tui.InputText("Release (base) branch",
		"Release pull requests are opened against this branch.", baseDefault)
	if err != nil {
		return err
	}

	dev, err := tui.InputText("Development branch",
		"Release branches are cut from this branch. Use the same name as the base branch for a single branch workflow.",
		devDefault)
	if err != nil {
		return err
	}

	p.BaseBranch, p.DevBranch = base, dev
	if p.SingleBranch() {
		fmt.Printf("ℹ️  Single branch workflow: '%s' is used for both sides, so 'rel sync' will be a no-op.\n", base)
	}
	return nil
}

func askName(p *config.Profile) error {
	suggested := p.Name
	if suggested == "" {
		suggested = p.Owner
		if p.Team != "" {
			suggested += "-" + p.Team
		}
		suggested = strings.ToLower(suggested)
	}

	name, err := tui.InputText("Profile name",
		"Used to switch between setups with 'rel profile'.", suggested)
	if err != nil {
		return err
	}
	if name == "" {
		return errors.New("profile name is required")
	}

	p.Name = name
	return nil
}
