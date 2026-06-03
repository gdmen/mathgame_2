CREATE INDEX idx_events_user_event_value_ts ON events (user_id, event_type, value(32), timestamp);
DROP INDEX idx_events_user_event ON events;
