"use client";

import { useEffect, useMemo, useState } from "react";
import type { RequestLog, UsageSummaryItem } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formatMicroUSD, type PageProps } from "./pageUtils";
import { groupLabel } from "./UsageSummaryPage";

type DailyMetric = {
  label: string;
  requestCount: number;
  costMicroUSD: number;
};

export function DashboardPage({ client }: PageProps) {
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [summary, setSummary] = useState<UsageSummaryItem[]>([]);
  const [daily, setDaily] = useState<DailyMetric[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    const days = lastDays(7);
    Promise.all([
      client.listRequestLogs("?limit=5"),
      client.usageSummary("?group_by=model"),
      ...days.map((day) => client.usageSummary(dayQuery(day.from, day.to)))
    ])
      .then(([logResponse, summaryResponse, ...dailyResponses]) => {
        setLogs(logResponse.items);
        setSummary(summaryResponse.items);
        setDaily(
          dailyResponses.map((response, index) => ({
            label: days[index].label,
            requestCount: response.items.reduce((sum, item) => sum + item.request_count, 0),
            costMicroUSD: response.items.reduce((sum, item) => sum + item.total_cost_micro_usd, 0)
          }))
        );
      })
      .catch((err: Error) => setError(err.message));
  }, [client]);

  const metrics = useMemo(
    () => ({
      requestCount: summary.reduce((sum, item) => sum + item.request_count, 0),
      totalTokens: summary.reduce((sum, item) => sum + item.total_tokens, 0),
      totalCost: summary.reduce((sum, item) => sum + item.total_cost_micro_usd, 0),
      errorCount: summary.reduce((sum, item) => sum + item.error_count, 0),
      errorRate: rate(
        summary.reduce((sum, item) => sum + item.error_count, 0),
        summary.reduce((sum, item) => sum + item.request_count, 0)
      )
    }),
    [summary]
  );

  return (
    <div className="page-grid">
      <section className="metric-grid">
        <Metric label="Requests" value={metrics.requestCount.toLocaleString()} />
        <Metric label="Tokens" value={metrics.totalTokens.toLocaleString()} />
        <Metric label="Cost" value={formatMicroUSD(metrics.totalCost)} />
        <Metric label="Errors" value={metrics.errorCount.toLocaleString()} />
        <Metric label="Error Rate" value={`${metrics.errorRate.toFixed(2)}%`} />
      </section>
      {error && <div className="alert error">{error}</div>}
      <section className="dashboard-grid">
        <div className="panel">
          <h2>Daily Requests</h2>
          <ColumnChart
            items={daily.map((item) => ({
              label: item.label,
              value: item.requestCount,
              display: item.requestCount.toLocaleString()
            }))}
          />
        </div>
        <div className="panel">
          <h2>Daily Cost</h2>
          <ColumnChart
            items={daily.map((item) => ({
              label: item.label,
              value: item.costMicroUSD,
              display: formatMicroUSD(item.costMicroUSD)
            }))}
          />
        </div>
        <div className="panel">
          <h2>Model Cost Breakdown</h2>
          <BreakdownChart
            items={summary.map((item) => ({
              label: groupLabel(item, "model"),
              value: item.total_cost_micro_usd,
              display: formatMicroUSD(item.total_cost_micro_usd)
            }))}
          />
        </div>
        <div className="panel">
          <h2>Recent Request Logs</h2>
          <DataTable
            items={logs}
            empty="No recent requests."
            columns={[
              { key: "model", header: "Model", render: (item) => item.request_model ?? "-" },
              { key: "status", header: "Status", render: (item) => item.status },
              { key: "latency", header: "Latency", render: (item) => `${item.latency_ms}ms` },
              { key: "started", header: "Started", render: (item) => formatDate(item.started_at) }
            ]}
          />
        </div>
        <div className="panel">
          <h2>Usage By Model</h2>
          <DataTable
            items={summary}
            empty="No usage yet."
            columns={[
              { key: "model", header: "Model", render: (item) => groupLabel(item, "model") },
              { key: "requests", header: "Requests", render: (item) => item.request_count },
              { key: "tokens", header: "Tokens", render: (item) => item.total_tokens },
              { key: "cost", header: "Cost", render: (item) => formatMicroUSD(item.total_cost_micro_usd) }
            ]}
          />
        </div>
      </section>
    </div>
  );
}

function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="metric-card">
      <div className="eyebrow">{label}</div>
      <div className="metric-value">{value}</div>
    </div>
  );
}

function ColumnChart({ items }: { items: Array<{ label: string; value: number; display: string }> }) {
  const max = Math.max(1, ...items.map((item) => item.value));

  return (
    <div className="column-chart">
      {items.map((item) => (
        <div className="column-item" key={item.label}>
          <div className="column-value">{item.display}</div>
          <div className="column-track" aria-label={`${item.label}: ${item.display}`}>
            <div className="column-bar" style={{ height: `${Math.max(6, (item.value / max) * 100)}%` }} />
          </div>
          <div className="column-label">{item.label}</div>
        </div>
      ))}
    </div>
  );
}

function BreakdownChart({ items }: { items: Array<{ label: string; value: number; display: string }> }) {
  const sorted = [...items].sort((a, b) => b.value - a.value).slice(0, 6);
  const max = Math.max(1, ...sorted.map((item) => item.value));
  if (sorted.length === 0) {
    return <div className="empty-state">No model cost yet.</div>;
  }

  return (
    <div className="breakdown-chart">
      {sorted.map((item) => (
        <div className="breakdown-row" key={item.label}>
          <div className="breakdown-label">{item.label}</div>
          <div className="breakdown-track" aria-label={`${item.label}: ${item.display}`}>
            <div className="breakdown-bar" style={{ width: `${Math.max(3, (item.value / max) * 100)}%` }} />
          </div>
          <div className="breakdown-value">{item.display}</div>
        </div>
      ))}
    </div>
  );
}

function lastDays(count: number) {
  const today = new Date();
  today.setHours(0, 0, 0, 0);
  return Array.from({ length: count }, (_, index) => {
    const from = new Date(today);
    from.setDate(today.getDate() - (count - index - 1));
    const to = new Date(from);
    to.setDate(from.getDate() + 1);
    return {
      from,
      to,
      label: from.toLocaleDateString(undefined, { month: "short", day: "numeric" })
    };
  });
}

function dayQuery(from: Date, to: Date) {
  const params = new URLSearchParams({
    group_by: "model",
    from: from.toISOString(),
    to: to.toISOString()
  });
  return `?${params.toString()}`;
}

function rate(part: number, total: number) {
  if (total <= 0) {
    return 0;
  }
  return (part / total) * 100;
}
