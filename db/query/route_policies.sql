-- name: CreateRoutePolicy :one
INSERT INTO route_policies (
  id,
  org_id,
  name,
  match_model,
  strategy,
  config,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
) RETURNING *;

-- name: GetRoutePolicy :one
SELECT *
FROM route_policies
WHERE org_id = $1 AND id = $2;

-- name: GetRoutePolicyByMatchModel :one
SELECT *
FROM route_policies
WHERE org_id = $1 AND match_model = $2 AND status = 'active';

-- name: ListRoutePolicies :many
SELECT *
FROM route_policies
WHERE org_id = $1
ORDER BY created_at DESC, id DESC;

-- name: CreateRouteTarget :one
INSERT INTO route_targets (
  id,
  org_id,
  route_policy_id,
  provider_id,
  model_id,
  priority,
  weight,
  created_at
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
) RETURNING *;

-- name: ListRouteTargets :many
SELECT *
FROM route_targets
WHERE org_id = $1 AND route_policy_id = $2
ORDER BY priority ASC, id ASC;
