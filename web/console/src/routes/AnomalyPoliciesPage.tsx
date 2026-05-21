"use client";

import { useEffect, useState } from "react";
import type { AnomalyPolicy } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formatMicroUSD, formValue, numberValue, preventDefault, type PageProps } from "./pageUtils";

type RuleType = "daily_cost" | "api_key_cost_spike" | "model_cost_spike" | "error_rate_spike";
type ScopeType = "org" | "api_key" | "model";
type ActionType = "notify" | "downgrade" | "block";

export function AnomalyPoliciesPage({ client }: PageProps) {
  const [items, setItems] = useState<AnomalyPolicy[]>([]);
  const [ruleType, setRuleType] = useState<RuleType>("daily_cost");
  const [scopeType, setScopeType] = useState<ScopeType>("org");
  const [action, setAction] = useState<ActionType>("notify");
  const [error, setError] = useState("");

  async function load() {
    const response = await client.listAnomalyPolicies();
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  function updateRule(next: RuleType) {
    setRuleType(next);
    if (next === "api_key_cost_spike") {
      setScopeType("api_key");
    } else if (next === "model_cost_spike") {
      setScopeType("model");
    } else {
      setScopeType("org");
    }
  }

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    const scopeID = formValue(form, "scope_id");
    const modelAlias = formValue(form, "model_alias");
    const fallbackModel = formValue(form, "fallback_model");
    const thresholdMicro = optionalNumberValue(form, "threshold_micro_usd");
    const thresholdBPS = optionalNumberValue(form, "threshold_bps");
    const spikeMultiplierBPS = numberValue(form, "spike_multiplier_bps", 20000);
    const currentWindowMinutes = numberValue(form, "current_window_minutes", 60);
    const baselineWindowMinutes = numberValue(form, "baseline_window_minutes", 1440);
    const minRequests = numberValue(form, "min_requests", 1);
    if (!name) {
      setError("名称不能为空。");
      return;
    }
    if (ruleType === "daily_cost" && thresholdMicro === null) {
      setError("每日成本策略需要设置成本阈值。");
      return;
    }
    if (ruleType === "error_rate_spike" && thresholdBPS === null) {
      setError("错误率策略需要设置 bps 阈值。");
      return;
    }
    if (scopeType === "api_key" && !scopeID) {
      setError("API Key 作用域需要填写 API Key ID。");
      return;
    }
    if (scopeType === "model" && !modelAlias) {
      setError("模型作用域需要模型别名。");
      return;
    }
    if (action === "downgrade" && !fallbackModel) {
      setError("降级动作需要设置回退模型。");
      return;
    }

    setError("");
    await client.createAnomalyPolicy({
      name,
      rule_type: ruleType,
      scope_type: scopeType,
      ...(scopeID ? { scope_id: scopeID } : {}),
      ...(modelAlias ? { model_alias: modelAlias } : {}),
      threshold_micro_usd: thresholdMicro,
      threshold_bps: thresholdBPS,
      spike_multiplier_bps: Math.trunc(spikeMultiplierBPS),
      current_window_minutes: Math.trunc(currentWindowMinutes),
      baseline_window_minutes: Math.trunc(baselineWindowMinutes),
      min_requests: Math.trunc(minRequests),
      action,
      ...(fallbackModel ? { fallback_model: fallbackModel } : {})
    });
    form.reset();
    updateRule("daily_cost");
    setAction("notify");
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>创建异常策略</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>名称</span>
            <input className="input" name="name" placeholder="日常支出告警策略" />
          </label>
          <label className="field">
            <span>规则</span>
            <select className="input" value={ruleType} onChange={(event) => updateRule(event.target.value as RuleType)}>
              <option value="daily_cost">每日成本</option>
              <option value="api_key_cost_spike">API 密钥成本飙升</option>
              <option value="model_cost_spike">模型成本飙升</option>
              <option value="error_rate_spike">错误率飙升</option>
            </select>
          </label>
          <label className="field">
            <span>作用域</span>
            <select className="input" value={scopeType} onChange={(event) => setScopeType(event.target.value as ScopeType)}>
              <option value="org">组织</option>
              <option value="api_key">API 密钥</option>
              <option value="model">模型别名</option>
            </select>
          </label>
          <label className="field">
            <span>API 密钥 ID</span>
            <input className="input" name="scope_id" disabled={scopeType !== "api_key"} />
          </label>
          <label className="field">
            <span>模型别名</span>
            <input className="input" name="model_alias" disabled={scopeType !== "model"} placeholder="fast-chat" />
          </label>
          <label className="field">
            <span>成本阈值</span>
            <input className="input" name="threshold_micro_usd" min="0" type="number" placeholder="1000000" />
          </label>
          <label className="field">
            <span>错误阈值（bps）</span>
            <input className="input" name="threshold_bps" min="1" type="number" placeholder="3000" />
          </label>
          <label className="field">
            <span>突增倍数（bps）</span>
            <input className="input" name="spike_multiplier_bps" min="1" type="number" defaultValue={20000} />
          </label>
          <label className="field">
            <span>当前窗口（分钟）</span>
            <input className="input" name="current_window_minutes" min="1" type="number" defaultValue={60} />
          </label>
          <label className="field">
            <span>基线窗口（分钟）</span>
            <input className="input" name="baseline_window_minutes" min="1" type="number" defaultValue={1440} />
          </label>
          <label className="field">
            <span>最小请求数</span>
            <input className="input" name="min_requests" min="1" type="number" defaultValue={1} />
          </label>
          <label className="field">
            <span>动作</span>
            <select className="input" value={action} onChange={(event) => setAction(event.target.value as ActionType)}>
              <option value="notify">通知</option>
              <option value="downgrade">降级</option>
              <option value="block">阻断</option>
            </select>
          </label>
          <label className="field">
            <span>回退模型</span>
            <input className="input" name="fallback_model" disabled={action !== "downgrade"} placeholder="cheap-chat" />
          </label>
          <div className="form-actions span-2">
            <button className="button" type="submit">
              创建策略
            </button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>

      <section className="panel">
        <h2>异常策略</h2>
        <DataTable
          items={items}
          empty="暂无异常策略。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "rule", header: "规则", render: (item) => ruleLabel(item.rule_type) },
            { key: "scope", header: "作用域", render: (item) => scopeLabel(item) },
            { key: "threshold", header: "阈值", render: (item) => thresholdLabel(item) },
            { key: "action", header: "动作", render: (item) => actionLabel(item) },
            { key: "status", header: "状态", render: (item) => anomalyPolicyStatusLabel(item.status) },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
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

function ruleLabel(rule: string) {
  if (rule === "daily_cost") {
    return "每日成本";
  }
  if (rule === "api_key_cost_spike") {
    return "API Key 成本飙升";
  }
  if (rule === "model_cost_spike") {
    return "模型成本飙升";
  }
  return "错误率飙升";
}

function scopeLabel(item: AnomalyPolicy) {
  if (item.scope_type === "api_key") {
    return item.scope_id ?? "-";
  }
  if (item.scope_type === "model") {
    return item.model_alias ?? "-";
  }
  return "组织";
}

function thresholdLabel(item: AnomalyPolicy) {
  if (item.rule_type === "daily_cost" && item.threshold_micro_usd != null) {
    return formatMicroUSD(item.threshold_micro_usd);
  }
  if (item.rule_type === "error_rate_spike" && item.threshold_bps != null) {
    return `${item.threshold_bps} bps`;
  }
  return `${item.spike_multiplier_bps} bps`;
}

function actionLabel(item: AnomalyPolicy) {
  if (item.action === "downgrade" && item.fallback_model) {
    return `降级 -> ${item.fallback_model}`;
  }
  if (item.action === "notify") {
    return "通知";
  }
  if (item.action === "block") {
    return "阻断";
  }
  return item.action;
}

function anomalyPolicyStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "disabled") {
    return "停用";
  }
  return status;
}
