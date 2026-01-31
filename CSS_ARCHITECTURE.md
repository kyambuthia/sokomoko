# CSS Architecture Guide
## Sokomoko Retro Design System

This document explains the organization and maintenance of the CSS codebase.

---

## Overview

The stylesheet (`styles.css`) is organized into **logical sections** with clear comments. It uses **vanilla CSS** (no preprocessors) and follows a **mobile-first** responsive design approach.

**Total file size:** ~2,500 lines
**Browser support:** Modern browsers (Chrome, Firefox, Safari, Edge)

---

## File Organization

The stylesheet is divided into the following sections:

### 1. **Reset & Defaults** (Lines 1–60)
- Universal `*` reset
- Base `html` and `body` styling
- Text selection colors

### 2. **Typography** (Lines 65–135)
- Heading styles (h1–h6)
- Paragraph, bold, italic, code styling
- Line heights and margins

### 3. **Links** (Lines 140–185)
- Default link colors and states (unvisited, visited, hover, active)
- Skip link for accessibility
- Button-like link styling

### 4. **Layout: Mobile-First Container System** (Lines 190–245)
- `.wrapper` classes for header, main, footer
- `.container` max-width (960px)
- Responsive padding at breakpoints

### 5. **Header & Branding** (Lines 250–290)
- `.header` and site title styling
- Responsive flexbox layout for desktop

### 6. **Navigation** (Lines 295–360)
- `.nav` and `.nav__list` classes
- Mobile stacking vs. desktop horizontal layout
- Active state styling

### 7. **Forms** (Lines 365–545)
- Fieldset, legend, labels
- Input styling (text, email, password, etc.)
- Checkboxes and radio buttons
- Error and success states
- Focus indicators

### 8. **Buttons** (Lines 550–640)
- Primary, secondary, danger, success variants
- Size variants (small, normal, large)
- Block (full-width) layout
- Disabled states

### 9. **Cards & Boxes** (Lines 645–705)
- `.card` structure (header, body, footer)
- Alt card variant
- Styling for section containers

### 10. **Tables** (Lines 710–780)
- Thead (header) styling
- Tbody row coloring and hover
- Compact and bordered variants
- Cell padding and borders

### 11. **Lists** (Lines 785–820)
- Unordered (`ul`), ordered (`ol`), and definition lists (`dl`)
- Nested list margins
- Definition list styling

### 12. **Alerts & Messages** (Lines 825–885)
- Alert container styling
- Info, success, warning, error variants
- Message typography

### 13. **Products & Grid** (Lines 890–1000)
- `.product-grid` responsive columns
- `.product-card` structure (image, content, footer)
- Product title, description, price styling

### 14. **Pagination** (Lines 1005–1060)
- `.pagination` list styling
- Page number items
- Active and disabled states

### 15. **Utility Classes** (Lines 1065–1250)
- Text alignment (left, center, right)
- Text colors (muted, error, success, warning, info)
- Spacing utilities (mt-*, mb-*, pt-*, pb-*)
- Display utilities (display-none, sr-only, etc.)

### 16. **Flex Utilities** (Lines 1255–1320)
- `.flex` and `.flex-column` classes
- Gap utilities
- Justify and align helpers

### 17. **Grid Utilities** (Lines 1325–1345)
- `.grid` with column variants
- Mobile-first responsive collapse

### 18. **Responsive Typography** (Lines 1350–1380)
- Heading size scaling at breakpoints
- Mobile vs. desktop sizing

### 19. **Print Styles** (Lines 1385–1410)
- Print-specific styling
- Hiding interactive elements

### 20. **Accessibility: Focus States** (Lines 1415–1430)
- High-contrast focus indicators
- Keyboard navigation support

### 21. **Legacy/Admin Styles** (Lines 1435–1455)
- Admin terminal styling (green-on-black)
- Kept for backward compatibility

### 22. **Mobile Optimization** (Lines 1460–1530)
- Extra small device fixes
- Touch target sizing (48×48px min)
- Adjusted typography for tiny screens

---

## Design System Classes

### Naming Convention

Classes follow **BEM-like** naming (Block-Element-Modifier):

```css
/* Block */
.card { }

/* Element (child) */
.card__header { }
.card__body { }
.card__footer { }

/* Modifier (variant) */
.card--alt { }
.button--secondary { }
.button--danger { }
```

### Common Blocks

| Block | Purpose | Variants |
|-------|---------|----------|
| `.nav` | Navigation menu | `.nav__list`, `.nav__item`, `.nav__link` |
| `.card` | Content container | `.card__header`, `.card__body`, `.card__footer`, `.card--alt` |
| `.button` | Call-to-action | `.button--secondary`, `.button--danger`, `.button--success`, `.button--small`, `.button--large`, `.button--block` |
| `.alert` | Notification | `.alert--info`, `.alert--success`, `.alert--warning`, `.alert--error` |
| `.form-group` | Form field wrapper | `.has-error`, `.has-success` |
| `.product-card` | Product listing | `.product-card__image`, `.product-card__content`, `.product-card__title`, `.product-card__price` |
| `.pagination` | Page navigator | `.pagination__item`, `.pagination__link` |
| `.table` | Data table | `.table--compact`, `.table--bordered` |

---

## Responsive Design Strategy

### Mobile-First Approach

The stylesheet is **mobile-first**: base styles are optimized for small screens (0–479px), then enhanced with media queries.

### Breakpoints

```css
/* Mobile: 0–479px (default styles) */

/* Tablet: 480px–767px */
@media (min-width: 480px) { }

/* Desktop: 768px–1023px */
@media (min-width: 768px) { }

/* Large Desktop: 1024px+ */
@media (min-width: 1024px) { }
```

### Example: Navigation

```css
/* Mobile: Stacked buttons */
.nav__list {
  flex-direction: column;
}

/* Desktop: Horizontal */
@media (min-width: 768px) {
  .nav__list {
    flex-direction: row;
  }
}
```

---

## Color System

### CSS Variables (Not Used, But Recommended)

The current system uses **hardcoded hex values** for simplicity and compatibility. For future enhancement, consider CSS custom properties:

```css
:root {
  --color-white: #FFFFFF;
  --color-black: #000000;
  --color-text: #000000;
  --color-link: #0000EE;
  --color-link-visited: #551A8B;
  --color-success: #008000;
  --color-error: #CC0000;
  --color-bg-primary: #FFFFFF;
  --color-bg-alt: #F5F5F5;
  --color-border: #CCCCCC;
}

body {
  color: var(--color-text);
  background-color: var(--color-bg-primary);
}

a {
  color: var(--color-link);
}
```

### Current Colors Used

```
#FFFFFF (white)     - Primary background
#000000 (black)     - Text, borders, buttons
#F5F5F5 (off-white) - Alt backgrounds, header fills
#CCCCCC (light gray) - Secondary borders
#666666 (dark gray)  - Secondary text
#0000EE (blue)      - Links
#551A8B (purple)    - Visited links
#008000 (green)     - Success states
#CC0000 (red)       - Error states
#FF8C00 (orange)    - Warning states
```

---

## Making Changes to the Stylesheet

### Adding a New Component

**Step 1: Plan the HTML structure**
```html
<div class="my-component">
  <div class="my-component__header">Title</div>
  <div class="my-component__body">Content</div>
</div>
```

**Step 2: Add CSS section**
```css
/* ============================================================================
   MY COMPONENT
   ============================================================================ */

.my-component {
  border: 1px solid #000000;
  padding: 15px;
  margin-bottom: 1.2em;
  background-color: #FFFFFF;
}

.my-component__header {
  border-bottom: 1px solid #CCCCCC;
  margin: -15px -15px 15px -15px;
  padding: 10px 15px;
  background-color: #F5F5F5;
}

.my-component__body {
  padding: 0;
}
```

**Step 3: Add variants (if needed)**
```css
.my-component--dark {
  background-color: #F5F5F5;
  border-color: #CCCCCC;
}
```

**Step 4: Add responsive tweaks**
```css
@media (min-width: 768px) {
  .my-component {
    padding: 20px;
  }
}
```

### Modifying Existing Styles

**Before making changes:**
1. Check where the class is used (`rg "my-class-name"`)
2. Look for dependent styles
3. Test on multiple screen sizes
4. Verify accessibility (contrast, focus states)

**Example: Changing button padding**

```css
/* OLD */
button {
  padding: 10px 20px;
}

/* NEW */
button {
  padding: 12px 24px;  /* Increased for better touch targets */
}
```

---

## Common Patterns

### Spacing System

The stylesheet uses **em-based margins** for consistent spacing:

```
0.4em  = Small spacing (6–8px)
0.8em  = Normal spacing (12–14px)
1.2em  = Medium spacing (18–20px)
1.5em  = Large spacing (24px)
2em    = Extra large spacing (32px)
```

**Usage:**
```css
.card {
  margin-bottom: 1.2em;
  padding: 15px;
}

.form-group {
  margin-bottom: 1.2em;
}
```

### Border System

```css
/* Solid black (primary) */
border: 1px solid #000000;

/* Light gray (secondary) */
border: 1px solid #CCCCCC;

/* Dashed (tertiary/decorative) */
border: 1px dashed #CCCCCC;
```

### Hover Effects

**Only color changes—no transitions:**

```css
a {
  color: #0000EE;
}

a:hover {
  background-color: #0000EE;
  color: #FFFFFF;
}

button {
  background-color: #000000;
}

button:hover {
  background-color: #333333;  /* Slightly lighter */
}
```

---

## Accessibility Considerations

### Focus States

All interactive elements have **high-contrast focus indicators**:

```css
button:focus,
input:focus,
a:focus {
  outline: 2px solid #000000;
  outline-offset: 2px;
}
```

### Color Contrast

- **Minimum 4.5:1** for normal text
- **21:1** for black on white (AAA standard)
- Never rely on color alone—use text labels, icons, borders

### Touch Targets

- **Minimum 44×44px** for buttons and links
- **8px spacing** between touch targets

```css
button,
input[type="button"],
.button {
  min-width: 44px;
  min-height: 44px;
}

.pagination__item {
  /* Flex items ensure min-height */
  min-height: 44px;
}
```

### Screen Reader Support

```html
<!-- Skip link -->
<a href="#main" class="skip-link">Skip to main content</a>

<!-- SR-only text -->
<span class="sr-only">Additional context for screen readers</span>
```

```css
.sr-only {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  clip: rect(0, 0, 0, 0);
  white-space: nowrap;
  border-width: 0;
}

.skip-link {
  position: absolute;
  top: -9999px;
  left: -9999px;
}

.skip-link:focus {
  top: 0;
  left: 0;
}
```

---

## Performance Considerations

### File Size
- **~2,500 lines** of CSS
- **No external font files** (system fonts only)
- **No preprocessor compilation** needed
- **Single file** for simplicity

### Optimization Tips

1. **Minify in production** (but keep source readable)
2. **Cache-bust** on updates (add version hash)
3. **Remove unused classes** periodically
4. **Avoid inline styles** (use classes instead)

---

## Testing the Stylesheet

### Responsive Testing

Test on these devices:
- **Mobile:** iPhone SE (375px), Android phone (360px)
- **Tablet:** iPad (768px), iPad Pro (1024px)
- **Desktop:** Laptop (1366px), 4K (2560px)

### Browser Testing

- Chrome/Edge (Chromium)
- Firefox
- Safari
- Mobile browsers

### Accessibility Testing

- Use keyboard navigation (Tab, Enter, Space)
- Test with screen reader (NVDA, JAWS, VoiceOver)
- Check color contrast (WebAIM, WAVE)
- Verify focus indicators visible

---

## Maintenance Checklist

### Monthly
- [ ] Check for broken links in templates
- [ ] Verify form styling on new fields
- [ ] Test responsive layouts on real devices

### Quarterly
- [ ] Audit CSS for unused classes
- [ ] Review color palette for consistency
- [ ] Update typography sizes if needed
- [ ] Test accessibility (contrast, focus, SR)

### Annually
- [ ] Review overall design consistency
- [ ] Update browser compatibility notes
- [ ] Consider CSS custom properties migration
- [ ] Audit performance and file size

---

## Future Enhancements

### Potential Improvements

1. **CSS Custom Properties** – Make colors/spacing configurable
2. **CSS Grid** – For complex layouts (already using Flexbox)
3. **Utility-First** – Consider Tailwind-like approach (optional)
4. **Component Library** – Storybook or similar (optional)
5. **Preprocessor (SCSS/SASS)** – If complexity grows (optional)

### Dark Mode

If dark mode is needed, add:

```css
@media (prefers-color-scheme: dark) {
  body {
    background-color: #1a1a1a;
    color: #e0e0e0;
  }
  /* ... rest of dark mode styles ... */
}
```

---

## Troubleshooting

### Common Issues

**Issue:** Spacing inconsistency
**Solution:** Use utility classes (`.mt-*`, `.mb-*`) instead of inline `style` attributes

**Issue:** Focus state not visible
**Solution:** Add outline to all interactive elements (already in CSS)

**Issue:** Text too small on mobile
**Solution:** Check `@media (max-width: 479px)` section—font sizes are set to 13px

**Issue:** Forms look broken
**Solution:** Ensure `<label>` and `<input>` have matching `for`/`id` attributes

---

## Resources

- **Design System Guide:** `DESIGN_SYSTEM.md`
- **Quick Reference:** `DESIGN_QUICK_REFERENCE.md`
- **Component Showcase:** `/design` route
- **W3C Standards:** https://www.w3.org/
- **WebAIM Accessibility:** https://webaim.org/

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0 | Jan 2025 | Initial release – mobile-first retro design system |

---

**Last Updated:** January 2025
**Maintainer:** Sokomoko Development Team
