// Package config stores the user's rel profiles: which GitHub owner (org or
// user) to work with, an optional team, the branch names used by the release
// flow and the repository name filters.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// OwnerType tells whether a profile targets a GitHub organization or a user.
type OwnerType string

const (
	OwnerOrg  OwnerType = "org"
	OwnerUser OwnerType = "user"
)

// ErrNoProfiles is returned when no profile has been configured yet.
var ErrNoProfiles = errors.New("no profiles configured")

// Profile describes one GitHub context rel can operate on.
type Profile struct {
	Name       string    `json:"name"`
	Owner      string    `json:"owner"`
	OwnerType  OwnerType `json:"owner_type"`
	Team       string    `json:"team,omitempty"`
	BaseBranch string    `json:"base_branch"`
	DevBranch  string    `json:"dev_branch"`
	Include    []string  `json:"include,omitempty"`
	Exclude    []string  `json:"exclude,omitempty"`
}

// Config is the on-disk configuration file.
type Config struct {
	CurrentProfile string     `json:"current_profile"`
	Profiles       []*Profile `json:"profiles"`
}

// Path returns the location of the configuration file.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rel", "config.json"), nil
}

// Load reads the configuration file. A missing file yields an empty config.
func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}

	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", p, err)
	}
	return &cfg, nil
}

// Save writes the configuration file, creating the directory if needed.
func (c *Config) Save() error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o644)
}

// Get returns the profile with the given name.
func (c *Config) Get(name string) (*Profile, bool) {
	for _, p := range c.Profiles {
		if strings.EqualFold(p.Name, name) {
			return p, true
		}
	}
	return nil, false
}

// Current returns the active profile. It falls back to the only profile when
// current_profile is unset or dangling.
func (c *Config) Current() (*Profile, error) {
	if len(c.Profiles) == 0 {
		return nil, ErrNoProfiles
	}
	if p, ok := c.Get(c.CurrentProfile); ok {
		return p, nil
	}
	return c.Profiles[0], nil
}

// SetCurrent marks a profile as active.
func (c *Config) SetCurrent(name string) error {
	p, ok := c.Get(name)
	if !ok {
		return fmt.Errorf("profile %q not found", name)
	}
	c.CurrentProfile = p.Name
	return nil
}

// Add stores a profile, replacing an existing one with the same name.
func (c *Config) Add(p *Profile) {
	for i, existing := range c.Profiles {
		if strings.EqualFold(existing.Name, p.Name) {
			c.Profiles[i] = p
			return
		}
	}
	c.Profiles = append(c.Profiles, p)
}

// Delete removes a profile by name.
func (c *Config) Delete(name string) error {
	for i, p := range c.Profiles {
		if !strings.EqualFold(p.Name, name) {
			continue
		}
		c.Profiles = append(c.Profiles[:i], c.Profiles[i+1:]...)
		if strings.EqualFold(c.CurrentProfile, name) {
			c.CurrentProfile = ""
			if len(c.Profiles) > 0 {
				c.CurrentProfile = c.Profiles[0].Name
			}
		}
		return nil
	}
	return fmt.Errorf("profile %q not found", name)
}

// Names returns the profile names in file order.
func (c *Config) Names() []string {
	names := make([]string, 0, len(c.Profiles))
	for _, p := range c.Profiles {
		names = append(names, p.Name)
	}
	return names
}

// Validate reports whether the profile can be used for API calls.
func (p *Profile) Validate() error {
	switch {
	case p == nil:
		return errors.New("profile is nil")
	case strings.TrimSpace(p.Name) == "":
		return errors.New("profile name is required")
	case strings.TrimSpace(p.Owner) == "":
		return errors.New("owner (organization or user) is required")
	case p.OwnerType != OwnerOrg && p.OwnerType != OwnerUser:
		return fmt.Errorf("owner_type must be %q or %q", OwnerOrg, OwnerUser)
	case strings.TrimSpace(p.BaseBranch) == "":
		return errors.New("base_branch is required")
	case strings.TrimSpace(p.DevBranch) == "":
		return errors.New("dev_branch is required")
	case p.OwnerType == OwnerUser && p.Team != "":
		return errors.New("teams are only available for organizations")
	}

	for _, pattern := range append(append([]string{}, p.Include...), p.Exclude...) {
		if _, err := path.Match(pattern, "x"); err != nil {
			return fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
	}
	return nil
}

// SingleBranch reports whether the profile uses one branch for both the dev and
// the release side, which makes the sync flow meaningless.
func (p *Profile) SingleBranch() bool {
	return strings.EqualFold(p.BaseBranch, p.DevBranch)
}

// Matches reports whether a repository name passes the profile filters. An
// empty Include list matches everything; Exclude always wins.
func (p *Profile) Matches(repo string) bool {
	name := strings.ToLower(repo)

	for _, pattern := range p.Exclude {
		if globMatch(pattern, name) {
			return false
		}
	}
	if len(p.Include) == 0 {
		return true
	}
	for _, pattern := range p.Include {
		if globMatch(pattern, name) {
			return true
		}
	}
	return false
}

// FilterRepos returns the repository names accepted by the profile filters.
func (p *Profile) FilterRepos(repos []string) []string {
	out := make([]string, 0, len(repos))
	for _, r := range repos {
		if p.Matches(r) {
			out = append(out, r)
		}
	}
	return out
}

func globMatch(pattern, name string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if pattern == "" {
		return false
	}
	ok, err := path.Match(pattern, name)
	return err == nil && ok
}

// Fingerprint is a short hash of everything that changes which repositories a
// profile returns. It is used to invalidate caches.
func (p *Profile) Fingerprint() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00%s\x00%s",
		p.Owner, p.OwnerType, p.Team,
		strings.Join(p.Include, ","), strings.Join(p.Exclude, ","))
	return hex.EncodeToString(h.Sum(nil))[:8]
}

// Summary is a one line human readable description of the profile target.
func (p *Profile) Summary() string {
	target := p.Owner
	if p.Team != "" {
		target += "/" + p.Team
	}
	s := fmt.Sprintf("%s (%s) · %s → %s", target, p.OwnerType, p.DevBranch, p.BaseBranch)
	if len(p.Include) > 0 {
		s += " · include " + strings.Join(p.Include, ",")
	}
	if len(p.Exclude) > 0 {
		s += " · exclude " + strings.Join(p.Exclude, ",")
	}
	return s
}

// ParsePatterns splits a comma separated glob list entered by the user.
func ParsePatterns(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
