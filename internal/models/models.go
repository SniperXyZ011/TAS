package models

import "time"

// ─── Node ────────────────────────────────────────────────────────────────────

// Node represents a registered ESP32 edge kiosk in the armory mesh network.
type Node struct {
	NodeID     string     `json:"node_id"`
	Name       string     `json:"name"`
	Location   string     `json:"location"`
	Tier       string     `json:"tier"`
	IsActive   bool       `json:"is_active"`
	CreatedAt  time.Time  `json:"created_at"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
}

// RegisterNodeRequest is the payload for POST /api/v1/nodes (admin only).
type RegisterNodeRequest struct {
	Name     string `json:"name"`
	Location string `json:"location"`
	Tier     string `json:"tier"`
}

// RegisterNodeResponse is returned after successful node registration.
// The plaintext APIKey and Secret are only returned ONCE — store them securely.
type RegisterNodeResponse struct {
	NodeID  string `json:"node_id"`
	Name    string `json:"name"`
	APIKey  string `json:"api_key"`  // plaintext — shown once
	Secret  string `json:"secret"`   // plaintext HMAC secret — shown once
}

// ─── Transaction ─────────────────────────────────────────────────────────────

// Action represents the type of weapon transaction.
type Action string

const (
	ActionCheckout Action = "checkout"
	ActionCheckin  Action = "checkin"
	ActionAudit    Action = "audit"
	ActionTransfer Action = "transfer"
	ActionLost     Action = "lost"
	ActionFound    Action = "found"
)

// Transaction represents a single weapon movement event from an edge node.
type Transaction struct {
	TransactionID string `json:"transaction_id"`
	NodeID        string `json:"node_id"`
	UserID        string `json:"user_id"`
	WeaponID      string `json:"weapon_id"`
	Action        Action `json:"action"`
	Quantity      int    `json:"quantity"`
	Notes         string `json:"notes,omitempty"`
	Timestamp     int64  `json:"timestamp"` // Unix seconds (UTC)
	Signature     string `json:"signature"` // HMAC-SHA256 of payload fields
}

// SyncRequest is the body for POST /api/v1/sync — a batch of transactions.
type SyncRequest struct {
	Transactions []Transaction `json:"transactions"`
}

// TransactionResult describes the server's disposition for a single transaction.
type TransactionResult struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"` // "accepted" | "duplicate" | "invalid_signature" | "invalid_action"
	Message       string `json:"message,omitempty"`
}

// SyncResponse is the body returned by POST /api/v1/sync.
type SyncResponse struct {
	Accepted  int                 `json:"accepted"`
	Duplicate int                 `json:"duplicate"`
	Rejected  int                 `json:"rejected"`
	Results   []TransactionResult `json:"results"`
}

// ─── Ammo ────────────────────────────────────────────────────────────────────

// AmmoLog represents an ammo consumption event from a load-cell sensor.
type AmmoLog struct {
	NodeID        string `json:"node_id"`
	TransactionID string `json:"transaction_id,omitempty"`
	AmmoType      string `json:"ammo_type"`
	DeltaGrams    int    `json:"delta_grams"`
	Rounds        int    `json:"rounds"`
	Timestamp     int64  `json:"timestamp"`
}

// AmmoSyncRequest is the body for POST /api/v1/sync/ammo.
type AmmoSyncRequest struct {
	Logs []AmmoLog `json:"logs"`
}

// AmmoSyncResponse summarises how many ammo logs were inserted.
type AmmoSyncResponse struct {
	Inserted int `json:"inserted"`
}

// ─── Shared ──────────────────────────────────────────────────────────────────

// ErrorResponse is the standard error envelope for all API errors.
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}
