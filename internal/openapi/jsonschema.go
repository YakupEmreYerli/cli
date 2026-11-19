package openapi

import (
	"encoding/json"
	"strings"
)

// JSONSchema is a self-contained JSON Schema (the subset MCP clients and LLM
// tool callers understand) produced from an OpenAPI schema with every $ref
// inlined. Property order follows the API document.
type JSONSchema struct {
	Type                 string        `json:"type,omitempty"`
	Format               string        `json:"format,omitempty"`
	Description          string        `json:"description,omitempty"`
	Enum                 []any         `json:"enum,omitempty"`
	Items                *JSONSchema   `json:"items,omitempty"`
	Properties           JSONProps     `json:"properties,omitempty"`
	Required             []string      `json:"required,omitempty"`
	AdditionalProperties *JSONSchema   `json:"additionalProperties,omitempty"`
	MinLength            *int          `json:"minLength,omitempty"`
	MaxLength            *int          `json:"maxLength,omitempty"`
	Minimum              *float64      `json:"minimum,omitempty"`
	Maximum              *float64      `json:"maximum,omitempty"`
	MinItems             *int          `json:"minItems,omitempty"`
	Pattern              string        `json:"pattern,omitempty"`
	Default              any           `json:"default,omitempty"`
	Examples             []any         `json:"examples,omitempty"`
	OneOf                []*JSONSchema `json:"oneOf,omitempty"`
}

// JSONProp is one ordered property of a JSONSchema.
type JSONProp struct {
	Name   string
	Schema *JSONSchema
}

// JSONProps is an ordered property list that marshals as a JSON object.
type JSONProps []JSONProp

// MarshalJSON keeps declaration order.
func (p JSONProps) MarshalJSON() ([]byte, error) {
	obj := make(Object, 0, len(p))
	for _, prop := range p {
		obj = append(obj, KV{prop.Name, prop.Schema})
	}
	return obj.MarshalJSON()
}

// Get returns a property by name.
func (p JSONProps) Get(name string) *JSONSchema {
	for _, prop := range p {
		if prop.Name == name {
			return prop.Schema
		}
	}
	return nil
}

// Describer returns extra documentation (and, for string enums, the allowed
// values) for property prop of the component whose short name is owner (for
// example "DomainDto", "status"). It lets the caller attach enum meanings and
// usage notes the spec itself lacks.
type Describer func(owner, prop string) (description string, enum []any)

// ToJSONSchema converts an OpenAPI schema into a JSONSchema.
func (d *Document) ToJSONSchema(s *Schema, describe Describer) *JSONSchema {
	return d.toJSON(s, describe, "", map[string]bool{})
}

func (d *Document) toJSON(s *Schema, describe Describer, owner string, seen map[string]bool) *JSONSchema {
	if s == nil {
		return &JSONSchema{}
	}
	if ref := RefName(s); ref != "" {
		if seen[ref] { // recursion guard; the current spec has no cycles
			return &JSONSchema{Type: "object", Description: ShortName(ref)}
		}
		seen[ref] = true
		defer delete(seen, ref)
		return d.toJSON(d.Resolve(s), describe, ShortName(ref), seen)
	}
	if len(s.AllOf) == 1 {
		return d.toJSON(s.AllOf[0], describe, owner, seen)
	}
	out := &JSONSchema{
		Type:        s.Type,
		Format:      s.Format,
		Description: s.Description,
		Enum:        s.Enum,
		MinLength:   nonZeroMin(s.MinLength),
		MaxLength:   s.MaxLength,
		Minimum:     s.Minimum,
		Maximum:     s.Maximum,
		Pattern:     s.Pattern,
		Default:     s.Default,
	}
	switch s.Type {
	case "array":
		out.Items = d.toJSON(s.Items, describe, owner, seen)
	case "object", "":
		if len(s.Properties) > 0 {
			out.Type = "object"
			for _, prop := range s.Properties {
				ps := d.toJSON(prop.Schema, describe, owner, seen)
				if describe != nil && owner != "" {
					extra, enum := describe(owner, prop.Name)
					if extra != "" {
						ps.Description = joinDesc(ps.Description, extra)
					}
					if len(enum) > 0 && len(ps.Enum) == 0 && ps.Type == "string" {
						ps.Enum = enum
					}
				}
				out.Properties = append(out.Properties, JSONProp{prop.Name, ps})
				if s.IsRequired(prop.Name) {
					out.Required = append(out.Required, prop.Name)
				}
			}
		}
		if s.HasAdditional {
			out.Type = "object"
			out.AdditionalProperties = d.toJSON(s.AdditionalProperties, describe, owner, seen)
		}
	}
	return out
}

// nonZeroMin drops the meaningless "minLength: 0" the .NET generator emits.
func nonZeroMin(v *int) *int {
	if v == nil || *v == 0 {
		return nil
	}
	return v
}

func joinDesc(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

// KV is one key/value pair of an Object.
type KV struct {
	Key   string
	Value any
}

// Object is an ordered JSON object.
type Object []KV

// MarshalJSON encodes the object preserving key order.
func (o Object) MarshalJSON() ([]byte, error) {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			sb.WriteByte(',')
		}
		k, _ := json.Marshal(kv.Key)
		v, err := json.Marshal(kv.Value)
		if err != nil {
			return nil, err
		}
		sb.Write(k)
		sb.WriteByte(':')
		sb.Write(v)
	}
	sb.WriteByte('}')
	return []byte(sb.String()), nil
}

// exampleStrings are realistic placeholder values keyed by property name, so
// generated request skeletons look like something you could actually send.
var exampleStrings = map[string]any{
	"domainName":       "example.com",
	"newHostName":      "ns2.example.com",
	"hostName":         "ns1.example.com",
	"handle":           "DNA-12345",
	"handleCode":       "DNA-12345",
	"contactType":      "Registrant",
	"firstName":        "Ada",
	"lastName":         "Lovelace",
	"companyName":      "Example Ltd",
	"eMail":            "ada@example.com",
	"approverMail":     "admin@example.com",
	"address":          "1 Example Street",
	"phoneCountryCode": "90",
	"phone":            "5551234567",
	"faxCountryCode":   "90",
	"fax":              "5551234568",
	"postalCode":       "34000",
	"country":          "TR",
	"city":             "Istanbul",
	"state":            "Istanbul",
	"locality":         "Istanbul",
	"organization":     "Example Ltd",
	"organizationUnit": "IT",
	"currency":         "USD",
	"authCode":         "Xy7!kP2q",
	"ipAddress":        "192.0.2.10",
	"ipVersion":        "v4",
	"redirectAddress":  "https://www.example.com",
	"forwardType":      "Standard",
	"name":             "www",
	"type":             "A",
	"digest":           "2BB183AF5F22588179A53B0A98631FAD1A292118",
	"servicePlan":      "starter",
	"packageCode":      "positivessl",
	"csr":              "-----BEGIN CERTIFICATE REQUEST-----\n...\n-----END CERTIFICATE REQUEST-----",
	"fileName":         "id-card.pdf",
	"fileContent":      "<base64 file content>",
}

var exampleArrays = map[string]any{
	"nameServers":      []string{"ns1.example.com", "ns2.example.com"},
	"contents":         []string{"192.0.2.10"},
	"alternativeNames": []string{"www.example.com"},
}

// Example builds a request skeleton for a schema, filled with placeholder
// values. It is what `dna schema <op>` prints under "example".
func (d *Document) Example(s *Schema) any {
	return d.example(s, "", 0)
}

// ExampleNamed is Example for a property called name, which lets the
// placeholder match the field ("nameServers" gets two host names).
func (d *Document) ExampleNamed(name string, s *Schema) any {
	return d.example(s, name, 0)
}

func (d *Document) example(s *Schema, name string, depth int) any {
	s = d.Resolve(s)
	if s == nil || depth > 6 {
		return nil
	}
	if v, ok := exampleArrays[name]; ok && s.Type == "array" {
		return v
	}
	if len(s.Enum) > 0 {
		return s.Enum[0]
	}
	switch s.Type {
	case "string":
		if v, ok := exampleStrings[name]; ok {
			return v
		}
		switch s.Format {
		case "uuid":
			return "00000000-0000-0000-0000-000000000000"
		case "date-time":
			return "2026-01-01T00:00:00Z"
		case "email":
			return "user@example.com"
		}
		return "string"
	case "integer":
		switch name {
		case "ttl":
			return 3600
		case "period":
			return 1
		case "keyTag":
			return 2371
		case "algorithm":
			return 13
		case "digestType":
			return 2
		}
		if s.Minimum != nil && *s.Minimum > 0 {
			return int(*s.Minimum)
		}
		return 0
	case "number":
		return 0
	case "boolean":
		return false
	case "array":
		return []any{d.example(s.Items, name, depth+1)}
	}
	if s.HasAdditional && len(s.Properties) == 0 {
		return map[string]string{"KEY": "value"}
	}
	obj := Object{}
	for _, prop := range s.Properties {
		obj = append(obj, KV{prop.Name, d.example(prop.Schema, prop.Name, depth+1)})
	}
	return obj
}
