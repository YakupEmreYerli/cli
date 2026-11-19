package ops

import (
	"regexp"
	"strings"

	"github.com/domainreseller/cli/internal/dna"
	"github.com/domainreseller/cli/internal/openapi"
)

// cliHints removes command-line specific hints from help text so the same
// strings read well in MCP tool schemas.
var cliHints = regexp.MustCompile(`(?i)\s*(Easier: --[a-z-]+|\(or @file\)|, or @file| or @file|\(repeatable\)|; repeatable|\(repeat or comma separate[^)]*\)|repeat or comma separate)`)

// Neutral strips CLI-only hints from a help string.
func Neutral(help string) string {
	return strings.TrimSpace(cliHints.ReplaceAllString(help, ""))
}

func describer(owner, prop string) (string, []any) {
	e, ok := dna.EnumFor(owner, prop)
	if !ok {
		return "", nil
	}
	if e.Kind == "string" {
		vals := make([]any, len(e.Values))
		for i, v := range e.Values {
			vals[i] = v.Value
		}
		return e.Description, vals
	}
	return "Values: " + e.Summary() + ".", nil
}

func enumDoc(e *dna.EnumInfo) (string, []any) {
	if e.Kind == "string" {
		vals := make([]any, len(e.Values))
		for i, v := range e.Values {
			vals[i] = v.Value
		}
		return "", vals
	}
	return "Values: " + e.Summary() + ". Names are accepted too.", nil
}

// InputSchema is the JSON Schema of the operation's input: path and query
// parameters (camelCase) plus the request body's top-level properties. It is
// the inputSchema of the MCP tool and what `dna schema` prints.
func (o *Operation) InputSchema() *openapi.JSONSchema {
	d := doc()
	root := &openapi.JSONSchema{Type: "object"}
	add := func(name string, s *openapi.JSONSchema, required bool, ov Field) {
		if ov.Help != "" {
			s.Description = strings.TrimSpace(Neutral(ov.Help) + " " + s.Description)
		}
		if ov.Enum != "" {
			if e, ok := dna.LookupEnum(ov.Enum); ok {
				desc, vals := enumDoc(e)
				if ov.EnumList {
					desc = "Comma separated list. " + "Values: " + e.Summary() + "."
					vals = nil
					s.Type = "string"
				}
				s.Description = strings.TrimSpace(s.Description + " " + desc)
				if vals != nil && s.Type != "array" {
					s.Enum = vals
				}
				if vals != nil && s.Type == "array" && s.Items != nil {
					s.Items.Enum = vals
				}
				if e.Kind == "integer" && s.Type == "array" && s.Items != nil {
					s.Items.Enum = nil
				}
			}
		}
		if ov.Optional {
			required = false
		}
		if ov.Required {
			required = true
		}
		root.Properties = append(root.Properties, openapi.JSONProp{Name: name, Schema: s})
		if required {
			root.Required = append(root.Required, name)
		}
	}
	for _, p := range o.spec.Params {
		key := lowerFirst(p.Name)
		add(key, d.ToJSONSchema(p.Schema, describer), p.Required || p.In == "path", o.Fields[key])
	}
	for _, v := range o.Virtual {
		s := &openapi.JSONSchema{Type: "string", Description: v.Help}
		if v.Type == "strings" {
			s = &openapi.JSONSchema{Type: "array", Items: &openapi.JSONSchema{Type: "string"}, Description: v.Help}
		}
		add(v.Key, s, v.Required, Field{})
	}
	if body := d.Resolve(o.spec.Body); body != nil && body.Type != "array" {
		owner := openapi.ShortName(openapi.RefName(o.spec.Body))
		for _, prop := range body.Properties {
			s := d.ToJSONSchema(prop.Schema, describer)
			if desc, vals := describer(owner, prop.Name); desc != "" || vals != nil {
				s.Description = strings.TrimSpace(s.Description + " " + desc)
				if vals != nil && s.Enum == nil {
					s.Enum = vals
				}
			}
			// Required-ness of flattened objects' fields comes from registry overrides.
			if prefix, ok := o.Flatten[prop.Name]; ok {
				_ = prefix
				for i, inner := range s.Properties {
					ov := o.Fields[prop.Name+"."+inner.Name]
					if ov.Help != "" {
						s.Properties[i].Schema.Description = strings.TrimSpace(Neutral(ov.Help) + " " + inner.Schema.Description)
					}
					if ov.Required && !contains(s.Required, inner.Name) {
						s.Required = append(s.Required, inner.Name)
					}
				}
			}
			add(prop.Name, s, body.IsRequired(prop.Name), o.Fields[prop.Name])
		}
	}
	return root
}

// ExampleInput is a filled-in input skeleton for the operation.
func (o *Operation) ExampleInput() openapi.Object {
	d := doc()
	obj := openapi.Object{}
	for _, p := range o.spec.Params {
		key := lowerFirst(p.Name)
		if p.Required || p.In == "path" || o.Fields[key].Required {
			obj = append(obj, openapi.KV{Key: key, Value: d.Example(&openapi.Schema{Type: p.Schema.Type, Format: p.Schema.Format, Items: p.Schema.Items, Minimum: p.Schema.Minimum})})
			if ex, ok := exampleFor(key); ok {
				obj[len(obj)-1].Value = ex
			}
		}
	}
	for _, v := range o.Virtual {
		if v.Type == "strings" {
			obj = append(obj, openapi.KV{Key: v.Key, Value: []string{"example.com", "example.net"}})
		} else if v.Required {
			obj = append(obj, openapi.KV{Key: v.Key, Value: "string"})
		}
	}
	if body := d.Resolve(o.spec.Body); body != nil && body.Type != "array" {
		for _, prop := range body.Properties {
			obj = append(obj, openapi.KV{Key: prop.Name, Value: d.ExampleNamed(prop.Name, prop.Schema)})
			if ex, ok := exampleFor(prop.Name); ok && d.Resolve(prop.Schema).Type == "string" {
				obj[len(obj)-1].Value = ex
			}
		}
	}
	return obj
}

func exampleFor(key string) (any, bool) {
	switch key {
	case "domainName":
		return "example.com", true
	case "handleCode":
		return "DNA-12345", true
	case "productName":
		return "com", true
	case "orderCode":
		return "ORD-12345", true
	case "hostName":
		return "ns1.example.com", true
	}
	return nil, false
}

// ResultEnums maps field names of the result (or of its list items) to enums,
// so tables can show "Active" instead of 0.
func (o *Operation) ResultEnums() map[string]*dna.EnumInfo {
	d := doc()
	s := o.spec.Result
	if s == nil {
		return nil
	}
	r := d.Resolve(s)
	target := s
	if r.Type == "array" {
		target = r.Items
	} else if items, ok := r.Properties.Get("items"); ok {
		target = d.Resolve(items).Items
	} else if infos, ok := r.Properties.Get("infos"); ok {
		target = d.Resolve(infos).Items
	}
	owner := openapi.ShortName(openapi.RefName(target))
	t := d.Resolve(target)
	if t == nil {
		return nil
	}
	out := map[string]*dna.EnumInfo{}
	for _, prop := range t.Properties {
		if e, ok := dna.EnumFor(owner, prop.Name); ok && e.Kind == "integer" {
			out[prop.Name] = e
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
