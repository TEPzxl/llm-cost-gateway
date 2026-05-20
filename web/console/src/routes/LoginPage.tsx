"use client";

import { FormEvent, useState } from "react";

type LoginPageProps = {
  onLogin: (token: string) => void;
};

export function LoginPage({ onLogin }: LoginPageProps) {
  const [token, setToken] = useState("");
  const [error, setError] = useState("");

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
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
        {error && <div className="alert error">{error}</div>}
        <button className="button" type="submit">
          Sign in
        </button>
      </form>
    </main>
  );
}
