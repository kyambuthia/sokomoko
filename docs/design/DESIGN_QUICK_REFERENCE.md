# Sokomoko Design System – Quick Reference Guide

A fast lookup for the most common components and patterns. For comprehensive details, see `DESIGN_SYSTEM.md`.

---

## 🎨 Color Palette

| Name | Hex | Usage |
|------|-----|-------|
| White | `#FFFFFF` | Backgrounds |
| Black | `#000000` | Text, borders, buttons |
| Off-White | `#F5F5F5` | Alternative backgrounds |
| Dark Gray | `#666666` | Secondary text |
| Light Gray | `#CCCCCC` | Borders, dividers |
| Link Blue | `#0000EE` | Hyperlinks |
| Visited | `#551A8B` | Visited links |
| Success | `#008000` | Green for success |
| Error | `#CC0000` | Red for errors |
| Warning | `#FF8C00` | Orange for warnings |

---

## 📱 Page Layout

Every page follows this structure:

```html
<div class="wrapper--header">
  <div class="container">
    <!-- Header content -->
  </div>
</div>

<div class="wrapper--main">
  <div class="container">
    <!-- Page content -->
  </div>
</div>

<div class="wrapper--footer">
  <div class="container">
    <!-- Footer content -->
  </div>
</div>
```

**Container max-width:** 960px

---

## 🔤 Typography

### Font Stack
```css
font-family: Arial, Helvetica, sans-serif;        /* Body text */
font-family: "Courier New", Courier, monospace;    /* Code */
```

### Heading Sizes
- `h1`: 2rem (desktop), 1.8rem (mobile)
- `h2`: 1.6rem (desktop), 1.5rem (mobile)
- `h3`: 1.3rem (desktop), 1.2rem (mobile)

### Body Text
- `14px` base size
- `1.6` line height
- **Bold**: `<strong>` or `<b>`
- **Italic**: `<em>` or `<i>`

---

## 🔗 Links

```html
<!-- Standard link (blue, underlined) -->
<a href="/page">Link text</a>

<!-- Button-style link (no underline) -->
<a href="#" class="button">Button Link</a>
```

**States:**
- Unvisited: Blue (`#0000EE`)
- Visited: Purple (`#551A8B`)
- Hover: Black background, white text
- Active: Red

---

## 🔘 Buttons

```html
<!-- Primary button -->
<button>Submit</button>
<a href="#" class="button">Click Me</a>

<!-- Secondary (inverted) -->
<button class="button--secondary">Cancel</button>

<!-- Danger (red) -->
<button class="button--danger">Delete</button>

<!-- Success (green) -->
<button class="button--success">Approve</button>

<!-- Sizes -->
<button class="button--small">Small</button>
<button class="button--large">Large</button>

<!-- Full width -->
<button class="button--block">Full Width</button>
```

**Styling:**
- Normal: Black bg, white text
- Hover: Dark gray bg
- Disabled: Light gray bg

---

## 📝 Forms

```html
<form>
  <fieldset>
    <legend>Form Title</legend>

    <div class="form-group">
      <label for="field">Label *</label>
      <input type="text" id="field" name="field" required>
    </div>

    <!-- Checkboxes -->
    <div class="checkbox-group">
      <div class="checkbox-item">
        <input type="checkbox" id="agree" name="agree">
        <label for="agree" class="inline">I agree</label>
      </div>
    </div>

    <!-- Radio buttons -->
    <div class="radio-group">
      <div class="radio-item">
        <input type="radio" id="option1" name="choice" value="1">
        <label for="option1" class="inline">Option 1</label>
      </div>
    </div>

    <!-- Select dropdown -->
    <div class="form-group">
      <label for="select">Choose</label>
      <select id="select" name="select">
        <option>-- Select --</option>
        <option>Option A</option>
      </select>
    </div>

    <!-- Submit -->
    <button type="submit" class="button--block">Submit</button>
  </fieldset>
</form>
```

**Error State:**
```html
<div class="form-group has-error">
  <label for="email">Email</label>
  <input type="email" id="email" name="email">
  <div class="form-error">Please enter valid email</div>
</div>
```

**Success State:**
```html
<div class="form-group has-success">
  <label for="email">Email</label>
  <input type="email" id="email" name="email">
  <div class="form-success">Email verified!</div>
</div>
```

---

## 📦 Cards

```html
<div class="card">
  <div class="card__header">
    <h3>Card Title</h3>
  </div>
  <div class="card__body">
    <p>Content goes here.</p>
  </div>
  <div class="card__footer">
    <small>Footer info</small>
  </div>
</div>

<!-- Light gray variant -->
<div class="card card--alt">
  <!-- ... -->
</div>
```

---

## 📊 Tables

```html
<table>
  <thead>
    <tr>
      <th>Header 1</th>
      <th>Header 2</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Data</td>
      <td>Data</td>
    </tr>
  </tbody>
</table>

<!-- Compact table (smaller padding) -->
<table class="table--compact"><!-- ... --></table>

<!-- All cells bordered -->
<table class="table--bordered"><!-- ... --></table>
```

**Features:**
- Alternating row colors (white/light gray)
- Row hover: Yellow highlight
- Black header with white text

---

## ⚠️ Alerts

```html
<!-- Info -->
<div class="alert alert--info">
  <h4>Information</h4>
  <p>Your message here.</p>
</div>

<!-- Success -->
<div class="alert alert--success">
  <h4>Success</h4>
  <p>Action completed!</p>
</div>

<!-- Warning -->
<div class="alert alert--warning">
  <h4>Warning</h4>
  <p>Be careful.</p>
</div>

<!-- Error -->
<div class="alert alert--error">
  <h4>Error</h4>
  <p>Something went wrong.</p>
</div>
```

---

## 🏪 Product Grid

```html
<div class="product-grid">
  <div class="product-card">
    <img src="image.jpg" alt="Product" class="product-card__image">
    <div class="product-card__content">
      <h3 class="product-card__title">Product Name</h3>
      <p class="product-card__description">Brief description</p>
      <div class="product-card__price">$29.99</div>
      <div class="product-card__footer">
        <button class="product-card__button">Add to Cart</button>
      </div>
    </div>
  </div>
  <!-- More products -->
</div>
```

**Grid Sizes:**
- Mobile: 2 columns
- Tablet: 3 columns
- Desktop: 4+ columns

---

## 📄 Navigation

```html
<nav class="nav">
  <ul class="nav__list">
    <li class="nav__item">
      <a href="/" class="nav__link active">Home</a>
    </li>
    <li class="nav__item">
      <a href="/products" class="nav__link">Products</a>
    </li>
    <li class="nav__item">
      <a href="/about" class="nav__link">About</a>
    </li>
  </ul>
</nav>
```

**Mobile:** Stacked vertically
**Desktop:** Horizontal with borders

**Active state:** Black background, white text

---

## 📑 Pagination

```html
<ul class="pagination">
  <li class="pagination__item disabled">
    <span class="pagination__link">← Previous</span>
  </li>
  <li class="pagination__item active">
    <a href="#" class="pagination__link">1</a>
  </li>
  <li class="pagination__item">
    <a href="#" class="pagination__link">2</a>
  </li>
  <li class="pagination__item">
    <a href="#" class="pagination__link">Next →</a>
  </li>
</ul>
```

---

## 📐 Flexbox Utilities

```html
<!-- Basic flex container -->
<div class="flex">
  <div>Item 1</div>
  <div>Item 2</div>
</div>

<!-- Flex column (stacked) -->
<div class="flex flex-column">
  <div>Item 1</div>
  <div>Item 2</div>
</div>

<!-- With gap -->
<div class="flex flex-gap-2">
  <div>Item 1</div>
  <div>Item 2</div>
</div>

<!-- Flex items (flex: 1 each) -->
<div class="flex">
  <div class="flex-1">Grows equally</div>
  <div class="flex-1">Grows equally</div>
</div>

<!-- Justify -->
<div class="flex flex-justify-center">Centered</div>
<div class="flex flex-justify-between">Space between</div>

<!-- Align items -->
<div class="flex flex-items-center">Vertically centered</div>
```

---

## 📊 Grid Utilities

```html
<!-- 2-column grid (mobile: 1 column) -->
<div class="grid grid-2-cols">
  <div>Column 1</div>
  <div>Column 2</div>
</div>

<!-- 3-column grid (mobile: 1 column) -->
<div class="grid grid-3-cols">
  <div>Column 1</div>
  <div>Column 2</div>
  <div>Column 3</div>
</div>
```

---

## 🛠️ Utility Classes

### Spacing
```html
<!-- Margin top -->
<div class="mt-1">0.5em top margin</div>
<div class="mt-2">1em top margin</div>
<div class="mt-3">1.5em top margin</div>

<!-- Margin bottom -->
<div class="mb-1">0.5em bottom margin</div>
<div class="mb-2">1em bottom margin</div>
<div class="mb-3">1.5em bottom margin</div>

<!-- Padding top/bottom -->
<div class="pt-2">1em top padding</div>
<div class="pb-2">1em bottom padding</div>
```

### Text
```html
<p class="text-left">Left aligned</p>
<p class="text-center">Center aligned</p>
<p class="text-right">Right aligned</p>

<p class="text-muted">Gray text</p>
<p class="text-success">Green text</p>
<p class="text-error">Red text</p>
<p class="text-warning">Orange text</p>
<p class="text-info">Blue text</p>
```

### Display
```html
<div class="display-none">Hidden</div>
<div class="display-block">Block display</div>
<div class="display-inline">Inline display</div>

<!-- Screen readers only -->
<span class="sr-only">Info for screen readers</span>
```

---

## 🎯 Responsive Breakpoints

```css
/* Mobile-first (0–479px) - default */
/* Tablet (480px–767px) */
@media (min-width: 480px) { }

/* Desktop (768px+) */
@media (min-width: 768px) { }

/* Large desktop (1024px+) */
@media (min-width: 1024px) { }
```

---

## 📋 Accessibility

### Keyboard Navigation
- All interactive elements must be keyboard accessible
- Links use `<a>` tags
- Buttons use `<button>` tags
- Focus indicator: 2px solid black outline

### Screen Readers
```html
<!-- Form labels -->
<label for="email">Email</label>
<input id="email" type="email" name="email">

<!-- Skip link -->
<a href="#main" class="skip-link">Skip to main content</a>

<!-- Screen reader only text -->
<span class="sr-only">Additional context</span>
```

### Color Contrast
- Text on background: Minimum 4.5:1
- Black on white: 21:1 (AAA)

### Touch Targets
- Minimum size: 44×44px
- Minimum spacing: 8px between targets

---

## ❌ What NOT to Do

- ❌ No gradients or shadows
- ❌ No animations or transitions
- ❌ No custom fonts
- ❌ No rounded corners (except rare cases)
- ❌ Don't use `<div onclick="">` instead of `<button>`
- ❌ Don't forget alt text on images
- ❌ Don't use tables for layout (data only)
- ❌ Don't rely on color alone for meaning

---

## 📂 File Structure

```
sokomoko/
├── DESIGN_SYSTEM.md                    # Full documentation
├── DESIGN_QUICK_REFERENCE.md           # This file
├── internal/ui/static/stylesheets/
│   └── styles.css                      # Main stylesheet (2,500+ lines)
├── internal/ui/templates/pages/
│   ├── design_showcase.html            # Component showcase
│   ├── login.html
│   ├── signup.html
│   ├── admin.html
│   ├── admin_login.html
│   ├── add_product.html
│   ├── partner_signup.html
│   ├── index.html
│   └── ... (other pages)
└── internal/ui/templates/base/
    └── base.tmpl.html                  # Main template wrapper
```

---

## 🚀 Common Patterns

### Login Form
```html
<div style="max-width: 400px; margin: 2em auto;">
  <h1>Login</h1>
  <form action="/login" method="POST">
    <fieldset>
      <div class="form-group">
        <label for="email">Email *</label>
        <input type="email" id="email" name="email" required>
      </div>
      <div class="form-group">
        <label for="password">Password *</label>
        <input type="password" id="password" name="password" required>
      </div>
      <button type="submit" class="button--block">Sign In</button>
    </fieldset>
  </form>
</div>
```

### Two-Column Layout
```html
<div class="grid grid-2-cols">
  <div>
    <h2>Left Column</h2>
    <p>Content here</p>
  </div>
  <div>
    <h2>Right Column</h2>
    <p>Content here</p>
  </div>
</div>
```

### Hero Section
```html
<div style="padding: 2em; border: 1px solid #CCCCCC; background-color: #F5F5F5;">
  <h1>Welcome to Sokomoko</h1>
  <p>Your trusted online marketplace</p>
  <a href="/products" class="button">Browse Products</a>
</div>
```

---

## 🔍 Inspecting Components

Visit `/design` to see the complete component showcase with live examples of:
- Typography
- Forms
- Buttons
- Cards
- Tables
- Alerts
- Product grids
- And more!

---

## Questions?

Refer to `DESIGN_SYSTEM.md` for comprehensive documentation, or check existing templates in `/internal/ui/templates/pages/` for implementation examples.
