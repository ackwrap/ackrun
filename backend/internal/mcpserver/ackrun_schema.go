package mcpserver

import (
	"reflect"
	"slices"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// REST request models use Gin binding tags for required fields. Preserve those
// requirements instead of treating every non-omitempty zero value as mandatory.
// The Go JSON decoder also accepts null for optional slices and pointers.
func adaptAPISchema(schema *jsonschema.Schema, typ reflect.Type) {
	if schema == nil {
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if schema.Type == "integer" {
		// The pinned SDK validates JSON numbers through float64. Reject IDs
		// outside the exact JSON integer range rather than rounding a target ID.
		schema.Minimum = jsonschema.Ptr(float64(-9007199254740991))
		schema.Maximum = jsonschema.Ptr(float64(9007199254740991))
	}
	if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
		adaptAPISchema(schema.Items, typ.Elem())
		return
	}
	if typ.Kind() != reflect.Struct {
		return
	}
	modelType := typ.PkgPath() == "github.com/ackwrap/ackrun/internal/model"
	for _, field := range reflect.VisibleFields(typ) {
		key := strings.Split(field.Tag.Get("json"), ",")[0]
		property := schema.Properties[key]
		if property == nil {
			continue
		}
		required := slices.Contains(strings.Split(field.Tag.Get("binding"), ","), "required")
		if !required && (modelType || field.Type.Kind() == reflect.Pointer) {
			schema.Required = slices.DeleteFunc(schema.Required, func(name string) bool { return name == key })
		}
		if !required && (field.Type.Kind() == reflect.Pointer || field.Type.Kind() == reflect.Slice) && property.Type != "" {
			property.Types = []string{property.Type, "null"}
			property.Type = ""
		}
		adaptAPISchema(property, field.Type)
	}
}
