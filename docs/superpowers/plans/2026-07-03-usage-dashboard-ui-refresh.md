# Usage Dashboard UI Refresh Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the usage dashboard UI refresh from `docs/superpowers/specs/2026-07-03-usage-dashboard-ui-refresh-design.md`.

**Architecture:** Keep the existing `MonthlyUsage` contract unchanged and refactor the current dashboard into focused render units inside `src/web/components/usage-dashboard.tsx`. Use project-owned CSS in `src/web/app/globals.css` for the compact operational layout, responsive model rows, stacked daily bar chart, and Light/System/Dark theme behavior.

**Tech Stack:** Next.js 16, React 19, TypeScript, Vitest, Testing Library, CSS custom properties, localStorage.

---

## File Structure

- Modify `src/web/components/usage-dashboard.tsx`: create focused internal components for `ThemePreference`, `SummaryRail`, `ModelBreakdown`, and `DailyUsageChart`; preserve exported `UsageDashboard`.
- Modify `src/web/components/usage-dashboard.test.tsx`: add TDD coverage for the refreshed layout, theme control, responsive/accessibility DOM, and daily chart tooltip data.
- Modify `src/web/app/globals.css`: replace the current card-heavy dashboard styling with compact operational layout, theme variables, responsive model rows, chart bars, and tooltip styles.
- Create or modify no backend files.

## Task 1: Test The Refreshed Dashboard Contract

**Files:**
- Modify: `src/web/components/usage-dashboard.test.tsx`

- [ ] **Step 1: Write failing tests for summary rail, model rows, daily chart, and theme control**

Add tests that assert the new UI contract before changing production code:

```tsx
it("renders the compact dashboard shell with summary rail and model-first workspace", () => {
  render(<UsageDashboard usage={usage} />);

  expect(screen.getByRole("main", { name: /copilot usage dashboard/i })).toBeInTheDocument();
  expect(screen.getByRole("complementary", { name: /usage summary/i })).toBeInTheDocument();
  expect(screen.getByRole("heading", { name: "Usage by model" })).toBeInTheDocument();
  expect(screen.getByText("Jun 2026")).toBeInTheDocument();
  expect(screen.getByText("@ana")).toBeInTheDocument();
  expect(screen.getByText("ana@company.name")).toBeInTheDocument();
});

it("renders model values without depending on horizontal table scrolling", () => {
  render(<UsageDashboard usage={usage} />);

  const modelRegion = screen.getByRole("region", { name: /usage by model/i });
  expect(within(modelRegion).getByText("gpt-4.1")).toBeInTheDocument();
  expect(within(modelRegion).getByText("claude-3.7-sonnet")).toBeInTheDocument();
  expect(within(modelRegion).getByText("Included credits")).toBeInTheDocument();
  expect(within(modelRegion).getByText("Additional credits")).toBeInTheDocument();
  expect(within(modelRegion).getByText("Gross amount")).toBeInTheDocument();
  expect(within(modelRegion).getByText("Price per credit")).toBeInTheDocument();
  expect(document.querySelector(".table-wrap")).not.toBeInTheDocument();
});

it("renders one accessible stacked daily bar per day with exact tooltip data", () => {
  render(<UsageDashboard usage={usage} />);

  const daily = screen.getByRole("region", { name: /daily usage/i });
  const bars = within(daily).getAllByRole("button");
  expect(bars).toHaveLength(2);
  expect(bars[0]).toHaveAccessibleName(
    "Jun 1: 900 included credits, 100 additional credits, 1,000 total credits, $1.00 additional usage"
  );
  expect(within(daily).getByText("Total credits")).toBeInTheDocument();
  expect(within(daily).getByText("1,000")).toBeInTheDocument();
});

it("switches and persists the theme preference", () => {
  const setItem = vi.spyOn(Storage.prototype, "setItem");
  render(<UsageDashboard usage={usage} />);

  const dark = screen.getByRole("button", { name: "Dark" });
  dark.click();

  expect(document.documentElement.dataset.theme).toBe("dark");
  expect(dark).toHaveAttribute("aria-pressed", "true");
  expect(setItem).toHaveBeenCalledWith("copilot-usage-theme", "dark");
});
```

- [ ] **Step 2: Run tests and verify they fail for missing UI contract**

Run:

```bash
cd src/web
npm test -- usage-dashboard.test.tsx
```

Expected: FAIL because the current dashboard has no `ThemePreference`, no summary rail role, no accessible chart bars, and still renders `.table-wrap`.

## Task 2: Implement Dashboard Components And Theme Behavior

**Files:**
- Modify: `src/web/components/usage-dashboard.tsx`

- [ ] **Step 1: Replace the old dashboard internals with focused components**

Implement these component boundaries in `usage-dashboard.tsx`:

```tsx
type ThemePreference = "light" | "system" | "dark";
const themeOptions: ThemePreference[] = ["light", "system", "dark"];
const THEME_STORAGE_KEY = "copilot-usage-theme";

function ThemePreferenceControl() {
  const [preference, setPreference] = useState<ThemePreference>("system");
  return (
    <div className="theme-toggle" role="group" aria-label="Theme preference">
      {themeOptions.map((option) => (
        <button type="button" aria-pressed={preference === option} key={option}>
          {option[0].toUpperCase() + option.slice(1)}
        </button>
      ))}
    </div>
  );
}

function SummaryRail({ usage }: { usage: MonthlyUsage }) {
  return (
    <aside className="summary-rail" aria-label="Usage summary">
      <p>{usage.user.githubLogin ? `@${usage.user.githubLogin}` : usage.user.email}</p>
      <p>{usage.user.email}</p>
      <dl>
        <div><dt>Period</dt><dd>{periodLongLabel(usage)}</dd></div>
        <div><dt>Included</dt><dd>{formatNumber(usage.totals.includedCredits)}</dd></div>
        <div><dt>Additional</dt><dd>{formatNumber(usage.totals.additionalCredits)}</dd></div>
        <div><dt>Gross</dt><dd>{formatMoney(usage.totals.grossAmount)}</dd></div>
        <div><dt>Usage</dt><dd>{formatMoney(usage.totals.additionalUsage)}</dd></div>
      </dl>
    </aside>
  );
}

function ModelBreakdown({ models }: { models: ModelUsage[] }) {
  return (
    <section className="workspace-section model-section" aria-labelledby="model-breakdown-heading">
      <h1 id="model-breakdown-heading">Usage by model</h1>
      <table className="model-table" aria-label="Usage by model">
        <thead><tr><th>Model</th><th>Included credits</th><th>Additional credits</th><th>Gross amount</th><th>Additional usage</th><th>Price per credit</th></tr></thead>
        <tbody>{models.map((model) => <tr key={model.model}><th scope="row">{model.model}</th><td>{formatNumber(model.includedCredits)}</td><td>{formatNumber(model.additionalCredits)}</td><td>{formatMoney(model.grossAmount)}</td><td>{formatMoney(model.additionalUsage)}</td><td>{formatMoney(model.pricePerCredit)}</td></tr>)}</tbody>
      </table>
    </section>
  );
}

function DailyUsageChart({ days }: { days: DailyUsage[] }) {
  return (
    <section className="workspace-section daily-section" aria-labelledby="daily-usage-heading">
      <h2 id="daily-usage-heading">Daily usage</h2>
      <div className="daily-chart">{days.map((day) => <button type="button" className="daily-bar" key={day.day} aria-label={dailyAriaLabel(day)} />)}</div>
    </section>
  );
}
```

Keep formatting helpers in the same file unless extraction becomes necessary.

- [ ] **Step 2: Run focused tests and verify component behavior passes**

Run:

```bash
cd src/web
npm test -- usage-dashboard.test.tsx
```

Expected: PASS for the new dashboard tests.

## Task 3: Implement Styling And Responsive Layout

**Files:**
- Modify: `src/web/app/globals.css`

- [ ] **Step 1: Replace the current dashboard CSS with theme-aware operational styling**

Implement CSS using these class families:

```css
:root {
  color-scheme: light;
  --bg: #f3f4f6;
  --panel: #ffffff;
  --text: #111827;
  --muted: #6b7280;
  --border: #d9e1ea;
  --included: #38bdf8;
  --additional: #f59e0b;
}

:root[data-theme="dark"] {
  color-scheme: dark;
  --bg: #0f172a;
  --panel: rgba(15, 23, 42, 0.78);
  --text: #f8fafc;
  --muted: #94a3b8;
  --border: rgba(148, 163, 184, 0.18);
}

.dashboard-shell {
  width: min(1180px, calc(100% - 2rem));
  margin: 1rem auto 4rem;
}

.dashboard-layout {
  display: grid;
  grid-template-columns: minmax(12rem, 0.28fr) minmax(0, 1fr);
  gap: 1.25rem;
}

.summary-rail {
  border-right: 1px solid var(--border);
  padding-right: 1.25rem;
}

.workspace {
  min-width: 0;
}

.model-table {
  width: 100%;
  border-collapse: collapse;
}

.model-row-details {
  display: none;
}

.daily-chart {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(1rem, 1fr));
  align-items: end;
  min-height: 10rem;
}

.daily-bar {
  position: relative;
  display: flex;
  flex-direction: column-reverse;
}

.daily-tooltip {
  position: absolute;
  z-index: 2;
}
```

Required responsive behavior:

- Desktop: summary rail left, workspace right.
- Tablet/mobile: summary rail stacks above workspace.
- No `.table-wrap` horizontal scroll wrapper.
- Mobile model rows expose secondary values as labeled details.
- Chart bars fit without page-level horizontal scrolling.

- [ ] **Step 2: Run tests and typecheck after CSS changes**

Run:

```bash
cd src/web
npm test -- usage-dashboard.test.tsx
npm run typecheck
```

Expected: both commands exit 0.

## Task 4: Full Verification And Browser QA

**Files:**
- No committed source changes unless verification finds defects.

- [ ] **Step 1: Run full automated checks**

Run:

```bash
cd src/web
npm test
npm run lint
npm run typecheck
npm run build
```

Expected: all commands exit 0.

- [ ] **Step 2: Run the fixture API and web app**

Run the API:

```bash
cd src/api
GOCACHE=/private/tmp/copilot-peruser-gocache \
PORT=8080 \
COMPANY_EMAIL_DOMAINS=company.name \
APP_TOKEN_SECRET=local-demo-secret-local-demo-secret-local-demo-secret \
GITHUB_ENTERPRISE_SLUG=local-demo \
GITHUB_ADMIN_TOKEN= \
GITHUB_IDENTITY_RESOLVER=static \
GITHUB_IDENTITY_STATIC_MAP_PATH=internal/testfixtures/identity-map.json \
GITHUB_BILLING_FIXTURE_PATH=internal/testfixtures/ai-credit-usage.json \
go run ./cmd/server
```

Run the web app:

```bash
cd src/web
AUTH_SECRET=local-auth-secret-local-auth-secret-local-auth-secret \
AUTH_GOOGLE_ID=unused \
AUTH_GOOGLE_SECRET=unused \
AUTH_DEV_EMAIL=user@company.name \
AUTH_DEV_NAME="Local Demo User" \
COMPANY_EMAIL_DOMAINS=company.name \
APP_TOKEN_SECRET=local-demo-secret-local-demo-secret-local-demo-secret \
API_BASE_URL=http://127.0.0.1:8080 \
npm run dev -- --hostname 127.0.0.1 --port 3000
```

- [ ] **Step 3: Verify rendered UI in browser**

Check desktop and mobile viewports:

- `Usage by model` appears before `Daily usage`.
- No model breakdown horizontal scrollbar.
- No page-level horizontal scrollbar.
- Light/System/Dark control changes the document theme.
- Daily bar hover/focus shows tooltip with exact values.
- Mobile daily chart remains usable through tap/focus or selected-day detail.

## Task 5: Subagent Quality Gates And Commit

**Files:**
- No source change unless reviews require fixes.

- [ ] **Step 1: Dispatch spec compliance reviewer**

Ask a subagent to compare the implementation against:

- `docs/superpowers/specs/2026-07-03-usage-dashboard-ui-refresh-design.md`
- This plan file
- The final git diff

Expected: reviewer reports spec compliance or concrete issues.

- [ ] **Step 2: Dispatch code quality reviewer**

After spec compliance passes, ask a subagent to review code quality over the implementation range.

Expected: reviewer reports no Critical or Important issues.

- [ ] **Step 3: Fix any review findings and rerun verification**

Run:

```bash
cd src/web
npm test
npm run lint
npm run typecheck
npm run build
```

Expected: all commands exit 0 after fixes.

- [ ] **Step 4: Commit the implementation**

Stage only files belonging to this task, excluding pre-existing `src/web/next-env.d.ts` unless it becomes intentionally required:

```bash
git add docs/superpowers/plans/2026-07-03-usage-dashboard-ui-refresh.md \
  src/web/components/usage-dashboard.tsx \
  src/web/components/usage-dashboard.test.tsx \
  src/web/app/globals.css
git commit -m "feat: refresh usage dashboard UI"
```

Expected: commit succeeds on `codex/usage-dashboard-ui-refresh`.

## Self-Review

- Spec coverage: layout, responsive model breakdown, daily chart tooltip behavior, theme preference, accessibility, testing, and rendered verification are each covered by a task.
- Red-flag scan: no unresolved placeholder markers are present.
- Type consistency: all named components and CSS class families are defined in the task where they are introduced.
