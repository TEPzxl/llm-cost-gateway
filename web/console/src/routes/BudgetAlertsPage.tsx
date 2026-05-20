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
      setError("Budget and webhook URL are required.");
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
        <h2>Create Budget Alert</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Budget</span>
            <select className="select" name="budget_id" defaultValue="">
              <option value="" disabled>Select budget</option>
              {budgets.map((budget) => (
                <option key={budget.id} value={budget.id}>{budget.name}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>Status</span>
            <select className="select" name="status" defaultValue="active">
              <option value="active">active</option>
              <option value="disabled">disabled</option>
            </select>
          </label>
          <label className="field span-2">
            <span>Webhook URL</span>
            <input className="input" name="webhook_url" placeholder="https://example.com/budget-alert" />
          </label>
          <label className="field span-2">
            <span>Webhook Secret</span>
            <input className="input" name="webhook_secret" type="password" autoComplete="off" />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">Create alert</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>

      <section className="panel">
        <h2>Budget Alerts</h2>
        <DataTable
          items={alerts}
          empty="No budget alerts yet."
          columns={[
            { key: "budget", header: "Budget", render: (item) => budgetNames.get(item.budget_id) ?? item.budget_id },
            { key: "url", header: "Webhook URL", render: (item) => item.webhook_url },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>

      <section className="panel">
        <h2>Deliveries</h2>
        <DataTable
          items={deliveries}
          empty="No alert deliveries yet."
          columns={[
            { key: "budget", header: "Budget", render: (item) => budgetNames.get(item.budget_id) ?? item.budget_id },
            { key: "threshold", header: "Threshold", render: (item) => `${item.threshold}%` },
            { key: "used", header: "Used", render: (item) => formatMicroUSD(item.used_micro_usd) },
            { key: "limit", header: "Limit", render: (item) => formatMicroUSD(item.limit_micro_usd) },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "http", header: "HTTP", render: (item) => item.http_status ?? "-" },
            { key: "error", header: "Error", render: (item) => item.error_message ?? "-" },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}
