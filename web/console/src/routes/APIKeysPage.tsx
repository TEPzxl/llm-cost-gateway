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
      setError("Name, positive RPM limit, and non-negative quota limits are required.");
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
        <h2>Create API Key</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Name</span>
            <input className="input" name="name" placeholder="docs-assistant-prod" />
          </label>
          <label className="field">
            <span>RPM Limit</span>
            <input className="input" name="rpm_limit" type="number" defaultValue={60} min={1} />
          </label>
          <label className="field span-2">
            <span>Scopes</span>
            <input className="input" name="scopes" defaultValue="chat.completions" />
          </label>
          <label className="field">
            <span>Daily quota</span>
            <input className="input" name="daily_cost_limit_micro_usd" type="number" min={0} placeholder="1000000" />
          </label>
          <label className="field">
            <span>Monthly quota</span>
            <input className="input" name="monthly_cost_limit_micro_usd" type="number" min={0} placeholder="30000000" />
          </label>
          <label className="field">
            <span>Quota action</span>
            <select className="input" name="quota_action" defaultValue="block">
              <option value="block">Block</option>
              <option value="warn">Warn</option>
            </select>
          </label>
          <div className="form-actions span-2">
            <button className="button" type="submit">Create key</button>
          </div>
        </form>
        {createdKey && (
          <div className="alert">
            <strong>Plaintext key, shown once:</strong>
            <code className="secret-block">{createdKey.key}</code>
            <button className="button-secondary" type="button" onClick={() => setCreatedKey(null)}>
              Dismiss
            </button>
          </div>
        )}
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>API Keys</h2>
        <DataTable
          items={items}
          empty="No API keys yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "prefix", header: "Prefix", render: (item) => item.key_prefix },
            { key: "rpm", header: "RPM", render: (item) => item.rpm_limit },
            { key: "daily", header: "Daily", render: (item) => formatOptionalMicroUSD(item.daily_cost_limit_micro_usd) },
            { key: "monthly", header: "Monthly", render: (item) => formatOptionalMicroUSD(item.monthly_cost_limit_micro_usd) },
            { key: "quota_action", header: "Action", render: (item) => item.quota_action },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) },
            {
              key: "actions",
              header: "Actions",
              render: (item) =>
                item.status === "active" ? (
                  <button className="button-danger" type="button" onClick={() => revoke(item.id)}>
                    Revoke
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
