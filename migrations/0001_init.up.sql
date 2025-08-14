CREATE TABLE IF NOT EXISTS metrics (
    name TEXT PRIMARY KEY,
    type TEXT,
    value DOUBLE PRECISION,
    delta BIGINT
);