package ui

import (
	"encoding/json"
	"errors"
	"html/template"
	"reflect"
	"time"
)

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"asset":    AssetURL,
		"icon":     Icon,
		"list":     list,
		"field":    field,
		"signedIn": signedIn,
		"dict":     dict,
		"json":     toJSON,
		"rfc3339":  rfc3339,
	}
}

// dict builds a map from alternating keys and values so partial templates can
// receive several named arguments.
func dict(pairs ...any) (map[string]any, error) {
	if len(pairs)%2 != 0 {
		return nil, errors.New("dict requires key/value pairs")
	}
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		out[key] = pairs[i+1]
	}
	return out, nil
}

// toJSON encodes v for use inside an HTML attribute; html/template escapes the
// result for the attribute context.
func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// Icon renders a decorative icon from the sprite. Names are the template
// names listed in scripts/iconsprite. Pair an icon with visible text or an
// aria-label on its parent; the SVG itself is hidden from assistive tech.
func Icon(name string) template.HTML {
	href := AssetURL("icons/sprite.svg") + "#i-" + template.HTMLEscapeString(name)
	return template.HTML(`<svg class="icon" aria-hidden="true" focusable="false"><use href="` + href + `"></use></svg>`)
}

// list builds a string slice in templates, for example the options of a
// status select.
func list(items ...string) []string { return items }

// field returns the named exported field of a page data struct, or "" when
// the struct has no such field. Shared layout templates use it for optional
// values such as the search query, because page structs differ.
func field(data any, name string) any {
	v := reflect.ValueOf(data)
	for v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName(name)
	if !f.IsValid() || !f.CanInterface() {
		return ""
	}
	return f.Interface()
}

// signedIn reports whether page data carries a signed-in Viewer.
func signedIn(data any) bool {
	viewer := field(data, "Viewer")
	if viewer == "" {
		return false
	}
	ok, _ := field(viewer, "SignedIn").(bool)
	return ok
}

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
