package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/ops"
	"github.com/domainreseller/cli/internal/output"
	"github.com/domainreseller/cli/internal/ui"
)

// Exit codes. They are part of the CLI's contract; see docs/errors.md.
const (
	ExitOK           = 0  // success
	ExitError        = 1  // unexpected error
	ExitUsage        = 2  // bad flags, arguments or input (nothing was sent)
	ExitAuth         = 3  // authentication / authorization failed
	ExitNotFound     = 4  // resource not found
	ExitRejected     = 5  // the API rejected the request (validation / business rule)
	ExitRateLimited  = 6  // rate limited after retries
	ExitServer       = 7  // API server error
	ExitNetwork      = 8  // network failure or timeout
	ExitConfirmation = 9  // confirmation required (re-run with --yes) or declined
	ExitPolicy       = 10 // blocked by read-only mode
)

// ExitCodes documents every exit code (for `dna agent guide` and docs).
var ExitCodes = []struct {
	Code    int    `json:"code"`
	Name    string `json:"name"`
	Meaning string `json:"meaning"`
}{
	{ExitOK, "ok", "Success"},
	{ExitError, "error", "Unexpected error"},
	{ExitUsage, "usage", "Invalid flags, arguments or input; nothing was sent to the API"},
	{ExitAuth, "auth", "Authentication or authorization failed (bad API key, unknown or suspended reseller ID, wrong environment)"},
	{ExitNotFound, "not_found", "The resource does not exist"},
	{ExitRejected, "rejected", "The API rejected the request (validation error or business rule)"},
	{ExitRateLimited, "rate_limited", "Rate limited even after automatic retries"},
	{ExitServer, "server", "API server error (5xx); may be transient"},
	{ExitNetwork, "network", "Network failure or timeout"},
	{ExitConfirmation, "confirmation_required", "Billable/destructive operation needs --yes (or was declined at the prompt)"},
	{ExitPolicy, "read_only", "Blocked because read-only mode is on"},
}

// usageError marks errors caused by how the CLI was invoked.
type usageError struct{ err error }

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usageErr(err error) error {
	if err == nil {
		return nil
	}
	return &usageError{err}
}

func usagef(format string, args ...any) error { return &usageError{fmt.Errorf(format, args...)} }

// ConfirmationError is returned when a billable or destructive operation is
// attempted without --yes in a non-interactive session, or is declined.
type ConfirmationError struct {
	Operation string
	Safety    ops.Safety
	Declined  bool
}

func (e *ConfirmationError) Error() string {
	if e.Declined {
		return "aborted: " + e.Operation + " was not confirmed"
	}
	return fmt.Sprintf("%s is %s and needs confirmation: re-run with --yes (preview first with --dry-run)", e.Operation, e.Safety)
}

// PolicyError is returned when read-only mode blocks an operation.
type PolicyError struct {
	Operation string
	Source    string
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("%s changes data but read-only mode is on (%s)", e.Operation, e.Source)
}

// errorPayload is the JSON shape of errors printed to stderr.
type errorPayload struct {
	Kind          string           `json:"kind"`
	Code          string           `json:"code,omitempty"`
	Message       string           `json:"message"`
	Status        int              `json:"status,omitempty"`
	Details       string           `json:"details,omitempty"`
	Fields        []ops.FieldError `json:"fields,omitempty"`
	CorrelationID string           `json:"correlationId,omitempty"`
	Hint          string           `json:"hint,omitempty"`
	Retryable     bool             `json:"retryable"`
	ExitCode      int              `json:"exitCode"`
}

func classify(err error) errorPayload {
	var (
		ae  *dna.APIError
		ne  *dna.NetworkError
		ie  *ops.InputError
		ce  *ConfirmationError
		pe  *PolicyError
		ue  *usageError
		jqe *output.JQError
	)
	switch {
	case errors.As(err, &ae):
		p := errorPayload{Kind: string(ae.Kind()), Code: ae.Code, Message: ae.Message, Status: ae.StatusCode,
			Details: ae.Details, CorrelationID: ae.CorrelationID, Hint: ae.Hint(), Retryable: ae.Retryable()}
		for _, v := range ae.ValidationErrors {
			p.Fields = append(p.Fields, ops.FieldError{Field: strings.Join(v.Members, ","), Message: v.Message})
		}
		switch ae.Kind() {
		case dna.KindAuth, dna.KindForbidden:
			p.ExitCode = ExitAuth
		case dna.KindNotFound:
			p.ExitCode = ExitNotFound
		case dna.KindValidation, dna.KindConflict:
			p.ExitCode = ExitRejected
		case dna.KindRateLimited:
			p.ExitCode = ExitRateLimited
		case dna.KindServer:
			p.ExitCode = ExitServer
		default:
			p.ExitCode = ExitError
		}
		return p
	case errors.As(err, &ie):
		return errorPayload{Kind: "invalid_input", Message: "invalid input for " + ie.Operation + " (nothing was sent)", Fields: ie.Errors, ExitCode: ExitUsage,
			Hint: "Run `dna schema " + ie.Operation + "` to see every input with its type and constraints."}
	case errors.As(err, &ce):
		p := errorPayload{Kind: "confirmation_required", Message: ce.Error(), ExitCode: ExitConfirmation,
			Hint: "Ask the user to approve, then re-run with --yes. Use --dry-run to show exactly what will be sent."}
		if ce.Declined {
			p.Kind, p.Hint = "declined", ""
		}
		return p
	case errors.As(err, &pe):
		return errorPayload{Kind: "read_only", Message: pe.Error(), ExitCode: ExitPolicy,
			Hint: "Read-only mode is meant to protect the account. Remove --read-only / DNA_READ_ONLY only if the change is intended."}
	case errors.Is(err, dna.ErrNoCredentials):
		return errorPayload{Kind: "auth", Code: "NO_CREDENTIALS", Message: "no API credentials configured", ExitCode: ExitAuth,
			Hint: "Run `dna auth login` (add --env ote for the test environment) or set DNA_RESELLER_ID and DNA_API_KEY."}
	case errors.As(err, &ne):
		p := errorPayload{Kind: "network", Message: ne.Error(), ExitCode: ExitNetwork, Retryable: true,
			Hint: "Check connectivity to the API (`dna doctor`)."}
		if errors.Is(err, context.Canceled) {
			p.Kind, p.Retryable, p.Hint = "canceled", false, ""
		} else if ne.Timeout() || errors.Is(err, context.DeadlineExceeded) {
			p.Kind = "timeout"
			p.Hint = "The request timed out. Raise --timeout or retry; for billable operations check the result (e.g. `dna domain info`) before retrying."
		}
		return p
	case errors.As(err, &jqe):
		return errorPayload{Kind: "usage", Message: jqe.Error(), ExitCode: ExitUsage}
	case errors.As(err, &ue):
		return errorPayload{Kind: "usage", Message: ue.Error(), ExitCode: ExitUsage, Hint: "See --help."}
	}
	msg := err.Error()
	// cobra's own argument/flag errors
	for _, prefix := range []string{"unknown command", "unknown flag", "unknown shorthand", "invalid argument", "accepts ", "requires ", "flag needs", "required flag"} {
		if strings.HasPrefix(msg, prefix) || strings.Contains(msg, " "+prefix) {
			return errorPayload{Kind: "usage", Message: msg, ExitCode: ExitUsage, Hint: "See --help."}
		}
	}
	return errorPayload{Kind: "error", Message: msg, ExitCode: ExitError}
}

func (a *App) reportError(err error) int {
	p := classify(err)
	if a.machineOutput() {
		b, _ := json.Marshal(map[string]any{"error": p})
		fmt.Fprintln(a.Err, string(b))
		return p.ExitCode
	}
	fmt.Fprint(a.Err, a.humanError(p))
	return p.ExitCode
}

// errorTitles are the headlines of human error messages, by kind.
var errorTitles = map[string]string{
	"auth": "Authentication failed", "forbidden": "Not allowed", "not_found": "Not found",
	"validation": "Rejected by the API", "conflict": "Conflict", "rate_limited": "Rate limited",
	"server": "API server error", "network": "Network error", "timeout": "Timed out", "canceled": "Canceled",
	"invalid_input": "Invalid input", "usage": "Usage error", "confirmation_required": "Confirmation required",
	"declined": "Canceled, nothing was sent", "read_only": "Blocked by read-only mode",
}

// humanError renders an error for a person: a headline, the message, the
// API code and fields, and the suggested next step.
func (a *App) humanError(e errorPayload) string {
	p := a.errP()
	tone, icon := ui.Bad, "✗"
	switch e.Kind {
	case "confirmation_required", "declined", "read_only", "rate_limited", "canceled":
		tone, icon = ui.Warn, "▲"
	}
	title := errorTitles[e.Kind]
	if title == "" {
		title = "Error"
	}
	head := p.Bold(title) + "  " + p.Muted(fmt.Sprintf("exit %d", e.ExitCode))

	var lines []string
	if e.Message != "" && !strings.EqualFold(e.Message, title) {
		lines = append(lines, styleCode(p, e.Message))
	}
	var meta []string
	if e.Code != "" && e.Code != e.Message {
		meta = append(meta, "code "+e.Code)
	}
	if e.Status != 0 {
		meta = append(meta, fmt.Sprintf("HTTP %d", e.Status))
	}
	if len(meta) > 0 {
		lines = append(lines, p.Muted(strings.Join(meta, " · ")))
	}
	if len(e.Fields) > 0 {
		lines = append(lines, "")
		w := 0
		for _, f := range e.Fields {
			if fw := ui.Width(f.Field); fw > w {
				w = fw
			}
		}
		for _, f := range e.Fields {
			if f.Field == "" {
				lines = append(lines, p.Fg(tone.Color(), "•")+" "+styleCode(p, f.Message))
				continue
			}
			lines = append(lines, p.Fg(tone.Color(), "•")+" "+p.Bold(ui.PadRight(f.Field, w))+"  "+styleCode(p, f.Message))
		}
	}
	if e.Details != "" && e.Details != e.Message {
		lines = append(lines, "", p.Muted(e.Details))
	}
	if e.CorrelationID != "" {
		lines = append(lines, p.Muted("correlation id "+e.CorrelationID))
	}
	if e.Hint != "" {
		lines = append(lines, "", p.Hint(styleCode(p, e.Hint)))
	}
	return p.Block(tone, icon, head, lines) + "\n"
}

// styleCode colors `command` spans of a message as commands (and drops the
// backticks when colors make them visible anyway).
func styleCode(p ui.Painter, s string) string {
	if !p.On() || strings.Count(s, "`") < 2 {
		return s
	}
	var sb strings.Builder
	for i, part := range strings.Split(s, "`") {
		if i%2 == 1 {
			sb.WriteString(p.Command(part))
			continue
		}
		sb.WriteString(part)
	}
	return sb.String()
}
