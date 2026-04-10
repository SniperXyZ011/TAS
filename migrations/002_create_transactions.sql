-- Migration 002: Transactions table
-- Stores weapon checkout/checkin events synced from edge nodes.
-- transaction_id is the PRIMARY KEY — provides natural deduplication via ON CONFLICT DO NOTHING.

CREATE TABLE IF NOT EXISTS transactions (
    transaction_id TEXT        PRIMARY KEY,
    node_id        UUID        NOT NULL REFERENCES nodes(node_id) ON DELETE RESTRICT,
    user_id        TEXT        NOT NULL,
    weapon_id      TEXT        NOT NULL,
    action         TEXT        NOT NULL CHECK (action IN ('checkout','checkin','audit','transfer','lost','found')),
    quantity       INTEGER     NOT NULL DEFAULT 1 CHECK (quantity > 0),
    notes          TEXT,
    timestamp      BIGINT      NOT NULL,                   -- UTC Unix epoch (seconds) from the edge device
    received_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),     -- when this server received it
    signature      TEXT        NOT NULL,
    status         TEXT        NOT NULL DEFAULT 'accepted' CHECK (status IN ('accepted','rejected','flagged'))
);

-- Composite index for dashboard queries: "all transactions for a node in time range"
CREATE INDEX IF NOT EXISTS idx_tx_node_timestamp  ON transactions(node_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_tx_user_id         ON transactions(user_id);
CREATE INDEX IF NOT EXISTS idx_tx_weapon_id       ON transactions(weapon_id);
CREATE INDEX IF NOT EXISTS idx_tx_action          ON transactions(action);
CREATE INDEX IF NOT EXISTS idx_tx_received_at     ON transactions(received_at DESC);
