"use client";

import { useEffect, useState } from "react";
import type { Model, Provider } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formValue, numberValue, preventDefault, type PageProps } from "./pageUtils";

export function ModelsPage({ client }: PageProps) {
  const [models, setModels] = useState<Model[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [error, setError] = useState("");

  async function load() {
    const [modelResponse, providerResponse] = await Promise.all([
      client.listModels(),
      client.listProviders()
    ]);
    setModels(modelResponse.items);
    setProviders(providerResponse.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const providerID = formValue(form, "provider_id");
    const providerModelName = formValue(form, "provider_model_name");
    const displayName = formValue(form, "display_name");
    const inputPrice = numberValue(form, "input_price", 0);
    const outputPrice = numberValue(form, "output_price", 0);
    const contextWindow = numberValue(form, "context_window", 0);
    if (!providerID || !providerModelName || !displayName) {
      setError("供应商、供应商模型名和显示名不能为空。");
      return;
    }
    if (inputPrice < 0 || outputPrice < 0) {
      setError("价格必须为非负数。");
      return;
    }
    setError("");
    await client.createModel({
      provider_id: providerID,
      provider_model_name: providerModelName,
      display_name: displayName,
      input_price_micro_usd_per_1k_tokens: inputPrice,
      output_price_micro_usd_per_1k_tokens: outputPrice,
      context_window: contextWindow > 0 ? contextWindow : null
    });
    form.reset();
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>创建模型</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>供应商</span>
            <select className="select" name="provider_id" defaultValue="">
              <option value="" disabled>请选择供应商</option>
              {providers.map((provider) => (
                <option key={provider.id} value={provider.id}>{provider.name}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>供应商模型名</span>
            <input className="input" name="provider_model_name" placeholder="mock-small" />
          </label>
          <label className="field">
            <span>显示名称</span>
            <input className="input" name="display_name" placeholder="示例模型" />
          </label>
          <label className="field">
            <span>输入价格 / 1000 令牌</span>
            <input className="input" name="input_price" type="number" defaultValue={0} min={0} />
          </label>
          <label className="field">
            <span>输出价格 / 1000 令牌</span>
            <input className="input" name="output_price" type="number" defaultValue={0} min={0} />
          </label>
          <label className="field">
            <span>上下文窗口</span>
            <input className="input" name="context_window" type="number" placeholder="8192" min={1} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">创建模型</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>模型</h2>
        <DataTable
          items={models}
          empty="暂无模型。"
          columns={[
            { key: "display", header: "显示名称", render: (item) => item.display_name },
            { key: "provider_model", header: "供应商模型名", render: (item) => item.provider_model_name },
            { key: "provider", header: "供应商", render: (item) => providerName(providers, item.provider_id) },
            { key: "input", header: "输入价格", render: (item) => item.input_price_micro_usd_per_1k_tokens },
            { key: "output", header: "输出价格", render: (item) => item.output_price_micro_usd_per_1k_tokens },
            { key: "status", header: "状态", render: (item) => modelStatusLabel(item.status) }
          ]}
        />
      </section>
    </div>
  );
}

function providerName(providers: Provider[], id: string) {
  return providers.find((provider) => provider.id === id)?.name ?? id;
}

function modelStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "inactive") {
    return "停用";
  }
  return status;
}
