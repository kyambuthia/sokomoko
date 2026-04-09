# Sokomoko Design Constitution

This document defines the visual and interaction contract for Sokomoko. It is the default source of truth for any AI agent changing HTML, CSS, or vanilla JavaScript in this repository.

Use this document to extend the existing UI, not to replace it. The direction is paper-clean, structured, quiet, and practical: light surfaces, restrained color, capsule controls, system-native type, strong spacing rhythm, and progressive enhancement.

## 1. Philosophy & Principles

### Core stance

- Build interfaces that feel like a well-laid sheet of paper: calm, readable, bordered, and lightly tactile.
- Prefer structure over decoration. Use spacing, borders, and weight before color, shadow, or motion.
- Default to white or warm off-white surfaces on neutral backgrounds.
- Keep the UI legible without JavaScript. JavaScript may enhance, never gate, core tasks.
- Reuse established primitives before adding new variants.

### Mandatory principles

- Mobile-first: write the smallest-screen version first, then layer enhancements with `min-width` breakpoints.
- BEM-first: use `block__element--modifier`; keep selectors shallow and local.
- System fonts only: no hosted fonts, no font downloads, no icon fonts.
- Capsule controls: buttons, nav links, filters, segmented controls, and toggles should use pill radii unless a square edge is semantically better.
- Token-only styling: every reusable visual value must come from a CSS custom property.
- Enhancement-only JavaScript: forms, navigation, and auth must remain functional without JS.
- Accessibility by default: focus visibility, keyboard reachability, and WCAG AA contrast are baseline requirements.

### Visual personality

- Quiet, not sterile.
- Monospace-led, but not cramped.
- Neutral palette with a dark slate accent.
- Borders are the primary separator.
- Shadows are minimal and low-contrast.
- Motion is brief and functional.

### Implementation rules

- Prefer semantic HTML first, then add classes.
- Do not introduce one-off spacing, color, or radius values in components.
- Do not create a new component when a modifier on an existing block is enough.
- Avoid deeply nested wrappers unless needed for layout or accessibility.
- If a component needs JavaScript, define its no-JS fallback in markup first.

## 2. Design Tokens

All new CSS should extend this token model. Existing tokens in `internal/ui/static/stylesheets/styles.css` should trend toward these names over time.

### Color tokens

Use warm neutrals for structure and reserved accents for meaning. Semantic colors must preserve legibility on both light and dark schemes.

```css
:root {
  color-scheme: light dark;

  /* Backgrounds */
  --color-bg-canvas: #ece8df;
  --color-bg-surface: #fffdfa;
  --color-bg-surface-muted: #f7f3eb;
  --color-bg-surface-subtle: #fbf8f2;
  --color-bg-elevated: #ffffff;

  /* Foreground */
  --color-fg-primary: #1f1d19;
  --color-fg-secondary: #595449;
  --color-fg-muted: #726b5f;
  --color-fg-inverse: #fffdfa;

  /* Borders */
  --color-border-default: #d7d0c3;
  --color-border-subtle: #e6dfd3;
  --color-border-strong: #bcb3a2;

  /* Brand and semantic */
  --color-accent: #2f3e4f;
  --color-accent-hover: #243240;
  --color-accent-soft: #dbe5ee;

  --color-success: #24563a;
  --color-success-soft: #edf7f0;
  --color-success-border: #9ebda8;

  --color-warning: #6d571b;
  --color-warning-soft: #faf4e6;
  --color-warning-border: #d3be8d;

  --color-danger: #7b2929;
  --color-danger-soft: #fbefef;
  --color-danger-border: #d7aaaa;

  --color-info: #32516a;
  --color-info-soft: #eef5fa;
  --color-info-border: #b7cada;

  /* Focus and overlays */
  --color-focus-ring: #d9e5ef;
  --color-overlay: rgb(31 29 25 / 0.08);
  --shadow-color: rgb(31 29 25 / 0.06);
}

@media (prefers-color-scheme: dark) {
  :root {
    --color-bg-canvas: #171513;
    --color-bg-surface: #201d1a;
    --color-bg-surface-muted: #26221f;
    --color-bg-surface-subtle: #2b2723;
    --color-bg-elevated: #24211e;

    --color-fg-primary: #f7f2ea;
    --color-fg-secondary: #d3ccbf;
    --color-fg-muted: #b2ab9d;
    --color-fg-inverse: #171513;

    --color-border-default: #4b443a;
    --color-border-subtle: #3a352e;
    --color-border-strong: #62594d;

    --color-accent: #b8cadb;
    --color-accent-hover: #d7e3ee;
    --color-accent-soft: #314354;

    --color-success: #a9d4b5;
    --color-success-soft: #203026;
    --color-success-border: #486854;

    --color-warning: #e3ca8a;
    --color-warning-soft: #342b16;
    --color-warning-border: #756239;

    --color-danger: #ebb2b2;
    --color-danger-soft: #351b1b;
    --color-danger-border: #7f4b4b;

    --color-info: #bdd8ed;
    --color-info-soft: #1e2d39;
    --color-info-border: #496378;

    --color-focus-ring: #425b72;
    --color-overlay: rgb(247 242 234 / 0.08);
    --shadow-color: rgb(0 0 0 / 0.24);
  }
}
```

### Color usage rules

- Use `--color-bg-canvas` for the page background.
- Use `--color-bg-surface` for the main sheet, cards, forms, and menus.
- Use `--color-bg-surface-muted` for headers, footers, table heads, and secondary sections.
- Use `--color-fg-primary` for body copy and headings.
- Use `--color-fg-secondary` for helper text, subtitles, and supporting labels.
- Use `--color-fg-muted` for tertiary metadata only.
- Use `--color-accent` for primary actions, active states, and links.
- Use semantic colors only for states, alerts, stock, validation, and irreversible actions.

### Typography tokens

Sokomoko uses local/system monospace stacks only. The tone should feel editorial and technical, not developer-tool harsh.

```css
:root {
  --font-family-base: ui-monospace, "SFMono-Regular", Menlo, Monaco, Consolas,
    "Liberation Mono", "Courier New", monospace;
  --font-family-mono: var(--font-family-base);

  --font-size-xs: 0.75rem;
  --font-size-sm: 0.875rem;
  --font-size-base: 0.96875rem;
  --font-size-lg: 1.125rem;
  --font-size-xl: 1.3125rem;
  --font-size-2xl: 1.625rem;
  --font-size-3xl: 2rem;

  --line-height-tight: 1.2;
  --line-height-snug: 1.35;
  --line-height-base: 1.55;
  --line-height-loose: 1.7;

  --letter-spacing-tight: -0.01em;
  --letter-spacing-base: 0;
  --letter-spacing-wide: 0.04em;
  --letter-spacing-caps: 0.08em;

  --font-weight-normal: 400;
  --font-weight-medium: 500;
  --font-weight-semibold: 600;
  --font-weight-bold: 700;
}
```

### Typography rules

- `body` uses `--font-size-base`, `--line-height-base`, `--font-family-base`.
- `h1` through `h3` stay compact and weight-driven, not oversized.
- Uppercase labels and metadata use `--letter-spacing-caps`.
- Monospace is the brand voice, so avoid mixing in serif or sans-serif stacks.
- Limit long-form line length to approximately 60 to 72 characters where practical.

### Spacing tokens

The spacing scale is fixed. Do not invent intermediate ad-hoc values unless the token set is formally expanded.

```css
:root {
  --space-0: 0;
  --space-1: 0.25rem;
  --space-2: 0.5rem;
  --space-3: 0.75rem;
  --space-4: 1rem;
  --space-5: 1.25rem;
  --space-6: 1.5rem;
  --space-8: 2rem;
  --space-10: 2.5rem;
  --space-12: 3rem;
  --space-16: 4rem;
}
```

### Spacing rules

- Between label and input: `--space-2`.
- Between input and inline hint/error: `--space-2`.
- Between form groups: `--space-4`.
- Card internal padding: `--space-4` on mobile, `--space-6` in larger compositions.
- Section spacing inside a page: `--space-8`.
- Page shell padding: `--space-4` mobile, `--space-6` tablet, `--space-8` desktop.

### Radius, border, shadow, and layout tokens

```css
:root {
  --radius-sm: 0.375rem;
  --radius-md: 0.625rem;
  --radius-lg: 0.875rem;
  --radius-xl: 1.25rem;
  --radius-pill: 999px;

  --border-thin: 1px;
  --border-thick: 2px;

  --shadow-xs: 0 1px 0 var(--shadow-color);
  --shadow-sm: 0 1px 2px var(--shadow-color);
  --shadow-focus: 0 0 0 3px var(--color-focus-ring);

  --container-sm: 40rem;
  --container-md: 48rem;
  --container-lg: 64rem;
  --container-xl: 80rem;

  --transition-fast: 150ms;
  --transition-base: 220ms;
  --transition-slow: 300ms;
  --ease-emphasized: cubic-bezier(0.2, 0, 0, 1);
  --ease-standard: ease-in-out;
  --ease-exit: ease-in;
}
```

### Base shell example

```css
html {
  color-scheme: light dark;
}

body {
  margin: 0;
  background: var(--color-bg-canvas);
  color: var(--color-fg-primary);
  font: var(--font-weight-normal) var(--font-size-base) / var(--line-height-base)
    var(--font-family-base);
}

.site__header,
.site__main,
.site__footer {
  max-width: var(--container-lg);
  margin-inline: auto;
  background: var(--color-bg-surface);
  border-inline: var(--border-thin) solid var(--color-border-default);
  box-shadow: var(--shadow-xs);
}
```

## 3. Component Library

All components below are opinionated defaults. Modifiers may simplify or mute a component; they must not change its underlying tone.

### Buttons

#### Rules

- Use `.btn` as the base class.
- All button variants are capsule-shaped by default.
- Buttons must work on `<button>`, `<a>`, and submit inputs.
- Buttons align icon and label horizontally with equal internal spacing.
- Use primary sparingly. Most utility actions should be secondary or ghost.

#### Variants

- `.btn--primary`: primary action, filled accent.
- `.btn--secondary`: paper surface with border.
- `.btn--outline`: transparent with visible border.
- `.btn--ghost`: no fill, no heavy border, for low emphasis.
- `.btn--destructive`: destructive action only.

#### Sizes

- `.btn--sm`: compact inline actions.
- `.btn--md`: default.
- `.btn--lg`: prominent actions in auth, checkout, or hero areas.
- `.btn--full`: full available width on small screens and form stacks.

#### States

- Default: readable, high-contrast, clear border.
- Hover: slightly darker fill or stronger border.
- Active: slight inset feel via darker background; never dramatic movement.
- Disabled: reduced contrast, no pointer affordance.
- Loading: label remains stable or changes to a loading label; reserve width to prevent layout shift.

```css
.btn {
  --btn-bg: var(--color-bg-elevated);
  --btn-fg: var(--color-fg-primary);
  --btn-border: var(--color-border-strong);

  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  min-height: 2.5rem;
  padding-inline: var(--space-4);
  padding-block: var(--space-3);
  border: var(--border-thin) solid var(--btn-border);
  border-radius: var(--radius-pill);
  background: var(--btn-bg);
  color: var(--btn-fg);
  font: inherit;
  line-height: 1;
  text-decoration: none;
  transition:
    background var(--transition-base) var(--ease-standard),
    border-color var(--transition-base) var(--ease-standard),
    color var(--transition-base) var(--ease-standard),
    box-shadow var(--transition-fast) var(--ease-standard);
}

.btn--primary {
  --btn-bg: var(--color-accent);
  --btn-fg: var(--color-fg-inverse);
  --btn-border: var(--color-accent);
}

.btn--secondary {
  --btn-bg: var(--color-bg-elevated);
  --btn-fg: var(--color-fg-primary);
  --btn-border: var(--color-border-strong);
}

.btn--outline {
  --btn-bg: transparent;
  --btn-fg: var(--color-fg-primary);
  --btn-border: var(--color-border-strong);
}

.btn--ghost {
  --btn-bg: transparent;
  --btn-fg: var(--color-fg-secondary);
  --btn-border: transparent;
}

.btn--destructive {
  --btn-bg: var(--color-danger);
  --btn-fg: var(--color-fg-inverse);
  --btn-border: var(--color-danger);
}

.btn:hover {
  background: color-mix(in srgb, var(--btn-bg) 88%, var(--color-bg-surface-muted));
  border-color: color-mix(in srgb, var(--btn-border) 78%, var(--color-fg-primary));
}

.btn:active {
  background: color-mix(in srgb, var(--btn-bg) 82%, var(--color-fg-primary));
}

.btn:disabled,
.btn[aria-disabled="true"],
.btn.is-loading {
  cursor: not-allowed;
  opacity: 0.68;
}

.btn--sm {
  min-height: 2.125rem;
  padding-inline: var(--space-3);
  font-size: var(--font-size-sm);
}

.btn--lg {
  min-height: 2.875rem;
  padding-inline: var(--space-5);
  font-size: var(--font-size-lg);
}

.btn--full {
  width: 100%;
}
```

#### Icon support

- Use `.btn__icon` for leading or trailing icons.
- Icons should inherit text color and size to `1em`.
- Default gap between icon and text: `--space-2`.
- Icon-only buttons must still have an accessible name.

### Cards

#### Rules

- Use cards to group related information, not as generic wrappers for everything.
- Default cards are bordered and quiet.
- Use elevated cards sparingly to call attention to a panel without breaking the paper aesthetic.
- Clickable cards must keep a visible focus state on the interactive root.

#### Variants

- `.card`: default bordered card.
- `.card--elevated`: stronger shadow and slightly raised surface.
- `.card--bordered`: explicit border-forward treatment.
- `.card--interactive`: clickable or hoverable summary card.

```css
.card {
  background: var(--color-bg-surface);
  border: var(--border-thin) solid var(--color-border-strong);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-xs);
  overflow: clip;
}

.card--elevated {
  background: var(--color-bg-elevated);
  box-shadow: var(--shadow-sm);
}

.card__header,
.card__footer {
  padding: var(--space-4);
  background: var(--color-bg-surface-subtle);
}

.card__header {
  border-bottom: var(--border-thin) solid var(--color-border-default);
}

.card__body {
  padding: var(--space-4);
}

.card__footer {
  border-top: var(--border-thin) solid var(--color-border-default);
}

.card--interactive {
  transition:
    border-color var(--transition-base) var(--ease-standard),
    background var(--transition-base) var(--ease-standard),
    box-shadow var(--transition-base) var(--ease-standard);
}

.card--interactive:hover {
  border-color: var(--color-fg-muted);
  background: var(--color-bg-elevated);
}
```

### Form inputs

#### Rules

- Labels are mandatory unless a control has an equivalent accessible name.
- Each field group should keep label, control, hint, and validation message adjacent.
- Inputs are rectangular with small radii; buttons remain pill-shaped.
- Read-only fields should look intentionally non-editable, not disabled.

#### Field structure

```html
<div class="form__group">
  <label class="form__label" for="email">Email</label>
  <input class="form__input" id="email" type="email" aria-describedby="email-hint">
  <p class="form__hint" id="email-hint">Use the address tied to your account.</p>
</div>
```

#### Input states

- Default: bordered, surface background.
- Focus: accent border and visible focus ring.
- Error: danger border, danger hint text, `aria-invalid="true"`.
- Disabled: muted foreground and muted surface.
- Read-only: visually distinct but still selectable.

```css
.form__group {
  display: grid;
  gap: var(--space-2);
}

.form__label {
  color: var(--color-fg-primary);
  font-size: var(--font-size-sm);
  font-weight: var(--font-weight-semibold);
}

.form__input,
.form__textarea,
.form__select {
  width: 100%;
  min-height: 2.75rem;
  padding-inline: var(--space-4);
  padding-block: var(--space-3);
  border: var(--border-thin) solid var(--color-border-strong);
  border-radius: var(--radius-sm);
  background: var(--color-bg-elevated);
  color: var(--color-fg-primary);
  font: inherit;
  transition:
    border-color var(--transition-fast) var(--ease-standard),
    box-shadow var(--transition-fast) var(--ease-standard),
    background var(--transition-fast) var(--ease-standard);
}

.form__input:focus-visible,
.form__textarea:focus-visible,
.form__select:focus-visible {
  border-color: var(--color-accent);
  box-shadow: var(--shadow-focus);
  outline: 0;
}

.form__input[aria-invalid="true"],
.form__textarea[aria-invalid="true"],
.form__select[aria-invalid="true"] {
  border-color: var(--color-danger);
}

.form__input:disabled,
.form__textarea:disabled,
.form__select:disabled {
  background: var(--color-bg-surface-muted);
  color: var(--color-fg-muted);
  cursor: not-allowed;
}

.form__input[readonly],
.form__textarea[readonly] {
  background: var(--color-bg-surface-subtle);
  color: var(--color-fg-secondary);
}

.form__hint,
.form__message {
  margin: 0;
  font-size: var(--font-size-sm);
  color: var(--color-fg-secondary);
}

.form__message--error {
  color: var(--color-danger);
}
```

#### Textarea

- Default minimum height: `7.5rem`.
- Resize vertically only unless a scripted autosize enhancement is present.
- If autosize is added, it must preserve manual keyboard entry and no-JS fallback.

#### Select

- Use native `<select>` by default.
- Keep native affordances where possible.
- Avoid replacing native selects with custom popovers unless required by a real product need.

#### Checkbox and radio

- Keep label clickable.
- Minimum target size: `44px` tall row or equivalent tap area.
- Use accent color when supported, with border fallback.

```css
.form__check {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  color: var(--color-fg-primary);
}

.form__check-input {
  inline-size: 1rem;
  block-size: 1rem;
  margin: 0.125rem 0 0;
  accent-color: var(--color-accent);
}
```

### Search form

#### Rules

- Search must work as a normal GET form without JS.
- Enhancements may add clear, loading, autosubmit, or suggestion behavior, but must not replace the plain form contract.
- The search icon is decorative unless it acts as a submit control.

#### Required parts

- Search input.
- Leading search icon.
- Optional clear action.
- Submit button.
- Loading state.
- Empty-results state.

```css
.search-form {
  display: grid;
  gap: var(--space-4);
  padding: var(--space-4);
  border: var(--border-thin) solid var(--color-border-strong);
  border-radius: var(--radius-md);
  background: var(--color-bg-surface);
}

.search-form__field {
  position: relative;
}

.search-form__icon {
  position: absolute;
  inset-block-start: 50%;
  inset-inline-start: var(--space-4);
  transform: translateY(-50%);
  color: var(--color-fg-muted);
  pointer-events: none;
}

.search-form__input {
  padding-inline-start: calc(var(--space-4) + 1.25rem + var(--space-3));
  padding-inline-end: calc(var(--space-4) + 2rem);
}

.search-form__clear {
  position: absolute;
  inset-block-start: 50%;
  inset-inline-end: var(--space-3);
  transform: translateY(-50%);
}

.search-form__actions {
  display: flex;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.search-results--empty {
  padding: var(--space-6);
  text-align: center;
  border: var(--border-thin) dashed var(--color-border-strong);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
}
```

#### Loading state

- Add `aria-busy="true"` to the form or results container.
- Disable only the submit control, not the search input, unless double-submit risk matters.
- If JS adds async search later, preserve enter-to-submit semantics.

### Authentication form

#### Rules

- Auth forms are centered sheets, not modal overlays.
- Use a visible title, a single supporting paragraph, and concise field hints.
- Reserve space for error and success messaging near the top.
- Password visibility toggle must be a real button.

#### Required parts

- Email or username field, depending on flow.
- Password field with show/hide button.
- Remember me checkbox where relevant.
- Submit button.
- Secondary links for signup and reset.
- Error message region.
- Success message region.

```css
.auth-form {
  display: grid;
  gap: var(--space-6);
  max-width: var(--container-sm);
  margin-inline: auto;
  padding: var(--space-6);
  border: var(--border-thin) solid var(--color-border-default);
  border-radius: var(--radius-md);
  background: var(--color-bg-surface);
}

.auth-form__header,
.auth-form__messages,
.auth-form__body,
.auth-form__footer {
  display: grid;
  gap: var(--space-3);
}

.auth-form__password-row {
  display: grid;
  grid-template-columns: 1fr auto;
  gap: var(--space-3);
  align-items: center;
}

.auth-form__remember {
  display: flex;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}
```

#### Message areas

- Errors use danger styling and `role="alert"` for blocking problems.
- Success and informational messages use polite live regions unless immediate interruption is needed.
- Do not rely on color alone; include explicit text labels when messages are important.

### Switches and toggles

#### Rules

- Use switches only for immediate on/off settings.
- Use checkboxes for form submission values that are reviewed later.
- The entire label row should be clickable.

```css
.switch {
  display: inline-flex;
  align-items: center;
  gap: var(--space-3);
  cursor: pointer;
}

.switch__control {
  position: relative;
  inline-size: 2.75rem;
  block-size: 1.5rem;
  border: var(--border-thin) solid var(--color-border-strong);
  border-radius: var(--radius-pill);
  background: var(--color-bg-surface-muted);
  transition:
    background var(--transition-base) var(--ease-standard),
    border-color var(--transition-base) var(--ease-standard);
}

.switch__control::after {
  content: "";
  position: absolute;
  inset-block-start: 50%;
  inset-inline-start: 0.1875rem;
  inline-size: 1rem;
  block-size: 1rem;
  border-radius: 50%;
  background: var(--color-bg-elevated);
  transform: translateY(-50%);
  transition: transform var(--transition-base) var(--ease-emphasized);
}

.switch__input:checked + .switch__control {
  background: var(--color-accent-soft);
  border-color: var(--color-accent);
}

.switch__input:checked + .switch__control::after {
  transform: translate(1.2rem, -50%);
}
```

#### Sizes

- Default: `2.75rem x 1.5rem`.
- Small: `2.25rem x 1.25rem`.
- Large: `3.25rem x 1.75rem`.

#### Disabled

- Reduce contrast and remove pointer affordance.
- Keep label readable.

### Scroll and scrollbar

#### Rules

- Scrollbars should be thin, quiet, and visible enough to discover.
- Do not make the thumb so light that it disappears.
- Only style scrollbars to match the design, not to mimic another OS.

```css
* {
  scrollbar-width: thin;
  scrollbar-color: var(--color-border-strong) var(--color-bg-surface-muted);
}

*::-webkit-scrollbar {
  width: 0.75rem;
  height: 0.75rem;
}

*::-webkit-scrollbar-track {
  background: var(--color-bg-surface-muted);
}

*::-webkit-scrollbar-thumb {
  background: var(--color-border-strong);
  border: 0.1875rem solid var(--color-bg-surface-muted);
  border-radius: var(--radius-pill);
}

*::-webkit-scrollbar-thumb:hover {
  background: var(--color-fg-muted);
}
```

### Header

#### Rules

- Header and navigation are separate layout primitives. Do not hard-wire nav markup into the brand block.
- The header owns brand, context, and lightweight utility copy only.
- Default storefront header stays compact and top-aligned.
- Workspace headers may pair with a left or right rail nav, but the header itself should remain shallow.

### Navigation

#### Rules

- Navigation is a modular component with one semantic structure and placement modifiers.
- Supported placements are `top`, `bottom`, `left`, and `right`.
- Supported attachment modes are `static`, `sticky`, `fixed`, and `floating`.
- Storefront default is `top`.
- Admin and partner workspaces may use `left` or `right` rails when information density justifies it.
- Mobile uses a toggle button that reveals the same link list.
- The nav must be usable without JS. JS may collapse it on small screens after load.
- Logged-out and logged-in states should share the same structure so orientation does not change drastically.
- Placement changes must not require a different DOM shape.

#### Required behavior

- Brand or context label near the nav.
- Primary links.
- Secondary/auth cluster when needed.
- Active state indicator.
- Toggle button on small screens or constrained rails.
- Shared element names across all placements.

```css
.nav-shell--top .site-nav {
  display: grid;
  gap: var(--space-3);
}

.nav--left .site-nav__list,
.nav--right .site-nav__list {
  flex-direction: column;
}

.site-nav__list,
.nav__list {
  display: flex;
  gap: var(--space-2);
  margin: 0;
  padding: 0;
  list-style: none;
}

.site-nav__link,
.nav__link {
  display: inline-flex;
  align-items: center;
  min-height: 2.25rem;
  padding-inline: var(--space-3);
  border: var(--border-thin) solid transparent;
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  text-transform: uppercase;
  letter-spacing: var(--letter-spacing-wide);
}

.site-nav__link:hover,
.site-nav__link:focus-visible,
.site-nav__link[aria-current="page"] {
  background: var(--color-bg-surface-muted);
  border-color: var(--color-border-default);
  color: var(--color-fg-primary);
}

@media (min-width: 40rem) {
  .nav--top .site-nav__list,
  .nav--bottom .site-nav__list {
    flex-direction: row;
    flex-wrap: wrap;
  }
}
```

#### Auth-aware layout

- Logged out: show `Sign in`, `Create account`, and `Cart`.
- Logged in: show `Account`, `Orders` if present, `Cart`, and `Sign out`.
- Admin or partner routes may add a role-specific cluster, but it should visually read as an extension of the same nav.

### Badges and tags

#### Rules

- Badges are compact status markers, not buttons.
- Use pill shape.
- Use semantic colors sparingly.
- Count badges should never rely on color alone to communicate meaning.

```css
.badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-height: 1.5rem;
  padding-inline: var(--space-3);
  border: var(--border-thin) solid var(--color-border-strong);
  border-radius: var(--radius-pill);
  background: var(--color-bg-surface-muted);
  color: var(--color-fg-secondary);
  font-size: var(--font-size-xs);
  letter-spacing: var(--letter-spacing-wide);
  text-transform: uppercase;
}

.badge--success {
  background: var(--color-success-soft);
  border-color: var(--color-success-border);
  color: var(--color-success);
}

.badge--warning {
  background: var(--color-warning-soft);
  border-color: var(--color-warning-border);
  color: var(--color-warning);
}

.badge--danger {
  background: var(--color-danger-soft);
  border-color: var(--color-danger-border);
  color: var(--color-danger);
}
```

### Alerts and inline messages

The codebase already uses `ui-alert`. Keep it aligned with token colors and semantic variants.

- `info`: neutral informational notice.
- `success`: confirmation or completed action.
- `warning`: caution or recoverable issue.
- `danger`: error or destructive warning.

Rules:

- Use `role="status"` for passive info/success.
- Use `role="alert"` for urgent errors that block task completion.
- Keep message bodies short and specific.

### Product cards and list items

Product presentation already exists and should remain quiet:

- Image first.
- Name and description in the body.
- Price and stock grouped together.
- Add-to-cart action anchored at the bottom.
- Do not add heavy hover transforms, glossy surfaces, or dense metadata.

## 4. Animation & Motion

Motion should feel like interface confirmation, not decoration.

### Principles

- Default duration range: `150ms` to `300ms`.
- Use `ease-out` or emphasized ease for entrances.
- Use `ease-in-out` for hover, focus, and toggle transitions.
- Prefer opacity, border-color, background, and short transform changes.
- Avoid large-scale movement, bounce, or parallax.

### Allowed motion patterns

- Focus ring fade-in.
- Button hover/background transition.
- Nav menu expand/collapse.
- Password toggle label swap.
- Loading spinner rotation.
- Skeleton shimmer at low contrast.

### Loading spinner example

```css
.spinner {
  inline-size: 1rem;
  block-size: 1rem;
  border: var(--border-thick) solid var(--color-border-default);
  border-top-color: var(--color-accent);
  border-radius: 50%;
  animation: spin 800ms linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}
```

### Skeleton example

```css
.skeleton {
  background:
    linear-gradient(
      90deg,
      var(--color-bg-surface-muted) 0%,
      var(--color-bg-surface-subtle) 50%,
      var(--color-bg-surface-muted) 100%
    );
  background-size: 200% 100%;
  animation: shimmer 1.2s linear infinite;
}

@keyframes shimmer {
  to {
    background-position: -200% 0;
  }
}
```

### Reduced motion

```css
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 1ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 1ms !important;
    scroll-behavior: auto !important;
  }
}
```

## 5. Accessibility Checklist

Every UI change should pass this checklist before it is considered complete.

### Structure

- Use semantic HTML first: `header`, `nav`, `main`, `section`, `form`, `label`, `button`, `table`.
- Keep heading order logical and sequential.
- Use landmarks consistently across pages.

### Focus

- Every interactive element must show a visible `:focus-visible` state.
- Focus styles must be distinct from hover states.
- Do not remove outlines without replacing them.

### Keyboard

- All actions reachable by keyboard.
- Menus, toggles, and password visibility controls must work with Enter and Space when appropriate.
- Mobile nav must not trap focus.

### Forms

- Every control has a programmatic label.
- Errors are associated with controls via `aria-describedby` or equivalent.
- Invalid controls use `aria-invalid="true"`.
- Required fields should use native `required` plus clear visual cues.

### Messaging

- Use `role="alert"` for immediate errors.
- Use polite live regions for status updates.
- Do not communicate state with color alone.

### Contrast

- Text and meaningful UI controls must meet WCAG AA minimum contrast.
- Muted text is for secondary content only; never for essential instructions.
- Focus rings must remain visible on both light and dark surfaces.

### Touch and target size

- Minimum interactive target size should be about `44px` in at least one dimension for primary touch controls.
- Dense controls are acceptable only in desktop tables and should still remain keyboard accessible.

### Screen readers

- Decorative icons use `aria-hidden="true"`.
- Icon-only buttons require explicit accessible names.
- Search, auth, and nav toggles must expose correct state via `aria-expanded`, `aria-controls`, and descriptive labels.

## 6. Responsive Strategy

Sokomoko is mobile-first. Start with a single-column flow that works at narrow widths and layer in additional columns only when content benefits.

### Breakpoints

```css
:root {
  --breakpoint-sm: 40rem;  /* 640px */
  --breakpoint-md: 48rem;  /* 768px */
  --breakpoint-lg: 64rem;  /* 1024px */
  --breakpoint-xl: 80rem;  /* 1280px */
}
```

### Layout rules

- Mobile default: single column, stacked actions, full-width inputs and buttons where appropriate.
- `sm`: horizontal nav list, multi-action search forms, denser auth layouts.
- `md`: two-column utility layouts, wider content areas, more comfortable page padding.
- `lg`: multi-column dashboards, product grids, and account/admin summary rows.
- `xl`: increase whitespace and container max-width; do not endlessly widen text blocks.

### Container example

```css
.container {
  width: min(100% - (var(--space-4) * 2), var(--container-lg));
  margin-inline: auto;
}

@media (min-width: 48rem) {
  .container {
    width: min(100% - (var(--space-6) * 2), var(--container-lg));
  }
}
```

### Grid and flex patterns

- Product grids: `repeat(auto-fit, minmax(13rem, 1fr))`.
- Metric grids: single column on mobile, 2 to 4 columns from `md` upward.
- Form rows: stack on mobile, split at `sm` only when labels and actions still remain readable.
- Nav actions: stack on mobile, inline from `sm`.

### Responsive behavior requirements

- No sideways page scrolling at default viewport widths.
- Tables must scroll inside a wrapper, not break the page.
- Card padding may increase with viewport size; structure should not change arbitrarily.
- Avoid hiding critical actions behind hover-only affordances.

## Default Decision Rule

When this document does not explicitly answer a design question, choose the option that is:

1. More structurally consistent with the existing paper-sheet UI.
2. More accessible without JavaScript.
3. More likely to reuse existing tokens and component classes.
4. Easier for another agent to extend without introducing a new visual dialect.
