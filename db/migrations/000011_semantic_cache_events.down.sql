DELETE FROM cache_events
WHERE event_type IN (
  'semantic_hit',
  'semantic_miss',
  'semantic_skip',
  'semantic_store',
  'semantic_read_error',
  'semantic_write_error'
);

ALTER TABLE cache_events
  DROP CONSTRAINT cache_events_event_type_check;

ALTER TABLE cache_events
  ADD CONSTRAINT cache_events_event_type_check
  CHECK (event_type IN ('hit', 'miss', 'store', 'skip', 'read_error', 'write_error'));
