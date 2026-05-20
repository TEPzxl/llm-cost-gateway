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
