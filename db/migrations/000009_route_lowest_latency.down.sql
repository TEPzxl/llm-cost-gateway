DELETE FROM route_targets
WHERE route_policy_id IN (
  SELECT id
  FROM route_policies
  WHERE strategy = 'lowest_latency'
);

DELETE FROM route_policies
WHERE strategy = 'lowest_latency';

ALTER TABLE route_policies
  DROP CONSTRAINT route_policies_strategy_check;

ALTER TABLE route_policies
  ADD CONSTRAINT route_policies_strategy_check
  CHECK (strategy IN ('single', 'fallback', 'lowest_cost'));
