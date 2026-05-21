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
      setError("名称不能为空。");
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
        <h2>创建内容策略</h2>
        <form className="form-grid" onSubmit={handleCreate}>
          <label className="field">
            <span>名称</span>
            <input className="input" name="name" placeholder="redact-pii" />
          </label>
          <label className="field">
            <span>PII 动作</span>
            <select
              className="select"
              name="pii_action"
              value={piiAction}
              onChange={(event) => setPIIAction(event.target.value as PIIAction)}
            >
              <option value="allow">允许</option>
              <option value="redact">脱敏</option>
              <option value="block">阻断</option>
            </select>
          </label>
          <div className="form-actions">
            <button className="button" type="submit">
              创建策略
            </button>
          </div>
        </form>
        {error && <div className="alert error">{error}</div>}
      </section>
      <section className="panel">
        <h2>内容策略</h2>
        <DataTable
          items={policies}
          empty="暂无内容策略。"
          columns={[
            { key: "name", header: "名称", render: (item) => item.name },
            { key: "pii_action", header: "PII 动作", render: (item) => piiActionLabel(item.pii_action) },
            { key: "status", header: "状态", render: (item) => contentPolicyStatusLabel(item.status) },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function piiActionLabel(action: string) {
  if (action === "allow") {
    return "允许";
  }
  if (action === "redact") {
    return "脱敏";
  }
  if (action === "block") {
    return "阻断";
  }
  return action;
}

function contentPolicyStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "disabled") {
    return "停用";
  }
  return status;
}
