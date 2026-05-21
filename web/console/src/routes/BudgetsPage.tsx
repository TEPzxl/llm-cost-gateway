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
      setError("名称和非负金额上限不能为空。");
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
        <h2>创建预算</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>名称</span>
            <input className="input" name="name" placeholder="本月预算" />
          </label>
          <label className="field">
            <span>周期</span>
            <select className="select" name="period" defaultValue="monthly">
              <option value="daily">每日</option>
              <option value="monthly">每月</option>
            </select>
          </label>
          <label className="field">
            <span>超额动作</span>
            <select className="select" name="action" defaultValue="block">
              <option value="warn">告警</option>
              <option value="block">阻断</option>
            </select>
          </label>
          <label className="field">
            <span>限额（micro USD）</span>
            <input className="input" name="limit_micro_usd" type="number" defaultValue={1000000} min={0} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">创建预算</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>预算状态</h2>
        <DataTable
          items={statuses}
          empty="暂无预算状态。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "period", header: "周期", render: (item) => budgetPeriodLabel(item.period) },
            { key: "used", header: "已用", render: (item) => formatMicroUSD(item.used_micro_usd) },
            { key: "limit", header: "限额", render: (item) => formatMicroUSD(item.limit_micro_usd) },
            { key: "remaining", header: "剩余额度", render: (item) => formatMicroUSD(item.remaining_micro_usd) },
            { key: "action", header: "动作", render: (item) => budgetActionLabel(item.action) },
            { key: "exceeded", header: "已超出", render: (item) => (item.exceeded ? "是" : "否") }
          ]}
        />
      </section>
      <section className="panel">
        <h2>预算</h2>
        <DataTable
          items={budgets}
          empty="暂无预算。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "period", header: "周期", render: (item) => budgetPeriodLabel(item.period) },
            { key: "limit", header: "限额", render: (item) => formatMicroUSD(item.limit_micro_usd) },
            { key: "action", header: "动作", render: (item) => budgetActionLabel(item.action) },
            { key: "status", header: "状态", render: (item) => budgetStatusLabel(item.status) }
          ]}
        />
      </section>
    </div>
  );
}

function budgetStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "disabled") {
    return "停用";
  }
  return status;
}

function budgetPeriodLabel(period: string) {
  if (period === "daily") {
    return "每日";
  }
  if (period === "monthly") {
    return "每月";
  }
  return period;
}

function budgetActionLabel(action: string) {
  if (action === "warn") {
    return "告警";
  }
  if (action === "block") {
    return "阻断";
  }
  return action;
}
