package cache

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultTTL is how long a cache entry stays valid.
const DefaultTTL = 30 * time.Minute

type entry struct {
	CreatedAt time.Time       `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

func path(name string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "rel", name+".json"), nil
}

// Load reads a cache entry into v. It reports whether a fresh entry (younger
// than ttl) was found. Any read/parse problem is treated as a cache miss.
func Load(name string, ttl time.Duration, v any) bool {
	p, err := path(name)
	if err != nil {
		return false
	}

	b, err := os.ReadFile(p)
	if err != nil {
		return false
	}

	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		return false
	}
	if time.Since(e.CreatedAt) > ttl {
		return false
	}

	return json.Unmarshal(e.Data, v) == nil
}

// Save writes v to the cache under the given name.
func Save(name string, v any) error {
	p, err := path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	b, err := json.Marshal(entry{CreatedAt: time.Now(), Data: data})
	if err != nil {
		return err
	}

	return os.WriteFile(p, b, 0o644)
}

// Age returns the age of a cache entry and whether it exists.
func Age(name string) (time.Duration, bool) {
	p, err := path(name)
	if err != nil {
		return 0, false
	}

	b, err := os.ReadFile(p)
	if err != nil {
		return 0, false
	}

	var e entry
	if err := json.Unmarshal(b, &e); err != nil {
		return 0, false
	}

	return time.Since(e.CreatedAt), true
}

// Clear removes a cache entry.
func Clear(name string) error {
	p, err := path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Names lists the entries currently stored in the cache directory.
func Names() ([]string, error) {
	p, err := path("x")
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(filepath.Dir(p))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	return names, nil
}

// ClearAll removes every cache entry and reports how many were deleted.
func ClearAll() (int, error) {
	names, err := Names()
	if err != nil {
		return 0, err
	}
	for _, n := range names {
		if err := Clear(n); err != nil {
			return 0, err
		}
	}
	return len(names), nil
}
