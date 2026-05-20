-- name: UpsertProviderSecret :one
INSERT INTO provider_secrets (
  id,
  org_id,
  provider_id,
  encrypted_api_key,
  nonce,
  key_version,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
ON CONFLICT (org_id, provider_id)
DO UPDATE SET
  encrypted_api_key = EXCLUDED.encrypted_api_key,
  nonce = EXCLUDED.nonce,
  key_version = EXCLUDED.key_version,
  updated_at = EXCLUDED.updated_at
RETURNING *;

-- name: GetProviderSecret :one
SELECT *
FROM provider_secrets
WHERE org_id = $1 AND provider_id = $2;
