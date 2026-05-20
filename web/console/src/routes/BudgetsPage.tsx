"use client";

import { useEffect, useState } from "react";
import type { Budget, BudgetStatus } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatMicroUSD, formValue, numberValue, preventDefault, type PageProps } from "./pageUtils";

export function BudgetsPage({ client }: PageProps) {
  const [budgets, setBudgets] = useState<Budget[]>([]);
  const [statuses, setStatuses] = useState<BudgetStatus[]>([]);
  const [error, setError] = useState("");

  async function load() {
    const [budgetResponse, statusResponse] = await Promise.all([
      client.listBudgets(),
      client.budgetStatus()
    ]);
    setBudgets(budgetResponse.items);
    setStatuses(statusResponse.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    const period = formValue(form, "period") as "daily" | "monthly";
    const action = formValue(form, "action") as "warn" | "block";
    const limit = numberValue(form, "limit_micro_usd", 0);
    if (!name || limit < 0) {
      setError("Name and non-negative limit are required.");
      return;
    }
    setError("");
    await client.createBudget({
      name,
      scope_type: "org",
      scope_id: null,
      period,
      action,
      limit_micro_usd: limit
    });
    form.reset();
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Create Budget</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Name</span>
            <input className="input" name="name" placeholder="monthly-budget" />
          </label>
          <label className="field">
            <span>Period</span>
            <select className="select" name="period" defaultValue="monthly">
              <option value="daily">daily</option>
              <option value="monthly">monthly</option>
            </select>
          </label>
          <label className="field">
            <span>Action</span>
            <select className="select" name="action" defaultValue="block">
              <option value="warn">warn</option>
              <option value="block">block</option>
            </select>
          </label>
          <label className="field">
            <span>Limit micro USD</span>
            <input className="input" name="limit_micro_usd" type="number" defaultValue={1000000} min={0} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">Create budget</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Budget Status</h2>
        <DataTable
          items={statuses}
          empty="No budget status yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "period", header: "Period", render: (item) => item.period },
            { key: "used", header: "Used", render: (item) => formatMicroUSD(item.used_micro_usd) },
            { key: "limit", header: "Limit", render: (item) => formatMicroUSD(item.limit_micro_usd) },
            { key: "remaining", header: "Remaining", render: (item) => formatMicroUSD(item.remaining_micro_usd) },
            { key: "action", header: "Action", render: (item) => item.action },
            { key: "exceeded", header: "Exceeded", render: (item) => (item.exceeded ? "yes" : "no") }
          ]}
        />
      </section>
      <section className="panel">
        <h2>Budgets</h2>
        <DataTable
          items={budgets}
          empty="No budgets yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "period", header: "Period", render: (item) => item.period },
            { key: "limit", header: "Limit", render: (item) => formatMicroUSD(item.limit_micro_usd) },
            { key: "action", header: "Action", render: (item) => item.action },
            { key: "status", header: "Status", render: (item) => item.status }
          ]}
        />
      </section>
    </div>
  );
}
