CREATE TABLE cache_events (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  request_log_id uuid NULL REFERENCES request_logs(id) ON DELETE SET NULL,
  event_type text NOT NULL CHECK (event_type IN ('hit', 'miss', 'store', 'skip', 'read_error', 'write_error')),
  requested_model text NOT NULL,
  cache_key_hash text NOT NULL,
  messages_hash text NOT NULL,
  reason text NULL,
  metadata jsonb NULL,
  created_at timestamptz NOT NULL
);

CREATE INDEX cache_events_org_created_idx
  ON cache_events(org_id, created_at DESC, id DESC);

CREATE INDEX cache_events_org_key_created_idx
  ON cache_events(org_id, cache_key_hash, created_at DESC);
