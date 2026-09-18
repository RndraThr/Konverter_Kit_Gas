# Konkit Design System — Master

> Global source of truth for Konkit's web interface. Page-level files in
> `pages/` may override these rules only when the deviation is documented.

## Product direction

Konkit is a data-dense operational application for preparing assistance
programs, managing recipients, validating evidence, and recording distribution.
The interface must feel trustworthy, calm, efficient, and suitable for prolonged
administrative work.

- Platform: responsive desktop-first web application, fully usable on mobile.
- Stack: React 19, TypeScript, Vite, Tailwind CSS 4, shadcn/Base UI, Lucide.
- Visual character: warm institutional green, restrained gold accent, light
  neutral surfaces, compact information density, and strong hierarchy.
- Accessibility: keyboard-first operation, visible focus, semantic controls,
  sufficient contrast, reduced-motion support, and clear validation feedback.

## Design principles

1. **Clarity before decoration.** Operational data and task status always win.
2. **Progressive disclosure.** Keep tables scannable; place detailed editing in
   well-structured dialogs or dedicated workspaces.
3. **Stable interaction.** Hover, loading, sorting, and sticky states must not
   shift layout or make underlying content bleed through.
4. **Evidence at a glance.** Completion, required/optional evidence, and blocking
   conditions must be visible without opening a record.
5. **Safe actions.** Destructive actions need explicit labeling, confirmation,
   and a visual treatment distinct from the primary action.

## Token architecture

Use three layers: primitive values → semantic purpose → component tokens.
Components must consume semantic or component tokens, never introduce arbitrary
brand colors locally.

### Primitive palette

| Token | Value | Purpose |
|---|---:|---|
| `green-900` | `#173d2b` | Deep navigation and strong green text |
| `green-700` | `#28733a` | Section headings and active states |
| `green-600` | `#2f7d4a` | Primary action and focus identity |
| `green-500` | `#4fad42` | Progress and positive emphasis |
| `green-100` | `#edf3eb` | Secondary surfaces |
| `canvas` | `#f6f7f2` | Application background |
| `surface` | `#ffffff` | Cards, dialogs, table cells |
| `ink` | `#17251c` | Primary text |
| `muted-ink` | `#667269` | Secondary text |
| `line` | `#dde4da` | Borders and separators |
| `gold-500` | `#c89b39` | Reserved accent and emphasis |
| `gold-100` | `#f5ecd8` | Warm accent surface |
| `danger-600` | `#b3403b` | Destructive actions and errors |

### Semantic tokens

The canonical implementation is `frontend/src/styles/global.css`.

```css
--background: #f6f7f2;
--foreground: #17251c;
--card: #ffffff;
--card-foreground: #17251c;
--primary: #2f7d4a;
--primary-foreground: #ffffff;
--secondary: #edf3eb;
--secondary-foreground: #173d2b;
--muted: #eef1eb;
--muted-foreground: #667269;
--accent: #f5ecd8;
--accent-foreground: #76591a;
--destructive: #b3403b;
--border: #dde4da;
--input: #d4ddd2;
--ring: #2f7d4a;
--radius: 0.625rem;
```

Legacy module tokens (`--dashboard-*`) remain valid while Program Setup,
Distribution, and DCP3 layouts are migrated. New shared components should use
the semantic tokens above.

### Component token intent

- Primary button: `primary` background, `primary-foreground` label.
- Secondary button: `secondary` background, `secondary-foreground` label.
- Destructive button: `destructive` treatment; never use primary green.
- Inputs: `card` background, `input` border, `ring` focus treatment.
- Cards/dialogs: `card` surface, `border` outline, radius `md`–`xl`.
- Sticky table cells: opaque `card`-derived background in default and hover
  states; never transparent.
- Selected navigation: deep green foreground with restrained green surface.

## Typography

- Font stack: `Aptos`, `Segoe UI Variable`, `Segoe UI`, sans-serif.
- Body text: 14–16px depending on density; never below 12px for meaningful copy.
- Table headers: 10–12px, uppercase, high weight, moderate letter spacing.
- Page titles: 24–32px; panel titles: 18–22px; section titles: 14–16px.
- Use tabular numerals for counts, identifiers, dates, and statistics.
- Avoid all-caps body copy and excessively bold secondary information.

## Spacing and shape

- Base rhythm: 4px; common gaps are 8, 12, 16, 24, and 32px.
- Interactive control height: minimum 44px.
- Default radius: 10px; nested rows may use 8–9px; pills use full radius.
- Cards use borders and subtle tonal separation before shadows.
- Dialog sections use 14–16px internal padding and clear section boundaries.

## Layout and responsiveness

- Desktop content is dense but breathable; filters and actions align to a
  predictable grid.
- At narrow widths, multi-column forms collapse to one column without overlap.
- Dialogs may be wide on desktop but must retain viewport gutters and an
  independently scrollable body with visible, stable footer actions.
- Tables may scroll horizontally. Keep high-value identity columns and the action
  column sticky only when opaque backgrounds and correct z-index are guaranteed.
- On mobile, action groups stack full-width when labels would wrap or collide.

## Component rules

### Tables

- Align header and body column sizing through one shared table structure.
- Put recipient identity and NIK before secondary program metadata.
- Headers are uppercase and sortable headers show an explicit directional icon.
- Sticky columns need opaque default, hover, selected, and header backgrounds.
- Real-time search and combinable filters update result counts and summary cards.
- Pagination supports 10, 20, 50, and 100 rows with flexible page numbering.

### Forms and validation

- Every field has a persistent visible label; placeholders are examples only.
- Required and optional status must be explicit where evidence is configured.
- Validate close to the field and preserve entered values after an error.
- Multi-error submissions should expose a summary and focus the first invalid
  field or linked error.
- Read-only fields must look intentionally locked, not disabled by accident.

### Dialogs

- Use a descriptive title and short context sentence.
- Group related fields into bordered sections with headings.
- Keep close, cancel, and save actions keyboard reachable.
- Newly added repeatable rows must scroll into view and focus their first field.
- Destructive confirmation dialogs state the affected entity and consequence.

### Feedback

- Use toast notifications for transient success/error feedback.
- For login success, show the toast long enough to be perceived before redirect.
- Use inline loading, empty, and error states in data regions; never rely only on
  a global spinner.
- Status is communicated through text/icon plus color, never color alone.

### Icons and motion

- Use Lucide icons consistently; no emoji as structural icons.
- Icon-only buttons require an accessible name and tooltip when meaning is not
  immediately obvious.
- Transitions should be subtle, generally 150–220ms, and must not shift layout.
- Respect `prefers-reduced-motion`; animations are enhancement, never required
  to understand state.

## Accessibility contract

- Normal text contrast: at least 4.5:1; meaningful non-text UI: at least 3:1.
- Focus indicators must remain visible and unobscured by sticky headers/footers.
- All controls are operable with keyboard and use native semantics where possible.
- Touch/click targets are at least 44×44px.
- Dialog focus is trapped, restored on close, and starts at a meaningful element.
- Search result updates and important asynchronous state changes use appropriate
  live-region semantics without excessive announcements.
- Test at 375, 768, 1024, and 1440px plus browser zoom at 200%.

## Do not introduce

- Purple/pink AI gradients, ornamental glassmorphism, or dark-tech styling.
- Transparent sticky cells, ambiguous icon-only critical actions, or tiny text.
- New hard-coded brand colors inside components when an existing token applies.
- Hover-only functionality, motion-dependent meaning, or color-only status.
- Page-specific visual systems that conflict with this Master without a documented
  file under `pages/`.

## Handoff checklist

- [ ] Uses existing semantic tokens and Aptos/Segoe font stack.
- [ ] Responsive at 375/768/1024/1440px with no overlap or clipped controls.
- [ ] Keyboard focus, labels, dialog behavior, and errors are verified.
- [ ] Sticky table cells remain opaque in every interaction state.
- [ ] Loading, empty, error, success, and disabled states are implemented.
- [ ] Reduced-motion behavior and contrast are checked.
- [ ] Tests cover the primary workflow and the regression being changed.
