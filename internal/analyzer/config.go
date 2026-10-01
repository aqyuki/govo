package analyzer

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

// rawConfig mirrors the YAML file. Pointers distinguish omitted keys.
type rawConfig struct {
	Tests  *bool           `yaml:"tests"`
	Ignore rawIgnoreConfig `yaml:"ignore"`
}

type rawIgnoreConfig struct {
	MissingReason *string `yaml:"missing-reason"`
}

// Config controls diagnostics emitted by the analyzer.
type Config struct {
	Tests  bool
	Ignore IgnoreConfig
}

// IgnoreConfig controls diagnostics about ignore directives.
type IgnoreConfig struct {
	MissingReason string
}

// Values accepted by ignore.missing-reason.
const (
	configMissingReasonOff = "off"
	//declscope:package // ignore.go reports ignore directives without a reason
	configMissingReasonError = "error"
)

var configPath string

// registerConfigFlag adds the shared configuration flag to the analyzer.
// Keeping the flag on Analyzer.Flags lets both the standalone command and
// go vet -vettool pass it through the analysis driver.
//
//declscope:package // analyzer.go registers it on Analyzer
func registerConfigFlag(flags *flag.FlagSet) {
	flags.StringVar(&configPath, "config", "", "path to a govo YAML configuration file")
}

func defaultConfig() Config {
	return Config{
		Tests:  true,
		Ignore: IgnoreConfig{MissingReason: configMissingReasonOff},
	}
}

// loadConfig reads configuration relative to this process's working
// directory. go vet starts its vettool once per package in that package's
// directory, so an explicit relative -config path is resolved there too.
//
//declscope:package // analyzer.go loads it for each package
func loadConfig() (Config, error) {
	path := configPath

	explicit := path != ""
	if !explicit {
		path = ".govo.yaml"
	}

	path, err := filepath.Abs(path)
	if err != nil {
		return Config{}, fmt.Errorf("resolve config path: %w", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) && !explicit {
			return defaultConfig(), nil
		}

		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	config := defaultConfig()

	var raw rawConfig

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return config, nil
		}

		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	var extra any
	if err := decoder.Decode(&extra); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	} else if err == nil {
		return Config{}, fmt.Errorf("parse config %q: multiple YAML documents are not supported", path)
	}

	if raw.Tests != nil {
		config.Tests = *raw.Tests
	}

	if raw.Ignore.MissingReason != nil {
		config.Ignore.MissingReason = *raw.Ignore.MissingReason
	}

	switch config.Ignore.MissingReason {
	case configMissingReasonOff, configMissingReasonError:
	default:
		return Config{}, fmt.Errorf("parse config %q: ignore.missing-reason must be off or error", path)
	}

	return config, nil
}
