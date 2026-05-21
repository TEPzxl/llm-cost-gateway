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
            <span>事件</span>
            <select className="select" name="event_type" defaultValue="">
              <option value="">全部</option>
              <option value="hit">命中</option>
              <option value="miss">未命中</option>
              <option value="store">存储</option>
              <option value="skip">跳过</option>
              <option value="read_error">读取失败</option>
              <option value="write_error">写入失败</option>
              <option value="semantic_hit">语义命中</option>
              <option value="semantic_miss">语义未命中</option>
              <option value="semantic_skip">语义跳过</option>
              <option value="semantic_store">语义存储</option>
              <option value="semantic_read_error">语义读取失败</option>
              <option value="semantic_write_error">语义写入失败</option>
            </select>
          </label>
          <label className="field">
            <span>请求模型</span>
            <input className="input" name="requested_model" />
          </label>
          <label className="field">
            <span>数量</span>
            <input className="input" name="limit" type="number" defaultValue={50} min={1} max={200} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">应用筛选</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>缓存事件</h2>
        <DataTable
          items={items}
          empty="当前窗口暂无缓存事件。"
          columns={[
            { key: "event", header: "事件", render: (item) => cacheEventLabel(item.event_type) },
            { key: "model", header: "模型", render: (item) => item.requested_model },
            { key: "reason", header: "原因", render: (item) => item.reason ?? "-" },
            { key: "cache_key", header: "缓存键", render: (item) => shortHash(item.cache_key_hash) },
            { key: "messages", header: "消息数", render: (item) => shortHash(item.messages_hash) },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function shortHash(value: string) {
  return value ? value.slice(0, 12) : "-";
}

function cacheEventLabel(eventType: string) {
  if (eventType === "hit") {
    return "命中";
  }
  if (eventType === "miss") {
    return "未命中";
  }
  if (eventType === "store") {
    return "存储";
  }
  if (eventType === "skip") {
    return "跳过";
  }
  if (eventType === "read_error") {
    return "读取失败";
  }
  if (eventType === "write_error") {
    return "写入失败";
  }
  if (eventType === "semantic_hit") {
    return "语义命中";
  }
  if (eventType === "semantic_miss") {
    return "语义未命中";
  }
  if (eventType === "semantic_skip") {
    return "语义跳过";
  }
  if (eventType === "semantic_store") {
    return "语义存储";
  }
  if (eventType === "semantic_read_error") {
    return "语义读取失败";
  }
  if (eventType === "semantic_write_error") {
    return "语义写入失败";
  }
  return eventType;
}
