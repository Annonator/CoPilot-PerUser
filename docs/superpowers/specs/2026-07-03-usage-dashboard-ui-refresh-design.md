# Usage Dashboard UI Refresh Design

Date: 2026-07-03

## Purpose

Refresh the Copilot AI usage dashboard so it feels like a focused developer tool rather than a dark card-based status page. The redesign keeps the current self-service scope and data contract, but changes the information architecture, responsive model breakdown, daily usage visualization, and theme behavior.

The immediate product problem is that the current model breakdown table forces a horizontal scrollbar and clips important financial columns. The broader UX problem is that the page feels visually heavy, with oversized header treatment, repeated card chrome, and a table squeezed into a secondary panel.

## Confirmed Decisions

- Use a compact operational dashboard layout, based on direction C from the visual companion.
- Make model usage the primary workspace.
- Move period, user, and total metrics into a compact summary rail.
- Replace the current daily row list with a stacked daily bar chart.
- Each daily bar must expose exact values through hover and keyboard focus.
- Add a Light/System/Dark theme preference.
- Default theme behavior follows the system preference; the dark palette should use the softer slate/cyan treatment, not a high-contrast terminal look.
- Preserve the backend and frontend usage contract. This is a UI redesign, not a data model change.

## Source Context

- Existing implementation: `src/web/components/usage-dashboard.tsx` and `src/web/app/globals.css`.
- Visual decisions were made through local mockups in the design discussion.
- The implementation should use original markup and styling written for this project.

## Layout

The primary dashboard becomes a two-region application surface:

```text
Top navigation
  brand
  theme preference control
  sign out

Dashboard shell
  left summary rail
    user / GitHub login
    period
    included credits
    additional credits
    gross amount
    additional usage

  main workspace
    model breakdown table
    daily usage stacked bar chart
```

The model breakdown table should be first in the main workspace because the user is trying to understand which models drove usage. The daily chart sits below it as period context.

The summary rail is not a navigation sidebar. It is a compact, persistent usage summary. It should avoid large decorative cards and instead use simple dividers, labels, and values.

## Model Breakdown

The model breakdown must not require horizontal scrolling at normal desktop or mobile widths.

Desktop table columns:

- Model
- Included credits
- Additional credits
- Gross amount
- Additional usage
- Price per credit

Responsive behavior:

- Keep `Model`, `Included credits`, `Additional credits`, and `Additional usage` visible as long as practical.
- Move lower-priority fields such as `Gross amount` and `Price per credit` into secondary row details or hide them at narrower widths.
- On mobile, each model may render as a compact stacked row/card with label-value pairs instead of a wide table.
- Do not use a horizontally scrollable table as the primary responsive solution.

Empty state:

- Preserve the existing `No model usage` meaning.
- Render it inside the model section without creating a large empty card.

## Daily Usage Chart

Daily usage becomes a stacked bar chart where each day is a discrete bar.

Each bar represents:

- Included credits segment.
- Additional credits segment.
- Total bar height based on included plus additional credits.

The chart should include:

- Legend for included and additional segments.
- Date axis with enough labels to orient the month without overcrowding.
- Stable bar dimensions so hover/focus states do not shift layout.
- Empty state equivalent to the current `No daily usage`.

Hover and focus interaction:

- Hovering a bar shows a tooltip.
- Keyboard focus on a bar shows the same tooltip.
- Tooltip includes:
  - Day label.
  - Included credits.
  - Additional credits.
  - Total credits.
  - Additional usage amount.
- Each focusable bar has an accessible label containing the same values.
- Tooltips must not be clipped by the chart container.

Mobile behavior:

- Tapping or focusing a bar should reveal the same exact values.
- If a hover-style tooltip is awkward on narrow screens, show a selected-day details row below the chart.
- The chart must not introduce horizontal page scroll.

## Theme Behavior

Add a small theme preference control with three states:

- Light
- System
- Dark

System is the default behavior. Persist an explicit user choice locally in the browser. The persisted choice affects only presentation and does not need backend storage.

Dark mode should use:

- Deep slate background.
- Muted slate borders and dividers.
- Cyan for included credits and selected theme state.
- Amber for additional credits.
- Avoid strong glow, decorative gradients, and terminal-green novelty styling.

Light mode should use:

- Neutral gray application background.
- White or near-white content surfaces.
- Subtle gray borders and dividers.
- The same cyan/amber data semantics where practical.

## Component Boundaries

Refactor the dashboard into smaller UI units while preserving the current data inputs:

- `UsageDashboard`: top-level composition and state/error routing.
- `SummaryRail`: user identity, period, and totals.
- `ModelBreakdown`: responsive model table/rows.
- `DailyUsageChart`: stacked daily bars, legend, tooltip/selected-day details.
- `ThemePreference`: Light/System/Dark control and local preference handling.

Formatting helpers can remain local or move to a small utility module if reuse becomes clearer during implementation.

## Data Flow

The redesign uses the existing `MonthlyUsage` shape:

- `usage.user` feeds the summary rail.
- `usage.period` feeds the period display.
- `usage.totals` feeds the summary rail.
- `usage.models` feeds the model breakdown.
- `usage.daily` feeds the stacked daily chart.

No backend or API changes are required.

## Accessibility

- Theme control must be keyboard operable and expose selected state.
- Daily bars must be keyboard focusable or have an equivalent accessible data table/list.
- Tooltip values must not be hover-only for keyboard or touch users.
- Color must not be the only signal for included versus additional credits; labels and accessible text must carry the meaning.
- Responsive model rows must preserve semantic labels for values.

## Testing And Verification

Unit/component tests should cover:

- Model breakdown renders all model values and responsive-only secondary values remain present in the DOM or accessible details.
- Daily chart renders one interactive bar per day.
- Daily chart tooltip or selected-day details expose exact included, additional, total, and additional usage values.
- Empty states for daily and model usage still render.
- Theme preference can switch Light/System/Dark and persists locally.

Rendered verification should cover:

- Desktop layout has no model breakdown horizontal scrollbar.
- Mobile layout has no page-level or model-section horizontal scrollbar.
- Tooltip is visible and not clipped.
- Keyboard focus reveals the same daily values as hover.
- Light, system/dark, and explicit dark mode are visually legible.

## Out Of Scope

- Admin views.
- New filters beyond the existing period query behavior.
- Backend or API changes.
- Persisting theme preference on the server.
- Adding a charting dependency unless implementation proves native HTML/CSS/SVG too costly.
