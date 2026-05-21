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
        <h2>用量汇总</h2>
        <div className="segmented">
          {([
            { key: "provider", label: "供应商" },
            { key: "model", label: "模型" },
            { key: "api_key", label: "API 密钥" }
          ] as Array<{ key: GroupBy; label: string }>).map((item) => (
            <button
              key={item.key}
              className={groupBy === item.key ? "button" : "button-secondary"}
              type="button"
              onClick={() => changeGroup(item.key)}
            >
              {item.label}
            </button>
          ))}
        </div>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <DataTable
          items={items}
          empty="当前窗口暂无用量。"
          columns={[
            { key: "group", header: "分组", render: (item) => groupLabel(item, groupBy) },
            { key: "requests", header: "请求数", render: (item) => item.request_count },
            { key: "success", header: "成功", render: (item) => item.success_count },
            { key: "errors", header: "错误", render: (item) => item.error_count },
            { key: "tokens", header: "令牌数", render: (item) => item.total_tokens },
            { key: "cost", header: "成本", render: (item) => formatMicroUSD(item.total_cost_micro_usd) },
            { key: "latency", header: "平均延迟", render: (item) => `${Math.round(item.avg_latency_ms)}ms` }
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
