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
      setError("Provider, provider model name and display name are required.");
      return;
    }
    if (inputPrice < 0 || outputPrice < 0) {
      setError("Prices must be non-negative.");
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
        <h2>Create Model</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Provider</span>
            <select className="select" name="provider_id" defaultValue="">
              <option value="" disabled>Select provider</option>
              {providers.map((provider) => (
                <option key={provider.id} value={provider.id}>{provider.name}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>Provider Model Name</span>
            <input className="input" name="provider_model_name" placeholder="mock-small" />
          </label>
          <label className="field">
            <span>Display Name</span>
            <input className="input" name="display_name" placeholder="Mock Small" />
          </label>
          <label className="field">
            <span>Input price / 1K tokens</span>
            <input className="input" name="input_price" type="number" defaultValue={0} min={0} />
          </label>
          <label className="field">
            <span>Output price / 1K tokens</span>
            <input className="input" name="output_price" type="number" defaultValue={0} min={0} />
          </label>
          <label className="field">
            <span>Context Window</span>
            <input className="input" name="context_window" type="number" placeholder="8192" min={1} />
          </label>
          <div className="form-actions">
            <button className="button" type="submit">Create model</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Models</h2>
        <DataTable
          items={models}
          empty="No models yet."
          columns={[
            { key: "display", header: "Display", render: (item) => item.display_name },
            { key: "provider_model", header: "Provider Model", render: (item) => item.provider_model_name },
            { key: "provider", header: "Provider", render: (item) => providerName(providers, item.provider_id) },
            { key: "input", header: "Input Price", render: (item) => item.input_price_micro_usd_per_1k_tokens },
            { key: "output", header: "Output Price", render: (item) => item.output_price_micro_usd_per_1k_tokens },
            { key: "status", header: "Status", render: (item) => item.status }
          ]}
        />
      </section>
    </div>
  );
}

function providerName(providers: Provider[], id: string) {
  return providers.find((provider) => provider.id === id)?.name ?? id;
}
