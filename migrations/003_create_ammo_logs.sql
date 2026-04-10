-- Migration 003: Ammo logs table
-- Stores ammunition consumption events from load-cell sensors on edge nodes.
-- Rounds are inferred from weight delta using ammo_type's known per-round weight.

CREATE TABLE IF NOT EXISTS ammo_logs (
    id           BIGSERIAL   PRIMARY KEY,
    node_id      UUID        NOT NULL REFERENCES nodes(node_id) ON DELETE RESTRICT,
    transaction_id TEXT      REFERENCES transactions(transaction_id) ON DELETE SET NULL,
    ammo_type    TEXT        NOT NULL,        -- e.g. "5.56mm", "9mm", "7.62mm"
    delta_grams  INTEGER     NOT NULL,        -- mass removed from bin (negative = consumed)
    rounds       INTEGER     NOT NULL CHECK (rounds >= 0),
    timestamp    BIGINT      NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ammo_node_timestamp ON ammo_logs(node_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_ammo_type           ON ammo_logs(ammo_type);

-- Ammo type reference table (for weight-to-round conversion)
CREATE TABLE IF NOT EXISTS ammo_types (
    ammo_type           TEXT    PRIMARY KEY,
    grams_per_round     NUMERIC(6,2) NOT NULL,
    description         TEXT
);

-- Seed common ammo types
INSERT INTO ammo_types (ammo_type, grams_per_round, description) VALUES
    ('5.56mm',  12.31, 'NATO 5.56×45mm'),
    ('7.62mm',  25.40, 'NATO 7.62×51mm'),
    ('9mm',     12.00, '9×19mm Parabellum'),
    ('.50cal', 114.31, '12.7×99mm NATO'),
    ('40mm',   227.00, '40mm grenade')
ON CONFLICT (ammo_type) DO NOTHING;
