"use client";

import { useEffect, useMemo, useState } from "react";
import { createApiClient } from "../api/client";
import { Layout, type ConsolePage } from "../components/Layout";
import { LoginPage } from "../routes/LoginPage";

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
      <section className="panel">
        <p className="eyebrow">Selected page</p>
        <h2>{activePage}</h2>
        <p className="muted">
          Console shell is connected. Resource pages will be implemented next.
        </p>
        <p className="sr-only">{client ? "client ready" : "client missing"}</p>
      </section>
    </Layout>
  );
}
