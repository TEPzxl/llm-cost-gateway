-- name: CreateOrganization :one
INSERT INTO organizations (
  id,
  name,
  slug,
  status,
  created_at,
  updated_at
) VALUES (
  $1, $2, $3, $4, $5, $6
) RETURNING *;

-- name: GetOrganization :one
SELECT *
FROM organizations
WHERE id = $1;

-- name: GetOrganizationBySlug :one
SELECT *
FROM organizations
WHERE slug = $1;
