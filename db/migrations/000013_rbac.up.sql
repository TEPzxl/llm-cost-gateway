CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE CHECK (email <> ''),
  display_name text NOT NULL CHECK (display_name <> ''),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL
);
CREATE INDEX users_status_idx ON users(status);

CREATE TABLE org_memberships (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  user_id uuid NOT NULL REFERENCES users(id),
  role text NOT NULL CHECK (role IN ('owner', 'admin', 'viewer')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, user_id)
);
CREATE INDEX org_memberships_org_idx ON org_memberships(org_id);
CREATE INDEX org_memberships_user_idx ON org_memberships(user_id);
CREATE INDEX org_memberships_org_role_idx ON org_memberships(org_id, role);

CREATE TABLE user_sessions (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  user_id uuid NOT NULL REFERENCES users(id),
  token_prefix text NOT NULL CHECK (token_prefix <> ''),
  token_hash text NOT NULL UNIQUE CHECK (token_hash <> ''),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'revoked')),
  expires_at timestamptz NOT NULL,
  last_used_at timestamptz NULL,
  created_at timestamptz NOT NULL,
  revoked_at timestamptz NULL,
  UNIQUE(org_id, id),
  FOREIGN KEY (org_id, user_id) REFERENCES org_memberships(org_id, user_id)
);
CREATE INDEX user_sessions_org_idx ON user_sessions(org_id);
CREATE INDEX user_sessions_user_idx ON user_sessions(user_id);
CREATE INDEX user_sessions_prefix_idx ON user_sessions(token_prefix);
CREATE INDEX user_sessions_org_status_idx ON user_sessions(org_id, status);

ALTER TABLE admin_audit_logs
  ALTER COLUMN actor_admin_token_id DROP NOT NULL,
  ADD COLUMN actor_user_id uuid NULL,
  ADD CONSTRAINT admin_audit_logs_actor_check
    CHECK (
      (actor_admin_token_id IS NOT NULL AND actor_user_id IS NULL)
      OR (actor_admin_token_id IS NULL AND actor_user_id IS NOT NULL)
    ),
  ADD CONSTRAINT admin_audit_logs_actor_user_fkey
    FOREIGN KEY (org_id, actor_user_id) REFERENCES org_memberships(org_id, user_id);

CREATE INDEX admin_audit_logs_actor_user_idx ON admin_audit_logs(actor_user_id, created_at DESC);
