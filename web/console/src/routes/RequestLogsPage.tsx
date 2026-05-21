"use client";

import { useEffect, useState } from "react";
import type { RequestLog } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formValue, preventDefault, type PageProps } from "./pageUtils";

export function RequestLogsPage({ client }: PageProps) {
  const [items, setItems] = useState<RequestLog[]>([]);
  const [currentQuery, setCurrentQuery] = useState("");
  const [nextCursor, setNextCursor] = useState<string | null>(null);
  const [error, setError] = useState("");

  async function load(query = "") {
    const response = await client.listRequestLogs(query);
    setItems(response.items);
    setNextCursor(response.next_cursor);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleFilter(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const params = new URLSearchParams();
    for (const name of ["from", "to", "status", "error_code", "request_model", "api_key_id", "provider_id", "model_id", "limit"]) {
      const value = formValue(form, name);
      if (value) {
        params.set(name, value);
      }
    }
    const query = params.toString();
    setCurrentQuery(query);
    setError("");
    await load(query ? `?${query}` : "");
  }

  async function loadNextPage() {
    if (!nextCursor) {
      return;
    }
    const params = new URLSearchParams(currentQuery);
    params.set("cursor", nextCursor);
    setError("");
    await load(`?${params.toString()}`);
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>筛选</h2>
        <form className="form-grid" onSubmit={handleFilter}>
          <label className="field">
            <span>开始时间</span>
            <input className="input" name="from" placeholder="2026-05-20T00:00:00Z" />
          </label>
          <label className="field">
            <span>结束时间</span>
            <input className="input" name="to" placeholder="2026-05-21T00:00:00Z" />
          </label>
          <label className="field">
            <span>状态</span>
            <select className="select" name="status" defaultValue="">
              <option value="">全部</option>
              <option value="success">成功</option>
              <option value="error">错误</option>
              <option value="rate_limited">速率受限</option>
              <option value="budget_warned">预算告警</option>
              <option value="budget_blocked">预算阻断</option>
            </select>
          </label>
          <label className="field">
            <span>数量</span>
            <input className="input" name="limit" type="number" defaultValue={50} min={1} max={200} />
          </label>
          <label className="field"><span>错误码</span><input className="input" name="error_code" /></label>
          <label className="field"><span>请求模型</span><input className="input" name="request_model" /></label>
          <label className="field"><span>API 密钥 ID</span><input className="input" name="api_key_id" /></label>
          <label className="field"><span>供应商 ID</span><input className="input" name="provider_id" /></label>
          <label className="field"><span>模型 ID</span><input className="input" name="model_id" /></label>
          <div className="form-actions">
            <button className="button" type="submit">应用筛选</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>请求日志</h2>
        <DataTable
          items={items}
          empty="当前窗口暂无请求日志。"
          columns={[
            { key: "model", header: "模型", render: (item) => item.request_model ?? "-" },
            { key: "status", header: "状态", render: (item) => requestLogStatusLabel(item.status) },
            { key: "code", header: "状态码", render: (item) => item.status_code },
            { key: "error", header: "错误", render: (item) => item.error_code ?? "-" },
            { key: "latency", header: "延迟", render: (item) => `${item.latency_ms}ms` },
            { key: "started", header: "开始时间", render: (item) => formatDate(item.started_at) }
          ]}
        />
        <div className="form-actions">
          <button className="button-secondary" type="button" disabled={!nextCursor} onClick={loadNextPage}>
            下一页
          </button>
        </div>
      </section>
    </div>
  );
}

function requestLogStatusLabel(status: string) {
  if (status === "success") {
    return "成功";
  }
  if (status === "error") {
    return "错误";
  }
  if (status === "rate_limited") {
    return "速率受限";
  }
  if (status === "budget_warned") {
    return "预算告警";
  }
  if (status === "budget_blocked") {
    return "预算阻断";
  }
  return status;
}
