# Getting Started with the Sokomoko Design System

A **5-minute quick start guide** to the design system.

---

## Step 1: See It In Action (2 min)

**Start the app:**
```bash
cd /home/mbuthi/Projects/sokomoko
go run ./cmd/sokomoko serve
```

**Open browser:**
```
http://localhost:6969/design
```

**You should see:** Typography, buttons, forms, cards, tables, alerts, and product grids—all styled with the new design system.

**Bookmark this page!** Refer back here for component examples.

---

## Step 2: Understand the System (1 min)

### What Makes It Special?

✅ **No decorations** – No gradients, shadows, blurs, or animations
✅ **Flat design** – Solid colors only (white, black, grays, primary colors)
✅ **Accessible** – Works with keyboard, screen readers, and color-blind users
✅ **Mobile-first** – Looks great from 360px phones to 2560px monitors
✅ **Fast** – Vanilla CSS, no frameworks, no external fonts
✅ **Utilitarian** – Inspired by McMaster-Carr (form over function)

### Core Components

- **Links** – Blue, underlined (hover → dark background)
- **Buttons** – Black bg, white text (no rounded corners)
- **Forms** – Proper labels, bold headings, clear spacing
- **Cards** – Bordered boxes with header/body/footer
- **Tables** – Black header, alternating rows, hover effects
- **Alerts** – Color-coded: blue/green/orange/red
- **Grids** – Responsive columns (mobile → tablet → desktop)

---

## Step 3: Read the Right Guide (2 min)

**Pick ONE guide based on what you need:**

| Your Role | Start Here | Why |
|-----------|-----------|-----|
| **Designer** | `DESIGN_SYSTEM.md` | Understand all components |
| **Frontend Dev** | `DESIGN_QUICK_REFERENCE.md` | Fast class name lookup |
| **HTML/Template Dev** | `TEMPLATE_BEST_PRACTICES.md` | Learn HTML patterns |
| **CSS Modifier** | `CSS_ARCHITECTURE.md` | Understand CSS structure |
| **Tester/QA** | `VERIFICATION_CHECKLIST.md` | Testing checklist |
| **Everyone** | `DESIGN_README.md` | Overview and navigation |

---

## Step 4: Build Your First Page

### Option A: Copy a Template

Looking at `/internal/ui/templates/pages/login.html` as example:

```html
{{ define "main-area" }}
<div class="container">
  <div style="max-width: 400px; margin: 2em auto;">
    <h1>Your Page Title</h1>

    <form action="/endpoint" method="POST">
      <fieldset>
        <legend>Form Title</legend>

        <div class="form-group">
          <label for="name">Name *</label>
          <input type="text" id="name" name="name" required>
        </div>

        <button type="submit" class="button--block">Submit</button>
      </fieldset>
    </form>
  </div>
</div>
{{ end }}
```

### Option B: Use a Component from `/design`

See something you like on `/design`? Right-click → Inspect → Copy the HTML.

### Option C: Reference the Docs

Need a specific component? Look it up:

```
Product Grid?           → DESIGN_QUICK_REFERENCE.md → Product Grid
Button variants?        → DESIGN_QUICK_REFERENCE.md → Buttons
Form with errors?       → DESIGN_QUICK_REFERENCE.md → Forms
Two-column layout?      → DESIGN_QUICK_REFERENCE.md → Grid Utilities
Alert boxes?            → DESIGN_QUICK_REFERENCE.md → Alerts
```

---

## Step 5: Test Your Page

**Quick Checklist:**

- [ ] Does it look good on mobile (360px)?
- [ ] Does it look good on desktop (1366px)?
- [ ] Can I navigate with Tab key?
- [ ] Are all buttons clickable?
- [ ] Are all form labels connected to inputs?
- [ ] Are all images present?
- [ ] Are all links underlined?

---

## Common Questions

### Q: Where are the styles?
**A:** `/internal/ui/static/stylesheets/styles.css` (2,500+ lines)

### Q: What colors should I use?
**A:** See `DESIGN_QUICK_REFERENCE.md` → Color Palette

### Q: How do I make a button?
**A:** `<button class="button">Click Me</button>`
- Primary: `.button`
- Secondary: `.button--secondary`
- Danger: `.button--danger`
- Full-width: `.button--block`

### Q: How do I style text?
**A:** Use existing classes:
- `.text-muted` (gray)
- `.text-success` (green)
- `.text-error` (red)
- `.text-warning` (orange)

### Q: How do I make a card?
**A:**
```html
<div class="card">
  <div class="card__header"><h3>Title</h3></div>
  <div class="card__body"><p>Content</p></div>
  <div class="card__footer"><small>Footer</small></div>
</div>
```

### Q: How do I make a form?
**A:**
```html
<form action="/submit" method="POST">
  <fieldset>
    <legend>Form Title</legend>
    <div class="form-group">
      <label for="field">Label *</label>
      <input type="text" id="field" name="field" required>
    </div>
    <button type="submit" class="button--block">Submit</button>
  </fieldset>
</form>
```

### Q: How do I add an alert?
**A:**
```html
<div class="alert alert--success">
  <h4>Success!</h4>
  <p>Your changes were saved.</p>
</div>
```
- Info: `.alert--info`
- Success: `.alert--success`
- Warning: `.alert--warning`
- Error: `.alert--error`

### Q: How do I make things responsive?
**A:** Use utility classes:
```html
<!-- Grid that becomes 1 column on mobile -->
<div class="grid grid-2-cols">
  <div>Column 1</div>
  <div>Column 2</div>
</div>

<!-- Flex that wraps on mobile -->
<div class="flex flex-gap-2 flex-wrap">
  <button>Button 1</button>
  <button>Button 2</button>
</div>
```

---

## Three Main CSS Files to Know

```
styles.css                     → All the CSS styles (2,500+ lines)
```

That's it. One file. Everything is vanilla CSS.

---

## Seven Design Guides at a Glance

```
DESIGN_README.md                    → Start here (overview)
DESIGN_QUICK_REFERENCE.md           → Quick lookups (cheat sheet)
DESIGN_SYSTEM.md                    → Complete component specs
TEMPLATE_BEST_PRACTICES.md          → HTML patterns
CSS_ARCHITECTURE.md                 → CSS organization
DESIGN_IMPLEMENTATION_SUMMARY.md    → Project overview
VERIFICATION_CHECKLIST.md           → Testing guide
```

---

## Seven Updated Templates to Reference

```
login.html                  → Form example
signup.html                 → Multi-field form example
admin.html                  → Dashboard with cards
admin_login.html            → Admin form
add_product.html            → Product form with dropdowns
partner_signup.html         → Registration form
index.html                  → Product grid example
```

---

## The Color Palette (Copy-Paste Ready)

```
White:       #FFFFFF
Black:       #000000
Off-white:   #F5F5F5
Dark gray:   #666666
Light gray:  #CCCCCC
Link blue:   #0000EE
Visited:     #551A8B
Success:     #008000
Error:       #CC0000
Warning:     #FF8C00
```

---

## The Responsive Breakpoints

```
Mobile:      0–479px    (1 column, stacked)
Tablet:      480–767px  (2–3 columns)
Desktop:     768px+     (3–4 columns, horizontal nav)
Large:       1024px+    (full featured)
```

---

## Three Rules to Remember

### Rule 1: Always Use Labels with Forms
```html
<!-- Good ✅ -->
<label for="email">Email</label>
<input type="email" id="email" name="email">

<!-- Bad ❌ -->
<input type="email" placeholder="Email">
```

### Rule 2: Always Include Alt Text
```html
<!-- Good ✅ -->
<img src="product.jpg" alt="Blue Wireless Headphones">

<!-- Bad ❌ -->
<img src="product.jpg">
```

### Rule 3: Always Use Semantic HTML
```html
<!-- Good ✅ -->
<button type="submit">Submit</button>
<nav><a href="#">Link</a></nav>
<h1>Page Title</h1>

<!-- Bad ❌ -->
<div onclick="submit()" class="button">Submit</div>
<div class="nav"><span>Link</span></div>
<div class="title">Page Title</div>
```

---

## Five-Minute Example: Build a Contact Form

**Step 1: Create file** → `internal/ui/templates/pages/contact.html`

**Step 2: Add content:**
```html
{{ define "title" }}Contact Us – Sokomoko{{ end }}

{{ define "main-area" }}
<div class="container">
  <div style="max-width: 500px; margin: 2em auto;">
    <h1>Contact Us</h1>
    <p style="color: #666666; margin-bottom: 1.5em;">
      We'd love to hear from you. Send us a message!
    </p>

    <form action="/contact" method="POST">
      <fieldset>
        <legend>Contact Form</legend>

        <div class="form-group">
          <label for="name">Name *</label>
          <input type="text" id="name" name="name" required>
        </div>

        <div class="form-group">
          <label for="email">Email *</label>
          <input type="email" id="email" name="email" required>
        </div>

        <div class="form-group">
          <label for="message">Message *</label>
          <textarea id="message" name="message" required rows="5"></textarea>
        </div>

        <button type="submit" class="button--block">Send Message</button>
      </fieldset>
    </form>

    <div class="alert alert--info" style="margin-top: 1.5em;">
      <p>We typically respond within 24 hours.</p>
    </div>
  </div>
</div>
{{ end }}
```

**Step 3: Add route** (in your Go handlers)

**Step 4: Test** → Visit the page, fill form, submit

**Done!** You've built your first design-system page.

---

## Next Steps

1. **Visit** `/design` to see all components
2. **Pick** a guide based on your role (see Step 2)
3. **Copy** a template from `/internal/ui/templates/pages/`
4. **Modify** it for your needs
5. **Test** using the checklist in `VERIFICATION_CHECKLIST.md`
6. **Deploy** when ready

---

## You Are Ready! 🎉

Everything you need:
- ✅ CSS system (2,500+ lines)
- ✅ 7 guides (documentation)
- ✅ 7+ examples (working templates)
- ✅ Component showcase (`/design`)
- ✅ Testing checklist

**Start building!**

---

**Need Help?**
- Quick lookup → `DESIGN_QUICK_REFERENCE.md`
- Want examples → `/design` in browser
- Building a page → `TEMPLATE_BEST_PRACTICES.md`
- Modifying CSS → `CSS_ARCHITECTURE.md`
- Complete overview → `DESIGN_SYSTEM.md`

---

**Version:** 1.0
**Status:** ✅ Ready
**Date:** January 25, 2025

Happy building! 🚀
