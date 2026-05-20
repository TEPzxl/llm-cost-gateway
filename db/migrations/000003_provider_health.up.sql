ALTER TABLE providers
  ADD COLUMN last_health_status text NULL,
  ADD COLUMN last_health_checked_at timestamptz NULL,
  ADD COLUMN last_error_code text NULL,
  ADD COLUMN last_error_message text NULL,
  ADD CONSTRAINT providers_last_health_status_check
    CHECK (last_health_status IS NULL OR last_health_status IN ('healthy', 'unhealthy'));
