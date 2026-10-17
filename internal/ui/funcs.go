package ui

import (
	"encoding/json"
	"errors"
	"html/template"
	"time"
)

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"asset":   AssetURL,
		"icon":    Icon,
		"dict":    dict,
		"json":    toJSON,
		"rfc3339": rfc3339,
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

func rfc3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
