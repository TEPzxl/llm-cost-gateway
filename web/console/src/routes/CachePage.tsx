"use client";

import { useEffect, useState } from "react";
import type { CacheEvent } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formValue, preventDefault, type PageProps } from "./pageUtils";

export function CachePage({ client }: PageProps) {
  const [items, setItems] = useState<CacheEvent[]>([]);
  const [error, setError] = useState("");

  async function load(query = "") {
    const response = await client.listCacheEvents(query);
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleFilter(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const params = new URLSearchParams();
    for (const name of ["from", "to", "event_type", "requested_model", "limit"]) {
      const value = formValue(form, name);
      if (value) {
        params.set(name, value);
      }
    }
    setError("");
    await load(params.toString() ? `?${params.toString()}` : "");
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
            <span>Event</span>
            <select className="select" name="event_type" defaultValue="">
              <option value="">Any</option>
              <option value="hit">hit</option>
              <option value="miss">miss</option>
              <option value="store">store</option>
              <option value="skip">skip</option>
              <option value="read_error">read_error</option>
              <option value="write_error">write_error</option>
              <option value="semantic_hit">semantic_hit</option>
              <option value="semantic_miss">semantic_miss</option>
              <option value="semantic_skip">semantic_skip</option>
              <option value="semantic_store">semantic_store</option>
              <option value="semantic_read_error">semantic_read_error</option>
              <option value="semantic_write_error">semantic_write_error</option>
            </select>
          </label>
          <label className="field">
            <span>Requested Model</span>
            <input className="input" name="requested_model" />
          </label>
          <label className="field">
            <span>Limit</span>
            <input className="input" name="limit" type="number" defaultValue={50} min={1} max={200} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">Apply filters</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Cache Events</h2>
        <DataTable
          items={items}
          empty="No cache events in this window."
          columns={[
            { key: "event", header: "Event", render: (item) => item.event_type },
            { key: "model", header: "Model", render: (item) => item.requested_model },
            { key: "reason", header: "Reason", render: (item) => item.reason ?? "-" },
            { key: "cache_key", header: "Cache Key", render: (item) => shortHash(item.cache_key_hash) },
            { key: "messages", header: "Messages", render: (item) => shortHash(item.messages_hash) },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function shortHash(value: string) {
  return value ? value.slice(0, 12) : "-";
}
