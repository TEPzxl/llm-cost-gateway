"use client";

import { FormEvent, useEffect, useMemo, useState } from "react";
import { createApiClient } from "../api/client";

type LoginPageProps = {
  onLogin: (token: string) => void;
};

const passwordlessMockEnabled =
  process.env.NEXT_PUBLIC_ENABLE_PASSWORDLESS_MOCK === "true" ||
  process.env.NODE_ENV !== "production";
const passwordlessEmailEnabled =
  process.env.NEXT_PUBLIC_ENABLE_PASSWORDLESS_EMAIL === "true";

type LoginMode = "admin-token" | "passwordless-mock" | "passwordless-email";

export function LoginPage({ onLogin }: LoginPageProps) {
  const client = useMemo(() => createApiClient({ getToken: () => null }), []);
  const [mode, setMode] = useState<LoginMode>("admin-token");
  const [token, setToken] = useState("");
  const [orgSlug, setOrgSlug] = useState("");
  const [email, setEmail] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!passwordlessEmailEnabled) {
      return;
    }
    const magicToken = new URLSearchParams(window.location.search).get("magic_token");
    if (!magicToken) {
      return;
    }
    setLoading(true);
    setError("");
    setNotice("");
    client
      .verifyPasswordlessMagicLink({ token: magicToken })
      .then((result) => {
        window.history.replaceState(null, "", window.location.pathname);
        onLogin(result.token);
      })
      .catch((err) => {
        setError(err instanceof Error ? err.message : "登录链接无效或已过期。");
      })
      .finally(() => setLoading(false));
  }, [client, onLogin]);

  function selectMode(nextMode: LoginMode) {
    setMode(nextMode);
    setError("");
    setNotice("");
    if (nextMode === "admin-token") {
      setOrgSlug("");
      setEmail("");
    } else {
      setToken("");
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (mode === "passwordless-mock" && passwordlessMockEnabled) {
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
    if (mode === "passwordless-email" && passwordlessEmailEnabled) {
      const slug = orgSlug.trim();
      const userEmail = email.trim();
      if (!slug || !userEmail) {
        setError("组织标识和邮箱不能为空。");
        return;
      }
      setLoading(true);
      setError("");
      setNotice("");
      try {
        await client.requestPasswordlessMagicLink({
          org_slug: slug,
          email: userEmail
        });
        setNotice("如果该邮箱属于此组织，登录链接会发送到邮箱中。");
      } catch (err) {
        setError(err instanceof Error ? err.message : "发送登录链接失败。");
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
              className={mode === "passwordless-mock" ? "button-secondary active" : "button-secondary"}
              type="button"
              onClick={() => selectMode("passwordless-mock")}
            >
              免密登录（演示）
            </button>
          )}
          {passwordlessEmailEnabled && (
            <button
              className={mode === "passwordless-email" ? "button-secondary active" : "button-secondary"}
              type="button"
              onClick={() => selectMode("passwordless-email")}
            >
              邮箱免密登录
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
        {notice && <div className="alert success">{notice}</div>}
        <button className="button" type="submit" disabled={loading}>
          {loading ? "处理中..." : mode === "passwordless-email" ? "发送登录链接" : "登录"}
        </button>
      </form>
    </main>
  );
}
