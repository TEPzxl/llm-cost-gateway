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
      setError("Email and role are required.");
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
          <h2>Add or update member</h2>
          <form className="form-grid" onSubmit={handleCreate}>
            <label className="field">
              <span>Email</span>
              <input className="input" name="email" type="email" placeholder="member@example.com" />
            </label>
            <label className="field">
              <span>Display name</span>
              <input className="input" name="display_name" placeholder="Member name" />
            </label>
            <label className="field">
              <span>Role</span>
              <select className="input" name="role" defaultValue="viewer">
                <option value="owner">Owner</option>
                <option value="admin">Admin</option>
                <option value="viewer">Viewer</option>
              </select>
            </label>
            <div className="form-actions">
              <button className="button" type="submit">
                Save member
              </button>
            </div>
          </form>
          {error && <div className="alert error">{error}</div>}
        </section>
      )}
      <section className="panel">
        <h2>Members</h2>
        {!canManage && <p className="muted">Admins can view members. Only owners can add or update them.</p>}
        {error && !canManage && <div className="alert error">{error}</div>}
        <DataTable
          items={items}
          empty="No members yet."
          columns={[
            { key: "email", header: "Email", render: (item) => item.email },
            { key: "name", header: "Name", render: (item) => item.display_name },
            { key: "role", header: "Role", render: (item) => roleLabel(item.role) },
            { key: "status", header: "Status", render: (item) => item.status },
            { key: "created", header: "Created", render: (item) => formatDate(item.created_at) }
          ]}
        />
      </section>
    </div>
  );
}

function roleLabel(role: string) {
  switch (role) {
    case "owner":
      return "Owner";
    case "admin":
      return "Admin";
    case "viewer":
      return "Viewer";
    default:
      return role;
  }
}
