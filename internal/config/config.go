// Package config stores dna CLI profiles and resolves the effective settings
// from flags, environment variables and the config file.
//
// Precedence (highest first): command-line flags, environment variables,
// the selected profile in the config file, built-in defaults.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/domainreseller/cli/internal/dna"
)

// Environment variable names understood by the CLI.
const (
	EnvConfig   = "DNA_CONFIG"
	EnvProfile  = "DNA_PROFILE"
	EnvOutput   = "DNA_OUTPUT"
	EnvReadOnly = "DNA_READ_ONLY"
	EnvNoColor  = "NO_COLOR"
)

// Profile is one named set of credentials and settings.
type Profile struct {
	Environment string `json:"environment,omitempty"`
	ResellerID  string `json:"reseller_id,omitempty"`
	APIKey      string `json:"api_key,omitempty"`
	BaseURL     string `json:"base_url,omitempty"`
	// ReadOnly blocks every operation that changes something. Useful for
	// profiles handed to AI agents.
	ReadOnly bool `json:"read_only,omitempty"`
}

// File is the on-disk configuration.
type File struct {
	CurrentProfile string              `json:"current_profile,omitempty"`
	Profiles       map[string]*Profile `json:"profiles,omitempty"`

	path string
}

// DefaultProfile is used when nothing else selects a profile.
const DefaultProfile = "production"

// DefaultPath returns the config file location: $DNA_CONFIG, or
// <user config dir>/dna/config.json (%AppData%\dna on Windows,
// ~/Library/Application Support/dna on macOS, ~/.config/dna on Linux).
func DefaultPath(getenv func(string) string) string {
	if p := getenv(EnvConfig); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "dna", "config.json")
}

// Load reads the config file. A missing file yields an empty config.
func Load(path string) (*File, error) {
	f := &File{Profiles: map[string]*Profile{}, path: path}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, f); err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]*Profile{}
	}
	f.path = path
	return f, nil
}

// Path returns where the file is stored.
func (f *File) Path() string { return f.path }

// Save writes the file with owner-only permissions (it contains API keys).
func (f *File) Save() error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

// ProfileNames lists profiles alphabetically.
func (f *File) ProfileNames() []string {
	names := make([]string, 0, len(f.Profiles))
	for n := range f.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Overrides are values given on the command line.
type Overrides struct {
	Profile    string
	Env        string
	ResellerID string
	APIKey     string
	BaseURL    string
	ReadOnly   bool
}

// Resolved is the effective configuration and where each value came from.
type Resolved struct {
	Profile          string          `json:"profile"`
	ProfileSource    string          `json:"profileSource"`
	ProfileExists    bool            `json:"profileExists"`
	Environment      dna.Environment `json:"environment"`
	EnvSource        string          `json:"environmentSource"`
	ResellerID       string          `json:"resellerId,omitempty"`
	ResellerIDSource string          `json:"resellerIdSource,omitempty"`
	APIKey           string          `json:"-"`
	APIKeySource     string          `json:"apiKeySource,omitempty"`
	BaseURL          string          `json:"baseUrl"`
	BaseURLSource    string          `json:"baseUrlSource"`
	ReadOnly         bool            `json:"readOnly"`
	ReadOnlySource   string          `json:"readOnlySource,omitempty"`
	Warnings         []string        `json:"warnings,omitempty"`
}

// HasCredentials reports whether both credentials are set.
func (r *Resolved) HasCredentials() bool { return r.ResellerID != "" && r.APIKey != "" }

// Resolve computes the effective settings.
//
// Profile selection: --profile, then DNA_PROFILE, then (when an environment
// is forced with --env/DNA_ENV/DNA_TEST_MODE) the profile named after that
// environment, then current_profile, then "production". `dna auth login
// --env ote` saves to a profile named "ote", so `dna --env ote ...` just works.
func Resolve(f *File, o Overrides, getenv func(string) string) (*Resolved, error) {
	r := &Resolved{}

	var forcedEnv dna.Environment
	envSrc := ""
	switch {
	case o.Env != "":
		envSrc = "flag --env"
		e, err := dna.ParseEnvironment(o.Env)
		if err != nil {
			return nil, err
		}
		forcedEnv = e
	case getenv(dna.EnvEnv) != "":
		envSrc = "env " + dna.EnvEnv
		e, err := dna.ParseEnvironment(getenv(dna.EnvEnv))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dna.EnvEnv, err)
		}
		forcedEnv = e
	case getenv(dna.EnvTestMode) != "":
		envSrc = "env " + dna.EnvTestMode
		e, err := dna.ParseEnvironment(getenv(dna.EnvTestMode))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", dna.EnvTestMode, err)
		}
		forcedEnv = e
	}

	switch {
	case o.Profile != "":
		r.Profile, r.ProfileSource = o.Profile, "flag --profile"
	case getenv(EnvProfile) != "":
		r.Profile, r.ProfileSource = getenv(EnvProfile), "env "+EnvProfile
	case forcedEnv != "":
		r.Profile, r.ProfileSource = string(forcedEnv), envSrc
	case f.CurrentProfile != "":
		r.Profile, r.ProfileSource = f.CurrentProfile, "config current_profile"
	default:
		r.Profile, r.ProfileSource = DefaultProfile, "default"
	}
	p := f.Profiles[r.Profile]
	r.ProfileExists = p != nil
	if p == nil {
		p = &Profile{}
	}
	profSrc := "profile " + r.Profile

	switch {
	case forcedEnv != "":
		r.Environment, r.EnvSource = forcedEnv, envSrc
	case p.Environment != "":
		e, err := dna.ParseEnvironment(p.Environment)
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", r.Profile, err)
		}
		r.Environment, r.EnvSource = e, profSrc
	default:
		r.Environment, r.EnvSource = dna.Production, "default"
	}

	pick := func(flag, flagName, envName, prof string) (string, string) {
		switch {
		case strings.TrimSpace(flag) != "":
			return strings.TrimSpace(flag), "flag " + flagName
		case strings.TrimSpace(getenv(envName)) != "":
			return strings.TrimSpace(getenv(envName)), "env " + envName
		case prof != "":
			return prof, profSrc
		}
		return "", ""
	}
	r.ResellerID, r.ResellerIDSource = pick(o.ResellerID, "--reseller-id", dna.EnvResellerID, p.ResellerID)
	r.APIKey, r.APIKeySource = pick(o.APIKey, "--api-key", dna.EnvAPIKey, p.APIKey)
	r.BaseURL, r.BaseURLSource = pick(o.BaseURL, "--base-url", dna.EnvBaseURL, p.BaseURL)
	if r.BaseURL == "" {
		r.BaseURL, r.BaseURLSource = r.Environment.BaseURL(), "environment "+string(r.Environment)
	}

	switch {
	case o.ReadOnly:
		r.ReadOnly, r.ReadOnlySource = true, "flag --read-only"
	case truthy(getenv(EnvReadOnly)):
		r.ReadOnly, r.ReadOnlySource = true, "env "+EnvReadOnly
	case p.ReadOnly:
		r.ReadOnly, r.ReadOnlySource = true, profSrc
	}

	if p.Environment != "" && forcedEnv != "" && r.ProfileExists {
		if pe, _ := dna.ParseEnvironment(p.Environment); pe != forcedEnv && (r.APIKeySource == profSrc) {
			r.Warnings = append(r.Warnings, fmt.Sprintf("profile %q is configured for %s but %s selects %s; its API key will most likely be rejected (production and OTE keys differ)", r.Profile, pe, envSrc, forcedEnv))
		}
	}
	return r, nil
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// MaskKey hides all but the last 4 characters of a secret.
func MaskKey(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", 8) + s[len(s)-4:]
}
