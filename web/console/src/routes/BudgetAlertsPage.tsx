"use client";

import { useEffect, useMemo, useState } from "react";
import type { Budget, BudgetAlert, BudgetAlertDelivery } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formatMicroUSD, formValue, preventDefault, type PageProps } from "./pageUtils";

export function BudgetAlertsPage({ client }: PageProps) {
  const [budgets, setBudgets] = useState<Budget[]>([]);
  const [alerts, setAlerts] = useState<BudgetAlert[]>([]);
  const [deliveries, setDeliveries] = useState<BudgetAlertDelivery[]>([]);
  const [error, setError] = useState("");

  const budgetNames = useMemo(() => {
    const names = new Map<string, string>();
    for (const budget of budgets) {
      names.set(budget.id, budget.name);
    }
    return names;
  }, [budgets]);

  async function load() {
    const [budgetResponse, alertResponse, deliveryResponse] = await Promise.all([
      client.listBudgets(),
      client.listBudgetAlerts(),
      client.listBudgetAlertDeliveries()
    ]);
    setBudgets(budgetResponse.items);
    setAlerts(alertResponse.items);
    setDeliveries(deliveryResponse.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const budgetID = formValue(form, "budget_id");
    const webhookURL = formValue(form, "webhook_url");
    const webhookSecret = formValue(form, "webhook_secret");
    const status = formValue(form, "status") as "active" | "disabled";
    if (!budgetID || !webhookURL) {
      setError("预算和 Webhook 地址不能为空。");
      return;
    }
    setError("");
    await client.createBudgetAlert({
      budget_id: budgetID,
      webhook_url: webhookURL,
      webhook_secret: webhookSecret || undefined,
      status
    });
    form.reset();
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>创建预算告警</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>预算</span>
            <select className="select" name="budget_id" defaultValue="">
              <option value="" disabled>请选择预算</option>
              {budgets.map((budget) => (
                <option key={budget.id} value={budget.id}>{budget.name}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>状态</span>
            <select className="select" name="status" defaultValue="active">
              <option value="active">启用</option>
              <option value="disabled">停用</option>
            </select>
          </label>
          <label className="field span-2">
            <span>Webhook 地址</span>
            <input className="input" name="webhook_url" placeholder="https://example.com/budget-alert" />
          </label>
          <label className="field span-2">
            <span>Webhook 密钥</span>
            <input className="input" name="webhook_secret" type="password" autoComplete="off" />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">创建告警</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>

      <section className="panel">
        <h2>预算告警</h2>
        <DataTable
          items={alerts}
          empty="暂无预算告警。"
          columns={[
            { key: "budget", header: "预算", render: (item) => budgetNames.get(item.budget_id) ?? item.budget_id },
            { key: "url", header: "Webhook 地址", render: (item) => item.webhook_url },
            { key: "status", header: "状态", render: (item) => budgetAlertStatusLabel(item.status) },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>

      <section className="panel">
        <h2>告警发送记录</h2>
        <DataTable
          items={deliveries}
          empty="暂无告警发送记录。"
          columns={[
            { key: "budget", header: "预算", render: (item) => budgetNames.get(item.budget_id) ?? item.budget_id },
            { key: "threshold", header: "阈值", render: (item) => `${item.threshold}%` },
            { key: "used", header: "已用", render: (item) => formatMicroUSD(item.used_micro_usd) },
            { key: "limit", header: "限额", render: (item) => formatMicroUSD(item.limit_micro_usd) },
            { key: "status", header: "状态", render: (item) => budgetAlertDeliveryStatusLabel(item.status) },
            { key: "http", header: "HTTP", render: (item) => item.http_status ?? "-" },
            { key: "error", header: "错误", render: (item) => item.error_message ?? "-" },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function budgetAlertStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "disabled") {
    return "停用";
  }
  return status;
}

function budgetAlertDeliveryStatusLabel(status: string) {
  if (status === "success") {
    return "成功";
  }
  if (status === "failed") {
    return "失败";
  }
  return status;
}
