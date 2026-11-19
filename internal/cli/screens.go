package cli

import (
	"fmt"
	"regexp"
	"strings"
	"text/template"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ui"
)

// displayVersion shortens a version for the home screen: Go pseudo-versions
// of local builds ("1.0.1-0.20261119121952-143bc31a99ef+dirty") become "dev".
func displayVersion(v string) string {
	if i := strings.Index(v, "+"); i > 0 {
		v = v[:i]
	}
	if v == "" || v == "dev" || strings.Contains(v, "-0.") {
		return "dev build"
	}
	return "v" + strings.TrimPrefix(v, "v")
}

// ---------------------------------------------------------------- help

// usageTemplate is cobra's default usage template; dna renders it itself so
// it can add colors on a terminal.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

Available Commands:{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

Additional Commands:{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

var usageTmpl = template.Must(template.New("usage").Funcs(template.FuncMap{
	"rpad":                    func(s string, n int) string { return fmt.Sprintf("%-*s", n, s) },
	"trimTrailingWhitespaces": func(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) },
}).Parse(usageTemplate))

// setupHelp makes help and usage output colored on a terminal. The text is
// the same as cobra's default; only escape sequences are added.
func (a *App) setupHelp(root *cobra.Command) {
	root.SetUsageFunc(func(c *cobra.Command) error {
		var sb strings.Builder
		if err := usageTmpl.Execute(&sb, c); err != nil {
			return err
		}
		_, err := fmt.Fprint(c.OutOrStderr(), a.colorHelp(sb.String()))
		return err
	})
	root.SetHelpFunc(func(c *cobra.Command, _ []string) {
		var sb strings.Builder
		text := c.Long
		if text == "" {
			text = c.Short
		}
		if text != "" {
			sb.WriteString(strings.TrimRightFunc(text, unicode.IsSpace) + "\n\n")
		}
		out := a.colorHelp(sb.String())
		if c.Runnable() || c.HasSubCommands() {
			out += c.UsageString()
		}
		fmt.Fprint(c.OutOrStdout(), out)
	})
}

var (
	helpHeading = regexp.MustCompile(`^[A-Z][A-Za-z()/-]*( [A-Za-z()/-]+){0,3}:$`)
	helpComment = regexp.MustCompile(`^(\s+)(#.*)$`)
	helpCommand = regexp.MustCompile(`^(\s+)((?:echo .*\| )?dna(?: .*?)?)(\s{2,}#.*)?$`)
	helpFlag    = regexp.MustCompile(`^(\s+)(-\w, )?(--[a-z0-9-]+)( [a-zA-Z]+)?(\s{2,}.*)?$`)
	helpSubcmd  = regexp.MustCompile(`^  ([a-z][a-z0-9-]*)(\s{2,})(.*)$`)
	helpSafety  = regexp.MustCompile(`^(Safety:\s+)(BILLABLE|DESTRUCTIVE)(.*)$`)
)

// colorHelp adds colors to help text when a person reads it on a terminal.
func (a *App) colorHelp(s string) string {
	if !a.human() {
		return s
	}
	p := a.outP()
	if !p.On() {
		return s
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		switch {
		case helpHeading.MatchString(l):
			lines[i] = p.BoldFg(ui.Accent, l)
		case helpComment.MatchString(l):
			m := helpComment.FindStringSubmatch(l)
			lines[i] = m[1] + p.Muted(m[2])
		case helpCommand.MatchString(l):
			m := helpCommand.FindStringSubmatch(l)
			lines[i] = m[1] + p.Command(m[2]) + p.Muted(m[3])
		case helpFlag.MatchString(l):
			m := helpFlag.FindStringSubmatch(l)
			lines[i] = m[1] + p.Fg(ui.Accent, m[2]+m[3]) + p.Muted(m[4]) + m[5]
		case helpSubcmd.MatchString(l):
			m := helpSubcmd.FindStringSubmatch(l)
			lines[i] = "  " + p.Bold(m[1]) + m[2] + m[3]
		case helpSafety.MatchString(l):
			m := helpSafety.FindStringSubmatch(l)
			c := ui.Amber
			if m[2] == "DESTRUCTIVE" {
				c = ui.Red
			}
			lines[i] = m[1] + p.BoldFg(c, m[2]) + m[3]
		}
	}
	return strings.Join(lines, "\n")
}

// ---------------------------------------------------------------- status

// humanStatus renders `dna auth status` for a person.
func (a *App) humanStatus(out map[string]any) string {
	p := a.outP()
	var sb strings.Builder
	profile, _ := out["profile"].(string)
	if profile == "" {
		profile = "(no profile)"
	}
	env := fmt.Sprint(out["environment"])
	base, _ := out["baseUrl"].(string)
	envText := p.BoldFg(ui.Red, strings.ToUpper(env))
	if env == string(dna.OTE) {
		envText = p.BoldFg(ui.Green, "OTE") + p.Muted(" (test)")
	}
	sb.WriteString("\n  " + p.BoldFg(ui.Accent, profile) + "  " + envText + p.Muted(" · "+strings.TrimPrefix(strings.TrimPrefix(base, "https://"), "http://")) + "\n\n")

	src := func(k string) string {
		if s, ok := out[k+"Source"].(string); ok && s != "" {
			return "  " + p.Muted("("+s+")")
		}
		return ""
	}
	val := func(k string) string {
		s := fmt.Sprint(out[k])
		if out[k] == nil || s == "" {
			return p.Muted("–")
		}
		return s
	}
	var pairs [][2]string
	if acct, ok := out["account"].(*account); ok {
		name := p.Bold(acct.ResellerName)
		if acct.ResellerGroupName != "" {
			name += "  " + p.Badge(ui.Navy, acct.ResellerGroupName)
		}
		pairs = append(pairs, [2]string{"Reseller", name})
		pairs = append(pairs, [2]string{"Balance", p.Bold(fmtMoney(acct.UsdBalance)) + p.Muted(" USD · ") + p.Bold(fmtMoney(acct.TryBalance)) + p.Muted(" TRY")})
	}
	pairs = append(pairs,
		[2]string{"Reseller ID", val("resellerId") + src("resellerId")},
		[2]string{"API key", val("apiKey") + src("apiKey")},
		[2]string{"Environment", env + src("environment")},
	)
	ro := p.Muted("off")
	if b, _ := out["readOnly"].(bool); b {
		ro = p.Badge(ui.Amber, "ON") + src("readOnly")
	}
	pairs = append(pairs, [2]string{"Read-only", ro})
	switch v := out["authenticated"].(type) {
	case bool:
		if v {
			pairs = append(pairs, [2]string{"Status", p.Fg(ui.Green, "✓ authenticated")})
		} else {
			pairs = append(pairs, [2]string{"Status", p.Fg(ui.Red, "✗ not authenticated")})
		}
	case string:
		pairs = append(pairs, [2]string{"Status", p.Muted(v)})
	}
	sb.WriteString(p.KV(pairs, 2))
	sb.WriteString("\n")
	return sb.String()
}

func fmtMoney(x float64) string {
	s := fmt.Sprintf("%.2f", x)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	intPart, frac, _ := strings.Cut(s, ".")
	var sb strings.Builder
	for i, r := range intPart {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			sb.WriteByte(',')
		}
		sb.WriteRune(r)
	}
	out := sb.String() + "." + frac
	if neg {
		out = "-" + out
	}
	return out
}
