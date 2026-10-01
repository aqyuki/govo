package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetConfigCache forgets configuration read by earlier loads, before and
// after the test, so a test can rewrite a file and read it again.
func resetConfigCache(t *testing.T) {
	t.Helper()

	reset := func() {
		configCacheMu.Lock()
		defer configCacheMu.Unlock()

		configCache = make(map[configCacheKey]*configCacheEntry)
	}

	reset()
	t.Cleanup(reset)
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	oldPath := configPath

	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		configPath = oldPath
		_ = os.Chdir(oldDir)
	})

	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	t.Run("missing auto config uses defaults", func(t *testing.T) {
		resetConfigCache(t)

		configPath = ""

		got, err := loadConfig()
		if err != nil || !got.Tests || got.Ignore.MissingReason != "off" {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}
	})

	t.Run("missing explicit config fails", func(t *testing.T) {
		resetConfigCache(t)

		if err := os.WriteFile(filepath.Join(dir, ".govo.yaml"), []byte("tests: false\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		configPath = "missing.yaml"

		_, err := loadConfig()
		if err == nil || !strings.Contains(err.Error(), "missing.yaml") {
			t.Fatalf("loadConfig() error = %v", err)
		}
	})

	t.Run("auto config loads from current directory", func(t *testing.T) {
		resetConfigCache(t)

		configPath = ""

		got, err := loadConfig()
		if err != nil || got.Tests {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}
	})

	t.Run("empty config uses defaults", func(t *testing.T) {
		resetConfigCache(t)

		path := filepath.Join(dir, "empty.yaml")
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		configPath = path

		got, err := loadConfig()
		if err != nil || !got.Tests || got.Ignore.MissingReason != "off" {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}
	})

	t.Run("valid config loads known keys", func(t *testing.T) {
		resetConfigCache(t)

		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte("tests: false\nignore:\n  missing-reason: error\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		configPath = "config.yaml"

		got, err := loadConfig()
		if err != nil || got.Tests || got.Ignore.MissingReason != "error" {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}
	})

	for _, tc := range []struct {
		name string
		data string
		want string
	}{
		{"invalid yaml", "tests: [\n", ""},
		{"unknown top-level key", "tests: false\ntest: false\n", "line 2: field test not found"},
		{"unknown nested key", "ignore:\n  missing_reason: error\n", "line 2: field missing_reason not found"},
		{"invalid known type", "tests: wrong\n", ""},
		{"invalid known enum", "ignore:\n  missing-reason: maybe\n", ""},
		{"removed warning level", "ignore:\n  missing-reason: warning\n", ""},
		{"multiple documents", "tests: true\n---\ntests: false\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetConfigCache(t)

			path := filepath.Join(dir, "invalid.yaml")
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}

			configPath = path

			_, err := loadConfig()
			if err == nil {
				t.Fatal("loadConfig() succeeded for invalid configuration")
			}

			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("loadConfig() error = %v, want it to contain %q", err, tc.want)
			}
		})
	}

	t.Run("config is read once per resolved path", func(t *testing.T) {
		resetConfigCache(t)

		path := filepath.Join(dir, "cached.yaml")
		if err := os.WriteFile(path, []byte("tests: false\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		configPath = "cached.yaml"

		got, err := loadConfig()
		if err != nil || got.Tests {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}

		if err := os.WriteFile(path, []byte("tests: [\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				got, err := loadConfig()
				if err != nil || got.Tests {
					t.Errorf("loadConfig() = %+v, %v, want the cached configuration", got, err)
				}
			})
		}

		wg.Wait()
	})

	t.Run("errors are cached", func(t *testing.T) {
		resetConfigCache(t)

		configPath = "later.yaml"

		if _, err := loadConfig(); err == nil {
			t.Fatal("loadConfig() succeeded for a missing explicit configuration")
		}

		if err := os.WriteFile(filepath.Join(dir, "later.yaml"), []byte("tests: false\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		if _, err := loadConfig(); err == nil {
			t.Fatal("loadConfig() did not return the cached error")
		}
	})

	t.Run("cache is keyed by the resolved path", func(t *testing.T) {
		resetConfigCache(t)

		sub := filepath.Join(dir, "sub")
		if err := os.Mkdir(sub, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, ".govo.yaml"), []byte("tests: false\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		configPath = ""

		got, err := loadConfig()
		if err != nil || got.Tests {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}

		if err := os.Chdir(sub); err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = os.Chdir(dir) })

		got, err = loadConfig()
		if err != nil || !got.Tests {
			t.Fatalf("loadConfig() in a directory without .govo.yaml = %+v, %v", got, err)
		}

		configPath = filepath.Join(dir, ".govo.yaml")

		got, err = loadConfig()
		if err != nil || got.Tests {
			t.Fatalf("loadConfig() with an explicit path = %+v, %v", got, err)
		}
	})
}
