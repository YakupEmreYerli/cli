// Package openapi embeds the DomainNameAPI "Public Gateway API" OpenAPI 3
// document and exposes the small subset of it the CLI needs: operations,
// parameters, request bodies and component schemas.
//
// The spec is the single source of truth for field names, types and
// constraints. CLI flags, validation, MCP tool input schemas, `dna schema`
// output and the reference docs are all derived from it, so updating the API
// means replacing openapi.json (see scripts/update-spec.sh) and running
// `make docs`.
package openapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
)

//go:embed openapi.json
var raw []byte

// Raw returns the embedded OpenAPI document exactly as published by the API.
func Raw() []byte { return raw }

// Document is a parsed OpenAPI document (only the parts the CLI uses).
type Document struct {
	OpenAPI    string                               `json:"openapi"`
	Info       Info                                 `json:"info"`
	Paths      map[string]map[string]*RawOp         `json:"paths"`
	Components struct{ Schemas map[string]*Schema } `json:"components"`

	// ops is the flattened, sorted operation list built by Load.
	ops []*Operation
}

// Info is the OpenAPI info object.
type Info struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

// RawOp is an operation object as it appears in the document.
type RawOp struct {
	Tags        []string             `json:"tags"`
	OperationID string               `json:"operationId"`
	Summary     string               `json:"summary"`
	Description string               `json:"description"`
	Parameters  []*Parameter         `json:"parameters"`
	RequestBody *RequestBody         `json:"requestBody"`
	Responses   map[string]*Response `json:"responses"`
}

// Parameter is a path, query or header parameter.
type Parameter struct {
	Name     string  `json:"name"`
	In       string  `json:"in"`
	Required bool    `json:"required"`
	Schema   *Schema `json:"schema"`
}

// RequestBody is an operation's request body.
type RequestBody struct {
	Content map[string]MediaType `json:"content"`
}

// Response is one response of an operation.
type Response struct {
	Description string               `json:"description"`
	Content     map[string]MediaType `json:"content"`
}

// MediaType holds the schema for one content type.
type MediaType struct {
	Schema *Schema `json:"schema"`
}

// Schema is an OpenAPI 3.0 schema object.
type Schema struct {
	Ref                  string     `json:"$ref,omitempty"`
	Type                 string     `json:"type,omitempty"`
	Format               string     `json:"format,omitempty"`
	Nullable             bool       `json:"nullable,omitempty"`
	Description          string     `json:"description,omitempty"`
	Enum                 []any      `json:"enum,omitempty"`
	Items                *Schema    `json:"items,omitempty"`
	Properties           Properties `json:"properties,omitempty"`
	Required             []string   `json:"required,omitempty"`
	AdditionalProperties *Schema    `json:"-"`
	HasAdditional        bool       `json:"-"`
	MinLength            *int       `json:"minLength,omitempty"`
	MaxLength            *int       `json:"maxLength,omitempty"`
	Minimum              *float64   `json:"minimum,omitempty"`
	Maximum              *float64   `json:"maximum,omitempty"`
	Pattern              string     `json:"pattern,omitempty"`
	AllOf                []*Schema  `json:"allOf,omitempty"`
	Default              any        `json:"default,omitempty"`
}

// UnmarshalJSON handles additionalProperties, which may be a bool or a schema.
func (s *Schema) UnmarshalJSON(b []byte) error {
	type plain Schema
	var aux struct {
		plain
		AdditionalProperties json.RawMessage `json:"additionalProperties"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*s = Schema(aux.plain)
	if len(aux.AdditionalProperties) > 0 {
		switch strings.TrimSpace(string(aux.AdditionalProperties)) {
		case "false":
		case "true":
			s.HasAdditional = true
			s.AdditionalProperties = &Schema{}
		default:
			var ap Schema
			if err := json.Unmarshal(aux.AdditionalProperties, &ap); err != nil {
				return err
			}
			s.HasAdditional = true
			s.AdditionalProperties = &ap
		}
	}
	return nil
}

// IsRequired reports whether property name is listed in Required.
func (s *Schema) IsRequired(name string) bool {
	for _, r := range s.Required {
		if r == name {
			return true
		}
	}
	return false
}

// Property is one named property of an object schema.
type Property struct {
	Name   string
	Schema *Schema
}

// Properties keeps object properties in document order, which is the order
// the API authors declared them in (and the order shown in --help).
type Properties []Property

// Get returns the property schema by name.
func (p Properties) Get(name string) (*Schema, bool) {
	for _, prop := range p {
		if prop.Name == name {
			return prop.Schema, true
		}
	}
	return nil, false
}

// UnmarshalJSON decodes a JSON object while preserving key order.
func (p *Properties) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return fmt.Errorf("properties: expected object")
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		name, _ := tok.(string)
		var s Schema
		if err := dec.Decode(&s); err != nil {
			return err
		}
		*p = append(*p, Property{Name: name, Schema: &s})
	}
	_, err = dec.Token()
	return err
}

// MarshalJSON encodes properties as an object, preserving order.
func (p Properties) MarshalJSON() ([]byte, error) {
	var sb strings.Builder
	sb.WriteByte('{')
	for i, prop := range p {
		if i > 0 {
			sb.WriteByte(',')
		}
		k, _ := json.Marshal(prop.Name)
		v, err := json.Marshal(prop.Schema)
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

// Operation is a flattened view of one method+path.
type Operation struct {
	Method      string // upper case: GET, POST, PUT, DELETE
	Path        string // e.g. /api/v1/domains/search
	OperationID string
	Tag         string
	Params      []*Parameter // path and query parameters (auth headers removed)
	Body        *Schema      // application/json request body schema, or nil
	Result      *Schema      // application/json schema of the 2xx response, or nil
	SuccessCode string       // "200" or "202"
}

// Key is "METHOD /path", a stable identifier for an operation.
func (o *Operation) Key() string { return o.Method + " " + o.Path }

var (
	loadOnce sync.Once
	loaded   *Document
	loadErr  error
)

// Load parses the embedded document once and caches the result.
func Load() (*Document, error) {
	loadOnce.Do(func() {
		loaded, loadErr = Parse(raw)
	})
	return loaded, loadErr
}

// MustLoad is Load that panics on error. The embedded spec is validated by
// tests, so a failure here is a build defect, not a runtime condition.
func MustLoad() *Document {
	d, err := Load()
	if err != nil {
		panic(fmt.Sprintf("openapi: embedded spec is invalid: %v", err))
	}
	return d
}

// AuthHeaders are the credential headers every operation declares. They are
// injected by the client, so they are hidden from flags and tool schemas.
var AuthHeaders = map[string]bool{"__reseller": true, "X-API-KEY": true}

// Parse parses an OpenAPI document.
func Parse(b []byte) (*Document, error) {
	var d Document
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	for path, methods := range d.Paths {
		for method, rop := range methods {
			m := strings.ToUpper(method)
			switch m {
			case "GET", "POST", "PUT", "DELETE", "PATCH":
			default:
				continue
			}
			op := &Operation{Method: m, Path: path, OperationID: rop.OperationID}
			if len(rop.Tags) > 0 {
				op.Tag = rop.Tags[0]
			}
			for _, p := range rop.Parameters {
				if p.In == "header" && AuthHeaders[p.Name] {
					continue
				}
				op.Params = append(op.Params, p)
			}
			if rop.RequestBody != nil {
				if mt, ok := rop.RequestBody.Content["application/json"]; ok {
					op.Body = mt.Schema
				}
			}
			for _, code := range []string{"200", "201", "202", "204"} {
				r, ok := rop.Responses[code]
				if !ok {
					continue
				}
				op.SuccessCode = code
				if mt, ok := r.Content["application/json"]; ok {
					op.Result = mt.Schema
				}
				break
			}
			d.ops = append(d.ops, op)
		}
	}
	sort.Slice(d.ops, func(i, j int) bool { return d.ops[i].Key() < d.ops[j].Key() })
	return &d, nil
}

// Operations returns every operation, sorted by method and path.
func (d *Document) Operations() []*Operation { return d.ops }

// Operation looks an operation up by method and path.
func (d *Document) Operation(method, path string) (*Operation, bool) {
	key := strings.ToUpper(method) + " " + path
	for _, op := range d.ops {
		if op.Key() == key {
			return op, true
		}
	}
	return nil, false
}

// Resolve follows a $ref (one level) and returns the referenced schema.
// Non-ref schemas are returned unchanged.
func (d *Document) Resolve(s *Schema) *Schema {
	for s != nil && s.Ref != "" {
		name := strings.TrimPrefix(s.Ref, "#/components/schemas/")
		next, ok := d.Components.Schemas[name]
		if !ok {
			return s
		}
		s = next
	}
	return s
}

// RefName returns the component name a schema points to ("" if inline).
func RefName(s *Schema) string {
	if s == nil || s.Ref == "" {
		return ""
	}
	return strings.TrimPrefix(s.Ref, "#/components/schemas/")
}

// ShortName turns a .NET-style component name into a readable type name:
//
//	Dna.DomainService.Domains.DomainSearchDto                    -> DomainSearchDto
//	Volo.Abp.Application.Dtos.PagedResultDto`1[[Dna...DomainDto, ...]] -> PagedResultDto<DomainDto>
func ShortName(component string) string {
	if i := strings.Index(component, "`1[["); i >= 0 {
		outer := lastDot(component[:i])
		inner := component[i+4:]
		if j := strings.Index(inner, ","); j >= 0 {
			inner = inner[:j]
		}
		return outer + "<" + lastDot(inner) + ">"
	}
	return lastDot(component)
}

func lastDot(s string) string {
	if i := strings.LastIndex(s, "."); i >= 0 {
		return s[i+1:]
	}
	return s
}

// TypeName describes a schema in one short human readable token, such as
// "string", "integer", "string[]", "DomainSearchDto" or "map<string,string>".
func (d *Document) TypeName(s *Schema) string {
	if s == nil {
		return "any"
	}
	if s.Ref != "" {
		return ShortName(RefName(s))
	}
	switch s.Type {
	case "array":
		return d.TypeName(s.Items) + "[]"
	case "object":
		if s.HasAdditional {
			return "map<string," + d.TypeName(s.AdditionalProperties) + ">"
		}
		return "object"
	case "":
		return "any"
	}
	return s.Type
}
