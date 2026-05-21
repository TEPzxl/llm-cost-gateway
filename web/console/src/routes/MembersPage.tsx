"use client";

import { useEffect, useState } from "react";
import type { AdminMe, Member } from "../api/types";
import { DataTable } from "../components/DataTable";
import { formatDate, formValue, preventDefault, type PageProps } from "./pageUtils";

type MembersPageProps = PageProps & {
  me: AdminMe | null;
};

export function MembersPage({ client, me }: MembersPageProps) {
  const [items, setItems] = useState<Member[]>([]);
  const [error, setError] = useState("");
  const canManage = me?.actor_type === "service_token" || me?.role === "owner";

  async function load() {
    const response = await client.listMembers();
    setItems(response.items);
  }

  useEffect(() => {
    load().catch((err: Error) => setError(err.message));
  }, [client]);

  async function handleCreate(event: React.FormEvent<HTMLFormElement>) {
    const form = preventDefault(event);
    const email = formValue(form, "email");
    const displayName = formValue(form, "display_name");
    const role = formValue(form, "role");
    if (!email || !["owner", "admin", "viewer"].includes(role)) {
      setError("邮箱和角色不能为空。");
      return;
    }
    setError("");
    await client.createMember({
      email,
      display_name: displayName,
      role: role as "owner" | "admin" | "viewer"
    });
    form.reset();
    await load();
  }

  return (
    <div className="page-grid">
      {canManage && (
        <section className="panel">
          <h2>新增或更新成员</h2>
          <form className="form-grid" onSubmit={handleCreate}>
            <label className="field">
              <span>邮箱</span>
              <input className="input" name="email" type="email" placeholder="member@example.com" />
            </label>
            <label className="field">
              <span>显示名称</span>
              <input className="input" name="display_name" placeholder="显示名称" />
            </label>
            <label className="field">
              <span>角色</span>
              <select className="input" name="role" defaultValue="viewer">
                <option value="owner">拥有者</option>
                <option value="admin">管理员</option>
                <option value="viewer">查看者</option>
              </select>
            </label>
            <div className="form-actions">
              <button className="button" type="submit">
                保存成员
              </button>
            </div>
          </form>
          {error && <div className="alert error">{error}</div>}
        </section>
      )}
      <section className="panel">
        <h2>成员</h2>
        {!canManage && <p className="muted">管理员可查看成员，只有拥有者可新增或更新成员。</p>}
        {error && !canManage && <div className="alert error">{error}</div>}
        <DataTable
          items={items}
          empty="暂无成员。"
          columns={[
            { key: "email", header: "邮箱", render: (item) => item.email },
            { key: "name", header: "名称", render: (item) => item.display_name },
            { key: "role", header: "角色", render: (item) => roleLabel(item.role) },
            { key: "status", header: "状态", render: (item) => memberStatusLabel(item.status) },
            { key: "created", header: "创建时间", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function roleLabel(role: string) {
  switch (role) {
    case "owner":
      return "拥有者";
    case "admin":
      return "管理员";
    case "viewer":
      return "查看者";
  default:
    return role;
  }
}

function memberStatusLabel(status: string) {
  if (status === "active") {
    return "启用";
  }
  if (status === "disabled") {
    return "停用";
  }
  return status;
}
