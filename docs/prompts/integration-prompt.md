# UI Overhaul Integration Prompt

You are merging the Sokomoko UI overhaul streams into the main branch.

## Context

The following branches have been created and pushed for a complete UI overhaul:
- `ui-overhaul/stream-1-tokens` - Design tokens and base styles
- `ui-overhaul/stream-2-buttons` - Button component system
- `ui-overhaul/stream-3-forms` - Form system components
- `ui-overhaul/stream-4-cards-nav` - Cards, navigation, layout components
- `ui-overhaul/stream-5-webcomponents` - Web Components and JS enhancements

The design constitution is in `design.md`.

## Your Task

### Phase 1: Review Each Branch

For each branch, run:
```bash
git fetch origin
git log origin/ui-overhaul/stream-N-description --oneline
git diff trunk..origin/ui-overhaul/stream-N-description --stat
```

Review what changed in each stream and note any potential conflicts.

### Phase 2: Create Integration Branch

```bash
git checkout trunk
git pull origin trunk
git checkout -b ui-overhaul/integration
```

### Phase 3: Merge Streams

Merge streams in dependency order (1 → 2 → 3 → 4 → 5):

```bash
git merge origin/ui-overhaul/stream-1-tokens --no-edit
git merge origin/ui-overhaul/stream-2-buttons --no-edit
git merge origin/ui-overhaul/stream-3-forms --no-edit
git merge origin/ui-overhaul/stream-4-cards-nav --no-edit
git merge origin/ui-overhaul/stream-5-webcomponents --no-edit
```

### Phase 4: Resolve Conflicts

If conflicts occur:
1. Read each conflicting file
2. Keep the version that best matches `design.md`
3. Use CSS custom properties exclusively
4. Prefer the most complete/larger change if styles differ
5. Run `git add` after resolving each file
6. Run `git commit` to complete merge

### Phase 5: Verify

```bash
go test ./...
```

If tests fail due to pre-existing issues (not your changes), note them but continue.

### Phase 6: Review Combined CSS

Open `internal/ui/static/stylesheets/styles.css` and verify:
- [ ] All color tokens are defined in `:root`
- [ ] Dark mode tokens exist under `@media (prefers-color-scheme: dark)`
- [ ] No duplicate token definitions
- [ ] No conflicting button/form/card selectors
- [ ] Components use CSS custom properties
- [ ] No arbitrary color values (all should be token-based)

Fix any issues found.

### Phase 7: Commit Integration

```bash
git commit -m "Merge UI overhaul streams 1-5

- Design tokens: colors, typography, spacing, radius, shadows
- Button system: 5 variants, 4 sizes, all states
- Form system: inputs, search, auth, switches
- Layout: cards, navigation, badges, alerts
- Web Components: UIAlert, UISearchForm, UISwitch
- Dark mode and reduced motion support
- Accessibility improvements"
```

### Phase 8: Push and Report

```bash
git push -u origin ui-overhaul/integration
```

Then output:
1. Summary of what was merged
2. Any conflicts encountered and how they were resolved
3. Any issues found in the combined CSS
4. Test results
5. Recommendation: ready to merge to trunk or needs manual review
