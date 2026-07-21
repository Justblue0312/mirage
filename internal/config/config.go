package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config holds the mirage configuration loaded from a YAML config file.
type Config struct {
	Source        []string `yaml:"source"`
	MigrationsDir string   `yaml:"migrations_dir"`
	DB            string   `yaml:"db"`
	Idempotent    bool     `yaml:"idempotent"`
	Verbose       bool     `yaml:"verbose"`
}

// configFileName is the list of config file names to search, in order.
var configFileName = []string{"mirage.yaml", ".mirage.yaml"}

// Load searches the current working directory and its parents for
// mirage.yaml or .mirage.yaml, returning a zero-value Config (not an
// error) when none is found. A missing config file is the normal case
// for a project that hasn't adopted one yet, not a failure.
//
// Environment variables in the db field are expanded via os.Expand
// (${VAR} or $VAR syntax).
func Load() (*Config, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return &Config{}, "", nil
	}

	dir := cwd
	for {
		for _, name := range configFileName {
			path := filepath.Join(dir, name)
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			cfg, err := parse(data)
			if err != nil {
				return nil, "", fmt.Errorf("parsing %s: %w", path, err)
			}
			return cfg, path, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return &Config{}, "", nil
}

// parse parses YAML config data, expanding environment variables in string
// fields that contain ${...} or $VAR references.
func parse(data []byte) (*Config, error) {
	// First pass: raw YAML parse.
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	// Expand environment variables in all string values.
	expanded := make(map[string]any, len(raw))
	for k, v := range raw {
		if s, ok := v.(string); ok {
			expanded[k] = os.Expand(s, func(key string) string {
				if val, ok := os.LookupEnv(key); ok {
					return val
				}
				return "${" + key + "}"
			})
		} else {
			expanded[k] = v
		}
	}

	// Marshal back to YAML and re-parse into Config.
	expandedBytes, err := yaml.Marshal(expanded)
	if err != nil {
		return nil, fmt.Errorf("re-marshal expanded config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(expandedBytes, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// FirstNonEmpty returns the first non-empty string from the given values.
func FirstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
