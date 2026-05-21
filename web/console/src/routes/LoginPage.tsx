"use client";

import { FormEvent, useMemo, useState } from "react";
import { createApiClient } from "../api/client";

type LoginPageProps = {
  onLogin: (token: string) => void;
};

export function LoginPage({ onLogin }: LoginPageProps) {
  const client = useMemo(() => createApiClient({ getToken: () => null }), []);
  const [mode, setMode] = useState<"admin-token" | "passwordless">("admin-token");
  const [token, setToken] = useState("");
  const [orgSlug, setOrgSlug] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  function selectMode(nextMode: "admin-token" | "passwordless") {
    setMode(nextMode);
    setError("");
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mode === "passwordless") {
      const slug = orgSlug.trim();
      const userEmail = email.trim();
      if (!slug || !userEmail) {
        setError("Org slug and email are required.");
        return;
      }
      setLoading(true);
      setError("");
      try {
        const result = await client.passwordlessMockLogin({
          org_slug: slug,
          email: userEmail
        });
        onLogin(result.token);
      } catch (err) {
        setError(err instanceof Error ? err.message : "Login failed.");
      } finally {
        setLoading(false);
      }
      return;
    }

    const trimmed = token.trim();
    if (!trimmed) {
      setError("Admin Token is required.");
      return;
    }
    setError("");
    onLogin(trimmed);
  }

  return (
    <main className="login-screen">
      <form className="login-panel" onSubmit={handleSubmit}>
        <p className="eyebrow">Management Console</p>
        <h1>LLM Cost Gateway</h1>
        <div className="segmented login-mode" aria-label="Login mode">
          <button
            className={mode === "admin-token" ? "button-secondary active" : "button-secondary"}
            type="button"
            onClick={() => selectMode("admin-token")}
          >
            Admin Token
          </button>
          <button
            className={mode === "passwordless" ? "button-secondary active" : "button-secondary"}
            type="button"
            onClick={() => selectMode("passwordless")}
          >
            Passwordless Mock
          </button>
        </div>
        {mode === "admin-token" ? (
          <label className="field">
            <span>Admin Token</span>
            <input
              className="input"
              type="password"
              value={token}
              onChange={(event) => setToken(event.target.value)}
              placeholder="llmgw_admin_..."
              autoComplete="off"
            />
          </label>
        ) : (
          <>
            <label className="field">
              <span>Org Slug</span>
              <input
                className="input"
                value={orgSlug}
                onChange={(event) => setOrgSlug(event.target.value)}
                placeholder="demo-org"
                autoComplete="organization"
              />
            </label>
            <label className="field">
              <span>Email</span>
              <input
                className="input"
                type="email"
                value={email}
                onChange={(event) => setEmail(event.target.value)}
                placeholder="owner@example.com"
                autoComplete="email"
              />
            </label>
          </>
        )}
        {error && <div className="alert error">{error}</div>}
        <button className="button" type="submit" disabled={loading}>
          {loading ? "Signing in..." : "Sign in"}
        </button>
      </form>
    </main>
  );
}
