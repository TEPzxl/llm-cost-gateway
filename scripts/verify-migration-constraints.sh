#!/usr/bin/env sh
set -eu

COMPOSE_FILE="${COMPOSE_FILE:-deploy/docker-compose.yml}"

docker compose -f "$COMPOSE_FILE" --project-directory . exec -T postgres \
  psql -U llmgw -d llmgw -v ON_ERROR_STOP=1 <<'SQL'
BEGIN;

DO $$
DECLARE
  org_a uuid := gen_random_uuid();
  org_b uuid := gen_random_uuid();
  api_key_a uuid := gen_random_uuid();
  provider_a uuid := gen_random_uuid();
  provider_b uuid := gen_random_uuid();
  model_a uuid := gen_random_uuid();
  route_policy_a uuid := gen_random_uuid();
  request_log_a uuid := gen_random_uuid();
  usage_record_a uuid := gen_random_uuid();
BEGIN
  INSERT INTO organizations (id, name, slug, created_at, updated_at)
  VALUES
    (org_a, 'Org A', 'org-a-constraints', now(), now()),
    (org_b, 'Org B', 'org-b-constraints', now(), now());

  INSERT INTO api_keys (id, org_id, name, key_prefix, key_hash, scopes, created_at)
  VALUES (api_key_a, org_a, 'key-a', 'llmgw_live_a', 'hash-a', ARRAY['chat.completions'], now());

  INSERT INTO providers (id, org_id, name, type, timeout_ms, created_at, updated_at)
  VALUES
    (provider_a, org_a, 'provider-a', 'mock', 30000, now(), now()),
    (provider_b, org_a, 'provider-b', 'mock', 30000, now(), now());

  INSERT INTO models (
    id,
    org_id,
    provider_id,
    provider_model_name,
    display_name,
    input_price_micro_usd_per_1k_tokens,
    output_price_micro_usd_per_1k_tokens,
    created_at,
    updated_at
  )
  VALUES (model_a, org_a, provider_a, 'mock-small', 'Mock Small', 100, 200, now(), now());

  INSERT INTO route_policies (id, org_id, name, match_model, strategy, created_at, updated_at)
  VALUES (route_policy_a, org_a, 'fast-chat', 'fast-chat', 'single', now(), now());

  INSERT INTO request_logs (
    id,
    org_id,
    api_key_id,
    provider_id,
    model_id,
    route_policy_id,
    method,
    path,
    status,
    status_code,
    latency_ms,
    started_at,
    completed_at
  )
  VALUES (
    request_log_a,
    org_a,
    api_key_a,
    provider_a,
    model_a,
    route_policy_a,
    'POST',
    '/v1/chat/completions',
    'success',
    200,
    10,
    now(),
    now()
  );

  BEGIN
    INSERT INTO provider_secrets (id, org_id, provider_id, encrypted_api_key, nonce, created_at, updated_at)
    VALUES (gen_random_uuid(), org_b, provider_a, 'encrypted', 'nonce', now(), now());
    RAISE EXCEPTION 'provider_secrets accepted cross-org provider_id';
  EXCEPTION
    WHEN foreign_key_violation THEN NULL;
  END;

  BEGIN
    INSERT INTO models (
      id,
      org_id,
      provider_id,
      provider_model_name,
      display_name,
      input_price_micro_usd_per_1k_tokens,
      output_price_micro_usd_per_1k_tokens,
      created_at,
      updated_at
    )
    VALUES (gen_random_uuid(), org_b, provider_a, 'cross-org-model', 'Cross Org Model', 100, 200, now(), now());
    RAISE EXCEPTION 'models accepted cross-org provider_id';
  EXCEPTION
    WHEN foreign_key_violation THEN NULL;
  END;

  BEGIN
    INSERT INTO route_targets (id, org_id, route_policy_id, provider_id, model_id, priority, weight, created_at)
    VALUES (gen_random_uuid(), org_a, route_policy_a, provider_b, model_a, 1, 100, now());
    RAISE EXCEPTION 'route_targets accepted model_id that belongs to another provider';
  EXCEPTION
    WHEN foreign_key_violation THEN NULL;
  END;

  BEGIN
    INSERT INTO request_logs (
      id,
      org_id,
      api_key_id,
      provider_id,
      model_id,
      route_policy_id,
      method,
      path,
      status,
      status_code,
      latency_ms,
      started_at,
      completed_at
    )
    VALUES (
      gen_random_uuid(),
      org_b,
      api_key_a,
      provider_a,
      model_a,
      route_policy_a,
      'POST',
      '/v1/chat/completions',
      'success',
      200,
      10,
      now(),
      now()
    );
    RAISE EXCEPTION 'request_logs accepted cross-org references';
  EXCEPTION
    WHEN foreign_key_violation THEN NULL;
  END;

  BEGIN
    INSERT INTO usage_records (
      id,
      request_log_id,
      org_id,
      api_key_id,
      provider_id,
      model_id,
      prompt_tokens,
      completion_tokens,
      total_tokens,
      created_at
    )
    VALUES (gen_random_uuid(), request_log_a, org_b, api_key_a, provider_a, model_a, 1, 1, 2, now());
    RAISE EXCEPTION 'usage_records accepted cross-org references';
  EXCEPTION
    WHEN foreign_key_violation THEN NULL;
  END;

  INSERT INTO usage_records (
    id,
    request_log_id,
    org_id,
    api_key_id,
    provider_id,
    model_id,
    prompt_tokens,
    completion_tokens,
    total_tokens,
    created_at
  )
  VALUES (usage_record_a, request_log_a, org_a, api_key_a, provider_a, model_a, 1, 1, 2, now());

  BEGIN
    INSERT INTO cost_records (
      id,
      usage_record_id,
      org_id,
      provider_id,
      model_id,
      currency,
      input_cost_micro,
      output_cost_micro,
      total_cost_micro,
      pricing_snapshot,
      created_at
    )
    VALUES (gen_random_uuid(), usage_record_a, org_b, provider_a, model_a, 'USD', 1, 1, 2, '{}'::jsonb, now());
    RAISE EXCEPTION 'cost_records accepted cross-org references';
  EXCEPTION
    WHEN foreign_key_violation THEN NULL;
  END;

  BEGIN
    INSERT INTO providers (id, org_id, name, type, timeout_ms, created_at, updated_at)
    VALUES (gen_random_uuid(), org_a, 'invalid-provider', 'invalid_type', 30000, now(), now());
    RAISE EXCEPTION 'providers accepted invalid type';
  EXCEPTION
    WHEN check_violation THEN NULL;
  END;

  BEGIN
    INSERT INTO budgets (id, org_id, name, scope_type, period, limit_micro_usd, action, created_at, updated_at)
    VALUES (gen_random_uuid(), org_a, 'negative-budget', 'org', 'monthly', -1, 'block', now(), now());
    RAISE EXCEPTION 'budgets accepted negative limit';
  EXCEPTION
    WHEN check_violation THEN NULL;
  END;

  RAISE NOTICE 'migration constraints verified';
END $$;

ROLLBACK;
SQL
