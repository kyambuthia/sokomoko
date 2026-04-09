# Design Constitution Prompt for Sokomoko

## Context

You are designing the visual and interaction language for **Sokomoko**, a minimal e-commerce platform. Your task is to create a comprehensive `design.md` file that will serve as a design constitution for all AI agents working on this project.

The codebase uses vanilla JavaScript, CSS, and HTML only. No frameworks, no React, no web component libraries. The existing project has a "paper-clean, structured, and quiet" aesthetic that you must respect and extend.

## Your Task

Study the existing UI in the `internal/ui/` directory and related CSS files. Then create a `design.md` file in the project root that establishes:

### 1. Design Philosophy
- Minimal, paper-clean aesthetic with structured whitespace
- System-native typography (no hosted fonts)
- Capsule-shaped interactive controls
- Mobile-first responsive approach
- BEM-style component naming
- Progressive enhancement (works without JS, JS enhances)

### 2. Color System
Define a complete token-based color system including:
- Background colors (surface, muted, subtle)
- Foreground/text colors (primary, secondary, muted)
- Border colors (default, subtle, strong)
- Accent/brand colors (primary action, destructive, success, warning)
- All colors must have proper contrast ratios
- Dark mode support via `prefers-color-scheme`

### 3. Typography System
- Local system font stacks only (no Google Fonts)
- Proper type scale with semantic sizing (xs, sm, base, lg, xl, 2xl, etc.)
- Line height and letter spacing tokens
- Monospace considerations

### 4. Spacing System
- Consistent spacing scale (0, 1, 2, 3, 4, 5, 6, 8, 10, 12, 16)
- Use CSS custom properties for all spacing values
- Section, component, and element-level spacing rules

### 5. Component Style Guide

Create detailed specifications for each of these components:

**Buttons**
- Variants: primary, secondary, outline, ghost, destructive
- Sizes: sm, md, lg
- States: default, hover, active, disabled, loading
- Icon support (leading, trailing)
- Full width option

**Cards**
- Variants: default, elevated, bordered
- Optional header, body, footer sections
- Interactive (clickable) variant
- Consistent padding and border radius

**Form Inputs**
- Text inputs with labels
- Textarea with auto-resize
- Select/dropdown
- Checkbox and radio buttons
- All states: default, focus, error, disabled, readonly
- Inline validation messages

**Search Form**
- Input with integrated search icon
- Clear button
- Submit button (icon or text)
- Loading state
- Empty results state

**Authentication Form**
- Email input
- Password input with show/hide toggle
- Remember me checkbox
- Submit button
- Link to signup/password reset
- Error message display area
- Success message display area

**Switches/Toggles**
- On/off toggle switch
- Label integration
- Disabled state
- Size variants

**Scroll/Scrollbar**
- Styled scrollbar matching theme
- Thin, minimal design
- Proper thumb and track colors
- Works across browsers

**Navigation**
- Top nav bar with logo and links
- Responsive hamburger menu for mobile
- Active state indicators
- Auth-aware (logged in vs logged out)

**Badges/Tags**
- Status badges
- Count indicators
- Pill-shaped with proper contrast

### 6. Animation & Motion
- Subtle, purposeful transitions (150-300ms)
- Easing functions (ease-out for entrances, ease-in-out for state changes)
- Respect `prefers-reduced-motion`
- Loading skeletons and spinners
- Focus ring animations

### 7. Accessibility Requirements
- Focus visible states
- ARIA attributes where needed
- Color contrast (WCAG AA minimum)
- Keyboard navigability
- Screen reader considerations

### 8. Responsive Breakpoints
- Mobile-first approach
- Breakpoints: sm (640px), md (768px), lg (1024px), xl (1280px)
- Container max-width and padding
- Grid/flex responsive patterns

## Output Format

Create `design.md` with these sections:
1. Philosophy & Principles
2. Design Tokens (colors, typography, spacing, borders, shadows)
3. Component Library (detailed specs for each component)
4. Animation & Motion
5. Accessibility Checklist
6. Responsive Strategy

## Quality Standards

- Every CSS value must use custom properties/tokens
- Every component must have complete state coverage
- Include actual CSS examples in code blocks
- Keep prose concise and actionable
- Design for consistency, not variety
- "Opinionated defaults, easy to override"

## Important Constraints

- NO React, Vue, or any framework
- NO external component libraries
- NO hosted web fonts
- NO Tailwind CSS
- Pure HTML, CSS, and vanilla JavaScript only
- Web Components (Custom Elements API) are acceptable and encouraged
- Follow BEM naming conventions
- Use CSS custom properties for all design tokens

Write your output directly to `design.md` in the project root. Make it comprehensive enough that another AI agent could implement the entire UI by following this constitution.
