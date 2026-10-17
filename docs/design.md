# Design

The rules every page on every host follows: storefront, admin, and partner.
The tenet is Apple's Human Interface Guidelines: **clarity** (content first,
legible at every size), **deference** (the interface gets out of the way), and
**depth** (hierarchy through layering, weight, and space, not decoration).

Source of truth: `internal/ui/static/stylesheets/app.css` (tokens and
components) and `internal/ui/static/scripts/ui/` (behaviour).

## Principles

1. **Black and white only.** No hue anywhere, including status. Hierarchy comes
   from greys, weight, size, and inverted fills.
2. **One typeface.** JetBrains Mono, served from `/static/fonts`. Nothing loads
   from a third party.
3. **Mobile first.** Write the phone layout first, then add `min-width` media
   queries.
4. **No filler.** No eyebrows, taglines, hero copy, or sentences that restate
   the heading. One-word headings where possible. Hints state the real rule
   ("10–72 characters"), never generic advice.
5. **HTML first.** Every flow works as plain forms and links. JavaScript only
   enhances, and only for interaction.
6. **Same system everywhere.** The three hosts share tokens, components, top
   bar, and spacing. Only the brand tag (`admin`, `partner`, `workspace`) and
   the navigation differ.

## Colour

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--bg` | `#fff` | `#000` | Page |
| `--fg` | `#000` | `#fff` | Text, primary buttons, active states |
| `--fg-2` | `#555` | `#a6a6a6` | Secondary text (7:1 or better) |
| `--fg-3` | `#8a8a8a` | `#6e6e6e` | Placeholders, disabled, chevrons |
| `--fill` | `#f4f4f4` | `#111` | Grouped surfaces, image wells, hover |
| `--fill-2` | `#e8e8e8` | `#222` | Pressed, empty progress |
| `--line` | 12% ink | 16% ink | Separators |
| `--line-strong` | 28% ink | 34% ink | Input and outline borders |

Dark mode is the same palette inverted. It follows the system setting; the
moon/sun button stores an explicit choice under `sokomoko-theme`.

**Status without colour:**

- Emphasis or "done": solid ink (`.badge-solid`, filled progress, active chip).
- Neutral: outlined (`.badge`, `.chip`).
- Inactive, suspended, or cancelled: dashed or muted (`.badge-muted`, dashed
  progress).
- Errors: an inverted notice (`.notice-error`), ink background with paper
  text, plus an alert icon. Success and info notices are outlined with their
  own icon.

## Type

JetBrains Mono, variable weight 100–800, Latin subset (OFL, licence in
`static/fonts/OFL.txt`). Figures are tabular, so prices line up.

| Token | Size | Use |
| --- | --- | --- |
| `--t-xs` | 12px | Captions, table headers, tab bar labels |
| `--t-sm` | 13px | Secondary text, chips, table cells |
| `--t-md` | 15px | Body |
| `--t-lg` | 18px | `h2`, brand |
| `--t-xl` | 22px | `h1`, stat values |
| `--t-2xl` | 28px | Product price |

Headings are 600–700 with slight negative tracking. Inputs use 16px so iOS
does not zoom on focus. Section labels (`.section-title`) are uppercase
12px with 0.06em tracking, like HIG grouped table headers.

## Space, shape, and size

- 4px grid: `--s1` 4, `--s2` 8, `--s3` 12, `--s4` 16, `--s5` 24, `--s6` 32,
  `--s7` 48.
- Page gutter 16px on phones, 24px from 56rem. Content max width 72rem.
- **Buttons are capsules** (`--pill`). Cards and lists use 12px, sheets and
  dialogs 16px, inputs 8px.
- **Touch targets are at least 44px** (`--tap`). Small buttons (32px) appear
  only in dense desktop tables and secondary row actions.
- No drop shadows on content. Only floating layers (toasts, dialogs, search
  results) get `--shadow`.

## Layout

Breakpoints: 40rem (wider grids), 48rem (tablet: desktop header, no tab bar),
56rem (two-column split), 64rem (workspace sidebar).

**Storefront**

- Sticky translucent top bar: brand, search field (48rem and up), theme,
  account, cart.
- Category chips under the bar on Shop and Search only.
- Phone tab bar: Shop, Search, Cart, Account. The current tab is bold ink.
- Product page: on phones the quantity stepper and Add to cart sit in a sticky
  bar above the tab bar.
- Cart and checkout: list plus summary card. The summary moves above the form
  on phones and becomes a sticky side column from 56rem.

**Workspaces (admin and partner)**

- The same top bar with a brand tag and a sign-out icon.
- Phones: a horizontally scrolling tab strip under the bar. From 64rem: a
  15rem sidebar.
- Pages open with an `h1` and at most one primary action on the right.

**Auth** (sign in, sign up, reset, first-run setup): brand and theme only, one
24rem column.

## Components

### CSS components

| Class | Purpose |
| --- | --- |
| `.btn` | Solid capsule, the primary action. One per view where possible. |
| `.btn-secondary` | Outlined capsule. |
| `.btn-plain` | Text button with hover fill. |
| `.btn-icon` | 44px circular icon button; always needs `aria-label`. |
| `.btn-sm`, `.btn-block` | 32px height; full width. |
| `.field`, `.label`, `.hint`, `.input`, `.select`, `.textarea`, `.choice` | Form controls. `.choice` is a bordered radio or checkbox row. |
| `.form`, `.form-row` | Vertical form; responsive row of fields. |
| `.search` | Capsule search field with a leading icon. |
| `.card`, `.list`, `.list-item` | Surface; HIG inset grouped list. |
| `.kv` | Key/value rows with an optional `.total`. |
| `.stats`, `.stat` | Dashboard numbers. |
| `.badge` (+ `-solid`, `-muted`) | Status. Lowercase. |
| `.notice` (+ `-error`) | Inline message with icon. |
| `.chips`, `.chip` | Horizontally scrolling filters or categories. |
| `.table-wrap`, `.table` | Dense data. Scrolls sideways on phones. |
| `.empty` | Centered empty state: icon, one line, optional action. |
| `.products`, `.product` | Two to four column product grid; the whole card is the link. |
| `.page-head`, `.stack`, `.row`, `.split` | Layout primitives. |

Template partials in `templates/base/partials.tmpl.html`: `product-card`,
`product-grid`, `notice`, `empty`. Pass arguments with `dict`.

### Icons

Tabler Icons outline (MIT), built into `static/icons/sprite.svg` by
`go run ./scripts/iconsprite`. Render with `{{ icon "name" }}`. Icons are
decorative (`aria-hidden`); the parent supplies the label. Stroke 1.75,
`currentColor`, 1.25em. To add an icon, append it to the list in
`scripts/iconsprite/main.go` and regenerate.

### Web components

Each component wraps server-rendered HTML that already works without
JavaScript. They are ES modules in `static/scripts/ui/`, registered by
`static/scripts/app.js`, and share `ui/core.js` (API calls with CSRF, cart
events, toasts, icons, money formatting).

| Element | Wraps | Behaviour |
| --- | --- | --- |
| `ui-theme-toggle` | button | Switches light/dark and remembers it. |
| `ui-search` | search form | Combobox suggestions from `/api/v1/catalog/search`; arrow keys, Enter, Escape. |
| `ui-category-nav` | chip row | Appends every category after the server-rendered "All". |
| `ui-cart-badge` | cart link with `.count` | Live item count; listens for cart events. |
| `ui-add-to-cart` | `/cart/add` form | Adds through the API, toasts, updates badges; signs in on 401. |
| `ui-cart-line` | cart row | Quantity changes and removal in place; updates line total and subtotal. |
| `ui-quantity` | number input | Minus/plus stepper; `autosubmit` submits after a pause. |
| `ui-countdown` | badge | Live countdown for checkout stock holds. |
| `ui-order-progress` | status text | Four-step progress (Placed, Accepted, Shipped, Delivered); polls open orders every 30s. |
| `ui-order-form` | fulfillment form | Saves admin or partner order updates through the API and updates badges. |
| `ui-confirm` | destructive form | Native `<dialog>` confirmation before submit. |
| `ui-password` | password input | Show/hide button inside the field. |
| `ui-table-filter` | (empty) | Search field that filters a table's rows. |
| `ui-relative-time` | absolute time | "3 hours ago", full time as tooltip. |
| `ui-copy` | button | Copies text and confirms with a toast. |
| `ui-char-counter` | (empty) | Live count for a textarea's `maxlength`. |
| `toast()` (in `core.js`) | – | Ink capsule above the tab bar, announced politely, auto-dismissed. |

Rules for new components:

- Prefix with `ui-`, use the light DOM, and add the element to `app.js`.
- Enhance existing markup; never render content that is missing without
  JavaScript.
- Use `api()` from `core.js` for `/api/v1`, so CSRF and errors are handled
  consistently. On 401 call `loginRedirect()`.
- Use classes from `app.css`; no inline styles (the CSP allows inline styles,
  but the system does not).
- Respect `prefers-reduced-motion`; keep motion under 200ms.

## Accessibility

- Every page has a skip link, one `h1`, and landmark regions.
- Focus is a 2px ink outline offset by 2px. Never remove it without a
  replacement.
- Icon-only buttons need `aria-label`. Status changes use `role="status"` or
  `role="alert"`.
- Text contrast is at least 4.5:1 in both themes (secondary text is about 7:1).
- Forms keep native validation attributes (`required`, `minlength`,
  `pattern`), mirroring the server rules.

## Writing

- Sentence case for headings and buttons ("Add to cart", "Sign in").
- Verbs on buttons; nouns for headings.
- Numbers over words: "3 items", "Held for 14:59".
- Errors say what to do: "Password must be 10 to 72 characters".
- No exclamation marks or marketing language.
