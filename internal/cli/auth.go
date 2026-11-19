package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/domainreseller/cli/internal/config"
	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func (a *App) newAuthCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Log in, check and remove API credentials",
		Long: `Manage the API credentials stored in the config file.

Credentials are a Reseller ID (UUID) and an API key, both shown in the
DomainNameAPI reseller panel under API settings. Production and OTE (test)
are separate environments with different API keys; each is stored in its own
profile ("production" and "ote" by default).

Instead of storing credentials you can export DNA_RESELLER_ID and DNA_API_KEY
(and DNA_ENV=ote for the test environment) - environment variables win over
the config file.`,
		Args: cobra.NoArgs,
		RunE: groupRunE,
	}
	cmd.AddCommand(a.newLoginCmd(), a.newStatusCmd(), a.newLogoutCmd())
	return cmd
}

func (a *App) newLoginCmd() *cobra.Command {
	var keyStdin, noVerify bool
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Store and verify API credentials in a profile",
		Example: `  # Interactive
  dna auth login
  dna auth login --env ote

  # Non-interactive (CI, agents): key from stdin keeps it out of shell history
  echo "$KEY" | dna auth login --env ote --reseller-id 1b2c...-... --api-key-stdin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			envName := a.flags.env
			if a.flags.ote {
				envName = "ote"
			}
			env, err := dna.ParseEnvironment(envName)
			if err != nil {
				return usageErr(err)
			}
			profile := a.flags.profile
			if profile == "" {
				profile = string(env)
			}
			existing := cfg.Profiles[profile]
			if existing == nil {
				existing = &config.Profile{}
			}

			resellerID := strings.TrimSpace(a.flags.resellerID)
			apiKey := strings.TrimSpace(a.flags.apiKey)
			in := a.stdin()
			if keyStdin {
				b, err := io.ReadAll(in)
				if err != nil {
					return err
				}
				apiKey = strings.TrimSpace(string(b))
			}
			if resellerID == "" {
				if !a.interactive() {
					return usagef("--reseller-id is required when not running interactively")
				}
				prompt := "Reseller ID (UUID, from the panel's API settings)"
				if existing.ResellerID != "" {
					prompt += " [" + existing.ResellerID + "]"
				}
				fmt.Fprint(a.Err, prompt+": ")
				line, _ := in.ReadString('\n')
				resellerID = strings.TrimSpace(line)
				if resellerID == "" {
					resellerID = existing.ResellerID
				}
			}
			if !uuidRe.MatchString(resellerID) {
				return usagef("reseller ID %q is not a UUID (xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx)", resellerID)
			}
			if apiKey == "" {
				if !a.interactive() {
					return usagef("provide the API key with --api-key-stdin (or --api-key) when not running interactively")
				}
				fmt.Fprintf(a.Err, "API key for %s (input hidden): ", env)
				apiKey, err = a.readSecret(in)
				fmt.Fprintln(a.Err)
				if err != nil {
					return err
				}
			}
			if apiKey == "" {
				return usagef("API key is empty")
			}

			result := map[string]any{"profile": profile, "environment": env, "resellerId": resellerID, "verified": false, "configPath": cfg.Path()}
			if !noVerify {
				baseURL := env.BaseURL()
				if a.flags.baseURL != "" {
					baseURL = a.flags.baseURL
				} else if existing.BaseURL != "" {
					baseURL = existing.BaseURL
				}
				opts := []dna.Option{dna.WithBaseURL(baseURL), dna.WithUserAgent(a.userAgent())}
				if a.HTTPClient != nil {
					opts = append(opts, dna.WithHTTPClient(a.HTTPClient))
				}
				client, err := dna.NewClient(resellerID, apiKey, opts...)
				if err != nil {
					return err
				}
				acct, err := fetchAccount(cmd.Context(), client)
				if err != nil {
					return fmt.Errorf("credentials were not saved: %w", err)
				}
				result["verified"] = true
				result["account"] = acct
			}

			existing.Environment = string(env)
			existing.ResellerID = resellerID
			existing.APIKey = apiKey
			cfg.Profiles[profile] = existing
			if cfg.CurrentProfile == "" {
				cfg.CurrentProfile = profile
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			result["currentProfile"] = cfg.CurrentProfile
			if !a.machineOutput() {
				p := a.outP()
				fmt.Fprintln(a.Out)
				if acct, ok := result["account"].(*account); ok {
					fmt.Fprintf(a.Out, "  %s Logged in as %s\n", p.BoldFg(ui.Green, "✓"), p.Bold(acct.ResellerName))
					fmt.Fprintf(a.Out, "    %s %s USD · %s TRY\n", p.Muted("Balance"), p.Bold(fmtMoney(acct.UsdBalance)), p.Bold(fmtMoney(acct.TryBalance)))
				} else {
					fmt.Fprintf(a.Out, "  %s Saved without checking the credentials (--no-verify)\n", p.BoldFg(ui.Amber, "▲"))
				}
				fmt.Fprintf(a.Out, "    %s %s (%s) in %s\n", p.Muted("Profile"), p.Bold(profile), env, p.Muted(cfg.Path()))
				if cfg.CurrentProfile != profile {
					fmt.Fprintf(a.Out, "\n  %s\n", p.Hint(p.Muted("Use it with ")+p.Command("--profile "+profile)+p.Muted(" (or ")+p.Command("--env "+string(env))+p.Muted("), or make it the default: ")+p.Command("dna config use "+profile)))
				}
				fmt.Fprintln(a.Out)
				return nil
			}
			return a.printJSON(result)
		},
	}
	cmd.Flags().BoolVar(&keyStdin, "api-key-stdin", false, "Read the API key from standard input")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Save without checking the credentials against the API")
	return cmd
}

func (a *App) readSecret(in *bufio.Reader) (string, error) {
	if f, ok := a.In.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		b, err := term.ReadPassword(int(f.Fd()))
		return strings.TrimSpace(string(b)), err
	}
	line, err := in.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (a *App) newStatusCmd() *cobra.Command {
	var offline bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show which credentials are in effect and verify them",
		Long:  "Shows the effective profile, environment and where each value comes from, then calls the API to verify the credentials (unless --offline). Exits with code 3 when they do not work.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			r, err := a.resolved()
			if err != nil {
				return err
			}
			out := map[string]any{
				"profile": r.Profile, "profileSource": r.ProfileSource, "profileExists": r.ProfileExists,
				"environment": r.Environment, "environmentSource": r.EnvSource, "baseUrl": r.BaseURL, "baseUrlSource": r.BaseURLSource,
				"resellerId": r.ResellerID, "resellerIdSource": r.ResellerIDSource,
				"apiKey": config.MaskKey(r.APIKey), "apiKeySource": r.APIKeySource,
				"readOnly": r.ReadOnly, "readOnlySource": r.ReadOnlySource, "authenticated": false,
			}
			if !r.HasCredentials() {
				_ = a.printStatus(out)
				return dna.ErrNoCredentials
			}
			if offline {
				out["authenticated"] = "not checked (--offline)"
			} else {
				client, err := a.client()
				if err != nil {
					return err
				}
				acct, err := fetchAccount(cmd.Context(), client)
				if err != nil {
					_ = a.printStatus(out)
					return err
				}
				out["authenticated"] = true
				out["account"] = acct
			}
			return a.printStatus(out)
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "Do not call the API")
	return cmd
}

func (a *App) printStatus(out map[string]any) error {
	if a.machineOutput() {
		return a.printJSON(out)
	}
	if a.human() {
		fmt.Fprint(a.Out, a.humanStatus(out))
		return nil
	}
	keys := []string{"profile", "environment", "baseUrl", "resellerId", "apiKey", "readOnly", "authenticated"}
	for _, k := range keys {
		v := out[k]
		src := ""
		if s, ok := out[k+"Source"].(string); ok && s != "" {
			src = "  (" + s + ")"
		}
		if v == nil || v == "" {
			v = "-"
		}
		fmt.Fprintf(a.Out, "%-14s %v%s\n", k+":", v, src)
	}
	if acct, ok := out["account"].(*account); ok {
		fmt.Fprintf(a.Out, "%-14s %s (%s)\n", "reseller:", acct.ResellerName, acct.ResellerGroupName)
		fmt.Fprintf(a.Out, "%-14s %.2f USD / %.2f TRY\n", "balance:", acct.UsdBalance, acct.TryBalance)
	}
	return nil
}

func (a *App) newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove a profile's stored credentials",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			r, err := a.resolved()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[r.Profile]; !ok {
				return usagef("profile %q does not exist (profiles: %s)", r.Profile, strings.Join(cfg.ProfileNames(), ", "))
			}
			delete(cfg.Profiles, r.Profile)
			if cfg.CurrentProfile == r.Profile {
				cfg.CurrentProfile = ""
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			if a.machineOutput() {
				return a.printJSON(map[string]any{"removed": r.Profile})
			}
			fmt.Fprintf(a.Out, "Removed profile %q\n", r.Profile)
			return nil
		},
	}
}

func (a *App) newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Show and edit profiles and settings",
		Args:  cobra.NoArgs,
		RunE:  groupRunE,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: "Print the config file path",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			fmt.Fprintln(a.Out, a.configPath())
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show profiles (API keys masked) and the effective settings",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			r, err := a.resolved()
			if err != nil {
				return err
			}
			profiles := map[string]any{}
			for _, name := range cfg.ProfileNames() {
				p := cfg.Profiles[name]
				profiles[name] = map[string]any{"environment": p.Environment, "reseller_id": p.ResellerID, "api_key": config.MaskKey(p.APIKey), "base_url": p.BaseURL, "read_only": p.ReadOnly}
			}
			return a.printJSON(map[string]any{"path": cfg.Path(), "current_profile": cfg.CurrentProfile, "profiles": profiles, "effective": r})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use <profile>",
		Short: "Make a profile the default",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := a.config()
			if err != nil {
				return err
			}
			if _, ok := cfg.Profiles[args[0]]; !ok {
				return usagef("profile %q does not exist (profiles: %s)", args[0], strings.Join(cfg.ProfileNames(), ", "))
			}
			cfg.CurrentProfile = args[0]
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Fprintf(a.Err, "Default profile is now %q\n", args[0])
			return nil
		},
	})
	keys := map[string]func(p *config.Profile, v string) error{
		"environment": func(p *config.Profile, v string) error {
			e, err := dna.ParseEnvironment(v)
			p.Environment = string(e)
			return err
		},
		"reseller_id": func(p *config.Profile, v string) error {
			if v != "" && !uuidRe.MatchString(v) {
				return fmt.Errorf("%q is not a UUID", v)
			}
			p.ResellerID = v
			return nil
		},
		"api_key":  func(p *config.Profile, v string) error { p.APIKey = v; return nil },
		"base_url": func(p *config.Profile, v string) error { p.BaseURL = v; return nil },
		"read_only": func(p *config.Profile, v string) error {
			b, ok := parseBoolStr(v)
			if !ok {
				return fmt.Errorf("read_only must be true or false")
			}
			p.ReadOnly = b
			return nil
		},
	}
	names := make([]string, 0, len(keys))
	for k := range keys {
		names = append(names, k)
	}
	sort.Strings(names)
	cmd.AddCommand(&cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a profile value (" + strings.Join(names, ", ") + ")",
		Long:  "Sets a value in the selected profile (--profile, default: the current one). Prefer `dna auth login` for credentials.\n\nExample: make an AI-only profile that can never change anything:\n  dna config set read_only true --profile ote",
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			set, ok := keys[args[0]]
			if !ok {
				return usagef("unknown key %q (use %s)", args[0], strings.Join(names, ", "))
			}
			cfg, err := a.config()
			if err != nil {
				return err
			}
			r, err := a.resolved()
			if err != nil {
				return err
			}
			p := cfg.Profiles[r.Profile]
			if p == nil {
				p = &config.Profile{}
				cfg.Profiles[r.Profile] = p
			}
			if err := set(p, args[1]); err != nil {
				return usageErr(err)
			}
			if err := cfg.Save(); err != nil {
				return err
			}
			fmt.Fprintf(a.Err, "Set %s for profile %q\n", args[0], r.Profile)
			return nil
		},
	})
	return cmd
}

func parseBoolStr(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	}
	return false, false
}

// verifyCredentials is used by doctor.
func (a *App) verifyCredentials(ctx context.Context) (*account, error) {
	client, err := a.client()
	if err != nil {
		return nil, err
	}
	acct, err := fetchAccount(ctx, client)
	return acct, err
}

// account is the part of GET /api/v1/deposit/accounts/me the CLI shows.
type account struct {
	ResellerID        string  `json:"resellerId"`
	ResellerName      string  `json:"resellerName"`
	ResellerGroupName string  `json:"resellerGroupName"`
	TryBalance        float64 `json:"tryBalance"`
	UsdBalance        float64 `json:"usdBalance"`
}

// fetchAccount verifies credentials with the cheapest authenticated call.
func fetchAccount(ctx context.Context, client *dna.Client) (*account, error) {
	var acct account
	if _, err := client.Do(ctx, &dna.Request{Method: "GET", Path: "/api/v1/deposit/accounts/me"}, &acct); err != nil {
		return nil, err
	}
	return &acct, nil
}
