"use client";

import { useEffect, useState } from "react";
import type { ContentPolicy } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formValue, preventDefault, type PageProps } from "./pageUtils";

type PIIAction = "allow" | "redact" | "block";

export function PoliciesPage({ client }: PageProps) {
  const [policies, setPolicies] = useState<ContentPolicy[]>([]);
  const [piiAction, setPIIAction] = useState<PIIAction>("redact");
  const [error, setError] = useState("");

  async function load() {
    const response = await client.listContentPolicies();
    setPolicies(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const name = formValue(form, "name");
    if (!name) {
      setError("Name is required.");
      return;
    }
    setError("");
    await client.createContentPolicy({
      name,
      pii_action: piiAction
    });
    form.reset();
    setPIIAction("redact");
    await load();
  }

  return (
    <div className="page-grid">
      <section className="panel">
        <h2>Create Content Policy</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>Name</span>
            <input className="input" name="name" placeholder="redact-pii" />
          </label>
          <label className="field">
            <span>PII Action</span>
            <select
              className="select"
              name="pii_action"
              value={piiAction}
              onChange={(event) => setPIIAction(event.target.value as PIIAction)}
            >
              <option value="allow">allow</option>
              <option value="redact">redact</option>
              <option value="block">block</option>
            </select>
          </label>
          <div className="form-actions">
            <button className="button" type="submit">
              Create policy
            </button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>Content Policies</h2>
        <DataTable
          items={policies}
          empty="No content policies yet."
          columns={[
            { key: "name", header: "Name", render: (item) => item.name },
            { key: "pii_action", header: "PII Action", render: (item) => item.pii_action },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}
