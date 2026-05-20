"use client";

import { useEffect, useMemo, useState } from "react";
import { createApiClient } from "../api/client";
import { Layout, type ConsolePage } from "../components/Layout";
import { APIKeysPage } from "../routes/APIKeysPage";
import { BudgetsPage } from "../routes/BudgetsPage";
import { DashboardPage } from "../routes/DashboardPage";
import { LoginPage } from "../routes/LoginPage";
import { ModelsPage } from "../routes/ModelsPage";
import { ProvidersPage } from "../routes/ProvidersPage";
import { RequestLogsPage } from "../routes/RequestLogsPage";
import { RoutePoliciesPage } from "../routes/RoutePoliciesPage";
import { UsageSummaryPage } from "../routes/UsageSummaryPage";

const tokenKey = "llmgw_admin_token";

export default function HomePage() {
  const [token, setToken] = useState<string | null>(null);
  const [activePage, setActivePage] = useState<ConsolePage>("dashboard");

  useEffect(() => {
    setToken(sessionStorage.getItem(tokenKey));
  }, []);

  const client = useMemo(() => createApiClient({ getToken: () => token }), [token]);

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
      onNavigate={setActivePage}
      onLogout={() => {
        sessionStorage.removeItem(tokenKey);
        setToken(null);
      }}
    >
      {activePage === "dashboard" && <DashboardPage client={client} />}
      {activePage === "api-keys" && <APIKeysPage client={client} />}
      {activePage === "providers" && <ProvidersPage client={client} />}
      {activePage === "models" && <ModelsPage client={client} />}
      {activePage === "route-policies" && <RoutePoliciesPage client={client} />}
      {activePage === "budgets" && <BudgetsPage client={client} />}
      {activePage === "request-logs" && <RequestLogsPage client={client} />}
      {activePage === "usage-summary" && <UsageSummaryPage client={client} />}
    </Layout>
  );
}
