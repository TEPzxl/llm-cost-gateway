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
      setError("名称和超时时间不能为空。");
      return;
    }
    if (providerType === "openai_compatible" && (!baseUrl || !apiKey)) {
      setError("OpenAI 兼容供应商需要填写基础地址和 API 密钥。");
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
      setError(err instanceof Error ? err.message : "健康检查失败。");
    } finally {
      setCheckingID("");
    }
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>新增供应商</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>名称</span>
            <input className="input" name="name" placeholder="mock-provider" />
          </label>
          <label className="field">
            <span>类型</span>
            <select
              className="select"
              name="type"
              value={providerType}
              onChange={(event) => setProviderType(event.target.value as "mock" | "openai_compatible")}
            >
              <option value="mock">模拟</option>
              <option value="openai_compatible">OpenAI 兼容</option>
            </select>
          </label>
          <label className="field">
            <span>基础地址</span>
            <input className="input" name="base_url" placeholder="https://api.example.com/v1" />
          </label>
          <label className="field">
            <span>API 密钥</span>
            <input className="input" name="api_key" type="password" autoComplete="off" />
          </label>
          <label className="field">
            <span>超时（毫秒）</span>
            <input className="input" name="timeout_ms" type="number" defaultValue={30000} min={1} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">新增供应商</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>供应商</h2>
        <DataTable
          items={items}
          empty="暂无供应商。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "type", header: "类型", render: (item) => providerTypeLabel(item.type) },
            { key: "base", header: "基础地址", render: (item) => item.base_url ?? "-" },
            { key: "timeout", header: "超时", render: (item) => item.timeout_ms },
            { key: "status", header: "状态", render: (item) => providerStatusLabel(item.status) },
            {
              key: "health",
              header: "健康",
              render: (item) => (
                <span className={`status-pill ${providerHealthClass(item.last_health_status)}`}>
                  {providerHealthStatusLabel(item.last_health_status)}
                </span>
              )
            },
            {
              key: "checked",
              header: "检测时间",
              render: (item) => formatDateTime(item.last_health_checked_at)
            },
            { key: "error", header: "最近错误", render: (item) => item.last_error_code ?? "-" },
            {
              key: "actions",
              header: "操作",
              render: (item) => (
                <button
                  className="button-secondary"
                  type="button"
                  onClick={() => checkHealth(item.id)}
                  disabled={checkingID === item.id}
                >
                  {checkingID === item.id ? "检查中" : "检查"}
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

function providerTypeLabel(type: string) {
  if (type === "mock") {
    return "Mock";
  }
  if (type === "openai_compatible") {
    return "OpenAI 兼容";
  }
  return type;
}

function providerStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "inactive") {
    return "停用";
  }
  return status;
}

function providerHealthStatusLabel(status?: string | null) {
  if (status === "healthy") {
    return "健康";
  }
  if (status === "unhealthy") {
    return "异常";
  }
  return "未知";
}

function providerHealthClass(status?: string | null) {
  if (status === "healthy" || status === "unhealthy") {
    return status;
  }
  return "unknown";
}
