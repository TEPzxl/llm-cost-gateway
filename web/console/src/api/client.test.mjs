import assert from "node:assert/strict";
import test from "node:test";

import { createApiClient } from "./client.ts";

test("injects bearer token into admin requests", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => "llmgw_admin_test",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(JSON.stringify({ items: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" }
      });
    }
  });

  await client.listAPIKeys();

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/api-keys");
  assert.equal(calls[0].init.headers.Authorization, "Bearer llmgw_admin_test");
});

test("posts passwordless mock login without bearer token", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => null,
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(
        JSON.stringify({
          token: "llmgw_session_plain",
          token_type: "session",
          expires_at: "2026-05-22T00:00:00Z",
          org_id: "00000000-0000-0000-0000-000000000001",
          org_slug: "demo-org",
          user_id: "00000000-0000-0000-0000-000000000002",
          email: "owner@example.com",
          display_name: "Owner",
          membership_id: "00000000-0000-0000-0000-000000000003",
          role: "owner"
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    }
  });

  const result = await client.passwordlessMockLogin({
    org_slug: "demo-org",
    email: "owner@example.com"
  });

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/sessions/passwordless-mock");
  assert.equal(calls[0].init.method, "POST");
  assert.equal("Authorization" in calls[0].init.headers, false);
  assert.equal(JSON.parse(calls[0].init.body).org_slug, "demo-org");
  assert.equal(result.token, "llmgw_session_plain");
});

test("reports non-json api responses clearly", async () => {
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => null,
    fetchImpl: async () =>
      new Response("<!DOCTYPE html><html></html>", {
        status: 404,
        headers: { "Content-Type": "text/html" }
      })
  });

  await assert.rejects(
    () =>
      client.passwordlessMockLogin({
        org_slug: "demo-org",
        email: "owner@example.com"
      }),
    /API returned non-JSON response with status 404/
  );
});

test("creates member through admin API", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => "llmgw_session_test",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(
        JSON.stringify({
          membership_id: "00000000-0000-0000-0000-000000000003",
          org_id: "00000000-0000-0000-0000-000000000001",
          user_id: "00000000-0000-0000-0000-000000000002",
          email: "admin@example.com",
          display_name: "Admin",
          role: "admin",
          status: "active",
          created_at: "2026-05-21T00:00:00Z",
          updated_at: "2026-05-21T00:00:00Z"
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      );
    }
  });

  const created = await client.createMember({
    email: "admin@example.com",
    display_name: "Admin",
    role: "admin"
  });

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/members");
  assert.equal(calls[0].init.headers.Authorization, "Bearer llmgw_session_test");
  assert.equal(JSON.parse(calls[0].init.body).role, "admin");
  assert.equal(created.email, "admin@example.com");
});

test("returns plaintext API key from create response once to caller", async () => {
  const client = createApiClient({
    baseUrl: "",
    getToken: () => "llmgw_admin_test",
    fetchImpl: async () =>
      new Response(
        JSON.stringify({
          id: "00000000-0000-0000-0000-000000000001",
          name: "demo",
          key: "llmgw_live_plain",
          key_prefix: "llmgw_live_plai",
          status: "active",
          rpm_limit: 60,
          created_at: "2026-05-20T00:00:00Z"
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      )
  });

  const created = await client.createAPIKey({
    name: "demo",
    scopes: ["chat.completions"],
    rpm_limit: 60
  });

  assert.equal(created.key, "llmgw_live_plain");
});

test("posts provider health check request", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => "llmgw_admin_test",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(
        JSON.stringify({
          id: "00000000-0000-0000-0000-000000000001",
          name: "mock-provider",
          type: "mock",
          status: "active",
          timeout_ms: 30000,
          last_health_status: "healthy",
          created_at: "2026-05-20T00:00:00Z"
        }),
        { status: 200, headers: { "Content-Type": "application/json" } }
      );
    }
  });

  const checked = await client.checkProviderHealth("00000000-0000-0000-0000-000000000001");

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/providers/00000000-0000-0000-0000-000000000001/health-check");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(checked.last_health_status, "healthy");
});

test("creates budget alert without exposing secret in response", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => "llmgw_admin_test",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(
        JSON.stringify({
          id: "00000000-0000-0000-0000-000000000011",
          budget_id: "00000000-0000-0000-0000-000000000022",
          webhook_url: "https://example.com/hook",
          status: "active",
          created_at: "2026-05-21T00:00:00Z"
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      );
    }
  });

  const created = await client.createBudgetAlert({
    budget_id: "00000000-0000-0000-0000-000000000022",
    webhook_url: "https://example.com/hook",
    webhook_secret: "secret",
    status: "active"
  });

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/budget-alerts");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(created.webhook_url, "https://example.com/hook");
  assert.equal("webhook_secret" in created, false);
});

test("creates content policy", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => "llmgw_admin_test",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(
        JSON.stringify({
          id: "00000000-0000-0000-0000-000000000033",
          name: "redact-pii",
          pii_action: "redact",
          status: "active",
          created_at: "2026-05-21T00:00:00Z"
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      );
    }
  });

  const created = await client.createContentPolicy({
    name: "redact-pii",
    pii_action: "redact"
  });

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/content-policies");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(JSON.parse(calls[0].init.body).pii_action, "redact");
  assert.equal(created.pii_action, "redact");
});

test("creates anomaly policy", async () => {
  const calls = [];
  const client = createApiClient({
    baseUrl: "http://gateway.test",
    getToken: () => "llmgw_admin_test",
    fetchImpl: async (url, init) => {
      calls.push({ url, init });
      return new Response(
        JSON.stringify({
          id: "00000000-0000-0000-0000-000000000044",
          name: "daily guard",
          rule_type: "daily_cost",
          scope_type: "org",
          threshold_micro_usd: 1000000,
          spike_multiplier_bps: 20000,
          current_window_minutes: 60,
          baseline_window_minutes: 1440,
          min_requests: 1,
          action: "block",
          status: "active",
          created_at: "2026-05-21T00:00:00Z",
          updated_at: "2026-05-21T00:00:00Z"
        }),
        { status: 201, headers: { "Content-Type": "application/json" } }
      );
    }
  });

  const created = await client.createAnomalyPolicy({
    name: "daily guard",
    rule_type: "daily_cost",
    scope_type: "org",
    threshold_micro_usd: 1000000,
    action: "block"
  });

  assert.equal(calls[0].url, "http://gateway.test/api/v1/admin/anomaly-policies");
  assert.equal(calls[0].init.method, "POST");
  assert.equal(JSON.parse(calls[0].init.body).threshold_micro_usd, 1000000);
  assert.equal(created.rule_type, "daily_cost");
});
