"use client";

import { useEffect, useMemo, useState } from "react";
import type { Model, Provider, RoutePolicy } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formValue, preventDefault, type PageProps } from "./pageUtils";

export function RoutePoliciesPage({ client }: PageProps) {
  const [policies, setPolicies] = useState<RoutePolicy[]>([]);
  const [providers, setProviders] = useState<Provider[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [selectedProviderID, setSelectedProviderID] = useState("");
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

  const filteredModels = useMemo(
    () => models.filter((model) => !selectedProviderID || model.provider_id === selectedProviderID),
    [models, selectedProviderID]
  );

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    const matchModel = formValue(form, "match_model");
    const providerID = formValue(form, "provider_id");
    const modelID = formValue(form, "model_id");
    if (!name || !matchModel || !providerID || !modelID) {
      setError("Name, match model, provider and model are required.");
      return;
    }
    setError("");
    await client.createRoutePolicy({
      name,
      match_model: matchModel,
      strategy: "single",
      targets: [{ provider_id: providerID, model_id: modelID, priority: 1, weight: 100 }]
    });
    form.reset();
    setSelectedProviderID("");
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Create Route Policy</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Name</span>
            <input className="input" name="name" placeholder="fast-chat route" />
          </label>
          <label className="field">
            <span>Match Model</span>
            <input className="input" name="match_model" placeholder="fast-chat" />
          </label>
          <label className="field">
            <span>Provider</span>
            <select
              className="select"
              name="provider_id"
              value={selectedProviderID}
              onChange={(event) => setSelectedProviderID(event.target.value)}
            >
              <option value="">Select provider</option>
              {providers.map((provider) => (
                <option key={provider.id} value={provider.id}>{provider.name}</option>
              ))}
            </select>
          </label>
          <label className="field">
            <span>Model</span>
            <select className="select" name="model_id" defaultValue="">
              <option value="" disabled>Select model</option>
              {filteredModels.map((model) => (
                <option key={model.id} value={model.id}>{model.display_name}</option>
              ))}
            </select>
          </label>
          <div className="form-actions">
            <button className="button" type="submit">Create policy</button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Route Policies</h2>
        <DataTable
          items={policies}
          empty="No route policies yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "match", header: "Match Model", render: (item) => item.match_model },
            { key: "strategy", header: "Strategy", render: (item) => item.strategy },
            { key: "status", header: "Status", render: (item) => item.status }
          ]}
        />
      </section>
    </div>
  );
}
