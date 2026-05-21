"use client";

import { useEffect, useState } from "react";
import type { APIKey, APIKeyCreateResponse } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formatMicroUSD, formValue, numberValue, preventDefault, type PageProps } from "./pageUtils";

export function APIKeysPage({ client }: PageProps) {
  const [items, setItems] = useState<APIKey[]>([]);
  const [createdKey, setCreatedKey] = useState<APIKeyCreateResponse | null>(null);
  const [error, setError] = useState("");

  async function load() {
    const response = await client.listAPIKeys();
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    const rpmLimit = numberValue(form, "rpm_limit", 60);
    const scopes = formValue(form, "scopes")
      .split(",")
      .map((scope) => scope.trim())
      .filter(Boolean);
    const dailyLimit = optionalNumberValue(form, "daily_cost_limit_micro_usd");
    const monthlyLimit = optionalNumberValue(form, "monthly_cost_limit_micro_usd");
    const quotaAction = formValue(form, "quota_action") === "warn" ? "warn" : "block";
    if (!name || rpmLimit <= 0 || (dailyLimit !== null && dailyLimit < 0) || (monthlyLimit !== null && monthlyLimit < 0)) {
      setError("名称、RPM 限制（正数）和非负配额是必填项。");
      return;
    }
    setError("");
    const created = await client.createAPIKey({
      name,
      rpm_limit: rpmLimit,
      scopes: scopes.length ? scopes : ["chat.completions"],
      daily_cost_limit_micro_usd: dailyLimit,
      monthly_cost_limit_micro_usd: monthlyLimit,
      quota_action: quotaAction
    });
    setCreatedKey(created);
    form.reset();
    await load();
  }

  async function revoke(id: string) {
    await client.revokeAPIKey(id);
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>创建 API 密钥</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>名称</span>
            <input className="input" name="name" placeholder="docs-assistant-prod" />
          </label>
          <label className="field">
            <span>每分钟请求数</span>
            <input className="input" name="rpm_limit" type="number" defaultValue={60} min={1} />
          </label>
          <label className="field span-2">
            <span>作用域</span>
            <input className="input" name="scopes" defaultValue="chat.completions" />
          </label>
          <label className="field">
            <span>每日配额</span>
            <input className="input" name="daily_cost_limit_micro_usd" type="number" min={0} placeholder="1000000" />
          </label>
          <label className="field">
            <span>每月配额</span>
            <input className="input" name="monthly_cost_limit_micro_usd" type="number" min={0} placeholder="30000000" />
          </label>
          <label className="field">
            <span>超额动作</span>
            <select className="input" name="quota_action" defaultValue="block">
              <option value="block">阻断</option>
              <option value="warn">告警</option>
            </select>
          </label>
          <div className="form-actions span-2">
            <button className="button" type="submit">创建密钥</button>
          </div>
        </form>
        {createdKey && (
          <div className="alert">
            <strong>明文密钥，仅展示一次：</strong>
            <code className="secret-block">{createdKey.key}</code>
            <button className="button-secondary" type="button" onClick={() => setCreatedKey(null)}>
              关闭
            </button>
          </div>
        )}
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>API 密钥</h2>
        <DataTable
          items={items}
          empty="暂无 API 密钥。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "prefix", header: "前缀", render: (item) => item.key_prefix },
            { key: "rpm", header: "每分钟请求数", render: (item) => item.rpm_limit },
            { key: "daily", header: "每日配额", render: (item) => formatOptionalMicroUSD(item.daily_cost_limit_micro_usd) },
            { key: "monthly", header: "每月配额", render: (item) => formatOptionalMicroUSD(item.monthly_cost_limit_micro_usd) },
            { key: "quota_action", header: "超额动作", render: (item) => quotaActionLabel(item.quota_action) },
            { key: "status", header: "状态", render: (item) => apiKeyStatusLabel(item.status) },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) },
            {
              key: "actions",
              header: "操作",
              render: (item) =>
                item.status === "active" ? (
                  <button className="button-danger" type="button" onClick={() => revoke(item.id)}>
                    撤销
                  </button>
                ) : "-"
            }
          ]}
        />
      </section>
    </div>
  );
}

function optionalNumberValue(form: HTMLFormElement, name: string) {
  const raw = formValue(form, name);
  if (!raw) {
    return null;
  }
  const value = Number(raw);
  return Number.isFinite(value) ? value : null;
}

function formatOptionalMicroUSD(value?: number | null) {
  return value == null ? "-" : formatMicroUSD(value);
}

function quotaActionLabel(action: string) {
  if (action === "warn") {
    return "告警";
  }
  if (action === "block") {
    return "阻断";
  }
  return action;
}

function apiKeyStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "inactive") {
    return "停用";
  }
  if (status === "revoked") {
    return "已撤销";
  }
  return status;
}
