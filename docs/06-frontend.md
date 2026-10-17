# Frontend

Server-rendered HTML with one stylesheet and a small library of ES-module web
components. There is no bundler or package manager; everything under
`internal/ui/` is embedded in the binary. Visual rules, tokens, and the
component catalogue are in [design.md](design.md).

```text
internal/ui/
  ui.go                 go:embed for templates/ and static/
  templates.go          ParseTemplates: one *template.Template per page
  funcs.go              template functions: asset, icon, dict, list, field, signedIn, json, rfc3339
  assets.go             AssetURL cache-busting
  product_images.go     ProductImageURL and bundled image fallback
  templates/
    base/               document, layouts, headers, navigation, footer, partials
    pages/              one file per page
  static/
    stylesheets/app.css design system
    scripts/theme.js    applies the saved theme before first paint
    scripts/app.js      registers the components
    scripts/ui/         one module per component, plus core.js
    fonts/              JetBrains Mono (OFL)
    icons/              sprite.svg (Tabler, MIT), brand mark
    images/, manifest.json
scripts/iconsprite/     regenerates icons/sprite.svg
```

## Templates

`ParseTemplates` parses the base set once, then clones it per page, so pages
cannot override one another. Handlers render the `root_template` entry point.

**Layouts.** `base.tmpl.html` defines the document head and a `layout` block.
The default is `storefront-layout`. Other pages select theirs with one line:

| Layout | Used by | Contents |
| --- | --- | --- |
| `storefront-layout` | shop, search, product, cart, checkout, account | Top bar, content, footer (desktop), tab bar (phones) |
| `auth-layout` | sign in, sign up, password reset | Brand and theme only |
| `admin-layout` | `admin.html`, `staff_signup.html` | Workspace bar (`admin` tag), admin nav, content |
| `partner-layout` | `partner_*.html` | Workspace bar (`partner` tag), partner nav, content |
| `workspace-auth-layout` | workspace sign in, first-run setup | Workspace bar (`workspace` tag), content |

**Blocks pages define:**

| Block | Purpose |
| --- | --- |
| `title` | Document title, `Page · sokomoko` |
| `layout` | Optional: choose a non-storefront layout |
| `main-area` | Page content |
| `tabbar-area` | Storefront: `{{ template "tabbar" dict "Tab" "cart" "Page" . }}` marks the current tab |
| `header-chips` | Storefront: `{{ template "category-chips" . }}` on browse pages |
| `body-class` | Storefront: extra body classes (for example `has-buybar`) |

**Shared templates.** `product-card`, `product-grid`, `notice`, and `empty`
(in `partials.tmpl.html`) take arguments through `dict`.

**Page data.** Every page struct has `CSRFToken`, written to
`<meta name="csrf-token">` and to a hidden `csrf_token` field in each form.
Storefront pages carry a `Viewer`. The shared header reads optional fields
through `field` and `signedIn`, so page structs need not all have the same
fields.

**Template functions.**

| Function | Purpose |
| --- | --- |
| `asset "path"` | `/static/<path>?v=<version>`: file modification time in development, else a content hash. |
| `icon "name"` | Inline SVG that references the sprite symbol. |
| `dict k v ...`, `list a b ...` | Build arguments for partials; build string lists for selects. |
| `field . "Name"`, `signedIn .` | Read optional page data from shared templates. |
| `json v`, `rfc3339 t` | JSON for attributes; UTC timestamps for components. |

**Product images.** `ui.ProductImageURL` keeps local paths and `data:` images
and replaces remote or empty URLs with a bundled image, because the CSP allows
only same-origin images.

## Static assets

`/static/` is served by `routes.Static`. When the server runs from the
repository root, files on disk under `internal/ui/static/` take precedence, so
CSS and JS edits show on reload. Template changes need a restart. The
production image serves only the embedded copy.

## JavaScript

The base layout loads `theme.js` (blocking, tiny) and `app.js` as a module.
`app.js` imports every component from `scripts/ui/` and adds a
`data-submit-once` guard against double submission. All scripts are files,
because the CSP forbids inline scripts. The components are listed in
[design.md](design.md#web-components).

`make lint` and CI run `node --check` on `scripts/*.js` and `scripts/ui/*.js`.

## Adding a page

1. Create `templates/pages/<name>.html` with `title` and `main-area`, plus
   `layout` for a workspace or auth page.
2. Add a field to `ui.Templates` and a parse step in `ParseTemplates`.
3. Add a page data struct with `CSRFToken` in `internal/routes`.
4. Register the handler in `routes/register.go`.
5. Add the page to `cmd/sokomoko/page_contracts_integration_test.go`.
6. Follow [design.md](design.md): reuse components, no filler text, check at
   390px and 1440px in both themes.
