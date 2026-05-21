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
      setError("Name is required.");
      return;
    }
    if (ruleType === "daily_cost" && thresholdMicro === null) {
      setError("Daily cost policies need a cost threshold.");
      return;
    }
    if (ruleType === "error_rate_spike" && thresholdBPS === null) {
      setError("Error-rate policies need a bps threshold.");
      return;
    }
    if (scopeType === "api_key" && !scopeID) {
      setError("API key scope needs an API key ID.");
      return;
    }
    if (scopeType === "model" && !modelAlias) {
      setError("Model scope needs a model alias.");
      return;
    }
    if (action === "downgrade" && !fallbackModel) {
      setError("Downgrade action needs a fallback model.");
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
        <h2>Create Anomaly Policy</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Name</span>
            <input className="input" name="name" placeholder="daily spend guard" />
          </label>
          <label className="field">
            <span>Rule</span>
            <select className="input" value={ruleType} onChange={(event) => updateRule(event.target.value as RuleType)}>
              <option value="daily_cost">Daily cost</option>
              <option value="api_key_cost_spike">API key cost spike</option>
              <option value="model_cost_spike">Model cost spike</option>
              <option value="error_rate_spike">Error-rate spike</option>
            </select>
          </label>
          <label className="field">
            <span>Scope</span>
            <select className="input" value={scopeType} onChange={(event) => setScopeType(event.target.value as ScopeType)}>
              <option value="org">Org</option>
              <option value="api_key">API key</option>
              <option value="model">Model alias</option>
            </select>
          </label>
          <label className="field">
            <span>API key ID</span>
            <input className="input" name="scope_id" disabled={scopeType !== "api_key"} />
          </label>
          <label className="field">
            <span>Model alias</span>
            <input className="input" name="model_alias" disabled={scopeType !== "model"} placeholder="fast-chat" />
          </label>
          <label className="field">
            <span>Cost threshold</span>
            <input className="input" name="threshold_micro_usd" min="0" type="number" placeholder="1000000" />
          </label>
          <label className="field">
            <span>Error threshold bps</span>
            <input className="input" name="threshold_bps" min="1" type="number" placeholder="3000" />
          </label>
          <label className="field">
            <span>Spike multiplier bps</span>
            <input className="input" name="spike_multiplier_bps" min="1" type="number" defaultValue={20000} />
          </label>
          <label className="field">
            <span>Current window</span>
            <input className="input" name="current_window_minutes" min="1" type="number" defaultValue={60} />
          </label>
          <label className="field">
            <span>Baseline window</span>
            <input className="input" name="baseline_window_minutes" min="1" type="number" defaultValue={1440} />
          </label>
          <label className="field">
            <span>Min requests</span>
            <input className="input" name="min_requests" min="1" type="number" defaultValue={1} />
          </label>
          <label className="field">
            <span>Action</span>
            <select className="input" value={action} onChange={(event) => setAction(event.target.value as ActionType)}>
              <option value="notify">Notify</option>
              <option value="downgrade">Downgrade</option>
              <option value="block">Block</option>
            </select>
          </label>
          <label className="field">
            <span>Fallback model</span>
            <input className="input" name="fallback_model" disabled={action !== "downgrade"} placeholder="cheap-chat" />
          </label>
          <div className="form-actions span-2">
            <button className="button" type="submit">
              Create policy
            </button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>

      <section className="panel">
        <h2>Anomaly Policies</h2>
        <DataTable
          items={items}
          empty="No anomaly policies yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "rule", header: "Rule", render: (item) => ruleLabel(item.rule_type) },
            { key: "scope", header: "Scope", render: (item) => scopeLabel(item) },
            { key: "threshold", header: "Threshold", render: (item) => thresholdLabel(item) },
            { key: "action", header: "Action", render: (item) => actionLabel(item) },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) }
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
  return rule.replace(/_/g, " ");
}

function scopeLabel(item: AnomalyPolicy) {
  if (item.scope_type === "api_key") {
    return item.scope_id ?? "-";
  }
  if (item.scope_type === "model") {
    return item.model_alias ?? "-";
  }
  return "org";
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
    return `downgrade -> ${item.fallback_model}`;
  }
  return item.action;
}
