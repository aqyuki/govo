package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
		configPath = ""

		got, err := loadConfig()
		if err != nil || !got.Tests || got.Ignore.MissingReason != "off" {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}
	})

	t.Run("missing explicit config fails", func(t *testing.T) {
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
		configPath = ""

		got, err := loadConfig()
		if err != nil || got.Tests {
			t.Fatalf("loadConfig() = %+v, %v", got, err)
		}
	})

	t.Run("empty config uses defaults", func(t *testing.T) {
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

	t.Run("valid config ignores unknown keys", func(t *testing.T) {
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte("tests: false\nignore:\n  missing-reason: error\n  future: yes\nfuture: true\n"), 0o600); err != nil {
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
	}{
		{"invalid yaml", "tests: [\n"},
		{"invalid known type", "tests: wrong\n"},
		{"invalid known enum", "ignore:\n  missing-reason: maybe\n"},
		{"removed warning level", "ignore:\n  missing-reason: warning\n"},
		{"multiple documents", "tests: true\n---\ntests: false\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, "invalid.yaml")
			if err := os.WriteFile(path, []byte(tc.data), 0o600); err != nil {
				t.Fatal(err)
			}

			configPath = path

			if _, err := loadConfig(); err == nil {
				t.Fatal("loadConfig() succeeded for invalid configuration")
			}
		})
	}
}
