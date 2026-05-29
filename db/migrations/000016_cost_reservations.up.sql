CREATE TABLE cost_limit_counters (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  scope_type text NOT NULL CHECK (scope_type IN ('org', 'api_key')),
  scope_id uuid NOT NULL,
  period text NOT NULL CHECK (period IN ('daily', 'monthly')),
  window_start timestamptz NOT NULL,
  window_end timestamptz NOT NULL,
  settled_micro_usd bigint NOT NULL DEFAULT 0 CHECK (settled_micro_usd >= 0),
  reserved_micro_usd bigint NOT NULL DEFAULT 0 CHECK (reserved_micro_usd >= 0),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, scope_type, scope_id, period, window_start),
  CHECK (window_end > window_start)
);
CREATE INDEX cost_limit_counters_scope_idx
  ON cost_limit_counters(org_id, scope_type, scope_id, period, window_start);

CREATE TABLE cost_reservations (
  id uuid PRIMARY KEY,
  request_id uuid NOT NULL,
  org_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  api_key_id uuid NOT NULL,
  scope_type text NOT NULL CHECK (scope_type IN ('org', 'api_key')),
  scope_id uuid NOT NULL,
  period text NOT NULL CHECK (period IN ('daily', 'monthly')),
  window_start timestamptz NOT NULL,
  window_end timestamptz NOT NULL,
  reserved_micro_usd bigint NOT NULL CHECK (reserved_micro_usd >= 0),
  settled_micro_usd bigint NOT NULL DEFAULT 0 CHECK (settled_micro_usd >= 0),
  status text NOT NULL CHECK (status IN ('reserved', 'settled', 'released')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(request_id, scope_type, scope_id, period, window_start),
  CHECK (window_end > window_start),
  FOREIGN KEY (org_id, api_key_id) REFERENCES api_keys(org_id, id)
);
CREATE INDEX cost_reservations_request_idx ON cost_reservations(request_id);
CREATE INDEX cost_reservations_scope_idx
  ON cost_reservations(org_id, scope_type, scope_id, period, window_start);
