"use client";

import { useEffect, useState } from "react";
import type { Provider } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formValue, numberValue, preventDefault, type PageProps } from "./pageUtils";

export function ProvidersPage({ client }: PageProps) {
  const [items, setItems] = useState<Provider[]>([]);
  const [providerType, setProviderType] = useState<"mock" | "openai_compatible">("mock");
  const [checkingID, setCheckingID] = useState("");
  const [error, setError] = useState("");

  async function load() {
    const response = await client.listProviderHealth();
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    const baseUrl = formValue(form, "base_url");
    const apiKey = formValue(form, "api_key");
    const timeout = numberValue(form, "timeout_ms", 30000);
    if (!name || timeout <= 0) {
      setError("Name and timeout are required.");
      return;
    }
    if (providerType === "openai_compatible" && (!baseUrl || !apiKey)) {
      setError("Base URL and API key are required for OpenAI-compatible providers.");
      return;
    }
    setError("");
    await client.createProvider({
      name,
      type: providerType,
      base_url: baseUrl || null,
      api_key: apiKey || null,
      timeout_ms: timeout
    });
    form.reset();
    setProviderType("mock");
    await load();
  }

  async function checkHealth(id: string) {
    setCheckingID(id);
    setError("");
    try {
      await client.checkProviderHealth(id);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Health check failed.");
    } finally {
      setCheckingID("");
    }
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Create Provider</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Name</span>
            <input className="input" name="name" placeholder="mock-provider" />
          </label>
          <label className="field">
            <span>Type</span>
            <select
              className="select"
              name="type"
              value={providerType}
              onChange={(event) => setProviderType(event.target.value as "mock" | "openai_compatible")}
            >
              <option value="mock">mock</option>
              <option value="openai_compatible">openai_compatible</option>
            </select>
          </label>
          <label className="field">
            <span>Base URL</span>
            <input className="input" name="base_url" placeholder="https://api.example.com/v1" />
          </label>
          <label className="field">
            <span>API Key</span>
            <input className="input" name="api_key" type="password" autoComplete="off" />
          </label>
          <label className="field">
            <span>Timeout MS</span>
            <input className="input" name="timeout_ms" type="number" defaultValue={30000} min={1} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">Create provider</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Providers</h2>
        <DataTable
          items={items}
          empty="No providers yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "type", header: "Type", render: (item) => item.type },
            { key: "base", header: "Base URL", render: (item) => item.base_url ?? "-" },
            { key: "timeout", header: "Timeout", render: (item) => item.timeout_ms },
            { key: "status", header: "Status", render: (item) => item.status },
            {
              key: "health",
              header: "Health",
              render: (item) => (
                <span className={`status-pill ${item.last_health_status ?? "unknown"}`}>
                  {item.last_health_status ?? "unknown"}
                </span>
              )
            },
            {
              key: "checked",
              header: "Checked",
              render: (item) => formatDateTime(item.last_health_checked_at)
            },
            { key: "error", header: "Last Error", render: (item) => item.last_error_code ?? "-" },
            {
              key: "actions",
              header: "Actions",
              render: (item) => (
                <button
                  className="button-secondary"
                  type="button"
                  onClick={() => checkHealth(item.id)}
                  disabled={checkingID === item.id}
                >
                  {checkingID === item.id ? "Checking" : "Check"}
                </button>
              )
            }
          ]}
        />
      </section>
    </div>
  );
}

function formatDateTime(value?: string | null) {
  if (!value) {
    return "-";
  }
  return new Intl.DateTimeFormat(undefined, {
    dateStyle: "short",
    timeStyle: "short"
  }).format(new Date(value));
}
