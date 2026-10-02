// Package tools defines every nak tool once and exposes the set both as MCP
// tools and to the CLI. Write tools are gated centrally: without confirm=true
// only their Preview runs, never their Do.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Raindancer118/nak-api/internal/app"
	"github.com/Raindancer118/nak-api/internal/drift"
	"github.com/Raindancer118/nak-api/internal/health"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type Kind int

const (
	Read  Kind = iota // reads only
	Local             // reads remotely, writes local files
	Write             // changes state in CIS/Moodle — confirm-gated
)

type Param struct {
	Name     string
	Type     string // string, integer, boolean, number, array:string, array:integer, object
	Desc     string
	Required bool
	Enum     []string
}

type Handler func(a *app.App, args Args) (any, error)

type Tool struct {
	Name   string
	Desc   string
	Kind   Kind
	Params []Param
	// Read/Local tools use Run. Write tools use Preview (no side effects) and
	// Do (the binding action); the registry decides which one runs.
	Run     Handler
	Preview Handler
	Do      Handler
	// MayWrite marks Read/Local tools that can change data behind their own
	// confirm parameter (moodle_call); only affects the MCP annotations.
	MayWrite bool
}

type Registry struct {
	tools []*Tool
	byKey map[string]*Tool
}

func NewRegistry() *Registry { return &Registry{byKey: map[string]*Tool{}} }

func (r *Registry) Add(ts ...*Tool) {
	for _, t := range ts {
		if _, dup := r.byKey[t.Name]; dup {
			panic("duplicate tool " + t.Name)
		}
		if t.Kind == Write && (t.Preview == nil || t.Do == nil) {
			panic("write tool without Preview/Do: " + t.Name)
		}
		if t.Kind != Write && t.Run == nil {
			panic("tool without Run: " + t.Name)
		}
		r.tools = append(r.tools, t)
		r.byKey[t.Name] = t
	}
}

func (r *Registry) All() []*Tool { return r.tools }

func (r *Registry) Get(name string) *Tool { return r.byKey[name] }

// ErrUnknownTool is returned by Call for names not in the registry.
var ErrUnknownTool = errors.New("unknown tool")

// Call executes a tool with argument validation and the write gate.
func (r *Registry) Call(a *app.App, name string, raw map[string]any) (any, error) {
	t := r.byKey[name]
	if t == nil {
		return nil, fmt.Errorf("%w %q", ErrUnknownTool, name)
	}
	args := Args(raw)
	if args == nil {
		args = Args{}
	}
	for _, p := range t.params() {
		if p.Required && args.missing(p.Name) {
			return nil, fmt.Errorf("%s is required", p.Name)
		}
	}
	if t.Kind != Write {
		return t.Run(a, args)
	}
	if !args.Bool("confirm", false) {
		res, err := t.Preview(a, args)
		if err != nil {
			return nil, err
		}
		return previewEnvelope(t, res), nil
	}
	res, err := t.Do(a, args)
	if err != nil {
		return nil, err
	}
	return map[string]any{"mode": "executed", "tool": t.Name, "result": res}, nil
}

// DriftHint explains a parser failure caused by a changed CIS page and asks
// for an issue (the outline it would contain has no personal data).
func DriftHint(err error) string {
	u := DriftReportURL(err, time.Now())
	if u == "" {
		return ""
	}
	return "\n\nDas CIS hat diese Seite offenbar geändert, nak muss angepasst werden. Bitte dem Nutzer vorschlagen, das zu melden: " +
		"nak_report_drift (legt nach Bestätigung ein GitHub-Issue nur mit der Seitenstruktur an, ohne persönliche Daten) oder dieser vorbefüllte Link: " + u
}

// DriftReportURL is a prefilled GitHub issue link for a drift error ("" for
// any other error).
func DriftReportURL(err error, at time.Time) string {
	var d *drift.Error
	if !errors.As(err, &d) {
		return ""
	}
	res := health.Result{Check: "tool call", Status: health.StatusDrift, Page: d.Page, Expected: d.Expected, Print: d.Fingerprint}
	return health.IssueURL(health.DefaultRepo, res, Version, at)
}

func previewEnvelope(t *Tool, res any) any {
	// Moodle previews already carry mode/next.
	if b, err := json.Marshal(res); err == nil && strings.Contains(string(b), `"mode":"preview"`) {
		return res
	}
	return map[string]any{
		"mode":    "preview",
		"tool":    t.Name,
		"preview": res,
		"next":    "NICHTS wurde gesendet. Zeige dem Nutzer diese Vorschau; nur nach seiner ausdrücklichen Zustimmung erneut mit confirm=true aufrufen.",
	}
}

func (t *Tool) params() []Param {
	if t.Kind != Write {
		return t.Params
	}
	return append(append([]Param{}, t.Params...), Param{Name: "confirm", Type: "boolean",
		Desc: "true = wirklich ausführen (VERBINDLICH, nur nach ausdrücklicher Zustimmung des Nutzers). Standard false = nur Vorschau, sendet nichts."})
}

// ── MCP ─────────────────────────────────────────────────────────────────────

func (r *Registry) Register(s *server.MCPServer, a *app.App) {
	for _, t := range r.tools {
		t := t
		s.AddTool(t.mcpTool(), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			res, err := r.Call(a, t.Name, req.GetArguments())
			if err != nil {
				return mcp.NewToolResultError("Fehler: " + err.Error() + DriftHint(err)), nil
			}
			if s, ok := res.(string); ok {
				return mcp.NewToolResultText(s), nil
			}
			b, err := json.MarshalIndent(res, "", "  ")
			if err != nil {
				return mcp.NewToolResultError("encode result: " + err.Error()), nil
			}
			return mcp.NewToolResultText(string(b)), nil
		})
	}
}

func (t *Tool) mcpTool() mcp.Tool {
	desc := t.Desc
	if t.Kind == Write {
		desc = "⚠ SCHREIBEND/VERBINDLICH. " + desc + " Ohne confirm=true nur Vorschau."
	}
	opts := []mcp.ToolOption{mcp.WithDescription(desc)}
	switch t.Kind {
	case Read:
		opts = append(opts, mcp.WithReadOnlyHintAnnotation(!t.MayWrite), mcp.WithDestructiveHintAnnotation(t.MayWrite), mcp.WithOpenWorldHintAnnotation(true))
	case Local:
		opts = append(opts, mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(false), mcp.WithOpenWorldHintAnnotation(true))
	case Write:
		opts = append(opts, mcp.WithReadOnlyHintAnnotation(false), mcp.WithDestructiveHintAnnotation(true), mcp.WithOpenWorldHintAnnotation(true))
	}
	for _, p := range t.params() {
		po := []mcp.PropertyOption{mcp.Description(p.Desc)}
		if p.Required {
			po = append(po, mcp.Required())
		}
		if len(p.Enum) > 0 {
			po = append(po, mcp.Enum(p.Enum...))
		}
		switch p.Type {
		case "integer", "number":
			opts = append(opts, mcp.WithNumber(p.Name, po...))
		case "boolean":
			opts = append(opts, mcp.WithBoolean(p.Name, po...))
		case "array:string":
			opts = append(opts, mcp.WithArray(p.Name, append(po, mcp.WithStringItems())...))
		case "array:integer":
			opts = append(opts, mcp.WithArray(p.Name, append(po, mcp.WithIntegerItems())...))
		case "object":
			opts = append(opts, mcp.WithObject(p.Name, po...))
		default:
			opts = append(opts, mcp.WithString(p.Name, po...))
		}
	}
	return mcp.NewTool(t.Name, opts...)
}

// ── arguments ───────────────────────────────────────────────────────────────

// Args coerces loosely typed tool arguments (clients send numbers as
// float64 or strings, lists as arrays or comma separated strings).
type Args map[string]any

func (a Args) missing(k string) bool {
	v, ok := a[k]
	if !ok || v == nil {
		return true
	}
	if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
		return true
	}
	return false
}

func (a Args) Has(k string) bool { return !a.missing(k) }

func (a Args) Str(k string) string {
	if a.missing(k) {
		return ""
	}
	switch v := a[k].(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprint(v)
	}
}

// StrPtr distinguishes "not given" (nil) from "" (explicitly cleared).
func (a Args) StrPtr(k string) *string {
	v, ok := a[k]
	if !ok || v == nil {
		return nil
	}
	s := strings.TrimSpace(fmt.Sprint(v))
	if f, isF := v.(float64); isF {
		s = strconv.FormatFloat(f, 'f', -1, 64)
	}
	return &s
}

func (a Args) Int(k string, def int64) (int64, error) {
	if a.missing(k) {
		return def, nil
	}
	switch v := a[k].(type) {
	case float64:
		return int64(v), nil
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case json.Number:
		return v.Int64()
	default:
		n, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(v)), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("%s must be a number, got %q", k, fmt.Sprint(v))
		}
		return n, nil
	}
}

func (a Args) ReqInt(k string) (int64, error) {
	if a.missing(k) {
		return 0, fmt.Errorf("%s is required", k)
	}
	return a.Int(k, 0)
}

func (a Args) IntPtr(k string) (*int64, error) {
	if a.missing(k) {
		return nil, nil
	}
	n, err := a.Int(k, 0)
	if err != nil {
		return nil, err
	}
	return &n, nil
}

func (a Args) Bool(k string, def bool) bool {
	v, ok := a[k]
	if !ok || v == nil {
		return def
	}
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	default:
		switch strings.ToLower(strings.TrimSpace(fmt.Sprint(v))) {
		case "true", "1", "yes", "ja":
			return true
		case "false", "0", "no", "nein":
			return false
		}
	}
	return def
}

func (a Args) BoolPtr(k string) *bool {
	if _, ok := a[k]; !ok || a[k] == nil {
		return nil
	}
	b := a.Bool(k, false)
	return &b
}

func (a Args) Strs(k string) []string {
	switch v := a[k].(type) {
	case nil:
		return nil
	case []any:
		out := make([]string, 0, len(v))
		for _, x := range v {
			out = append(out, strings.TrimSpace(fmt.Sprint(x)))
		}
		return out
	case []string:
		return v
	default:
		var out []string
		for _, p := range strings.Split(fmt.Sprint(v), ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
}

func (a Args) Ints(k string) ([]int64, error) {
	var out []int64
	for _, s := range a.Strs(k) {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil, fmt.Errorf("%s must contain numbers, got %q", k, s)
		}
		out = append(out, int64(f))
	}
	return out, nil
}

func (a Args) Map(k string) (map[string]any, error) {
	switch v := a[k].(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return v, nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil, nil
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(v), &m); err != nil {
			return nil, fmt.Errorf("%s must be a JSON object", k)
		}
		return m, nil
	}
	return nil, fmt.Errorf("%s must be an object", k)
}

// Names lists tool names sorted, for help output.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}
