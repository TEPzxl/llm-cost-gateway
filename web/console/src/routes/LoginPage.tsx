"use client";

import { FormEvent, useMemo, useState } from "react";
import { createApiClient } from "../api/client";

type LoginPageProps = {
  onLogin: (token: string) => void;
};

const passwordlessMockEnabled =
  process.env.NEXT_PUBLIC_ENABLE_PASSWORDLESS_MOCK === "true" ||
  process.env.NODE_ENV !== "production";

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
    if (nextMode === "admin-token") {
      setOrgSlug("");
      setEmail("");
    } else {
      setToken("");
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mode === "passwordless" && passwordlessMockEnabled) {
      const slug = orgSlug.trim();
      const userEmail = email.trim();
      if (!slug || !userEmail) {
        setError("组织标识和邮箱不能为空。");
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
        setError(err instanceof Error ? err.message : "登录失败。");
      } finally {
        setLoading(false);
      }
      return;
    }

    const trimmed = token.trim();
    if (!trimmed) {
      setError("管理员令牌不能为空。");
      return;
    }
    setError("");
    onLogin(trimmed);
  }

  return (
    <main className="login-screen">
      <form className="login-panel" onSubmit={handleSubmit}>
        <p className="eyebrow">管理控制台</p>
        <h1>LLM 成本网关</h1>
        <div className="segmented login-mode" aria-label="登录方式">
          <button
            className={mode === "admin-token" ? "button-secondary active" : "button-secondary"}
            type="button"
            onClick={() => selectMode("admin-token")}
          >
            管理员令牌登录
          </button>
          {passwordlessMockEnabled && (
            <button
              className={mode === "passwordless" ? "button-secondary active" : "button-secondary"}
              type="button"
              onClick={() => selectMode("passwordless")}
            >
              免密登录（演示）
            </button>
          )}
        </div>
        {mode === "admin-token" ? (
          <label className="field">
            <span>管理员令牌</span>
            <input
              className="input"
              type="password"
              value={token}
              onChange={(event) => {
                setError("");
                setToken(event.target.value);
              }}
              placeholder="llmgw_admin_..."
              autoComplete="off"
            />
          </label>
        ) : (
          <>
            <label className="field">
              <span>组织标识</span>
              <input
                className="input"
                value={orgSlug}
                onChange={(event) => {
                  setError("");
                  setOrgSlug(event.target.value);
                }}
                placeholder="demo-org"
                autoComplete="organization"
              />
            </label>
            <label className="field">
              <span>邮箱</span>
              <input
                className="input"
                type="email"
                value={email}
                onChange={(event) => {
                  setError("");
                  setEmail(event.target.value);
                }}
                placeholder="owner@example.com"
                autoComplete="email"
              />
            </label>
          </>
        )}
        {error && <div className="alert error">{error}</div>}
        <button className="button" type="submit" disabled={loading}>
          {loading ? "登录中..." : "登录"}
        </button>
      </form>
    </main>
  );
}
