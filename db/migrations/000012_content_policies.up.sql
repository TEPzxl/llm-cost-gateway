CREATE TABLE content_policies (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  name text NOT NULL,
  pii_action text NOT NULL CHECK (pii_action IN ('allow', 'redact', 'block')),
  status text NOT NULL CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, name)
);

CREATE INDEX content_policies_org_status_created_idx
  ON content_policies(org_id, status, created_at DESC, id DESC);
