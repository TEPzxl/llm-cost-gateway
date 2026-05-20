CREATE TABLE admin_audit_logs (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  actor_admin_token_id uuid NOT NULL,
  action text NOT NULL CHECK (action <> ''),
  resource_type text NOT NULL CHECK (resource_type <> ''),
  resource_id uuid NULL,
  request_id text NOT NULL CHECK (request_id <> ''),
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  FOREIGN KEY (org_id, actor_admin_token_id) REFERENCES admin_tokens(org_id, id)
);
CREATE INDEX admin_audit_logs_org_time_idx ON admin_audit_logs(org_id, created_at DESC, id DESC);
CREATE INDEX admin_audit_logs_actor_idx ON admin_audit_logs(actor_admin_token_id, created_at DESC);
CREATE INDEX admin_audit_logs_resource_idx ON admin_audit_logs(org_id, resource_type, resource_id);
