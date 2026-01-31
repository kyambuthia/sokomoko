# Sokomoko Design System
## Mobile-First Retro UI (Late 1990s–Early 2000s Aesthetic)

---

## 📋 Table of Contents

1. [Overview](#overview)
2. [Design Philosophy](#design-philosophy)
3. [Color Palette](#color-palette)
4. [Typography](#typography)
5. [Core Components](#core-components)
6. [Layout System](#layout-system)
7. [Forms](#forms)
8. [Tables](#tables)
9. [Accessibility](#accessibility)
10. [Responsive Behavior](#responsive-behavior)
11. [Component Examples](#component-examples)

---

## Overview

The Sokomoko design system is inspired by utilitarian early web design (McMaster-Carr style). It prioritizes:

- **Clarity over aesthetics**: Clean, functional interfaces
- **Content-first**: High information density with clear visual hierarchy
- **Accessibility**: Strong contrast, readable sizing, predictable layout
- **Performance**: No decorative effects (gradients, shadows, animations)
- **Mobile-first**: Responsive scaling for all screen sizes
- **Usability**: Deliberate, "unpolished" practicality

---

## Design Philosophy

### What We DON'T Use
- ❌ Gradients, shadows, or blurs
- ❌ Decorative animations or transitions
- ❌ Custom fonts or complex typography
- ❌ Rounded corners (except where absolutely necessary)
- ❌ Hover effects beyond color changes
- ❌ Lightboxes, modals, or overlays
- ❌ Auto-playing media or infinite scroll

### What We DO Use
- ✅ Solid, flat colors
- ✅ System fonts (Arial, Helvetica, Courier, Times)
- ✅ Visible borders and strong alignment
- ✅ Table-like layouts and grids
- ✅ Clear visual hierarchy via spacing and weight
- ✅ Standard web conventions (blue underlined links)
- ✅ Accessible focus states and keyboard navigation

---

## Color Palette

### Core Colors

| Color | Hex | Usage |
|-------|-----|-------|
| **White** | `#FFFFFF` | Backgrounds, text on dark |
| **Black** | `#000000` | Text, borders, buttons |
| **Off-White** | `#F5F5F5` | Alternative backgrounds, header fills |
| **Dark Gray** | `#666666` | Secondary text, muted content |
| **Light Gray** | `#CCCCCC` | Borders, dividers, secondary lines |

### Interactive Colors

| Color | Hex | Usage |
|-------|-----|-------|
| **Link** | `#0000EE` | Unvisited hyperlinks |
| **Visited** | `#551A8B` | Visited hyperlinks |
| **Hover** | `#000000 → #FFFFFF` | Link backgrounds on hover |
| **Success** | `#008000` | Positive actions, confirmations |
| **Error** | `#CC0000` | Warnings, errors, deletions |
| **Warning** | `#FF8C00` | Cautions, important notices |
| **Info** | `#0000EE` | Information, alerts |

### Accessibility Notes

- **Contrast ratio for text**: 21:1 (black on white) — WCAG AAA
- **Contrast ratio for UI**: 4.5:1 minimum — WCAG AA
- **Focus indicators**: 2px solid black outline with 2px offset

---

## Typography

### Font Stack

```css
/* Primary (sans-serif) */
font-family: Arial, Helvetica, sans-serif;

/* Code & Technical */
font-family: "Courier New", Courier, monospace;

/* Serif (optional, not recommended) */
font-family: Georgia, "Times New Roman", serif;
```

### Font Sizes

**Mobile (≤480px)**
- `html, body`: 13px
- `h1`: 1.5rem (19.5px)
- `h2`: 1.3rem (16.9px)
- `h3`: 1.1rem (14.3px)
- `p, label`: 1rem (14px)
- `small`: 0.85rem (11px)

**Tablet (481px–767px)**
- `html, body`: 14px
- `h1`: 1.8rem (25.2px)
- `h2`: 1.5rem (21px)
- `h3`: 1.2rem (16.8px)

**Desktop (768px+)**
- `h1`: 2rem (28px)
- `h2`: 1.6rem (22.4px)
- `h3`: 1.3rem (18.2px)

### Font Weights

- **Normal**: 400 (body text)
- **Bold**: 700 (headings, labels, emphasis)

### Line Height

- **Body text**: 1.6
- **Headings**: 1.3
- **Lists/Compact**: 1.4

### Heading Styles

All headings include bottom borders for visual separation:
- `h1`: 2px solid black
- `h2`: 1px solid black
- `h3`: 1px dashed #CCC

```html
<h1>Main Heading</h1>
<h2>Section Heading</h2>
<h3>Subsection</h3>
```

---

## Core Components

### Links

Links are always **blue** with an **underline** (solid on hover).

```html
<!-- Standard link -->
<a href="/products">Browse Products</a>

<!-- Visited link (purple) -->
<!-- Automatically styled when history allows -->

<!-- Link on hover: blue background, white text -->
```

**CSS States:**
- `default`: `#0000EE` with bottom border
- `visited`: `#551A8B` with bottom border
- `hover`: Black background, white text
- `active`: Red text and border

**Links in Navigation** (remove underline for button-like links):
```html
<a href="/" class="button">Home</a>
```

---

### Buttons

Buttons are **black** with **white text**, rectangular, no rounded corners.

```html
<!-- Primary button -->
<button>Submit</button>
<input type="submit" value="Submit">
<a href="#" class="button">Click Me</a>

<!-- Secondary (inverted) -->
<button class="button--secondary">Cancel</button>

<!-- Success (green) -->
<button class="button--success">Approve</button>

<!-- Danger (red) -->
<button class="button--danger">Delete</button>

<!-- Small button -->
<button class="button--small">OK</button>

<!-- Large button -->
<button class="button--large">Continue</button>

<!-- Full width -->
<button class="button--block">Submit Form</button>
```

**Button Behavior:**
- `normal`: Black bg, white text, 1px black border
- `hover`: Dark gray (#333) bg
- `active`: Lighter gray (#666) bg
- `disabled`: Light gray (#CCC) bg, disabled cursor
- `focus`: 2px black outline

**Min size for touch:** 44×44px

---

### Cards & Boxes

Simple bordered containers for grouping content.

```html
<div class="card">
  <div class="card__header">
    <h3>Card Title</h3>
  </div>
  <div class="card__body">
    <p>Card content goes here.</p>
  </div>
  <div class="card__footer">
    <small>Footer info</small>
  </div>
</div>
```

**Card Structure:**
- **Header**: `#F5F5F5` background, bold title
- **Body**: White background, main content
- **Footer**: `#F5F5F5` background, light text, dashed top border

**Variants:**
```html
<div class="card--alt">
  <!-- Light gray background variant -->
</div>
```

---

### Forms

Minimal, functional form design.

```html
<form>
  <fieldset>
    <legend>Contact Information</legend>

    <div class="form-group">
      <label for="name">Name *</label>
      <input type="text" id="name" name="name" required>
    </div>

    <div class="form-group">
      <label for="email">Email *</label>
      <input type="email" id="email" name="email" required>
    </div>

    <div class="form-group">
      <label for="message">Message</label>
      <textarea id="message" name="message"></textarea>
    </div>

    <!-- Checkboxes -->
    <div class="checkbox-group">
      <div class="checkbox-item">
        <input type="checkbox" id="subscribe" name="subscribe">
        <label for="subscribe" class="inline">Subscribe to newsletter</label>
      </div>
    </div>

    <!-- Radio buttons -->
    <div class="radio-group">
      <div class="radio-item">
        <input type="radio" id="option1" name="options" value="1">
        <label for="option1" class="inline">Option 1</label>
      </div>
      <div class="radio-item">
        <input type="radio" id="option2" name="options" value="2">
        <label for="option2" class="inline">Option 2</label>
      </div>
    </div>

    <!-- Select dropdown -->
    <div class="form-group">
      <label for="category">Category</label>
      <select id="category" name="category">
        <option>-- Select --</option>
        <option>Electronics</option>
        <option>Clothing</option>
      </select>
    </div>

    <!-- Submit button -->
    <button type="submit" class="button--block">Submit</button>
  </fieldset>
</form>
```

**Form Element Styling:**
- `input`, `textarea`, `select`: Black border, white bg, 8px padding
- `focus state`: Yellow background (`#FFFACD`), blue border
- `label`: Bold, uppercase, small font
- Margin between fields: 1.2em (20px)

**Error State:**
```html
<div class="form-group has-error">
  <label for="email">Email</label>
  <input type="email" id="email" name="email">
  <div class="form-error">Please enter a valid email</div>
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

### Alerts & Messages

High-contrast, clearly labeled notifications.

```html
<!-- Info alert -->
<div class="alert alert--info">
  <h4>Information</h4>
  <p>Your order has been placed successfully.</p>
</div>

<!-- Success alert -->
<div class="alert alert--success">
  <h4>Success</h4>
  <p>Action completed successfully.</p>
</div>

<!-- Warning alert -->
<div class="alert alert--warning">
  <h4>Warning</h4>
  <p>Please review your information before proceeding.</p>
</div>

<!-- Error alert -->
<div class="alert alert--error">
  <h4>Error</h4>
  <p>Something went wrong. Please try again.</p>
</div>
```

**Alert Colors:**
- **Info**: Blue border, light blue bg (`#F0F0FF`)
- **Success**: Green border, light green bg (`#F0FFF0`)
- **Warning**: Orange border, light orange bg (`#FFFAF0`)
- **Error**: Red border, light red bg (`#FFF5F5`)

---

## Layout System

### Container Structure

```html
<body>
  <!-- Header wrapper -->
  <div class="wrapper--header">
    <div class="container">
      <!-- Header content -->
    </div>
  </div>

  <!-- Main content wrapper -->
  <div class="wrapper--main">
    <div class="container">
      <!-- Page content -->
    </div>
  </div>

  <!-- Footer wrapper -->
  <div class="wrapper--footer">
    <div class="container">
      <!-- Footer content -->
    </div>
  </div>
</body>
```

**Container Max-Width:** 960px (supports up to 1024px viewports)

### Navigation

Horizontal navigation bar with clear states.

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
    <li class="nav__item">
      <a href="/contact" class="nav__link">Contact</a>
    </li>
  </ul>
</nav>
```

**Mobile (stacked):** Full-width buttons stacked vertically
**Desktop (768px+):** Horizontal items separated by borders

**Active State:**
- Black background with white text

---

## Tables

High-density tabular data presentation.

```html
<table>
  <thead>
    <tr>
      <th>Product</th>
      <th>Price</th>
      <th>Stock</th>
      <th>Action</th>
    </tr>
  </thead>
  <tbody>
    <tr>
      <td>Widget A</td>
      <td>$12.99</td>
      <td>5</td>
      <td><a href="#">Edit</a></td>
    </tr>
    <tr>
      <td>Widget B</td>
      <td>$24.99</td>
      <td>0</td>
      <td><a href="#">Edit</a></td>
    </tr>
  </tbody>
</table>
```

**Table Features:**
- `thead`: Black background, white text, uppercase labels
- `tbody tr:odd`: Light gray background (`#F5F5F5`)
- `tbody tr:hover`: Yellow highlight (`#FFFACD`)
- **Borders**: All cells have right/bottom borders for clear delineation

**Compact Tables:**
```html
<table class="table--compact">
  <!-- Smaller padding for dense data -->
</table>
```

**Fully Bordered Tables:**
```html
<table class="table--bordered">
  <!-- All cells have borders -->
</table>
```

---

## Accessibility

### Keyboard Navigation

All interactive elements are keyboard accessible:
- **Links**: Tab-navigable, underlined
- **Buttons**: Tab-navigable, click with Enter/Space
- **Form inputs**: Tab-navigable, proper labels
- **Focus indicators**: 2px solid black outline

### Screen Reader Support

```html
<!-- Skip to main content link -->
<a href="#main" class="skip-link">Skip to main content</a>

<!-- Screen reader text -->
<span class="sr-only">Information for screen readers only</span>

<!-- Proper form labels -->
<label for="email">Email Address</label>
<input id="email" type="email" name="email">

<!-- Semantic HTML -->
<button type="submit">Submit</button>  <!-- Good -->
<div onclick="...">Submit</div>  <!-- Bad -->
```

### Color Contrast

- **Text on background**: Minimum 4.5:1 (WCAG AA)
- **Black on white**: 21:1 (WCAG AAA)
- **Never rely on color alone**: Use borders, icons, text labels

### Text Sizing

- **Minimum font size**: 12px (small elements)
- **Body text**: 14px (mobile) to 16px (desktop)
- **Headings**: Scale responsively with viewport

### Touch Targets

- **Minimum size**: 44×44px for touch-friendly elements
- **Minimum spacing**: 8px between touch targets
- Buttons have padding to meet minimum size

---

## Responsive Behavior

### Mobile-First Approach

Design starts mobile (single-column), then enhances for larger screens.

### Breakpoints

```css
/* Mobile: 0–479px (default) */
/* Tablet: 480px–767px */
/* Desktop: 768px+ */
/* Large Desktop: 1024px+ */
```

### Responsive Examples

#### Navigation
```
Mobile:    Stacked, full-width buttons
Tablet+:   Horizontal, inline items
```

#### Product Grid
```
Mobile:    1 column (100% width)
Tablet:    2–3 columns
Desktop:   3–4 columns
```

#### Typography
```
Mobile:    h1: 1.5rem, body: 13px
Desktop:   h1: 2rem, body: 14px
```

---

## Component Examples

### Product Card

```html
<div class="product-card">
  <img src="product.jpg" alt="Product Name" class="product-card__image">
  <div class="product-card__content">
    <h3 class="product-card__title">Product Name</h3>
    <p class="product-card__description">Brief description of the product.</p>
    <div class="product-card__price">$49.99</div>
    <div class="product-card__footer">
      <button class="product-card__button button--small">Add to Cart</button>
    </div>
  </div>
</div>
```

### Product Grid

```html
<div class="product-grid">
  <div class="product-card"><!-- Card 1 --></div>
  <div class="product-card"><!-- Card 2 --></div>
  <div class="product-card"><!-- Card 3 --></div>
  <!-- More cards -->
</div>
```

**Grid Sizes:**
- Mobile: 2 columns (150px min)
- Tablet: 3 columns (180px min)
- Desktop: 4+ columns (220px min)

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

On mobile: Single column, stacked

### Search & Filter Bar

```html
<form class="flex flex-gap-2 flex-wrap">
  <input type="text" placeholder="Search..." style="flex: 1; min-width: 200px;">
  <select>
    <option>All Categories</option>
  </select>
  <button type="submit">Search</button>
</form>
```

### Pagination

```html
<ul class="pagination">
  <li class="pagination__item"><a href="#" class="pagination__link">← Prev</a></li>
  <li class="pagination__item"><a href="#" class="pagination__link">1</a></li>
  <li class="pagination__item active"><a href="#" class="pagination__link">2</a></li>
  <li class="pagination__item"><a href="#" class="pagination__link">3</a></li>
  <li class="pagination__item"><a href="#" class="pagination__link">Next →</a></li>
</ul>
```

### Breadcrumb Navigation

```html
<nav class="breadcrumb">
  <a href="/">Home</a> > <a href="/products">Products</a> > Widget A
</nav>
```

### Footer

```html
<footer class="footer">
  <p>&copy; 2025 Sokomoko. All rights reserved.</p>
  <ul class="nav__list">
    <li><a href="/about">About</a></li>
    <li><a href="/privacy">Privacy</a></li>
    <li><a href="/terms">Terms</a></li>
    <li><a href="/contact">Contact</a></li>
  </ul>
</footer>
```

---

## Best Practices

### DO
✅ Use semantic HTML (`<button>`, `<nav>`, `<header>`, `<footer>`)
✅ Include `alt` text on all images
✅ Use proper heading hierarchy (h1, h2, h3, not skipping levels)
✅ Test on real devices (mobile, tablet, desktop)
✅ Validate HTML and CSS
✅ Use descriptive link text ("Click here" → "View product details")
✅ Provide clear error messages and success feedback

### DON'T
❌ Use tables for layout (only for data)
❌ Rely on color alone for meaning
❌ Auto-play videos or audio
❌ Use `<div onclick="">` instead of `<button>`
❌ Forget alt text on images
❌ Use inconsistent spacing or alignment
❌ Create components requiring JavaScript for basic functionality

---

## Files

- **Stylesheet**: `/internal/ui/static/stylesheets/styles.css` (2,500+ lines)
- **This guide**: `DESIGN_SYSTEM.md`
- **Templates**: `/internal/ui/templates/` (HTML templates using the system)

---

## Version

**v1.0** – Mobile-First Retro Design System
Last updated: January 2025

---

## Questions?

Refer to component examples above, or check existing templates in `/internal/ui/templates/pages/`.
