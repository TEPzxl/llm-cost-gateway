DROP INDEX IF EXISTS admin_audit_logs_actor_user_idx;

DELETE FROM admin_audit_logs
WHERE actor_admin_token_id IS NULL;

ALTER TABLE admin_audit_logs
  DROP CONSTRAINT IF EXISTS admin_audit_logs_actor_user_fkey,
  DROP CONSTRAINT IF EXISTS admin_audit_logs_actor_check,
  DROP COLUMN IF EXISTS actor_user_id,
  ALTER COLUMN actor_admin_token_id SET NOT NULL;

DROP TABLE IF EXISTS user_sessions;
DROP TABLE IF EXISTS org_memberships;
DROP TABLE IF EXISTS users;
