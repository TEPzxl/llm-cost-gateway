ALTER TABLE route_policies
  DROP CONSTRAINT route_policies_strategy_check;

ALTER TABLE route_policies
  ADD CONSTRAINT route_policies_strategy_check
  CHECK (strategy IN ('single', 'fallback', 'lowest_cost', 'lowest_latency'));
