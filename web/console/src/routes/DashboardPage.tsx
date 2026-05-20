"use client";

import { useEffect, useMemo, useState } from "react";
import type { RequestLog, UsageSummaryItem } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formatMicroUSD, type PageProps } from "./pageUtils";
import { groupLabel } from "./UsageSummaryPage";

export function DashboardPage({ client }: PageProps) {
  const [logs, setLogs] = useState<RequestLog[]>([]);
  const [summary, setSummary] = useState<UsageSummaryItem[]>([]);
  const [error, setError] = useState("");

  useEffect(() => {
    Promise.all([
      client.listRequestLogs("?limit=5"),
      client.usageSummary("?group_by=model")
    ])
      .then(([logResponse, summaryResponse]) => {
        setLogs(logResponse.items);
        setSummary(summaryResponse.items);
      })
      .catch((err: Error) => setError(err.message));
  }, [client]);

  const metrics = useMemo(
    () => ({
      requestCount: summary.reduce((sum, item) => sum + item.request_count, 0),
      totalTokens: summary.reduce((sum, item) => sum + item.total_tokens, 0),
      totalCost: summary.reduce((sum, item) => sum + item.total_cost_micro_usd, 0),
      errorCount: summary.reduce((sum, item) => sum + item.error_count, 0)
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
      </section>
      {error && <div className="alert error">{error}</div>}
      <section className="dashboard-grid">
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
