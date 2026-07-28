# Local evaluator benchmark page design system

This contract governs the loopback-only evaluator benchmark page. It is not a marketing site, a production dashboard, or a campaign-authoring interface. Approved requirements in `docs/brainstorms/2026-07-28-evaluator-benchmark-page-requirements.md` take priority over the preliminary web templates.

## 0. Research log

- Existing surface: `cmd/email-pipeline/web/page.html` and `result.html` provide a preliminary server-rendered form, local HTMX asset, and result region. Their two controls and provisional CSS are not the product contract.
- Product inputs: read the approved requirements and implementation plan. The page has one evaluator journey: inspect a deterministic, safe preview; run or cancel one bounded local demonstration; read evidence without mistaking it for CLI benchmark proof.
- Embedded references: shortlisted `vercel.md`, `hashicorp.md`, and `clickhouse.md` for developer-tool restraint. Picked Layer A `taste-skill.md` plus Layer B `vercel.md`: use their mono-supported information hierarchy, modest radius, and fine shadow-border separation. Do not copy brand text, visual workflow colors, or external Geist fonts.
- Local typography constraint: the selected reference's local-system fallback principle is retained. This page loads no remote or new font asset; it uses the browser's system UI and system monospace stacks.
- Lazyweb: skipped. This is a repository-specific, service-free evaluator surface, and this task permits no browser use or external product research.
- Imagen drafts: skipped. The page needs no promotional imagery, and no external asset or dependency may be added.

## 1. Atmosphere and identity

Reading this as an operational evidence tool for a technical evaluator: calm, exact, and deliberately bounded. The page should feel like a local instrument panel on a light neutral workbench, with information grouped by purpose instead of a generic card grid. Its signature is the fixed message preview held beside the controls and safety boundary: it makes the rendered artifact inspectable before any work starts. Design variance is 3, motion intensity is 2, and visual density is 6. The page uses one light theme only so safety wording, validation, reports, and long source blocks remain reliably legible.

### Content and interaction plan

1. **Orient**: page title and a short purpose line identify this as a local evaluator, not benchmark authority.
2. **Constrain**: the verbatim safety statement stays adjacent to the controls at every width.
3. **Inspect**: the fixed `Customer 000001` preview stays independent from controls and completed runs. Plain text shows exact message bytes. HTML shows a constrained rendered view plus escaped source from that same application-owned message.
4. **Run**: four controls submit as an ordinary POST first. HTMX may replace only the stable run/result region.
5. **Interpret**: completed, partial-success, failure, and interrupted reports present existing accounting evidence unchanged, alongside separately labeled page metadata and two distinct timing scopes.

The primary action is `Evaluate`. During an owned enhanced run it becomes unavailable and a `Cancel run` action is available in the run region. There are no secondary conversion actions, status polling, history, charts, recipient fields, configuration drawers, or queue controls.

## 2. Color

### Palette

| Role | Token | Value | Usage |
|---|---|---:|---|
| Canvas | `--surface-canvas` | `#f7f8fa` | Page background |
| Surface | `--surface-base` | `#ffffff` | Main content and preview surfaces |
| Recessed surface | `--surface-recessed` | `#f1f3f5` | Code/source and report background |
| Interactive hover | `--surface-hover` | `#e9edf1` | Neutral control hover |
| Text strong | `--text-strong` | `#1b1f23` | Headings and essential values |
| Text default | `--text-default` | `#30363d` | Body and input text |
| Text muted | `--text-muted` | `#57606a` | Supporting copy and metadata |
| Text disabled | `--text-disabled` | `#6e7781` | Disabled controls only |
| Line | `--line-default` | `#d0d7de` | Inputs, field groups, region separation |
| Line subtle | `--line-subtle` | `#eaeef2` | Internal separators |
| Action | `--action-primary` | `#1f6feb` | Evaluate action, links, active format state |
| Action hover | `--action-primary-hover` | `#1558b0` | Primary action hover |
| Focus | `--focus-ring` | `#0969da` | Visible keyboard focus ring |
| Safe info | `--status-info` | `#0969da` | Safety and timing context |
| Success | `--status-success` | `#1a7f37` | Completed outcome only |
| Warning | `--status-warning` | `#9a6700` | Partial-success and interrupted context |
| Danger | `--status-danger` | `#cf222e` | Validation, failure, conflict, and busy context |
| Info tint | `--status-info-surface` | `#ddf4ff` | Safety and informational callout background |
| Success tint | `--status-success-surface` | `#dafbe1` | Completed outcome background |
| Warning tint | `--status-warning-surface` | `#fff8c5` | Interrupted or partial-success background |
| Danger tint | `--status-danger-surface` | `#ffebe9` | Validation, conflict, and busy background |

### Rules

- Blue is the sole interactive accent. Status colors communicate their named state and never decorate non-state content.
- The page uses the same light theme at all viewports. No dark-mode implementation is required by this local, operational brief.
- Safety is communicated by verbatim wording and information hierarchy, not color alone.
- New visual colors require a named token here before implementation. No raw visual color values belong in templates after this contract lands.

## 3. Typography

### Font stacks

- UI: `system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`
- Mono: `ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace`

These local/system-safe stacks are intentional. Do not add web fonts, font files, icon fonts, or a dependency to mimic the selected reference.

### Scale

| Role | Token | Size | Weight | Line height | Usage |
|---|---|---:|---:|---:|---|
| Page title | `--type-title` | `clamp(1.75rem, 4vw, 2.25rem)` | 650 | 1.15 | One page `h1` |
| Section title | `--type-section` | `1.25rem` | 650 | 1.3 | `h2` and report headings |
| Field label | `--type-label` | `0.875rem` | 600 | 1.35 | Form labels and control legends |
| Body | `--type-body` | `1rem` | 400 | 1.5 | Default reading text |
| Small body | `--type-small` | `0.875rem` | 400 | 1.5 | Help, status detail, and disclosures |
| Meta | `--type-meta` | `0.8125rem` | 500 | 1.4 | Submitted/effective control values and timing labels |
| Code | `--type-code` | `0.875rem` | 400 | 1.5 | Preview source, report JSON, and exact text bytes |

### Rules

- Use sans-serif for labels, prose, and actions; use mono only for code-like content, raw values, timing figures, fixture labels, and report bodies.
- Tabular numerals apply to timing and numeric evidence where supported. They improve scanning without changing content.
- Body text remains at or above `--type-small`. Do not render long reports in smaller type to fit more data on screen.
- Use sentence case. Avoid decorative uppercase labels, marketing headlines, and invented numerical claims.

## 4. Spacing and layout

### Tokens

All intentional spacing uses this 4px scale.

| Token | Value | Usage |
|---|---:|---|
| `--space-1` | `0.25rem` | Tight inline separation |
| `--space-2` | `0.5rem` | Label-to-help or inline control gap |
| `--space-3` | `0.75rem` | Field stack gap and compact padding |
| `--space-4` | `1rem` | Standard control padding and local groups |
| `--space-5` | `1.25rem` | Region inner spacing |
| `--space-6` | `1.5rem` | Region padding and primary group gap |
| `--space-8` | `2rem` | Related-section separation |
| `--space-10` | `2.5rem` | Major section separation |
| `--space-12` | `3rem` | Page block separation on desktop |

### Layout tokens and rules

| Token | Value | Usage |
|---|---:|---|
| `--content-max` | `72rem` | Page content maximum |
| `--reading-max` | `68ch` | Explanatory prose maximum |
| `--control-min` | `12rem` | Minimum useful control column width |
| `--preview-min` | `18rem` | Minimum preview pane width before stacking |
| `--radius-control` | `0.375rem` | Inputs, buttons, status regions |
| `--radius-region` | `0.5rem` | Major functional regions |
| `--target-min` | `2.75rem` | Minimum touch target block size |

- Page shell: centered `main`, max width `--content-max`, with `--space-4` inline padding at 375px and `--space-8` at 768px and above.
- The top block is a simple stack: title, purpose, safety statement. It is not a hero.
- The operational workspace follows in source order: controls and preview, then the stable run/result region. At 768px and above, controls and preview form a two-column grid; at 375px they stack with controls first.
- The safety statement belongs inside the controls region or immediately preceding its fieldset. It must remain visible without scrolling horizontally and must not move into a dismissible notice.
- Results use one full-width region. Within a successful terminal result, metadata and timing may form a two-column definition list at 768px and above, then stack on narrow screens.
- Long exact text, escaped HTML source, and report JSON preserve whitespace, wrap long unbroken tokens, and may scroll horizontally only inside their own code block. Primary page content must never create horizontal scrolling.
- Use functional regions and sparse separators, not a multi-card dashboard. A border or shadow boundary is allowed only where it distinguishes an actionable control area, preview, result, or alert.

### Responsive behavior

| Viewport | Layout contract |
|---|---|
| 375px | Single column. Controls are full width with a minimum 44px target. Count, seed, workers, and format remain individually labeled. Preview modes stack in source order. Primary and cancel actions use full available width when adjacent placement would crowd targets. Code blocks wrap and retain their own overflow fallback. |
| 768px | Main content uses wider gutters. Controls and fixed preview become two columns when both satisfy their minimum widths. Form fields use a two-column grid where their labels and errors remain readable. Result metadata may become two columns, while report output stays full width. |
| 1280px | Content remains capped at `--content-max`, not stretched to the viewport. Controls and preview are balanced columns. Fields may occupy four equal logical slots within the control grid. Safety, disconnect limitation, and timing explanation remain close to the action and result, not hidden in a footer. |

## 5. Reusable primitives and state contract

All primitives use semantic HTML first. Each named state must work in full-page and applicable HTMX-fragment representations.

### Page shell

- **Structure**: `main > header + workspace + run-result region`.
- **Layout**: single scroll owner is the document. No fixed viewport shell, sidebars, or independently scrolling panels.
- **States**: initial with fixed preview; terminal result; validation; busy; running; conflict; interrupted. The page shell never loses controls, safety statement, or fixed preview after an ordinary POST.
- **Accessibility**: one `h1`; logical `h2` sections; source order matches visual order; skip navigation is unnecessary for this single concise page unless added consistently by the application shell.

### Safety statement

- **Structure**: visible `aside` or labelled `section` using `--status-info-surface`, `--status-info`, and the exact approved wording.
- **Content**: render this text verbatim: “Synthetic demonstration only. Uses deterministic .test recipients and an in-memory digest sink. No email is sent, no recipient data is accepted, and SMTP, Redis, Asynq, and the distributed ledger are not initialized. Use the CLI benchmark for authoritative one-million-record evidence.”
- **States**: static and always visible beside or directly above the benchmark controls. It has no close affordance, confirmation checkbox, or animation.
- **Accessibility**: do not rely on an icon or color. Treat it as normal readable prose, not an assertive announcement.

### Benchmark controls

- **Structure**: `form > fieldset > legend + four field groups + action row + non-enhanced cancellation disclosure`.
- **Controls**: exactly count, fixture seed, worker count, and template format. No recipient, subject, content, transport, backend, queue, upload, or arbitrary HTML control may appear.
- **Default values**: count `100000`, seed `7`, reported logical CPU count, and plain text format. Supporting copy names count `4` with seed `7` as the quick acceptance run.
- **Field group**: `label` above `input` or `select`; bounded helper text below; field-specific error below helper text. `aria-describedby` references only the applicable helper and error elements.
- **Variants**: default, edited, invalid, disabled-during-run, and submitted/effective values shown in a terminal result.
- **States**:
  - Default: all four fields enabled; `Evaluate` is enabled.
  - Focus: `--focus-ring` outline with at least 2px visual thickness and 2px offset; never color-only indication.
  - Invalid: affected field gets `aria-invalid="true"`, error text, danger line treatment, and preserved raw submitted value. No campaign evidence or fake timing is shown.
  - Running: accepted controls are disabled to prevent duplicate submissions. Their values remain visible. A concise run state appears in the stable region.
  - Busy: no fields are silently changed; show an immediate 429 response explaining another demonstration is active. Do not offer queueing, retry timers, or cancel-and-replace.
- **Accessibility**: native numeric controls and native select/radio semantics are preferred. Error summary, if added, links to invalid fields; it does not replace inline errors. Do not use placeholder text as a label.

### Format selector and fixed preview

- **Structure**: format control is one of the four controls. Preview region has an `h2`, fixed recipient descriptor, and mode-specific content.
- **Plain-text state**: show the exact application-owned message bytes in a `pre` or `code` block. Do not reformat, synthesize, or derive the preview from submitted controls or the report.
- **HTML state**: show two labelled subsections from one application-owned message: a constrained rendered preview and escaped HTML source. The rendered preview is sandboxed or equivalently constrained so it cannot execute scripts or forms, load remote assets, or expose active external links.
- **States**: text default; HTML selected; long source; source unavailable only if a safe server error representation is already specified. Preview remains visible while validation errors, busy responses, runs, conflicts, and terminal reports occur.
- **Accessibility**: rendered and source forms have distinct headings. Source is text, never announced as a live update. Any embedded constrained preview gets a descriptive title if an `iframe` is used.

### Action row and cancellation

- **Structure**: action row holds one primary submit action. During an accepted enhanced run, it also exposes the owned `Cancel run` action in the stable run/result region.
- **Primary button**: `Evaluate` uses `--action-primary`; hover uses `--action-primary-hover`; active uses a 1px downward transform; disabled uses `--text-disabled` and a non-actionable surface.
- **Cancel button**: neutral outlined treatment, not a destructive red affordance. It cancels only the opaque identity owned by the current enhanced page state.
- **States**: ready, submitting, running, disabled, cancel available, cancellation requested, no applicable active run conflict.
- **Non-enhanced disclosure**: directly below the action row, state that navigation or disconnection cancels a non-enhanced run and that no detached work continues unseen.
- **Accessibility**: both actions meet `--target-min`; cancellation retains a visible text label; do not use icon-only actions or confirmation dialogs not required by the approved flow.

### Run and result region

- **Structure**: one stable element, for example `section#result`, with a section heading and `aria-live="polite"`. HTMX targets and swaps only this region.
- **Initial state**: concise guidance that a bounded fixture count and seed can be evaluated. It must not imply work has started.
- **Running state**: clear statement that the local campaign is running with the submitted configuration. The cancellation control is present only for an owned enhanced run. Do not use an artificial percent-complete indicator, spinner, polling, or invented throughput value.
- **Completed state**: identify outcome and show existing report in full. Display validated effective count, seed, worker count, and selected format as page metadata, separate from the unchanged report body.
- **Partial-success state**: use warning treatment and the existing report, without inventing a new accounting interpretation.
- **Failure state**: use danger treatment and the safe evaluator-readable report or error representation. Never expose raw parser, runtime, environment, recipient, credential, or personalized-body data.
- **Interrupted state**: use warning treatment, identify the run as interrupted, and show the reconciled existing accounting report.
- **Timing and evidence block**: in every terminal report that has valid work, show campaign-processing elapsed time and total server-request duration as separately named values. State that request duration begins after successful validation, ends when response data is ready, and excludes browser rendering and network transfer. State that this is machine-specific interactive evidence and direct readers to the CLI procedure for authoritative one-million-record evidence.
- **Report block**: use a mono `pre`/`code` treatment. Preserve report JSON formatting supplied by the server. For explicit `Accept: text/plain`, the response body remains the existing compact report JSON plus its trailing newline; page-only format and request-duration metadata must not alter that body.

### Validation, busy, and conflict alerts

- **Structure**: contextual alert inside the stable result region, with a concise heading and safe explanation.
- **Validation (400)**: errors are field-specific in the form and repeated as a short region-level summary only when needed. No campaign work, report, or timing claim is created.
- **Busy (429)**: explain that one demonstration is already active, the request did not enter a queue, and the active run was not changed.
- **Cancellation conflict (409)**: explain that there is no applicable active run to cancel and no other run changed.
- **Accessibility**: `role="status"` or `aria-live="polite"` is sufficient for ordinary updates. Do not use assertive announcements unless the implementation proves polite updates are missed. Alerts must be readable without their color background.

### Evidence definition list

- **Structure**: `dl` with semantic `dt`/`dd` pairs for effective controls, selected format, campaign time, total request duration, and evidence limitation.
- **Layout**: one column at 375px; two columns from 768px when labels and values remain intact.
- **States**: shown only after valid processing reaches a terminal report. Omitted from validation and busy responses because those have no benchmark measurement.
- **Accessibility**: labels remain explicit text, not visual badges. Numeric values use mono and tabular numerals but are announced as ordinary text.

## 6. Motion and interaction

Motion is limited to feedback, not decoration.

| Interaction | Duration | Easing | Purpose |
|---|---:|---|---|
| Button hover/focus color transition | `120ms` | `ease-out` | Signals affordance |
| Button active transform | `100ms` | `ease-out` | Confirms a press |
| Result replacement | `160ms` opacity only | `ease-out` | Makes HTMX update legible without delaying content |

- Animate only `opacity` and `transform`. Do not animate dimensions, layout, scrolling, color pulses, or progress indicators.
- Under `prefers-reduced-motion: reduce`, result replacement is immediate and active transforms are removed.
- Native browser validation behavior and HTMX request lifecycle remain the source of truth. Custom JavaScript is limited to the HTMX non-2xx response listener needed to swap safe 400, 409, and 429 fragments into the stable result region. No custom client state, polling, timers, charts, routing, or request orchestration is allowed.

## 7. Depth and surface

### Strategy

Use restrained mixed separation: subtle line tokens for functional boundaries plus a Vercel-inspired shadow-as-border treatment for major actionable regions. The implementation keeps it light and local:

| Level | Token or treatment | Usage |
|---|---|---|
| Canvas | `--surface-canvas` | Page background |
| Base | `--surface-base` | Controls, preview, and result regions |
| Recessed | `--surface-recessed` | Exact bytes, escaped source, and report code blocks |
| Functional boundary | `1px solid var(--line-default)` | Inputs and grouped controls |
| Region boundary | `0 0 0 1px color-mix(in srgb, var(--line-default), transparent 20%), 0 1px 2px rgb(27 31 35 / 4%)` | Major regions, if `color-mix` support is acceptable; otherwise a tokenized line plus 4% shadow |

- Use `--radius-control` for inputs and buttons and `--radius-region` for major regions. No pills except an actual compact state indicator if it communicates a real outcome.
- Do not create a decorative grid, glass treatment, gradient mesh, large hero object, or generic card gallery. The fixed preview and readable evidence are the visual anchors.
- Avoid icon dependencies. Text labels and semantic headings are sufficient for this scope. If an icon is later required, it must be an existing local SVG asset or added through an approved dependency decision, never an emoji.

## 8. Accessibility constraints and accepted debt

### Constraints

- Target WCAG 2.2 AA: 4.5:1 contrast for normal text, 3:1 for large text and component boundaries, visible focus for every interactive element, and keyboard access to every control and action.
- A1 is the primary persona: an evaluator who needs to verify scope, run a deterministic demonstration, inspect message content, and interpret accounting without prior product familiarity.
- Cognitive constraint: disclose the safety boundary, timing scope, and CLI authority in plain language at the point where an evaluator acts or reads evidence. Do not ask the evaluator to infer what is synthetic, what ran, or which duration means what.
- Error constraint: preserve raw submitted field values; name only affected fields; state that no work ran after invalid input; never expose internal error details.
- Status constraint: distinguish status through heading, text, position, and color. No semantic state is conveyed with a colored dot, color alone, a transient toast, or a disappearing notification.
- Screen-reader constraint: controls have labels and associated help/errors; stable results use polite live updates; source and report blocks have headings; preview mode changes are announced through the labelled content region without repeatedly reading long code blocks.
- Keyboard and touch constraint: target block size is at least `--target-min`; focus order follows source order; focus remains predictable after an HTMX swap. Move focus to the result heading only when an HTMX response requires attention and never interrupt typing in an invalid field.
- Adaptive constraint: no required hover interaction, no automatic time-based dismissal, no motion-dependent meaning, and reduced motion is respected.
- Content constraint: the exact safety statement must remain verbatim. Copy is concise, technical, and evidence-oriented. Avoid marketing language, emojis, and claims that browser timing is portable or authoritative benchmark evidence.

### Primitive showcase and state-harness plan

Build and verify the following before composing the final page template. The harness may be a temporary server-rendered route, fixture view model, or a test-only template path. It must not introduce external dependencies or persist as product functionality unless it is useful to the local evaluator.

| Primitive or state | Required evidence at 375 / 768 / 1280 |
|---|---|
| Controls | Defaults, labels, helper text, focus, touched values, disabled running values, and long field error |
| Safety statement | Exact text, line wrapping, contrast, and adjacent placement with controls |
| Text preview | Exact message bytes, wrapping, copy selection, and long unbroken content fallback |
| HTML preview | Constrained rendered view and escaped source, with no active content or remote asset behavior |
| Action row | Ready, focus, active, disabled, running, cancel available, and reduced-motion behavior |
| Result region | Initial, completed, partial-success, failure, interrupted, validation 400, conflict 409, and busy 429 |
| Evidence metadata | Separate count, seed, worker, format, campaign time, request time, and CLI limitation at all widths |
| Report code block | Full report readability, wrapping, own-axis overflow fallback, and no page-level horizontal scroll |

After implementation, run browser visual QA at 375px, 768px, and 1280px for full-page POST and HTMX representations. Exercise keyboard focus, invalid inputs, text and HTML preview changes, running/cancel, interrupted, busy, conflict, and complete states. Verify the non-2xx HTMX listener swaps the server response while preserving real HTTP status. Review any remaining accessibility or cognitive issue against this section before it can be accepted as debt.

### Accepted debt

| Item | Location | Why accepted | Owner / exit |
|---|---|---|---|
| No active progress measurement | Running result region | The approved contract has no polling or progress semantics; a fake percentage would misstate campaign evidence. | Retain unless a separately approved server-owned progress contract exists. |
| No interactive visual verification in this documentation phase | Not applicable yet | This task authorizes only the design contract and explicitly forbids browser use. | Run the state-harness and page QA specified above during U6 implementation. |

## Implementation rules

- Apply this document before modifying `cmd/email-pipeline/web/page.html`, `result.html`, or related server view models.
- Every visual color, typography size, spacing intent, radius, focus treatment, shadow, and motion duration must trace to a token or rule in this document. Extend this document before introducing a new reusable visual decision.
- Preserve progressive enhancement. Full-page POST is authoritative; HTMX is an enhancement over a stable result region.
- Preserve the local asset boundary. The existing embedded HTMX artifact may remain. No external asset, font, analytics script, image, API, optional-service connection, or browser network request beyond the local listener may be added.
- Do not treat the preliminary scaffold as an implementation constraint where it conflicts with this document or the approved requirements.
