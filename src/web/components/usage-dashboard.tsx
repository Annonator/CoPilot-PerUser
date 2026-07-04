"use client";

import { useEffect, useState, useSyncExternalStore } from "react";

import type { DailyUsage, ModelUsage, MonthlyUsage } from "@/lib/usage-types";

type UsageDashboardProps = {
  usage?: MonthlyUsage;
  error?: string;
};

type ThemePreference = "light" | "system" | "dark";
type ChartMode = "daily" | "cumulative";
type ChartUsagePoint = {
  day: string;
  totals: DailyUsage["totals"];
};

const themeOptions: ThemePreference[] = ["light", "system", "dark"];
const chartModes: ChartMode[] = ["daily", "cumulative"];
const THEME_STORAGE_KEY = "copilot-usage-theme";
const themePreferenceListeners = new Set<() => void>();

const numberFormatter = new Intl.NumberFormat("en-US");
const moneyFormatter = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD"
});
const compactDateFormatter = new Intl.DateTimeFormat("en-US", {
  month: "short",
  day: "numeric",
  timeZone: "UTC"
});
const periodDateFormatter = new Intl.DateTimeFormat("en-US", {
  month: "short",
  year: "numeric",
  timeZone: "UTC"
});

function formatNumber(value: number): string {
  return numberFormatter.format(value);
}

function formatMoney(value: number): string {
  return moneyFormatter.format(value);
}

function formatDay(day: string): string {
  return compactDateFormatter.format(new Date(`${day}T00:00:00Z`));
}

function periodLongLabel(usage: MonthlyUsage): string {
  return periodDateFormatter.format(new Date(Date.UTC(usage.period.year, usage.period.month - 1, 1)));
}

function totalCredits(totals: DailyUsage["totals"]): number {
  return totals.includedCredits + totals.additionalCredits;
}

function dailyTotal(day: ChartUsagePoint): number {
  return totalCredits(day.totals);
}

function maxDailyTotal(days: ChartUsagePoint[], includedLimit?: number): number {
  return Math.max(1, includedLimit ?? 0, ...days.map(dailyTotal));
}

function cumulativeUsagePoints(days: DailyUsage[]): ChartUsagePoint[] {
  let includedCredits = 0;
  let additionalCredits = 0;
  let grossAmount = 0;
  let additionalUsage = 0;

  return days.map((day) => {
    includedCredits += day.totals.includedCredits;
    additionalCredits += day.totals.additionalCredits;
    grossAmount += day.totals.grossAmount;
    additionalUsage += day.totals.additionalUsage;

    return {
      day: day.day,
      totals: {
        includedCredits,
        additionalCredits,
        grossAmount,
        additionalUsage
      }
    };
  });
}

function configuredIncludedLimit(usage: MonthlyUsage): number | undefined {
  const limit = usage.budget?.monthlyIncludedCredits;
  if (limit === undefined || limit <= 0) {
    return undefined;
  }

  return limit;
}

function isThemePreference(value: string | null): value is ThemePreference {
  return value === "light" || value === "system" || value === "dark";
}

function getThemePreferenceSnapshot(): ThemePreference {
  if (typeof window === "undefined") {
    return "system";
  }

  const storedPreference = window.localStorage.getItem(THEME_STORAGE_KEY);
  return isThemePreference(storedPreference) ? storedPreference : "system";
}

function subscribeToThemePreference(onStoreChange: () => void): () => void {
  themePreferenceListeners.add(onStoreChange);

  function handleStorage(event: StorageEvent) {
    if (event.key === THEME_STORAGE_KEY) {
      onStoreChange();
    }
  }

  window.addEventListener("storage", handleStorage);

  return () => {
    themePreferenceListeners.delete(onStoreChange);
    window.removeEventListener("storage", handleStorage);
  };
}

function persistThemePreference(option: ThemePreference) {
  window.localStorage.setItem(THEME_STORAGE_KEY, option);
  themePreferenceListeners.forEach((listener) => listener());
}

function applyTheme(preference: ThemePreference) {
  const root = document.documentElement;
  if (preference === "system") {
    root.removeAttribute("data-theme");
    return;
  }

  root.dataset.theme = preference;
}

function displayThemeOption(option: ThemePreference): string {
  return option[0].toUpperCase() + option.slice(1);
}

function chartAriaLabel(day: ChartUsagePoint, mode: ChartMode): string {
  const prefix = mode === "cumulative" ? `Cumulative through ${formatDay(day.day)}` : formatDay(day.day);
  return `${prefix}: ${formatNumber(day.totals.includedCredits)} included credits, ${formatNumber(
    day.totals.additionalCredits
  )} additional credits, ${formatNumber(dailyTotal(day))} total credits, ${formatMoney(
    day.totals.additionalUsage
  )} additional usage`;
}

function displayChartMode(mode: ChartMode): string {
  return mode[0].toUpperCase() + mode.slice(1);
}

function ThemePreferenceControl() {
  const preference = useSyncExternalStore<ThemePreference>(
    subscribeToThemePreference,
    getThemePreferenceSnapshot,
    () => "system"
  );

  useEffect(() => {
    applyTheme(preference);
  }, [preference]);

  function chooseTheme(option: ThemePreference) {
    applyTheme(option);
    persistThemePreference(option);
  }

  return (
    <div className="theme-toggle" role="group" aria-label="Theme preference">
      {themeOptions.map((option) => (
        <button
          type="button"
          aria-pressed={preference === option}
          className="theme-toggle-button"
          key={option}
          onClick={() => chooseTheme(option)}
        >
          {displayThemeOption(option)}
        </button>
      ))}
    </div>
  );
}

function SummaryMetric({ label, value }: { label: string; value: string }) {
  return (
    <div className="summary-metric">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function SummaryRail({ usage }: { usage: MonthlyUsage }) {
  return (
    <aside className="summary-rail" aria-label="Usage summary">
      <div className="summary-identity">
        <p className="summary-login">
          {usage.user.githubLogin ? (
            <a
              className="summary-login-link"
              href={`https://github.com/${usage.user.githubLogin}`}
              target="_blank"
              rel="noreferrer"
            >
              @{usage.user.githubLogin}
            </a>
          ) : (
            usage.user.email
          )}
        </p>
        <p>{usage.user.email}</p>
      </div>
      <dl className="summary-metrics">
        <SummaryMetric label="Included credits" value={formatNumber(usage.totals.includedCredits)} />
        <SummaryMetric label="Additional credits" value={formatNumber(usage.totals.additionalCredits)} />
        <SummaryMetric label="Gross amount" value={formatMoney(usage.totals.grossAmount)} />
        <SummaryMetric label="Additional usage" value={formatMoney(usage.totals.additionalUsage)} />
      </dl>
    </aside>
  );
}

function ModelDetail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  );
}

function ModelBreakdown({ models }: { models: ModelUsage[] }) {
  return (
    <section className="workspace-section model-section" aria-labelledby="model-breakdown-heading">
      <div className="workspace-section-heading">
        <div>
          <h2 id="model-breakdown-heading">Usage by model</h2>
          <p>Normalized from GitHub AI credit billing fields.</p>
        </div>
      </div>
      <table className="model-table" aria-label="Usage by model">
        <thead>
          <tr>
            <th>Model</th>
            <th>Included credits</th>
            <th>Additional credits</th>
            <th>Gross amount</th>
            <th>Additional usage</th>
            <th>Price per credit</th>
          </tr>
        </thead>
        <tbody>
          {models.length === 0 ? (
            <tr>
              <td colSpan={6}>No model usage</td>
            </tr>
          ) : (
            models.map((model) => (
              <tr key={model.model}>
                <th scope="row">
                  <span>{model.model}</span>
                  <dl className="model-row-details" aria-label={`${model.model} details`}>
                    <ModelDetail label="Included credits" value={formatNumber(model.includedCredits)} />
                    <ModelDetail label="Additional credits" value={formatNumber(model.additionalCredits)} />
                    <ModelDetail label="Gross amount" value={formatMoney(model.grossAmount)} />
                    <ModelDetail label="Additional usage" value={formatMoney(model.additionalUsage)} />
                    <ModelDetail label="Price per credit" value={formatMoney(model.pricePerCredit)} />
                  </dl>
                </th>
                <td>{formatNumber(model.includedCredits)}</td>
                <td>{formatNumber(model.additionalCredits)}</td>
                <td>{formatMoney(model.grossAmount)}</td>
                <td>{formatMoney(model.additionalUsage)}</td>
                <td>{formatMoney(model.pricePerCredit)}</td>
              </tr>
            ))
          )}
        </tbody>
      </table>
    </section>
  );
}

function DailyTooltip({ day, mode }: { day: ChartUsagePoint; mode: ChartMode }) {
  return (
    <div className="daily-tooltip" aria-hidden="true">
      <strong>{mode === "cumulative" ? `Cumulative through ${formatDay(day.day)}` : formatDay(day.day)}</strong>
      <dl>
        <div>
          <dt>Included credits</dt>
          <dd>{formatNumber(day.totals.includedCredits)}</dd>
        </div>
        <div>
          <dt>Additional credits</dt>
          <dd>{formatNumber(day.totals.additionalCredits)}</dd>
        </div>
        <div>
          <dt>Total credits</dt>
          <dd>{formatNumber(dailyTotal(day))}</dd>
        </div>
        <div>
          <dt>Additional usage</dt>
          <dd>{formatMoney(day.totals.additionalUsage)}</dd>
        </div>
      </dl>
    </div>
  );
}

function SelectedDayDetails({ day, mode }: { day: ChartUsagePoint; mode: ChartMode }) {
  return (
    <section
      className="selected-day-details"
      aria-label={mode === "cumulative" ? "Selected cumulative usage" : "Selected day usage"}
    >
      <div>
        <span>{mode === "cumulative" ? "Cumulative through" : "Selected day"}</span>
        <strong>{formatDay(day.day)}</strong>
      </div>
      <dl>
        <div>
          <dt>Included credits</dt>
          <dd>{formatNumber(day.totals.includedCredits)}</dd>
        </div>
        <div>
          <dt>Additional credits</dt>
          <dd>{formatNumber(day.totals.additionalCredits)}</dd>
        </div>
        <div>
          <dt>Total credits</dt>
          <dd>{formatNumber(dailyTotal(day))}</dd>
        </div>
        <div>
          <dt>Additional usage</dt>
          <dd>{formatMoney(day.totals.additionalUsage)}</dd>
        </div>
      </dl>
    </section>
  );
}

function DailyUsageChart({ usage }: { usage: MonthlyUsage }) {
  const days = usage.daily;
  const [mode, setMode] = useState<ChartMode>("daily");
  const [selectedDayKey, setSelectedDayKey] = useState(days[0]?.day ?? "");
  const includedLimit = mode === "cumulative" ? configuredIncludedLimit(usage) : undefined;
  const chartDays = mode === "cumulative" ? cumulativeUsagePoints(days) : days;
  const max = maxDailyTotal(chartDays, includedLimit);
  const selectedDay = chartDays.find((day) => day.day === selectedDayKey) ?? chartDays[0];
  const limitPosition = includedLimit ? Math.min(100, Math.max(0, (includedLimit / max) * 100)) : undefined;

  return (
    <section className="workspace-section daily-section" aria-labelledby="daily-usage-heading">
      <div className="workspace-section-heading">
        <div>
          <h2 id="daily-usage-heading">Daily usage</h2>
          <p>{mode === "cumulative" ? "Running credits across the period." : "Included and additional credits by day."}</p>
        </div>
        <div className="chart-heading-tools">
          <div className="chart-mode-toggle" role="group" aria-label="Chart mode">
            {chartModes.map((option) => (
              <button
                type="button"
                aria-pressed={mode === option}
                className="chart-mode-button"
                key={option}
                onClick={() => setMode(option)}
              >
                {displayChartMode(option)}
              </button>
            ))}
          </div>
          <div className="chart-legend" aria-label="Chart legend">
            <span>
              <i className="legend-swatch included-swatch" aria-hidden="true" />
              Included credits
            </span>
            <span>
              <i className="legend-swatch additional-swatch" aria-hidden="true" />
              Additional credits
            </span>
          </div>
        </div>
      </div>
      {days.length === 0 ? (
        <div className="empty-row">No daily usage</div>
      ) : (
        <>
          <div className="daily-chart-scroller">
            <div className="daily-chart-frame">
              <div className="daily-chart-area">
                {limitPosition === undefined ? null : (
                  <div
                    className="included-limit-line"
                    style={{ bottom: `${limitPosition}%` }}
                    role="note"
                    aria-label={`100% limit: ${formatNumber(includedLimit ?? 0)} credits`}
                  >
                    <span>100% limit</span>
                  </div>
                )}
                <div
                  className="daily-chart"
                  role="list"
                  aria-label="Daily usage by day"
                  style={{ gridTemplateColumns: `repeat(${chartDays.length}, minmax(0, 1fr))` }}
                >
                  {chartDays.map((day) => {
                    const total = dailyTotal(day);
                    const totalHeight = `${Math.max(8, Math.round((total / max) * 100))}%`;
                    const includedHeight =
                      total === 0 ? "0%" : `${Math.round((day.totals.includedCredits / total) * 100)}%`;
                    const additionalHeight =
                      total === 0 ? "0%" : `${Math.round((day.totals.additionalCredits / total) * 100)}%`;

                    return (
                      <div className="daily-bar-wrap" role="listitem" key={day.day}>
                        <button
                          type="button"
                          className="daily-bar"
                          aria-label={chartAriaLabel(day, mode)}
                          onClick={() => setSelectedDayKey(day.day)}
                          onFocus={() => setSelectedDayKey(day.day)}
                          onMouseEnter={() => setSelectedDayKey(day.day)}
                          style={{ height: totalHeight }}
                        >
                          <span className="daily-included-segment" style={{ height: includedHeight }} />
                          <span className="daily-additional-segment" style={{ height: additionalHeight }} />
                        </button>
                        <DailyTooltip day={day} mode={mode} />
                      </div>
                    );
                  })}
                </div>
              </div>
              <div className="daily-axis" aria-hidden="true">
                <span>{formatDay(days[0].day)}</span>
                {days.length > 2 ? <span>{formatDay(days[Math.floor(days.length / 2)].day)}</span> : null}
                <span>{formatDay(days[days.length - 1].day)}</span>
              </div>
            </div>
          </div>
          {selectedDay ? <SelectedDayDetails day={selectedDay} mode={mode} /> : null}
        </>
      )}
    </section>
  );
}

export function UsageDashboard({ usage, error }: UsageDashboardProps) {
  if (error) {
    return (
      <section className="state-panel" role="alert">
        <h1>Usage unavailable</h1>
        <p>{error}</p>
      </section>
    );
  }

  if (!usage) {
    return (
      <section className="state-panel">
        <h1>No usage found</h1>
        <p>No Copilot AI credit usage was returned for this period.</p>
      </section>
    );
  }

  return (
    <main className="dashboard-shell" aria-label="Copilot usage dashboard">
      <header className="dashboard-topline">
        <div>
          <p className="eyebrow">Copilot usage</p>
          <h1>{periodLongLabel(usage)}</h1>
        </div>
        <ThemePreferenceControl />
      </header>

      <div className="dashboard-layout">
        <SummaryRail usage={usage} />
        <div className="workspace">
          <ModelBreakdown models={usage.models} />
          <DailyUsageChart usage={usage} />
        </div>
      </div>
    </main>
  );
}
