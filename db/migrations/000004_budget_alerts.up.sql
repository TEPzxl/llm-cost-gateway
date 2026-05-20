CREATE TABLE budget_alerts (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  budget_id uuid NOT NULL,
  webhook_url text NOT NULL CHECK (webhook_url <> ''),
  webhook_secret text NULL,
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL,
  updated_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, budget_id),
  FOREIGN KEY (org_id, budget_id) REFERENCES budgets(org_id, id)
);
CREATE INDEX budget_alerts_org_idx ON budget_alerts(org_id);
CREATE INDEX budget_alerts_budget_idx ON budget_alerts(budget_id);
CREATE INDEX budget_alerts_org_status_idx ON budget_alerts(org_id, status);

CREATE TABLE budget_alert_deliveries (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  budget_alert_id uuid NOT NULL,
  budget_id uuid NOT NULL,
  threshold integer NOT NULL CHECK (threshold IN (80, 90, 100)),
  period text NOT NULL CHECK (period IN ('daily', 'monthly')),
  period_window_start timestamptz NOT NULL,
  period_window_end timestamptz NOT NULL,
  used_micro_usd bigint NOT NULL CHECK (used_micro_usd >= 0),
  limit_micro_usd bigint NOT NULL CHECK (limit_micro_usd >= 0),
  webhook_url text NOT NULL CHECK (webhook_url <> ''),
  status text NOT NULL CHECK (status IN ('success', 'failed')),
  http_status integer NULL CHECK (http_status IS NULL OR (http_status >= 100 AND http_status <= 599)),
  error_message text NULL,
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, budget_id, period_window_start, threshold),
  FOREIGN KEY (org_id, budget_alert_id) REFERENCES budget_alerts(org_id, id),
  FOREIGN KEY (org_id, budget_id) REFERENCES budgets(org_id, id),
  CHECK (period_window_end > period_window_start)
);
CREATE INDEX budget_alert_deliveries_org_idx ON budget_alert_deliveries(org_id);
CREATE INDEX budget_alert_deliveries_alert_idx ON budget_alert_deliveries(budget_alert_id);
CREATE INDEX budget_alert_deliveries_budget_idx ON budget_alert_deliveries(budget_id);
CREATE INDEX budget_alert_deliveries_created_idx ON budget_alert_deliveries(org_id, created_at DESC);
