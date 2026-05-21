import type { ReactNode } from "react";
import type { AdminMe } from "../api/types";

export type ConsolePage =
  | "dashboard"
  | "members"
  | "api-keys"
  | "providers"
  | "models"
  | "route-policies"
  | "policies"
  | "budgets"
  | "budget-alerts"
  | "audit-logs"
  | "cache"
  | "request-logs"
  | "usage-summary";

type LayoutProps = {
  activePage: ConsolePage;
  me: AdminMe | null;
  onNavigate: (page: ConsolePage) => void;
  onLogout: () => void;
  children: ReactNode;
};

const navItems: Array<{ id: ConsolePage; label: string }> = [
  { id: "dashboard", label: "Dashboard" },
  { id: "members", label: "Members" },
  { id: "api-keys", label: "API Keys" },
  { id: "providers", label: "Providers" },
  { id: "models", label: "Models" },
  { id: "route-policies", label: "Route Policies" },
  { id: "policies", label: "Policies" },
  { id: "budgets", label: "Budgets" },
  { id: "budget-alerts", label: "Budget Alerts" },
  { id: "audit-logs", label: "Audit Logs" },
  { id: "cache", label: "Cache" },
  { id: "request-logs", label: "Request Logs" },
  { id: "usage-summary", label: "Usage Summary" }
];

export function Layout({
  activePage,
  me,
  onNavigate,
  onLogout,
  children
}: LayoutProps) {
  const visibleItems = navItems.filter((item) => canAccessPage(item.id, me));
  const activeLabel =
    navItems.find((item) => item.id === activePage)?.label ?? "Dashboard";

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span className="brand-mark">LG</span>
          <div>
            <div className="brand-title">LLM Cost Gateway</div>
            <div className="brand-subtitle">Management Console</div>
          </div>
        </div>
        <nav className="sidebar-nav" aria-label="Console navigation">
          {visibleItems.map((item) => (
            <button
              key={item.id}
              className={item.id === activePage ? "nav-item active" : "nav-item"}
              type="button"
              onClick={() => onNavigate(item.id)}
            >
              {item.label}
            </button>
          ))}
        </nav>
        <button className="logout-button" type="button" onClick={onLogout}>
          Log out
        </button>
      </aside>
      <section className="content-shell">
        <header className="content-header">
          <div>
            <p className="eyebrow">Tenant Admin</p>
            <h1>{activeLabel}</h1>
          </div>
          {me && (
            <div className="principal-chip">
              <span>{me.actor_type === "service_token" ? "Service Token" : me.user?.email}</span>
              <strong>{roleLabel(me.role)}</strong>
            </div>
          )}
        </header>
        <div className="content-body">{children}</div>
      </section>
    </main>
  );
}

export function canAccessPage(page: ConsolePage, me: AdminMe | null) {
  if (!me) {
    return true;
  }
  if (me.actor_type === "service_token") {
    return true;
  }
  if (page === "dashboard" || page === "audit-logs" || page === "cache" || page === "request-logs" || page === "usage-summary") {
    return true;
  }
  if (page === "members") {
    return me.role === "owner" || me.role === "admin";
  }
  if (page === "api-keys") {
    return me.role === "owner";
  }
  return me.role === "owner" || me.role === "admin";
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
      return role || "Owner";
  }
}
