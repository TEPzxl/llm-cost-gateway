ALTER TABLE providers
  DROP CONSTRAINT providers_last_health_status_check,
  DROP COLUMN last_health_status,
  DROP COLUMN last_health_checked_at,
  DROP COLUMN last_error_code,
  DROP COLUMN last_error_message;
