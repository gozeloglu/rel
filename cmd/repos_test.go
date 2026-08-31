package cmd

import (
	"strings"
	"testing"

	"github.com/gozeloglu/rel/pkg/config"
)

func testProfile() *config.Profile {
	return &config.Profile{
		Name:       "getir payments/1",
		Owner:      "Getir",
		OwnerType:  config.OwnerOrg,
		Team:       "payment-integrations",
		BaseBranch: "master",
		DevBranch:  "dev",
		Include:    []string{"payment-*"},
	}
}

func TestRepoCacheKeyIsFilesystemSafe(t *testing.T) {
	key := repoCacheKey(testProfile())

	if strings.ContainsAny(key, " /\\") {
		t.Fatalf("cache key must not contain path separators or spaces: %q", key)
	}
	if !strings.HasPrefix(key, "repos-getir-payments-1-") {
		t.Fatalf("unexpected key %q", key)
	}
}

func TestRepoCacheKeyChangesWithFilters(t *testing.T) {
	a := testProfile()
	b := testProfile()
	if repoCacheKey(a) != repoCacheKey(b) {
		t.Fatal("identical profiles must share a cache key")
	}

	b.Include = []string{"billing-*"}
	if repoCacheKey(a) == repoCacheKey(b) {
		t.Fatal("changing the filters must invalidate the cache")
	}

	c := testProfile()
	c.Team = ""
	if repoCacheKey(a) == repoCacheKey(c) {
		t.Fatal("changing the team must invalidate the cache")
	}
}

func TestRepoCacheKeysDifferPerProfile(t *testing.T) {
	a := testProfile()
	b := testProfile()
	b.Name = "personal"
	if repoCacheKey(a) == repoCacheKey(b) {
		t.Fatal("different profiles must not share a cache entry")
	}
}
