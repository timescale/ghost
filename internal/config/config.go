package config

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/timescale/ghost/internal/util"
)

type Config struct {
	APIURL       string `mapstructure:"api_url"`
	Analytics    bool   `mapstructure:"analytics"`
	Color        bool   `mapstructure:"color"`
	DocsMCPURL   string `mapstructure:"docs_mcp_url"`
	ReadOnly     bool   `mapstructure:"read_only"`
	ReleasesURL  string `mapstructure:"releases_url"`
	ShareURL     string `mapstructure:"share_url"`
	VersionCheck bool   `mapstructure:"version_check"`

	ConfigDir string         `mapstructure:"-"`
	flags     *pflag.FlagSet `mapstructure:"-"`
}

type ConfigOutput struct {
	APIURL       *string `mapstructure:"api_url" json:"api_url,omitempty"`
	Analytics    *bool   `mapstructure:"analytics" json:"analytics,omitempty"`
	Color        *bool   `mapstructure:"color" json:"color,omitempty"`
	DocsMCPURL   *string `mapstructure:"docs_mcp_url" json:"docs_mcp_url,omitempty"`
	ReadOnly     *bool   `mapstructure:"read_only" json:"read_only,omitempty"`
	ReleasesURL  *string `mapstructure:"releases_url" json:"releases_url,omitempty"`
	ShareURL     *string `mapstructure:"share_url" json:"share_url,omitempty"`
	VersionCheck *bool   `mapstructure:"version_check" json:"version_check,omitempty"`

	ConfigDir string       `mapstructure:"-" json:"-"`
	viper     *viper.Viper `mapstructure:"-" json:"-"`
}

const configFileName = "config.yaml"

const (
	defaultAPIURL       = "https://api.ghost.build/v0"
	defaultAnalytics    = true
	defaultColor        = true
	defaultDocsMCPURL   = "https://mcp.tigerdata.com/docs"
	defaultReadOnly     = false
	defaultReleasesURL  = "https://install.ghost.build"
	defaultShareURL     = "https://ghost.build/share"
	defaultVersionCheck = true
)

var publicDefaultValues = map[string]any{
	"analytics":     defaultAnalytics,
	"color":         defaultColor,
	"read_only":     defaultReadOnly,
	"version_check": defaultVersionCheck,
}

var privateDefaultValues = map[string]any{
	"api_url":      defaultAPIURL,
	"docs_mcp_url": defaultDocsMCPURL,
	"releases_url": defaultReleasesURL,
	"share_url":    defaultShareURL,
}

var defaultValues = func() map[string]any {
	m := map[string]any{}
	maps.Copy(m, publicDefaultValues)
	maps.Copy(m, privateDefaultValues)
	return m
}()

func PublicConfigOptions() []string {
	return slices.Collect(maps.Keys(publicDefaultValues))
}

const DefaultConfigDir = "~/.config/ghost"

// Load creates a new Config instance. The provided flag set is used to bind
// CLI flags (analytics, color, version-check) so they override file/env values,
// and to resolve the effective config directory (via the config-dir flag).
func Load(flags *pflag.FlagSet) (*Config, error) {
	cfg := &Config{
		ConfigDir: getEffectiveConfigDir(flags.Lookup("config-dir")),
		flags:     flags,
	}
	if err := cfg.reload(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// LoadForOutput loads config values for display purposes using a fresh viper
// instance, independent of CLI flags and global state.
func LoadForOutput(configDir string, env bool, all bool) (*ConfigOutput, error) {
	v := viper.New()

	configFile := getConfigFile(configDir)
	v.SetConfigFile(configFile)

	if env {
		applyEnvOverrides(v)
	}
	if all {
		applyDefaults(v)
	} else {
		applyPublicDefaults(v)
	}

	if err := readInConfig(v); err != nil {
		return nil, err
	}

	cfg := &ConfigOutput{
		ConfigDir: configDir,
		viper:     v,
	}

	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config for output: %w", err)
	}

	return cfg, nil
}

func getEffectiveConfigDir(configDirFlag *pflag.Flag) string {
	if configDirFlag.Changed {
		return util.ExpandPath(configDirFlag.Value.String())
	}

	if dir := os.Getenv("GHOST_CONFIG_DIR"); dir != "" {
		return util.ExpandPath(dir)
	}

	return util.ExpandPath(DefaultConfigDir)
}

func getConfigFile(dir string) string {
	return filepath.Join(dir, configFileName)
}

func applyEnvOverrides(v *viper.Viper) {
	v.SetEnvPrefix("GHOST")
	v.AutomaticEnv()
}

func applyDefaults(v *viper.Viper) {
	for key, val := range defaultValues {
		v.SetDefault(key, val)
	}
}

func applyPublicDefaults(v *viper.Viper) {
	for key, val := range publicDefaultValues {
		v.SetDefault(key, val)
	}
}

func readInConfig(v *viper.Viper) error {
	// Try to read config file if it exists
	// If file doesn't exist, that's okay - we'll use defaults and env vars
	if err := v.ReadInConfig(); err != nil &&
		!errors.As(err, &viper.ConfigFileNotFoundError{}) &&
		!errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

func bindFlags(v *viper.Viper, flags *pflag.FlagSet) error {
	return errors.Join(
		v.BindPFlag("analytics", flags.Lookup("analytics")),
		v.BindPFlag("color", flags.Lookup("color")),
		v.BindPFlag("version_check", flags.Lookup("version-check")),
	)
}

// reload reads the config file and resolves effective values through viper's
// normal precedence (flags > env > file > default). Called by Load for the
// initial load, and by Set/Unset/Reset after writing the config file.
func (c *Config) reload() error {
	v := viper.New()
	v.SetConfigFile(getConfigFile(c.ConfigDir))
	applyEnvOverrides(v)
	applyDefaults(v)

	if err := bindFlags(v, c.flags); err != nil {
		return fmt.Errorf("failed to bind flags: %w", err)
	}

	if err := readInConfig(v); err != nil {
		return err
	}

	if err := v.Unmarshal(c); err != nil {
		return fmt.Errorf("error unmarshaling config: %w", err)
	}
	return nil
}

func (c *Config) Set(key, val string) error {
	// Validate and convert the value to the correct type for the config file
	validated, err := validateValue(key, val)
	if err != nil {
		return err
	}

	// Write to config file
	configFile, err := c.ensureConfigDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(configFile)
	v.ReadInConfig()

	v.Set(key, validated)

	if err := v.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return c.reload()
}

func (c *Config) Unset(key string) error {
	configFile, err := c.ensureConfigDir()
	if err != nil {
		return err
	}

	vCurrent := viper.New()
	vCurrent.SetConfigFile(configFile)
	vCurrent.ReadInConfig()

	vNew := viper.New()
	vNew.SetConfigFile(configFile)

	_, validKey := defaultValues[key]
	for k, v := range vCurrent.AllSettings() {
		if k != key {
			vNew.Set(k, v)
		} else {
			validKey = true
		}
	}

	if !validKey {
		return fmt.Errorf("unknown configuration key: %s", key)
	}

	if err := vNew.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return c.reload()
}

func (c *Config) Reset() error {
	configFile, err := c.ensureConfigDir()
	if err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(configFile)

	if err := v.WriteConfigAs(configFile); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return c.reload()
}

func (c *Config) ensureConfigDir() (string, error) {
	if err := os.MkdirAll(c.ConfigDir, 0755); err != nil {
		return "", fmt.Errorf("error creating config directory: %w", err)
	}
	return getConfigFile(c.ConfigDir), nil
}

// validateValue validates and converts a user-provided value for the given
// config key. String values are returned as-is; bool keys accept "true"/"false"
// strings and are converted to bool. Returns the converted value suitable for
// writing to the config file.
func validateValue(key, val string) (any, error) {
	switch key {
	case "api_url", "docs_mcp_url", "releases_url", "share_url":
		return val, nil
	case "analytics", "color", "read_only", "version_check":
		return parseBool(key, val)
	default:
		return nil, fmt.Errorf("unknown configuration key: %s", key)
	}
}

func parseBool(key, val string) (bool, error) {
	b, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("invalid %s value: %s (must be true or false)", key, val)
	}
	return b, nil
}
