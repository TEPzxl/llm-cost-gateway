"use client";

import { useEffect, useState } from "react";
import type { AdminAuditLog } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, type PageProps } from "./pageUtils";

export function AuditLogsPage({ client }: PageProps) {
  const [items, setItems] = useState<AdminAuditLog[]>([]);
  const [error, setError] = useState("");

  async function load() {
    const response = await client.listAuditLogs();
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  return (
    <div className="page-grid">
      {error && <div className="alert error">{error}</div>}
      <section className="panel">
        <h2>审计日志</h2>
        <DataTable
          items={items}
          empty="暂无审计日志。"
          columns={[
            { key: "action", header: "动作", render: (item) => item.action },
            { key: "resource", header: "资源", render: (item) => resourceLabel(item) },
            { key: "actor", header: "操作令牌", render: (item) => item.actor_admin_token_id },
            { key: "request", header: "请求 ID", render: (item) => item.request_id },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function resourceLabel(item: AdminAuditLog) {
  if (!item.resource_id) {
    return item.resource_type;
  }
  return `${item.resource_type} ${item.resource_id}`;
}
