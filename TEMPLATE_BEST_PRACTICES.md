# Template Best Practices
## HTML Structure & Implementation Guide

This guide explains how to properly structure HTML templates to work with the Sokomoko design system.

---

## Template Architecture

### Base Template Structure

Every page inherits from `base.tmpl.html`:

```go
{{ define "root_template" }}
<!DOCTYPE html>
<html>
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>{{ block "title" . }}{{end}}</title>
    <link rel="stylesheet" href="static/stylesheets/styles.css">
  </head>
  <body>
    <!-- Header -->
    <div class="wrapper--header">
      <div class="container">
        {{ block "header-area" . }}{{ end }}
      </div>
    </div>

    <!-- Main Content -->
    <div class="wrapper--main">
      <div class="container">
        {{ block "main-area" . }}{{ end }}
      </div>
    </div>

    <!-- Footer -->
    <div class="wrapper--footer">
      <div class="container">
        {{ block "footer-area" . }}{{ end }}
      </div>
    </div>

    <script src="static/scripts/main.js"></script>
  </body>
</html>
{{ end }}
```

### Page-Specific Blocks

Each page defines its content blocks:

```go
{{ define "title" }}Page Title – Sokomoko{{ end }}

{{ define "header-area" }}
<!-- Page-specific header (optional) -->
{{ end }}

{{ define "main-area" }}
<div class="container">
  <!-- Main page content -->
</div>
{{ end }}

{{ define "footer-area" }}
<!-- Footer content (optional) -->
{{ end }}
```

---

## Common Patterns

### 1. Page Header with Navigation

```html
{{ define "header-area" }}
<div class="header">
  <h1 class="site-title">Sokomoko</h1>
  <p class="site-tagline">Online Marketplace</p>
</div>

<nav class="nav">
  <ul class="nav__list">
    <li class="nav__item">
      <a href="/" class="nav__link {{ if eq .CurrentPage "home" }}active{{ end }}">Home</a>
    </li>
    <li class="nav__item">
      <a href="/products" class="nav__link {{ if eq .CurrentPage "products" }}active{{ end }}">Products</a>
    </li>
    <li class="nav__item">
      <a href="/account" class="nav__link {{ if eq .CurrentPage "account" }}active{{ end }}">Account</a>
    </li>
    <li class="nav__item">
      <a href="/cart" class="nav__link {{ if eq .CurrentPage "cart" }}active{{ end }}">Cart</a>
    </li>
  </ul>
</nav>
{{ end }}
```

### 2. Centered Form Page

```html
{{ define "main-area" }}
<div class="container">
  <div style="max-width: 400px; margin: 2em auto;">
    <h1>Login</h1>

    <form action="/login" method="POST">
      <fieldset>
        <legend>Sign In</legend>

        <div class="form-group">
          <label for="email">Email *</label>
          <input type="email" id="email" name="email" placeholder="your@email.com" required>
        </div>

        <div class="form-group">
          <label for="password">Password *</label>
          <input type="password" id="password" name="password" required>
        </div>

        <button type="submit" class="button--block">Sign In</button>
      </fieldset>
    </form>

    <p style="text-align: center; margin-top: 1.5em;">
      Don't have an account? <a href="/signup">Sign up here</a>
    </p>
  </div>
</div>
{{ end }}
```

### 3. Product Grid Page

```html
{{ define "main-area" }}
<div class="container">
  <h1>Featured Products</h1>

  <p>Browse our collection of quality items.</p>

  {{ if .HasProducts }}
  <div class="product-grid">
    {{ range .Products }}
    <div class="product-card">
      <img 
        src="{{ .ImageURL }}" 
        alt="{{ .Name }}" 
        class="product-card__image">
      <div class="product-card__content">
        <h3 class="product-card__title">{{ .Name }}</h3>
        <p class="product-card__description">{{ .Description }}</p>
        <div class="product-card__price">${{ printf "%.2f" .Price }}</div>
        <div class="product-card__footer">
          <button class="product-card__button button--small button--block">
            View Details
          </button>
        </div>
      </div>
    </div>
    {{ end }}
  </div>
  {{ else }}
  <div class="alert alert--info">
    <p>No products available at this time.</p>
  </div>
  {{ end }}
</div>
{{ end }}
```

### 4. Data Table Page

```html
{{ define "main-area" }}
<div class="container">
  <h1>Order History</h1>

  <table>
    <thead>
      <tr>
        <th>Order ID</th>
        <th>Date</th>
        <th>Total</th>
        <th>Status</th>
        <th>Action</th>
      </tr>
    </thead>
    <tbody>
      {{ range .Orders }}
      <tr>
        <td><strong>{{ .ID }}</strong></td>
        <td>{{ .Date }}</td>
        <td>${{ printf "%.2f" .Total }}</td>
        <td>{{ .Status }}</td>
        <td><a href="/orders/{{ .ID }}">View</a></td>
      </tr>
      {{ end }}
    </tbody>
  </table>
</div>
{{ end }}
```

### 5. Two-Column Layout

```html
{{ define "main-area" }}
<div class="container">
  <h1>Dashboard</h1>

  <div class="grid grid-2-cols">
    <div>
      <h2>Sales Summary</h2>
      <p>Revenue data here</p>
    </div>
    <div>
      <h2>Recent Orders</h2>
      <p>Order list here</p>
    </div>
  </div>

  <!-- Responsive note: Becomes single column on mobile -->
</div>
{{ end }}
```

### 6. Hero Section with CTA

```html
{{ define "main-area" }}
<div class="container">
  <div style="padding: 2em; border: 1px solid #CCCCCC; background-color: #F5F5F5; margin-bottom: 2em;">
    <h1>Welcome to Sokomoko</h1>
    <p style="font-size: 1.1rem; margin-bottom: 1.5em;">
      Your trusted online marketplace for quality products.
    </p>
    <a href="/products" class="button">Browse Now</a>
  </div>

  <!-- Main content below -->
</div>
{{ end }}
```

---

## Form Implementation

### Basic Form Template

```html
<form action="/submit" method="POST">
  <fieldset>
    <legend>Form Title</legend>

    <div class="form-group">
      <label for="field1">Field Label *</label>
      <input 
        type="text" 
        id="field1" 
        name="field1" 
        placeholder="Placeholder text"
        required>
    </div>

    <div class="form-group">
      <label for="field2">Email *</label>
      <input 
        type="email" 
        id="field2" 
        name="field2" 
        placeholder="your@email.com"
        required>
    </div>

    <div class="form-group">
      <label for="field3">Message</label>
      <textarea 
        id="field3" 
        name="field3" 
        placeholder="Your message here..."
        rows="5"></textarea>
    </div>

    <button type="submit" class="button--block">Submit</button>
  </fieldset>
</form>
```

### Form with Checkboxes

```html
<div class="checkbox-group">
  <div class="checkbox-item">
    <input type="checkbox" id="agree" name="agree" required>
    <label for="agree" class="inline">I agree to the terms</label>
  </div>
  <div class="checkbox-item">
    <input type="checkbox" id="newsletter" name="newsletter">
    <label for="newsletter" class="inline">Subscribe to newsletter</label>
  </div>
</div>
```

### Form with Radio Buttons

```html
<div class="radio-group">
  <label>Shipping Method:</label>
  <div class="radio-item">
    <input type="radio" id="standard" name="shipping" value="standard" checked>
    <label for="standard" class="inline">Standard (5–7 days)</label>
  </div>
  <div class="radio-item">
    <input type="radio" id="express" name="shipping" value="express">
    <label for="express" class="inline">Express (2–3 days)</label>
  </div>
</div>
```

### Form with Dropdown

```html
<div class="form-group">
  <label for="category">Category *</label>
  <select id="category" name="category" required>
    <option value="">-- Select a Category --</option>
    <option value="electronics">Electronics</option>
    <option value="clothing">Clothing</option>
    <option value="home">Home & Garden</option>
  </select>
</div>
```

### Form with Error Handling

```html
{{ if .Errors }}
<div class="alert alert--error">
  <h4>Please fix the following errors:</h4>
  <ul>
    {{ range .Errors }}
    <li>{{ . }}</li>
    {{ end }}
  </ul>
</div>
{{ end }}

<form action="/submit" method="POST">
  {{ if .HasError "email" }}
  <div class="form-group has-error">
    <label for="email">Email *</label>
    <input type="email" id="email" name="email" value="{{ .FormData.Email }}" required>
    <div class="form-error">{{ .GetError "email" }}</div>
  </div>
  {{ else }}
  <div class="form-group">
    <label for="email">Email *</label>
    <input type="email" id="email" name="email" value="{{ .FormData.Email }}" required>
  </div>
  {{ end }}
</form>
```

---

## Alerts and Messages

### Info Alert

```html
<div class="alert alert--info">
  <h4>Information</h4>
  <p>Your order has been received and is being processed.</p>
</div>
```

### Success Alert

```html
<div class="alert alert--success">
  <h4>Success!</h4>
  <p>Your changes have been saved successfully.</p>
</div>
```

### Warning Alert

```html
<div class="alert alert--warning">
  <h4>Warning</h4>
  <p>Your session will expire in 5 minutes. Save your work.</p>
</div>
```

### Error Alert

```html
<div class="alert alert--error">
  <h4>Error</h4>
  <p>Something went wrong. Please try again or contact support.</p>
</div>
```

---

## Cards and Containers

### Basic Card

```html
<div class="card">
  <div class="card__header">
    <h3>Card Title</h3>
  </div>
  <div class="card__body">
    <p>Main content goes here.</p>
  </div>
  <div class="card__footer">
    <small>Footer information</small>
  </div>
</div>
```

### Card Without Header

```html
<div class="card">
  <div class="card__body">
    <p>Content without header.</p>
  </div>
</div>
```

### Card Group (Side-by-Side)

```html
<div class="grid grid-3-cols">
  <div class="card">
    <div class="card__header"><h4>Feature 1</h4></div>
    <div class="card__body"><p>Description</p></div>
  </div>
  <div class="card">
    <div class="card__header"><h4>Feature 2</h4></div>
    <div class="card__body"><p>Description</p></div>
  </div>
  <div class="card">
    <div class="card__header"><h4>Feature 3</h4></div>
    <div class="card__body"><p>Description</p></div>
  </div>
</div>
```

---

## Navigation Patterns

### Breadcrumb Navigation

```html
<nav class="breadcrumb" style="margin-bottom: 1.5em;">
  <a href="/">Home</a> &gt;
  <a href="/products">Products</a> &gt;
  <a href="/products/electronics">Electronics</a> &gt;
  Laptop
</nav>
```

### Pagination

```html
{{ if .Pagination.Total > 1 }}
<ul class="pagination">
  {{ if .Pagination.HasPrev }}
  <li class="pagination__item">
    <a href="{{ .Pagination.PrevURL }}" class="pagination__link">← Previous</a>
  </li>
  {{ else }}
  <li class="pagination__item disabled">
    <span class="pagination__link">← Previous</span>
  </li>
  {{ end }}

  {{ range .Pagination.Pages }}
  {{ if eq . .Pagination.Current }}
  <li class="pagination__item active">
    <a href="#" class="pagination__link">{{ . }}</a>
  </li>
  {{ else }}
  <li class="pagination__item">
    <a href="{{ .Pagination.PageURL . }}" class="pagination__link">{{ . }}</a>
  </li>
  {{ end }}
  {{ end }}

  {{ if .Pagination.HasNext }}
  <li class="pagination__item">
    <a href="{{ .Pagination.NextURL }}" class="pagination__link">Next →</a>
  </li>
  {{ else }}
  <li class="pagination__item disabled">
    <span class="pagination__link">Next →</span>
  </li>
  {{ end }}
</ul>
{{ end }}
```

---

## Utility Classes Usage

### Spacing

```html
<!-- Margins -->
<div class="mt-2">Top margin 1em</div>
<div class="mb-3">Bottom margin 1.5em</div>
<div class="pt-2 pb-2">Top and bottom padding 1em</div>

<!-- Margin bottom on all paragraphs except last -->
<p class="mb-2">Paragraph 1</p>
<p class="mb-2">Paragraph 2</p>
<p>Paragraph 3 (no margin)</p>
```

### Text Alignment

```html
<p class="text-left">Left aligned</p>
<p class="text-center">Center aligned</p>
<p class="text-right">Right aligned</p>
```

### Text Colors

```html
<p class="text-muted">Muted text (gray)</p>
<p class="text-success">Success message (green)</p>
<p class="text-error">Error message (red)</p>
<p class="text-warning">Warning message (orange)</p>
<p class="text-info">Info message (blue)</p>
```

### Flexbox

```html
<!-- Horizontal flex (default) -->
<div class="flex flex-gap-2">
  <button>Button 1</button>
  <button>Button 2</button>
  <button>Button 3</button>
</div>

<!-- Vertical flex -->
<div class="flex flex-column flex-gap-1">
  <p>Item 1</p>
  <p>Item 2</p>
  <p>Item 3</p>
</div>

<!-- Flex items grow equally -->
<div class="flex">
  <div class="flex-1">Grows equally</div>
  <div class="flex-1">Grows equally</div>
</div>

<!-- Centered items -->
<div class="flex flex-items-center flex-gap-2">
  <input type="text" placeholder="Search...">
  <button>Go</button>
</div>
```

### Grid

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

## Accessibility Best Practices

### Semantic HTML

```html
<!-- Good -->
<button type="submit">Submit</button>
<nav><ul><li><a href="#">Link</a></li></ul></nav>
<header><h1>Page Title</h1></header>
<main>Content</main>
<footer>Footer</footer>

<!-- Bad -->
<div onclick="submit()">Submit</div>
<div class="nav"><span>Link</span></div>
<div class="header"><div class="title">Page Title</div></div>
```

### Form Labels

```html
<!-- Good: Label properly associated -->
<label for="email">Email Address</label>
<input type="email" id="email" name="email">

<!-- Bad: Label not connected -->
<label>Email Address</label>
<input type="email" name="email">
```

### Image Alt Text

```html
<!-- Good: Descriptive alt text -->
<img src="product.jpg" alt="Blue wireless headphones">

<!-- Bad: Vague or missing -->
<img src="product.jpg" alt="product">
<img src="product.jpg">
```

### Heading Hierarchy

```html
<!-- Good: Logical hierarchy -->
<h1>Page Title</h1>
<h2>Section 1</h2>
<h3>Subsection 1.1</h3>
<h3>Subsection 1.2</h3>
<h2>Section 2</h2>

<!-- Bad: Skipping levels -->
<h1>Page Title</h1>
<h3>Section 1</h3>  <!-- Skipped h2 -->
```

### Skip Link

```html
<a href="#main" class="skip-link">Skip to main content</a>

<div class="wrapper--header">
  <!-- Header content -->
</div>

<div class="wrapper--main" id="main">
  <!-- Main content -->
</div>
```

### Screen Reader Only Text

```html
<!-- Hidden from sighted users, visible to screen readers -->
<button>
  <span class="sr-only">Close</span>
  ✕
</button>
```

---

## Common Mistakes to Avoid

### ❌ Don't Do This

```html
<!-- Using div for buttons -->
<div onclick="submit()" class="button">Submit</div>

<!-- Missing labels -->
<input type="email" placeholder="Email">

<!-- Multiple h1 tags -->
<h1>Header 1</h1>
<h1>Header 2</h1>

<!-- Using tables for layout -->
<table>
  <tr>
    <td>Sidebar</td>
    <td>Content</td>
  </tr>
</table>

<!-- Inline styles (hard to maintain) -->
<p style="color: red; font-size: 16px; margin-bottom: 20px;">Message</p>

<!-- Missing alt text -->
<img src="logo.png">

<!-- Breaking semantic structure -->
<div class="nav"><div class="item"><span>Link</span></div></div>
```

### ✅ Do This Instead

```html
<!-- Use semantic button element -->
<button type="submit">Submit</button>

<!-- Use label for form fields -->
<label for="email">Email</label>
<input type="email" id="email" name="email">

<!-- Single h1 per page -->
<h1>Main Page Title</h1>
<h2>Section Heading</h2>

<!-- Use semantic nav with proper list -->
<nav>
  <ul>
    <li><a href="#home">Home</a></li>
  </ul>
</nav>

<!-- Use classes and the design system -->
<p class="alert alert--error">Error message</p>

<!-- Always include alt text -->
<img src="logo.png" alt="Sokomoko logo">

<!-- Maintain semantic structure -->
<nav class="nav">
  <ul class="nav__list">
    <li class="nav__item">
      <a href="#" class="nav__link">Link</a>
    </li>
  </ul>
</nav>
```

---

## Testing Checklist

Before publishing a new template, verify:

### Functionality
- [ ] Form submissions work
- [ ] Links navigate correctly
- [ ] Images load properly
- [ ] All content displays

### Responsive Design
- [ ] Mobile view (≤479px)
- [ ] Tablet view (480–767px)
- [ ] Desktop view (768px+)
- [ ] Landscape orientation

### Accessibility
- [ ] Tab navigation works
- [ ] Focus indicators visible
- [ ] Keyboard can submit forms
- [ ] Color contrast sufficient (21:1 ideal)
- [ ] Screen reader compatible

### Browser Compatibility
- [ ] Chrome/Edge
- [ ] Firefox
- [ ] Safari
- [ ] Mobile browsers

### Performance
- [ ] Page loads quickly
- [ ] Images optimized
- [ ] CSS loads without delay

---

## Resources

- **Design System:** `DESIGN_SYSTEM.md`
- **CSS Architecture:** `CSS_ARCHITECTURE.md`
- **Quick Reference:** `DESIGN_QUICK_REFERENCE.md`
- **Component Showcase:** `/design` route
- **W3C HTML Standard:** https://html.spec.whatwg.org/
- **MDN Web Docs:** https://developer.mozilla.org/

---

**Version:** 1.0
**Last Updated:** January 2025
