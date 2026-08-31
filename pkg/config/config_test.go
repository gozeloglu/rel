package config

import (
	"os"
	"path/filepath"
	"testing"
)

func withTempHome(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
}

func sampleProfile() *Profile {
	return &Profile{
		Name:       "getir-payments",
		Owner:      "Getir",
		OwnerType:  OwnerOrg,
		Team:       "payment-integrations",
		BaseBranch: "master",
		DevBranch:  "dev",
		Include:    []string{"payment-*"},
		Exclude:    []string{"*-manifests"},
	}
}

func TestLoadMissingFileReturnsEmptyConfig(t *testing.T) {
	withTempHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Profiles) != 0 {
		t.Fatalf("expected empty config, got %+v", cfg)
	}
	if _, err := cfg.Current(); err != ErrNoProfiles {
		t.Fatalf("expected ErrNoProfiles, got %v", err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	withTempHome(t)

	cfg := &Config{}
	cfg.Add(sampleProfile())
	if err := cfg.SetCurrent("getir-payments"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("config file not written: %v", err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	cur, err := got.Current()
	if err != nil {
		t.Fatal(err)
	}
	if cur.Owner != "Getir" || cur.Team != "payment-integrations" || cur.DevBranch != "dev" {
		t.Fatalf("round trip mismatch: %+v", cur)
	}
}

func TestAddReplacesSameName(t *testing.T) {
	cfg := &Config{}
	cfg.Add(sampleProfile())

	updated := sampleProfile()
	updated.Owner = "OtherOrg"
	cfg.Add(updated)

	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Profiles))
	}
	if cfg.Profiles[0].Owner != "OtherOrg" {
		t.Fatalf("profile was not replaced: %+v", cfg.Profiles[0])
	}
}

func TestDeleteMovesCurrent(t *testing.T) {
	cfg := &Config{}
	cfg.Add(sampleProfile())
	second := sampleProfile()
	second.Name = "personal"
	cfg.Add(second)

	if err := cfg.SetCurrent("getir-payments"); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Delete("getir-payments"); err != nil {
		t.Fatal(err)
	}
	if cfg.CurrentProfile != "personal" {
		t.Fatalf("current profile should fall back, got %q", cfg.CurrentProfile)
	}
	if err := cfg.Delete("missing"); err == nil {
		t.Fatal("expected error deleting unknown profile")
	}
}

func TestCurrentFallsBackWhenDangling(t *testing.T) {
	cfg := &Config{CurrentProfile: "gone"}
	cfg.Add(sampleProfile())

	cur, err := cfg.Current()
	if err != nil || cur.Name != "getir-payments" {
		t.Fatalf("expected fallback to first profile, got %v %v", cur, err)
	}
}

func TestMatches(t *testing.T) {
	p := sampleProfile()

	cases := map[string]bool{
		"payment-alpha":           true,
		"Payment-Beta":            true,
		"payment-core-manifests":  false,
		"billing-core":            false,
		"payment-alpha-manifests": false,
	}
	for repo, want := range cases {
		if got := p.Matches(repo); got != want {
			t.Errorf("Matches(%q) = %v, want %v", repo, got, want)
		}
	}

	// Empty include means "everything except the exclusions".
	p.Include = nil
	if !p.Matches("billing-core") {
		t.Error("empty include should match everything")
	}
	if p.Matches("billing-manifests") {
		t.Error("exclude should still apply")
	}
}

func TestFilterRepos(t *testing.T) {
	p := sampleProfile()
	got := p.FilterRepos([]string{"payment-a", "payment-a-manifests", "billing-b"})
	if len(got) != 1 || got[0] != "payment-a" {
		t.Fatalf("unexpected filter result %v", got)
	}
}

func TestValidate(t *testing.T) {
	if err := sampleProfile().Validate(); err != nil {
		t.Fatalf("valid profile rejected: %v", err)
	}

	userWithTeam := sampleProfile()
	userWithTeam.OwnerType = OwnerUser
	if err := userWithTeam.Validate(); err == nil {
		t.Error("user profile with a team should be rejected")
	}

	badGlob := sampleProfile()
	badGlob.Include = []string{"[bad"}
	if err := badGlob.Validate(); err == nil {
		t.Error("invalid glob should be rejected")
	}

	noBranch := sampleProfile()
	noBranch.DevBranch = ""
	if err := noBranch.Validate(); err == nil {
		t.Error("missing dev branch should be rejected")
	}
}

func TestFingerprintChangesWithFilters(t *testing.T) {
	a := sampleProfile()
	b := sampleProfile()
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("identical profiles should share a fingerprint")
	}

	b.Include = []string{"billing-*"}
	if a.Fingerprint() == b.Fingerprint() {
		t.Fatal("filters must affect the fingerprint")
	}

	// Branch names do not change which repos are listed.
	c := sampleProfile()
	c.BaseBranch = "main"
	if a.Fingerprint() != c.Fingerprint() {
		t.Fatal("branch names should not affect the fingerprint")
	}
}

func TestSingleBranch(t *testing.T) {
	p := sampleProfile()
	if p.SingleBranch() {
		t.Fatal("dev and master differ")
	}
	p.DevBranch = "Master"
	if !p.SingleBranch() {
		t.Fatal("expected single branch detection to be case-insensitive")
	}
}

func TestParsePatterns(t *testing.T) {
	got := ParsePatterns(" payment-* , *-manifests ,, ")
	if len(got) != 2 || got[0] != "payment-*" || got[1] != "*-manifests" {
		t.Fatalf("unexpected patterns %v", got)
	}
	if ParsePatterns("   ") != nil {
		t.Fatal("blank input should yield nil")
	}
}
