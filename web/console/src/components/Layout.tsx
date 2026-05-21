import type { ReactNode } from "react";
import type { AdminMe } from "../api/types";

export type ConsolePage =
  | "dashboard"
  | "members"
  | "api-keys"
  | "providers"
  | "models"
  | "route-policies"
  | "anomaly-policies"
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
  { id: "dashboard", label: "总览" },
  { id: "members", label: "成员" },
  { id: "api-keys", label: "API 密钥" },
  { id: "providers", label: "供应商" },
  { id: "models", label: "模型" },
  { id: "route-policies", label: "路由策略" },
  { id: "anomaly-policies", label: "异常策略" },
  { id: "policies", label: "策略" },
  { id: "budgets", label: "预算" },
  { id: "budget-alerts", label: "预算告警" },
  { id: "audit-logs", label: "审计日志" },
  { id: "cache", label: "缓存" },
  { id: "request-logs", label: "请求日志" },
  { id: "usage-summary", label: "使用汇总" }
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
    navItems.find((item) => item.id === activePage)?.label ?? "总览";

  return (
    <main className="app-shell">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <span className="brand-mark">LG</span>
          <div>
            <div className="brand-title">LLM 成本网关</div>
            <div className="brand-subtitle">管理控制台</div>
          </div>
        </div>
        <nav className="sidebar-nav" aria-label="控制台导航">
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
          退出登录
        </button>
      </aside>
      <section className="content-shell">
        <header className="content-header">
          <div>
            <p className="eyebrow">租户管理员</p>
            <h1>{activeLabel}</h1>
          </div>
          {me && (
            <div className="principal-chip">
              <span>{me.actor_type === "service_token" ? "服务令牌" : me.user?.email}</span>
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
      return "拥有者";
    case "admin":
      return "管理员";
    case "viewer":
      return "查看者";
    default:
      return role || "拥有者";
  }
}
