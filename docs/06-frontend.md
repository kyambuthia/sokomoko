# Frontend

The UI is server-rendered HTML with plain CSS and JavaScript. There is no
bundler, transpiler, or package manager. Everything under `internal/ui/` is
embedded in the binary.

```text
internal/ui/
  ui.go                 go:embed for templates/ and static/
  templates.go          ParseTemplates: one *template.Template per page
  funcs.go              template functions: asset, dict, json, rfc3339
  assets.go             AssetURL cache-busting
  product_images.go     ProductImageURL and bundled image fallback
  templates/
    base/               layout and shared partials
    pages/              one file per page
  static/
    stylesheets/styles.css
    scripts/            theme-init.js, api.js, main.js, components.js, dynamic.js
    images/, icons/, manifest.json
```

## Templates

`ParseTemplates` parses the base set once, then clones it for each page file,
so every page gets its own template tree and pages cannot override each other.
Handlers render the `root_template` entry point.

**Layout** (`base/base.tmpl.html`) defines the document and these blocks:

| Block | Default | Overridden by |
| --- | --- | --- |
| `title` | `Sokomoko` | most pages |
| `body-class` | `site site--storefront` | workspace pages use `site site--workspace site--nav-left-rail` |
| `header-area` | storefront header | workspace pages pick `admin-header-area` or `partner-header-area` |
| `nav-area` | storefront nav | workspace pages pick `admin-nav-area` or `partner-nav-area` |
| `main-area` | empty | every page |
| `footer-area` | site footer | |

**Shared partials** (`base/partials.tmpl.html`) define `product-card`, `flash`,
and `order-status`, meant to be called with `dict`, for example
`{{ template "product-card" dict "Product" . "CSRFToken" $.CSRFToken }}`.
No page calls them yet; pages such as `search.html` still inline their own
product markup.

**Page data.** Every page struct carries `CSRFToken`, which the layout writes to
`<meta name="csrf-token">` and forms include as a hidden `csrf_token` field.
Storefront pages also carry a `Viewer` (signed in, username).

**Template functions.**

| Function | Purpose |
| --- | --- |
| `asset "path"` | `/static/<path>?v=<version>`. The version is the file's modification time on disk in development, otherwise the first 12 hex characters of the SHA-256 of the embedded file. |
| `dict k v ...` | Builds a map for partial arguments. |
| `json v` | JSON for an HTML attribute (escaped by `html/template`). |
| `rfc3339 t` | UTC RFC 3339 timestamp for components such as `ui-countdown`. |

**Product images.** `ui.ProductImageURL` keeps local paths and `data:` images.
Remote or empty URLs are replaced with a bundled image picked from keywords in
the product's slug, name, or category (falling back to a stable hash), because
the CSP only allows same-origin images.

## Static assets

`/static/` is served by `routes.Static`. It first looks for the file under
`internal/ui/static/` on disk (so CSS and JS edits show immediately when you run
from the repository root), then falls back to the embedded copy. In the
production image there is no source tree, so only embedded files are served.

## CSS

`static/stylesheets/styles.css` is one file organised in this order:

1. **Design tokens** on `:root`: colours (canvas, surface, foreground, border,
   accent, semantic), spacing, radii, shadows, typography. Dark values apply
   under `prefers-color-scheme: dark` and `[data-theme="dark"]`.
2. **Base**, **Layout**, **Areas** (header, nav, main, footer).
3. **Components**: navigation, controls, buttons, cards, badges, alerts,
   page-specific blocks, product grid and tiles, auth panels, admin and partner
   pages, tables.
4. **Motion**, legacy compatibility classes, and responsive rules.
5. **Web component hosts** (breadcrumb, price, quantity stepper, toast).
6. **Refresh layers** added later for the storefront, commerce flow, workspace,
   and entry/setup pages. These override earlier rules.

Class names are BEM-style (`storefront-header__category-link--active`). Use
tokens rather than literal colours so both themes keep working.

## JavaScript

Scripts load from the layout in this order. All are external files because the
CSP forbids inline scripts.

| File | Loaded | Role |
| --- | --- | --- |
| `theme-init.js` | sync, in `<head>` | Applies the saved light or dark theme before first paint |
| `api.js` | `defer` | `window.SokomokoAPI.request(path, options)`: fetch wrapper for `/api/v1` that sends JSON, adds `X-CSRF-Token`, and unwraps `{data}` or throws on `{error}` |
| `main.js` | `defer` | Presentational components (below), theme toggle, password visibility toggles, nav toggles, submit-button states, global `UIToast` |
| `components.js` | `defer` | Behavioural components that enhance server-rendered forms (below); `data-submit-once` double-submit guard |
| `dynamic.js` | `defer`, end of `<body>` | Turns cart, checkout, and workspace order forms into `/api/v1` calls and refreshes metrics in place |

`static/scripts/search.js` exists but is not loaded by any template.

### Web components

All components are custom elements. Light-DOM components let the site
stylesheet apply; `ui-toast` and `ui-dialog` use shadow DOM.

Defined in `main.js`:

| Element | Used for |
| --- | --- |
| `ui-alert` | Success, danger, warning, and info messages |
| `ui-metric-card` | Dashboard numbers |
| `ui-search-form`, `ui-header-search` | Search forms; the header search queries `/api/v1/catalog/search` |
| `ui-price` | Price with optional strikethrough original |
| `ui-quantity` | Quantity stepper that still submits as a normal form field |
| `ui-badge` | Status badges |
| `ui-breadcrumb` | Breadcrumbs from a JSON `items` attribute |
| `ui-header-banner`, `ui-switch`, `ui-toast`, `ui-dialog` | Banner, toggle switch, toast notifications, modal dialog |

Defined in `components.js`:

| Element | Used for |
| --- | --- |
| `ui-add-to-cart` | Wraps a `/cart/add` form and adds without a reload |
| `ui-cart-line` | Wraps a `/cart/update` form; updates line totals in place |
| `ui-cart-count` | Live header cart badge from `/api/cart` |
| `ui-countdown` | Countdown for the checkout stock hold |
| `ui-order-status` | Fulfillment tracker that polls `/api/orders/status` until delivered or cancelled |
| `ui-category-nav` | Replaces static department links with `/api/categories` |
| `ui-char-counter` | Remaining characters for a textarea |
| `ui-copy` | Copy text to the clipboard |
| `ui-confirm`, `ui-relative-time`, `ui-table-filter` | Confirm before submit, relative timestamps, client-side table filter |

Several components are defined but not placed in any rendered page:
`ui-add-to-cart` and `ui-order-status` appear only in the unused partials, and
`ui-cart-count`, `ui-cart-line`, `ui-confirm`, `ui-relative-time`,
`ui-table-filter`, `ui-header-banner`, `ui-switch`, and `ui-dialog` appear in
no template. Toasts are created from script. Cart and checkout forms are
enhanced by `dynamic.js` instead.

### Progressive enhancement rule

Every form posts to a real server route and works without JavaScript. Scripts
only intercept a submit, call the JSON endpoint, and update the page. When the
API answers `401`, the script falls back to a normal form submit or a redirect
to login.

## Adding a page

1. Create `templates/pages/<name>.html` that defines `main-area` (and the
   workspace blocks if it is an admin or partner page).
2. Add a field to `ui.Templates` and a parse step in `ParseTemplates`.
3. Add a page data struct with `CSRFToken` in `internal/routes`.
4. Register the handler in `routes/register.go`.
5. Add a route test (see `cmd/sokomoko/page_contracts_integration_test.go`).
