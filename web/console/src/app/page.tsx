"use client";

import { useEffect, useMemo, useState } from "react";
import { createApiClient } from "../api/client";
import type { AdminMe } from "../api/types";
import { canAccessPage, Layout, type ConsolePage } from "../components/Layout";
import { AnomalyPoliciesPage } from "../routes/AnomalyPoliciesPage";
import { APIKeysPage } from "../routes/APIKeysPage";
import { AuditLogsPage } from "../routes/AuditLogsPage";
import { BudgetAlertsPage } from "../routes/BudgetAlertsPage";
import { BudgetsPage } from "../routes/BudgetsPage";
import { CachePage } from "../routes/CachePage";
import { DashboardPage } from "../routes/DashboardPage";
import { LoginPage } from "../routes/LoginPage";
import { MembersPage } from "../routes/MembersPage";
import { ModelsPage } from "../routes/ModelsPage";
import { PoliciesPage } from "../routes/PoliciesPage";
import { ProvidersPage } from "../routes/ProvidersPage";
import { RequestLogsPage } from "../routes/RequestLogsPage";
import { RoutePoliciesPage } from "../routes/RoutePoliciesPage";
import { UsageSummaryPage } from "../routes/UsageSummaryPage";

const tokenKey = "llmgw_admin_token";

export default function HomePage() {
  const [token, setToken] = useState<string | null>(null);
  const [me, setMe] = useState<AdminMe | null>(null);
  const [activePage, setActivePage] = useState<ConsolePage>("dashboard");

  useEffect(() => {
    setToken(sessionStorage.getItem(tokenKey));
  }, []);

  const client = useMemo(() => createApiClient({ getToken: () => token }), [token]);

  useEffect(() => {
    if (!token) {
      setMe(null);
      return;
    }
    let cancelled = false;
    client
      .getMe()
      .then((response) => {
        if (!cancelled) {
          setMe(response);
        }
      })
      .catch(() => {
        if (!cancelled) {
          sessionStorage.removeItem(tokenKey);
          setToken(null);
          setMe(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [client, token]);

  useEffect(() => {
    if (!canAccessPage(activePage, me)) {
      setActivePage("dashboard");
    }
  }, [activePage, me]);

  if (!token) {
    return (
      <LoginPage
        onLogin={(value) => {
          sessionStorage.setItem(tokenKey, value);
          setToken(value);
        }}
      />
    );
  }

  return (
    <Layout
      activePage={activePage}
      me={me}
      onNavigate={setActivePage}
      onLogout={() => {
        sessionStorage.removeItem(tokenKey);
        setToken(null);
        setMe(null);
      }}
    >
      {activePage === "dashboard" && <DashboardPage client={client} />}
      {activePage === "members" && <MembersPage client={client} me={me} />}
      {activePage === "api-keys" && <APIKeysPage client={client} />}
      {activePage === "providers" && <ProvidersPage client={client} />}
      {activePage === "models" && <ModelsPage client={client} />}
      {activePage === "route-policies" && <RoutePoliciesPage client={client} />}
      {activePage === "anomaly-policies" && <AnomalyPoliciesPage client={client} />}
      {activePage === "policies" && <PoliciesPage client={client} />}
      {activePage === "budgets" && <BudgetsPage client={client} />}
      {activePage === "budget-alerts" && <BudgetAlertsPage client={client} />}
      {activePage === "audit-logs" && <AuditLogsPage client={client} />}
      {activePage === "cache" && <CachePage client={client} />}
      {activePage === "request-logs" && <RequestLogsPage client={client} />}
      {activePage === "usage-summary" && <UsageSummaryPage client={client} />}
    </Layout>
  );
}
