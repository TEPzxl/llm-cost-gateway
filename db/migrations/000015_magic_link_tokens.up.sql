CREATE TABLE magic_link_tokens (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  user_id uuid NOT NULL REFERENCES users(id),
  email text NOT NULL CHECK (email <> ''),
  token_prefix text NOT NULL CHECK (token_prefix <> ''),
  token_hash text NOT NULL UNIQUE CHECK (token_hash <> ''),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'consumed')),
  expires_at timestamptz NOT NULL,
  consumed_at timestamptz NULL,
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  FOREIGN KEY (org_id, user_id) REFERENCES org_memberships(org_id, user_id),
  CHECK ((status = 'consumed') = (consumed_at IS NOT NULL))
);

CREATE INDEX magic_link_tokens_org_idx ON magic_link_tokens(org_id);
CREATE INDEX magic_link_tokens_user_idx ON magic_link_tokens(org_id, user_id);
CREATE INDEX magic_link_tokens_hash_idx ON magic_link_tokens(token_hash);
CREATE INDEX magic_link_tokens_expires_idx ON magic_link_tokens(expires_at);
