package server

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestOpenAPISpecMatchesStructs catches silent drift between the
// published api/v1/openapi.yaml contract and the actual wire-level
// structs returned by the handlers. For every schema in
// components.schemas it compares the YAML property names against the
// JSON tags reflected from the corresponding Go type. Mismatches in
// either direction fail the test — adding a field on the wire but
// forgetting to update the spec (or vice versa) trips this guard.
//
// Coverage scope (intentional):
//   - Property names of the six top-level schemas under
//     components.schemas (ScreenRequest, MatchResponse, ScreenResponse,
//     BatchRequest, BatchResponse, ListEntry).
//   - Empty json tag and `json:"-"` are skipped, matching
//     encoding/json semantics.
//
// Out of scope:
//
//   - Inline response schemas on paths/* (e.g. /health, /lists/{id}/count)
//     are not walked; they're trivial enough to notice by inspection.
//   - $ref'd sub-schemas are not recursed (a property declared as
//     `items: {$ref: '#/components/schemas/MatchResponse'}` is treated
//     as opaque — only its presence, not the referenced schema's
//     property list, is validated by this test).
//   - Types, enums, required arrays, and minimum/maximum constraints
//     are not checked; only that wire-level field NAMES match.
//   - ListEntry.ID/Name strings are asserted against the spec
//     property names but the constants `models.ListOFAC` etc.
//     are NOT checked against the enum — that's future work.
//
// This sits in `package server` (not `server_test`) so it can reflect on
// the unexported wire-level response structs without forcing
// `screenResponse` etc. into the public API.
func TestOpenAPISpecMatchesStructs(t *testing.T) {
	specPath := filepath.Join("..", "..", "api", "v1", "openapi.yaml")
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v", specPath, err)
	}

	var doc struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]yaml.Node `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse openapi.yaml: %v", err)
	}

	cases := []struct {
		schemaName string
		goType     any
	}{
		{"ScreenRequest", screenRequest{}},
		{"MatchResponse", matchResponse{}},
		{"ScreenResponse", screenResponse{}},
		{"BatchRequest", batchRequest{}},
		{"BatchResponse", batchResponse{}},
		{"ListEntry", listEntry{}},
	}

	for _, c := range cases {
		t.Run(c.schemaName, func(t *testing.T) {
			specSchema, ok := doc.Components.Schemas[c.schemaName]
			if !ok {
				t.Fatalf("schema %q missing from openapi.yaml components.schemas", c.schemaName)
			}

			specProps := make(map[string]bool, len(specSchema.Properties))
			for k := range specSchema.Properties {
				specProps[k] = true
			}

			typ := reflect.TypeOf(c.goType)
			goProps := make(map[string]bool)
			for i := 0; i < typ.NumField(); i++ {
				fld := typ.Field(i)
				tag := fld.Tag.Get("json")
				if tag == "" || tag == "-" {
					continue
				}
				if i := strings.Index(tag, ","); i >= 0 {
					tag = tag[:i]
				}
				goProps[tag] = true
				if !specProps[tag] {
					t.Errorf("Go field %s.%s (json:%q) missing from OpenAPI schema %q",
						typ.Name(), fld.Name, tag, c.schemaName)
				}
			}
			for k := range specProps {
				if !goProps[k] {
					t.Errorf("OpenAPI schema %q declares property %q but Go type %s has no matching json-tagged field",
						c.schemaName, k, typ.Name())
				}
			}
		})
	}
}
