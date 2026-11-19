package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/domainreseller/cli/internal/dna"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestResolvePrecedence(t *testing.T) {
	f := &File{CurrentProfile: "production", Profiles: map[string]*Profile{
		"production": {Environment: "production", ResellerID: "prod-id", APIKey: "prod-key"},
		"ote":        {Environment: "ote", ResellerID: "ote-id", APIKey: "ote-key", ReadOnly: true},
	}}

	r, err := Resolve(f, Overrides{}, env(nil))
	if err != nil || r.Profile != "production" || r.APIKey != "prod-key" || r.BaseURL != dna.ProductionURL {
		t.Fatalf("defaults: %+v %v", r, err)
	}

	// --env ote picks the profile named "ote".
	r, _ = Resolve(f, Overrides{Env: "ote"}, env(nil))
	if r.Profile != "ote" || r.APIKey != "ote-key" || r.Environment != dna.OTE || r.BaseURL != dna.OTEURL || !r.ReadOnly {
		t.Errorf("--env ote: %+v", r)
	}

	// Environment variables beat the profile; flags beat environment variables.
	r, _ = Resolve(f, Overrides{APIKey: "flag-key"}, env(map[string]string{"DNA_API_KEY": "env-key", "DNA_RESELLER_ID": "env-id"}))
	if r.APIKey != "flag-key" || r.ResellerID != "env-id" || r.APIKeySource != "flag --api-key" {
		t.Errorf("precedence: %+v", r)
	}

	// DNA_TEST_MODE (other SDKs' convention) selects OTE.
	r, _ = Resolve(f, Overrides{}, env(map[string]string{"DNA_TEST_MODE": "yes"}))
	if r.Environment != dna.OTE || r.Profile != "ote" {
		t.Errorf("DNA_TEST_MODE: %+v", r)
	}

	// Forcing OTE onto a production profile warns about the key mismatch.
	r, _ = Resolve(f, Overrides{Profile: "production", Env: "ote"}, env(nil))
	if len(r.Warnings) != 1 {
		t.Errorf("expected a warning, got %v", r.Warnings)
	}

	if _, err := Resolve(f, Overrides{Env: "staging2"}, env(nil)); err == nil {
		t.Error("bad env accepted")
	}
}

func TestSaveLoadPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "config.json")
	f, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Profiles["x"] = &Profile{APIKey: "secret"}
	f.CurrentProfile = "x"
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(path)
		if st.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", st.Mode().Perm())
		}
	}
	g, err := Load(path)
	if err != nil || g.Profiles["x"].APIKey != "secret" || g.CurrentProfile != "x" {
		t.Errorf("reload: %+v %v", g, err)
	}
}

func TestMaskKey(t *testing.T) {
	if MaskKey("abcdefgh") != "********efgh" || MaskKey("ab") != "****" || MaskKey("") != "" {
		t.Error("MaskKey")
	}
}

func TestContactBook(t *testing.T) {
	f, err := Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !f.RememberContact("ote", "ote", Contact{Handle: "D-1"}) {
		t.Fatal("a new contact should change the book")
	}
	if !f.RememberContact("ote", "ote", Contact{Handle: "d-1", Name: "Ada Lovelace"}) {
		t.Error("new details should change the book")
	}
	if f.RememberContact("ote", "ote", Contact{Handle: "D-1"}) {
		t.Error("empty details must not change (or erase) anything")
	}
	f.RememberContact("ote", "ote", Contact{Handle: "D-2", Name: "Grace Hopper"})
	f.SetDefaultContact("ote", "ote", "D-2")
	cs, def := f.Contacts("ote")
	if def != "D-2" || len(cs) != 2 || cs[0].Handle != "D-2" || cs[1].Name != "Ada Lovelace" {
		t.Errorf("contacts = %+v, default %q", cs, def)
	}
	if f.Profiles["ote"].Environment != "ote" {
		t.Errorf("a profile created for contacts keeps its environment: %+v", f.Profiles["ote"])
	}
	if !f.ForgetContact("ote", "d-2") || f.ForgetContact("ote", "D-2") {
		t.Error("forget should remove the handle once")
	}
	if _, def := f.Contacts("ote"); def != "" {
		t.Errorf("forgetting the default clears it, got %q", def)
	}
	if cs, _ := f.Contacts("production"); cs != nil {
		t.Errorf("books are per profile: %+v", cs)
	}
}
