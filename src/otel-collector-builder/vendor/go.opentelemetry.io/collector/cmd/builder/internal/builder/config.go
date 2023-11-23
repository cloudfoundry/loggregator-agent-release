// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package builder // import "go.opentelemetry.io/collector/cmd/builder/internal/builder"

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/hashicorp/go-version"
	"go.uber.org/multierr"
	"go.uber.org/zap"
)

const defaultOtelColVersion = "0.89.0"

// ErrInvalidGoMod indicates an invalid gomod
var ErrInvalidGoMod = errors.New("invalid gomod specification for module")

// Config holds the builder's configuration
type Config struct {
	Logger          *zap.Logger
	SkipGenerate    bool   `mapstructure:"-"`
	SkipCompilation bool   `mapstructure:"-"`
	SkipGetModules  bool   `mapstructure:"-"`
	LDFlags         string `mapstructure:"-"`
	Verbose         bool   `mapstructure:"-"`

	Distribution Distribution `mapstructure:"dist"`
	Exporters    []Module     `mapstructure:"exporters"`
	Extensions   []Module     `mapstructure:"extensions"`
	Receivers    []Module     `mapstructure:"receivers"`
	Processors   []Module     `mapstructure:"processors"`
	Connectors   []Module     `mapstructure:"connectors"`
	Replaces     []string     `mapstructure:"replaces"`
	Excludes     []string     `mapstructure:"excludes"`
}

// Distribution holds the parameters for the final binary
type Distribution struct {
	Module               string `mapstructure:"module"`
	Name                 string `mapstructure:"name"`
	Go                   string `mapstructure:"go"`
	Description          string `mapstructure:"description"`
	OtelColVersion       string `mapstructure:"otelcol_version"`
	RequireOtelColModule bool   `mapstructure:"-"` // required for backwards-compatibility with builds older than 0.86.0
	OutputPath           string `mapstructure:"output_path"`
	Version              string `mapstructure:"version"`
	BuildTags            string `mapstructure:"build_tags"`
	DebugCompilation     bool   `mapstructure:"debug_compilation"`
}

// Module represents a receiver, exporter, processor or extension for the distribution
type Module struct {
	Name   string `mapstructure:"name"`   // if not specified, this is package part of the go mod (last part of the path)
	Import string `mapstructure:"import"` // if not specified, this is the path part of the go mods
	GoMod  string `mapstructure:"gomod"`  // a gomod-compatible spec for the module
	Path   string `mapstructure:"path"`   // an optional path to the local version of this module
}

// NewDefaultConfig creates a new config, with default values
func NewDefaultConfig() Config {
	log, err := zap.NewDevelopment()
	if err != nil {
		panic(fmt.Sprintf("failed to obtain a logger instance: %v", err))
	}

	outputDir, err := os.MkdirTemp("", "otelcol-distribution")
	if err != nil {
		log.Error("failed to obtain a temporary directory", zap.Error(err))
	}

	return Config{
		Logger: log,
		Distribution: Distribution{
			OutputPath:     outputDir,
			OtelColVersion: defaultOtelColVersion,
			Module:         "go.opentelemetry.io/collector/cmd/builder",
		},
	}
}

// Validate checks whether the current configuration is valid
func (c *Config) Validate() error {
	return multierr.Combine(
		validateModules(c.Extensions),
		validateModules(c.Receivers),
		validateModules(c.Exporters),
		validateModules(c.Processors),
		validateModules(c.Connectors),
	)
}

// SetGoPath sets go path
func (c *Config) SetGoPath() error {
	if !c.SkipCompilation || !c.SkipGetModules {
		// #nosec G204
		if _, err := exec.Command(c.Distribution.Go, "env").CombinedOutput(); err != nil { // nolint G204
			path, err := exec.LookPath("go")
			if err != nil {
				return ErrGoNotFound
			}
			c.Distribution.Go = path
		}
		c.Logger.Info("Using go", zap.String("go-executable", c.Distribution.Go))
	}
	return nil
}

func (c *Config) SetRequireOtelColModule() error {
	constraint, err := version.NewConstraint(">= 0.86.0")
	if err != nil {
		return err
	}

	otelColVersion, err := version.NewVersion(c.Distribution.OtelColVersion)
	if err != nil {
		return err
	}

	c.Distribution.RequireOtelColModule = constraint.Check(otelColVersion)
	return nil
}

// ParseModules will parse the Modules entries and populate the missing values
func (c *Config) ParseModules() error {
	var err error

	c.Extensions, err = parseModules(c.Extensions)
	if err != nil {
		return err
	}

	c.Receivers, err = parseModules(c.Receivers)
	if err != nil {
		return err
	}

	c.Exporters, err = parseModules(c.Exporters)
	if err != nil {
		return err
	}

	c.Processors, err = parseModules(c.Processors)
	if err != nil {
		return err
	}

	c.Connectors, err = parseModules(c.Connectors)
	if err != nil {
		return err
	}

	return nil
}

func validateModules(mods []Module) error {
	for _, mod := range mods {
		if mod.GoMod == "" {
			return fmt.Errorf("module %q: %w", mod.GoMod, ErrInvalidGoMod)
		}
	}
	return nil
}

func parseModules(mods []Module) ([]Module, error) {
	var parsedModules []Module
	for _, mod := range mods {
		if mod.Import == "" {
			mod.Import = strings.Split(mod.GoMod, " ")[0]
		}

		if mod.Name == "" {
			parts := strings.Split(mod.Import, "/")
			mod.Name = parts[len(parts)-1]
		}

		// Check if path is empty, otherwise filepath.Abs replaces it with current path ".".
		if mod.Path != "" {
			var err error
			mod.Path, err = filepath.Abs(mod.Path)
			if err != nil {
				return mods, fmt.Errorf("module has a relative \"path\" element, but we couldn't resolve the current working dir: %w", err)
			}
		}

		parsedModules = append(parsedModules, mod)
	}

	return parsedModules, nil
}
