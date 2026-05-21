import type { ReactNode } from "react";

export type ConsolePage =
  | "dashboard"
  | "api-keys"
  | "providers"
  | "models"
  | "route-policies"
  | "budgets"
  | "budget-alerts"
  | "audit-logs"
  | "cache"
  | "request-logs"
  | "usage-summary";

type LayoutProps = {
  activePage: ConsolePage;
  onNavigate: (page: ConsolePage) => void;
  onLogout: () => void;
  children: ReactNode;
};

const navItems: Array<{ id: ConsolePage; label: string }> = [
  { id: "dashboard", label: "Dashboard" },
  { id: "api-keys", label: "API Keys" },
  { id: "providers", label: "Providers" },
  { id: "models", label: "Models" },
  { id: "route-policies", label: "Route Policies" },
  { id: "budgets", label: "Budgets" },
  { id: "budget-alerts", label: "Budget Alerts" },
  { id: "audit-logs", label: "Audit Logs" },
  { id: "cache", label: "Cache" },
  { id: "request-logs", label: "Request Logs" },
  { id: "usage-summary", label: "Usage Summary" }
];

export function Layout({
  activePage,
  onNavigate,
  onLogout,
  children
}: LayoutProps) {
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
          {navItems.map((item) => (
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
        </header>
        <div className="content-body">{children}</div>
      </section>
    </main>
  );
}
