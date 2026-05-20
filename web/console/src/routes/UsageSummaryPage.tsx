"use client";

import { useEffect, useState } from "react";
import type { UsageSummaryItem, UsageSummaryResponse } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatMicroUSD, type PageProps } from "./pageUtils";

type GroupBy = UsageSummaryResponse["group_by"];

export function UsageSummaryPage({ client }: PageProps) {
  const [groupBy, setGroupBy] = useState<GroupBy>("model");
  const [items, setItems] = useState<UsageSummaryItem[]>([]);
  const [error, setError] = useState("");

  async function load(nextGroupBy = groupBy) {
    const response = await client.usageSummary(`?group_by=${nextGroupBy}`);
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client, groupBy]);

  function changeGroup(next: GroupBy) {
    setGroupBy(next);
    setError("");
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Usage Summary</h2>
        <div className="segmented">
          {(["provider", "model", "api_key"] as GroupBy[]).map((item) => (
            <button
              key={item}
              className={groupBy === item ? "button" : "button-secondary"}
              type="button"
              onClick={() => changeGroup(item)}
            >
              {item}
            </button>
          ))}
        </div>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <DataTable
          items={items}
          empty="No usage in the selected window."
          columns={[
            { key: "group", header: "Group", render: (item) => groupLabel(item, groupBy) },
            { key: "requests", header: "Requests", render: (item) => item.request_count },
            { key: "success", header: "Success", render: (item) => item.success_count },
            { key: "errors", header: "Errors", render: (item) => item.error_count },
            { key: "tokens", header: "Tokens", render: (item) => item.total_tokens },
            { key: "cost", header: "Cost", render: (item) => formatMicroUSD(item.total_cost_micro_usd) },
            { key: "latency", header: "Avg latency", render: (item) => `${Math.round(item.avg_latency_ms)}ms` }
          ]}
        />
      </section>
    </div>
  );
}

export function groupLabel(item: UsageSummaryItem, groupBy: GroupBy) {
  switch (groupBy) {
    case "provider":
      return item.provider_name ?? item.provider_id ?? "-";
    case "api_key":
      return item.api_key_name ?? item.api_key_id ?? "-";
    default:
      return item.model_name ?? item.model_id ?? "-";
  }
}
