CREATE TABLE model_pricing_versions (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES organizations(id),
  model_id uuid NOT NULL,
  version integer NOT NULL CHECK (version > 0),
  input_price_micro_usd_per_1k_tokens bigint NOT NULL CHECK (input_price_micro_usd_per_1k_tokens >= 0),
  output_price_micro_usd_per_1k_tokens bigint NOT NULL CHECK (output_price_micro_usd_per_1k_tokens >= 0),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded')),
  effective_from timestamptz NOT NULL,
  effective_to timestamptz NULL,
  created_at timestamptz NOT NULL,
  UNIQUE(org_id, id),
  UNIQUE(org_id, model_id, version),
  FOREIGN KEY (org_id, model_id) REFERENCES models(org_id, id),
  CHECK (effective_to IS NULL OR effective_to >= effective_from)
);
CREATE INDEX model_pricing_versions_model_idx ON model_pricing_versions(org_id, model_id, version DESC);
CREATE UNIQUE INDEX model_pricing_versions_active_idx
  ON model_pricing_versions(org_id, model_id)
  WHERE status = 'active';

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
)
SELECT
  uuid_generate_v4(),
  org_id,
  id,
  1,
  input_price_micro_usd_per_1k_tokens,
  output_price_micro_usd_per_1k_tokens,
  'active',
  created_at,
  created_at
FROM models;

ALTER TABLE cost_records
  ADD COLUMN pricing_version_id uuid NULL,
  ADD CONSTRAINT cost_records_pricing_version_fk
    FOREIGN KEY (org_id, pricing_version_id) REFERENCES model_pricing_versions(org_id, id);
CREATE INDEX cost_records_pricing_version_idx ON cost_records(pricing_version_id);
