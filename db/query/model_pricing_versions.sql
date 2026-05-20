-- name: CreateModelPricingVersion :one
INSERT INTO model_pricing_versions (
  id,
  org_id,
  model_id,
  version,
  input_price_micro_usd_per_1k_tokens,
  output_price_micro_usd_per_1k_tokens,
  status,
  effective_from,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetActiveModelPricingVersion :one
SELECT *
FROM model_pricing_versions
WHERE org_id = $1 AND model_id = $2 AND status = 'active';

-- name: ListModelPricingVersions :many
SELECT *
FROM model_pricing_versions
WHERE org_id = $1 AND model_id = $2
ORDER BY version DESC;

-- name: GetNextModelPricingVersionNumber :one
SELECT COALESCE(MAX(version), 0)::integer + 1 AS next_version
FROM model_pricing_versions
WHERE org_id = $1 AND model_id = $2;

-- name: SupersedeActiveModelPricingVersion :one
UPDATE model_pricing_versions
SET status = 'superseded',
    effective_to = $3
WHERE org_id = $1 AND model_id = $2 AND status = 'active'
RETURNING *;
