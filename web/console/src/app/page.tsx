"use client";

import { useEffect, useMemo, useState } from "react";
import { createApiClient } from "../api/client";
import { Layout, type ConsolePage } from "../components/Layout";
import { APIKeysPage } from "../routes/APIKeysPage";
import { LoginPage } from "../routes/LoginPage";
import { ModelsPage } from "../routes/ModelsPage";
import { ProvidersPage } from "../routes/ProvidersPage";
import { RoutePoliciesPage } from "../routes/RoutePoliciesPage";

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
      {activePage === "api-keys" && <APIKeysPage client={client} />}
      {activePage === "providers" && <ProvidersPage client={client} />}
      {activePage === "models" && <ModelsPage client={client} />}
      {activePage === "route-policies" && <RoutePoliciesPage client={client} />}
      {!["api-keys", "providers", "models", "route-policies"].includes(activePage) && (
        <section className="panel">
          <p className="eyebrow">Selected page</p>
          <h2>{activePage}</h2>
          <p className="muted">This page will be implemented next.</p>
        </section>
      )}
    </Layout>
  );
}
