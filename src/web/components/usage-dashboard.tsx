import type { DailyUsage, ModelUsage, MonthlyUsage } from "@/lib/usage-types";

type UsageDashboardProps = {
  usage?: MonthlyUsage;
  error?: string;
};

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

function formatNumber(value: number): string {
  return numberFormatter.format(value);
}

function formatMoney(value: number): string {
  return moneyFormatter.format(value);
}

function formatDay(day: string): string {
  return compactDateFormatter.format(new Date(`${day}T00:00:00Z`));
}

function periodLabel(usage: MonthlyUsage): string {
  return usage.period.label ?? `${usage.period.year}-${String(usage.period.month).padStart(2, "0")}`;
}

function dailyTotal(day: DailyUsage): number {
  return day.totals.includedCredits + day.totals.additionalCredits;
}

function maxDailyTotal(days: DailyUsage[]): number {
  return Math.max(1, ...days.map(dailyTotal));
}

function MetricCard({
  label,
  value,
  detail
}: {
  label: string;
  value: string;
  detail: string;
}) {
  return (
    <section className="metric-card" aria-label={label}>
      <p>{label}</p>
      <strong>{value}</strong>
      <span>{detail}</span>
    </section>
  );
}

function DailyBars({ days }: { days: DailyUsage[] }) {
  const max = maxDailyTotal(days);

  return (
    <section className="panel" aria-labelledby="daily-usage-heading">
      <div className="panel-heading">
        <div>
          <h2 id="daily-usage-heading">Daily usage</h2>
          <p>Included and additional credits by day.</p>
        </div>
      </div>
      <div className="daily-bars">
        {days.map((day) => {
          const includedWidth = `${Math.round((day.totals.includedCredits / max) * 100)}%`;
          const additionalWidth = `${Math.round((day.totals.additionalCredits / max) * 100)}%`;

          return (
            <div className="daily-row" key={day.day}>
              <div className="daily-label">
                <span>{formatDay(day.day)}</span>
                <strong>{formatNumber(dailyTotal(day))} credits</strong>
                <em>{formatMoney(day.totals.additionalUsage)} additional usage</em>
              </div>
              <div className="daily-track" aria-hidden="true">
                <span className="daily-included" style={{ width: includedWidth }} />
                <span className="daily-additional" style={{ width: additionalWidth }} />
              </div>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function ModelBreakdown({ models }: { models: ModelUsage[] }) {
  return (
    <section className="panel model-panel" aria-labelledby="model-breakdown-heading">
      <div className="panel-heading">
        <div>
          <h2 id="model-breakdown-heading">Model breakdown</h2>
          <p>Normalized from GitHub AI credit billing fields.</p>
        </div>
      </div>
      <div className="table-wrap">
        <table aria-label="Model breakdown">
          <thead>
            <tr>
              <th>Model</th>
              <th>Included credits</th>
              <th>Additional credits</th>
              <th>Gross amount</th>
              <th>Additional usage</th>
            </tr>
          </thead>
          <tbody>
            {models.map((model) => (
              <tr key={model.model}>
                <th scope="row">{model.model}</th>
                <td>{formatNumber(model.includedCredits)}</td>
                <td>{formatNumber(model.additionalCredits)}</td>
                <td>{formatMoney(model.grossAmount)}</td>
                <td>{formatMoney(model.additionalUsage)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
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

  if (!usage || (usage.daily.length === 0 && usage.models.length === 0)) {
    return (
      <section className="state-panel">
        <h1>No usage found</h1>
        <p>No Copilot AI credit usage was returned for this period.</p>
      </section>
    );
  }

  return (
    <main className="dashboard-shell">
      <header className="dashboard-header">
        <div>
          <p className="eyebrow">
            {usage.user.githubLogin ? `@${usage.user.githubLogin}` : usage.user.email}
          </p>
          <h1>{periodLabel(usage)}</h1>
          <p>{usage.user.email}</p>
        </div>
      </header>

      <section className="metric-grid" aria-label="Usage totals">
        <MetricCard
          label="Included credits"
          value={formatNumber(usage.totals.includedCredits)}
          detail="GitHub discount quantity"
        />
        <MetricCard
          label="Additional credits"
          value={formatNumber(usage.totals.additionalCredits)}
          detail="Net quantity"
        />
        <MetricCard
          label="Gross amount"
          value={formatMoney(usage.totals.grossAmount)}
          detail={`Price per credit ${formatMoney(usage.totals.pricePerCredit ?? 0)}`}
        />
        <MetricCard
          label="Additional usage"
          value={formatMoney(usage.totals.additionalUsage)}
          detail="Net amount"
        />
      </section>

      <div className="dashboard-grid">
        <DailyBars days={usage.daily} />
        <ModelBreakdown models={usage.models} />
      </div>
    </main>
  );
}
