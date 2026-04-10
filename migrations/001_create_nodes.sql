-- Migration 001: Nodes table
-- Stores registered ESP32 edge nodes. Each node authenticates with a hashed API key.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS nodes (
    node_id      UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    name         TEXT        NOT NULL,
    location     TEXT        NOT NULL DEFAULT 'unknown',
    api_key_hash TEXT        NOT NULL UNIQUE,   -- bcrypt hash of the issued API key
    secret_hash  TEXT        NOT NULL,           -- bcrypt hash of the HMAC signing secret
    tier         TEXT        NOT NULL DEFAULT 'standard' CHECK (tier IN ('standard','priority','admin')),
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_nodes_api_key_hash ON nodes(api_key_hash);
