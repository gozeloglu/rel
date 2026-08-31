package cache

import (
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	in := []string{"payment-a", "payment-b"}
	if err := Save("test-repos", in); err != nil {
		t.Fatal(err)
	}
	var out []string
	if !Load("test-repos", DefaultTTL, &out) || len(out) != 2 || out[0] != "payment-a" {
		t.Fatalf("load failed: %v", out)
	}
	if Load("test-repos", time.Nanosecond, &out) {
		t.Fatal("expected expiry miss")
	}
	if _, ok := Age("test-repos"); !ok {
		t.Fatal("expected age")
	}
	if err := Clear("test-repos"); err != nil {
		t.Fatal(err)
	}
	if Load("test-repos", DefaultTTL, &out) {
		t.Fatal("expected miss after clear")
	}
}
