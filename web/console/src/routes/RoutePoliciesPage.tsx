"use client";

import { useEffect, useMemo, useState } from "react";
import type { Model, Provider, RoutePolicy } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formValue, preventDefault, type PageProps } from "./pageUtils";

type RouteStrategy = "single" | "fallback" | "lowest_cost" | "lowest_latency";

type TargetForm = {
  providerID: string;
  modelID: string;
  weight: number;
};

const emptyTarget = (): TargetForm => ({
  providerID: "",
  modelID: "",
  weight: 100
});

export function RoutePoliciesPage({ client }: PageProps) {
  const [policies, setPolicies] = useState<RoutePolicy[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [strategy, setStrategy] = useState<RouteStrategy>("single");
  const [targets, setTargets] = useState<TargetForm[]>([emptyTarget()]);
  const [maxEstimatedCost, setMaxEstimatedCost] = useState("");
  const [fallbackToPriority, setFallbackToPriority] = useState(true);
  const [latencyWindowMinutes, setLatencyWindowMinutes] = useState("15");
  const [error, setError] = useState("");

  async function load() {
    const [policyResponse, providerResponse, modelResponse] = await Promise.all([
      client.listRoutePolicies(),
      client.listProviders(),
      client.listModels()
    ]);
    setPolicies(policyResponse.items);
    setProviders(providerResponse.items);
    setModels(modelResponse.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  const modelsByProvider = useMemo(() => {
    const grouped = new Map<string, Model[]>();
    for (const model of models) {
      const current = grouped.get(model.provider_id) ?? [];
      current.push(model);
      grouped.set(model.provider_id, current);
    }
    return grouped;
  }, [models]);

  function updateStrategy(nextStrategy: RouteStrategy) {
    setStrategy(nextStrategy);
    setTargets((current) => {
      if (nextStrategy === "single") {
        return [current[0] ?? emptyTarget()];
      }
      if (current.length >= 2) {
        return current;
      }
      return [...current, emptyTarget()];
    });
  }

  function updateTarget(index: number, patch: Partial<TargetForm>) {
    setTargets((current) =>
      current.map((target, currentIndex) =>
        currentIndex === index ? { ...target, ...patch } : target
      )
    );
  }

  function addTarget() {
    setTargets((current) => [...current, emptyTarget()]);
  }

  function removeTarget(index: number) {
    setTargets((current) => {
      const minimum = strategy === "fallback" ? 2 : 1;
      if (current.length <= minimum) {
        return current;
      }
      return current.filter((_, currentIndex) => currentIndex !== index);
    });
  }

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    const matchModel = formValue(form, "match_model");
    const activeTargets = strategy === "single" ? targets.slice(0, 1) : targets;
    const minimumTargets = strategy === "fallback" ? 2 : 1;
    if (!name || !matchModel) {
      setError("名称和匹配模型不能为空。");
      return;
    }
    if (activeTargets.length < minimumTargets) {
      setError("当前策略需要更多目标。");
      return;
    }
    if (activeTargets.some((target) => !target.providerID || !target.modelID)) {
      setError("每个目标都需要供应商和模型。");
      return;
    }
    const parsedMaxCost = maxEstimatedCost === "" ? undefined : Number(maxEstimatedCost);
    if (parsedMaxCost !== undefined && (!Number.isFinite(parsedMaxCost) || parsedMaxCost < 0)) {
      setError("最大预估成本不能小于 0。");
      return;
    }
    const parsedLatencyWindow =
      latencyWindowMinutes === "" ? undefined : Number(latencyWindowMinutes);
    if (
      strategy === "lowest_latency" &&
      (parsedLatencyWindow === undefined ||
        !Number.isFinite(parsedLatencyWindow) ||
        parsedLatencyWindow <= 0)
    ) {
      setError("延迟窗口必须大于 0。");
      return;
    }
    setError("");
    await client.createRoutePolicy({
      name,
      match_model: matchModel,
      strategy,
      ...(strategy === "lowest_cost"
        ? {
            config: {
              ...(parsedMaxCost !== undefined
                ? { max_estimated_cost_micro_usd: Math.trunc(parsedMaxCost) }
                : {}),
              fallback_to_priority: fallbackToPriority
            }
          }
        : {}),
      ...(strategy === "lowest_latency"
        ? {
            config: {
              latency_window_minutes: Math.trunc(parsedLatencyWindow ?? 15)
            }
          }
        : {}),
      targets: activeTargets.map((target, index) => ({
        provider_id: target.providerID,
        model_id: target.modelID,
        priority: index + 1,
        weight: target.weight
      }))
    });
    form.reset();
    setStrategy("single");
    setTargets([emptyTarget()]);
    setMaxEstimatedCost("");
    setFallbackToPriority(true);
    setLatencyWindowMinutes("15");
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>创建路由策略</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>名称</span>
            <input className="input" name="name" placeholder="fast-chat route" />
          </label>
          <label className="field">
            <span>匹配模型</span>
            <input className="input" name="match_model" placeholder="fast-chat" />
          </label>
          <div className="field span-2">
            <span>策略</span>
            <div className="segmented" role="group" aria-label="路由策略">
              {(["single", "fallback", "lowest_cost", "lowest_latency"] as RouteStrategy[]).map((item) => (
                <button
                  key={item}
                  className={strategy === item ? "button-secondary active" : "button-secondary"}
                  type="button"
                  onClick={() => updateStrategy(item)}
                >
                  {strategyLabel(item)}
                </button>
              ))}
            </div>
          </div>
          {strategy === "lowest_cost" && (
            <div className="field span-2">
              <span>成本控制</span>
              <div className="target-row">
                <label className="target-field">
                  <span>最大预估成本</span>
                  <input
                    className="input"
                    min="0"
                    name="max_estimated_cost_micro_usd"
                    placeholder="50000"
                    step="1"
                    type="number"
                    value={maxEstimatedCost}
                    onChange={(event) => setMaxEstimatedCost(event.target.value)}
                  />
                </label>
                <label className="target-field">
                    <span>优先级回退</span>
                  <select
                    className="select"
                    value={fallbackToPriority ? "true" : "false"}
                    onChange={(event) => setFallbackToPriority(event.target.value === "true")}
                  >
                    <option value="true">启用</option>
                    <option value="false">禁用</option>
                  </select>
                </label>
              </div>
            </div>
          )}
          {strategy === "lowest_latency" && (
            <div className="field span-2">
              <span>延迟控制</span>
              <div className="target-row">
                <label className="target-field">
                  <span>统计窗口（分钟）</span>
                  <input
                    className="input"
                    min="1"
                    name="latency_window_minutes"
                    placeholder="15"
                    step="1"
                    type="number"
                    value={latencyWindowMinutes}
                    onChange={(event) => setLatencyWindowMinutes(event.target.value)}
                  />
                </label>
              </div>
            </div>
          )}
          <div className="field span-2">
            <span>目标</span>
            <div className="target-list">
              {targets.map((target, index) => {
                const selectableModels = target.providerID
                  ? (modelsByProvider.get(target.providerID) ?? [])
                  : [];
                return (
                  <div className="target-row" key={index}>
                    <div className="target-index">{index + 1}</div>
                    <label className="target-field">
                      <span>供应商</span>
                      <select
                        className="select"
                        value={target.providerID}
                        onChange={(event) =>
                          updateTarget(index, { providerID: event.target.value, modelID: "" })
                        }
                      >
                        <option value="">请选择供应商</option>
                        {providers.map((provider) => (
                          <option key={provider.id} value={provider.id}>{provider.name}</option>
                        ))}
                      </select>
                    </label>
                    <label className="target-field">
                      <span>模型</span>
                      <select
                        className="select"
                        value={target.modelID}
                        onChange={(event) => updateTarget(index, { modelID: event.target.value })}
                        disabled={!target.providerID}
                      >
                        <option value="">请选择模型</option>
                        {selectableModels.map((model) => (
                          <option key={model.id} value={model.id}>{model.display_name}</option>
                        ))}
                      </select>
                    </label>
                    <label className="target-field target-weight">
                      <span>权重</span>
                      <input
                        className="input"
                        min="1"
                        step="1"
                        type="number"
                        value={target.weight}
                        onChange={(event) =>
                          updateTarget(index, { weight: Number(event.target.value) || 1 })
                        }
                      />
                    </label>
                    <button
                      className="button-secondary target-remove"
                      type="button"
                      onClick={() => removeTarget(index)}
                      disabled={targets.length <= (strategy === "fallback" ? 2 : 1)}
                    >
                      移除
                    </button>
                  </div>
                );
              })}
            </div>
            {strategy !== "single" && (
              <div className="target-actions">
                <button className="button-secondary" type="button" onClick={addTarget}>
                  添加目标
                </button>
              </div>
            )}
          </div>
          <div className="form-actions">
            <button className="button" type="submit">创建策略</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>路由策略</h2>
        <DataTable
          items={policies}
          empty="暂无路由策略。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "match", header: "匹配模型", render: (item) => item.match_model },
            { key: "strategy", header: "策略", render: (item) => strategyLabel(item.strategy) },
            {
              key: "config",
              header: "配置",
              render: (item) => routePolicyConfigLabel(item)
            },
            { key: "status", header: "状态", render: (item) => routePolicyStatusLabel(item.status) }
          ]}
        />
      </section>
    </div>
  );
}

function routePolicyConfigLabel(item: RoutePolicy) {
  if (item.strategy === "lowest_latency") {
    return `${item.config?.latency_window_minutes ?? 15} 分钟窗口`;
  }
  if (item.strategy === "lowest_cost") {
    const maxCost = item.config?.max_estimated_cost_micro_usd;
    const fallback = item.config?.fallback_to_priority ?? true;
    return `${maxCost === undefined ? "无上限" : `${maxCost} micro USD`} / ${fallback ? "回退" : "严格"}`;
  }
  return "-";
}

function routePolicyStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "inactive") {
    return "停用";
  }
  return status;
}

function strategyLabel(strategy: string) {
  if (strategy === "single") {
    return "单一";
  }
  if (strategy === "fallback") {
    return "回退";
  }
  if (strategy === "lowest_cost") {
    return "最低成本";
  }
  return "最低延迟";
}
