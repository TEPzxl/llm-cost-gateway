-- name: CreateModel :one
INSERT INTO models (
  id,
  org_id,
  provider_id,
  provider_model_name,
  display_name,
  input_price_micro_usd_per_1k_tokens,
  output_price_micro_usd_per_1k_tokens,
  context_window,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
) RETURNING *;

-- name: GetModel :one
SELECT *
FROM models
WHERE org_id = $1 AND id = $2;

-- name: ListModels :many
SELECT *
FROM models
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: ListModelsByProvider :many
SELECT *
FROM models
WHERE org_id = $1 AND provider_id = $2
ORDER BY created_at DESC, id DESC;

-- name: UpdateModelPricing :one
UPDATE models
SET input_price_micro_usd_per_1k_tokens = $3,
    output_price_micro_usd_per_1k_tokens = $4,
    updated_at = $5
WHERE org_id = $1 AND id = $2
RETURNING *;
