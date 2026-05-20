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
        <h2>Filters</h2>
        <form className="form-grid" onSubmit={handleFilter}>
          <label className="field">
            <span>From</span>
            <input className="input" name="from" placeholder="2026-05-20T00:00:00Z" />
          </label>
          <label className="field">
            <span>To</span>
            <input className="input" name="to" placeholder="2026-05-21T00:00:00Z" />
          </label>
          <label className="field">
            <span>Status</span>
            <select className="select" name="status" defaultValue="">
              <option value="">Any</option>
              <option value="success">success</option>
              <option value="error">error</option>
              <option value="rate_limited">rate_limited</option>
              <option value="budget_warned">budget_warned</option>
              <option value="budget_blocked">budget_blocked</option>
            </select>
          </label>
          <label className="field">
            <span>Limit</span>
            <input className="input" name="limit" type="number" defaultValue={50} min={1} max={200} />
          </label>
          <label className="field"><span>Error Code</span><input className="input" name="error_code" /></label>
          <label className="field"><span>Request Model</span><input className="input" name="request_model" /></label>
          <label className="field"><span>API Key ID</span><input className="input" name="api_key_id" /></label>
          <label className="field"><span>Provider ID</span><input className="input" name="provider_id" /></label>
          <label className="field"><span>Model ID</span><input className="input" name="model_id" /></label>
          <div className="form-actions">
            <button className="button" type="submit">Apply filters</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Request Logs</h2>
        <DataTable
          items={items}
          empty="No request logs in this window."
          columns={[
            { key: "model", header: "Model", render: (item) => item.request_model ?? "-" },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "code", header: "Code", render: (item) => item.status_code },
            { key: "error", header: "Error", render: (item) => item.error_code ?? "-" },
            { key: "latency", header: "Latency", render: (item) => `${item.latency_ms}ms` },
            { key: "started", header: "Started", render: (item) => formatDate(item.started_at) }
          ]}
        />
        <div className="form-actions">
          <button className="button-secondary" type="button" disabled={!nextCursor} onClick={loadNextPage}>
            Next page
          </button>
        </div>
      </section>
    </div>
  );
}
