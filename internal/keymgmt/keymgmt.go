package keymgmt

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"

	"antigravity-gateway/internal/config"
)

var (
	ErrInvalidKey     = errors.New("invalid downstream api key")
	ErrKeyExpired     = errors.New("downstream api key has expired")
	ErrKeyRevoked     = errors.New("downstream api key has been revoked")
	ErrModelForbidden = errors.New("model not permitted for this api key")
	ErrStaticKey      = errors.New("static key cannot be modified or revoked")
	ErrKeyNotFound    = errors.New("key not found")
)

type KeyInfo struct {
	ID            string   `json:"id"`
	KeyPrefix     string   `json:"key_prefix"`
	HMACHash      string   `json:"-"`
	Name          string   `json:"name"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	CreatedAt     int64    `json:"created_at"`
	ExpiresAt     int64    `json:"expires_at,omitempty"`
	RevokedAt     int64    `json:"revoked_at,omitempty"`
	Status        string   `json:"status"` // active, revoked, expired
	IsStatic      bool     `json:"is_static"`
}

type CreateKeyResult struct {
	ID            string   `json:"id"`
	Key           string   `json:"key"` // Returned ONLY once on creation!
	KeyPrefix     string   `json:"key_prefix"`
	Name          string   `json:"name"`
	AllowedModels []string `json:"allowed_models,omitempty"`
	CreatedAt     int64    `json:"created_at"`
	ExpiresAt     int64    `json:"expires_at,omitempty"`
	Status        string   `json:"status"`
}

type KeySnapshot struct {
	byHash map[string]*KeyInfo
	byID   map[string]*KeyInfo
	all    []*KeyInfo
}

type Manager struct {
	db         *sql.DB
	hmacSecret []byte
	staticKeys []config.StaticKeyConfig
	snapshot   atomic.Pointer[KeySnapshot]
	writeMu    sync.Mutex
}

func NewManager(dbPath string, hmacSecret string, staticKeys []config.StaticKeyConfig) (*Manager, error) {
	if len(hmacSecret) < 16 {
		return nil, errors.New("key HMAC secret must be at least 16 characters")
	}

	// Ensure parent directory exists
	if dbPath != ":memory:" {
		dir := filepath.Dir(dbPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0755); err != nil {
				return nil, fmt.Errorf("failed to create db directory: %w", err)
			}
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}

	// Enable WAL mode and foreign keys
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON;`); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set sqlite pragma: %w", err)
	}

	// Run migration
	schema := `
	CREATE TABLE IF NOT EXISTS downstream_keys (
		id TEXT PRIMARY KEY,
		key_prefix TEXT NOT NULL,
		hmac_hash TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		allowed_models TEXT,
		created_at INTEGER NOT NULL,
		expires_at INTEGER,
		revoked_at INTEGER,
		status TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_downstream_keys_hmac ON downstream_keys(hmac_hash);
	`
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize db schema: %w", err)
	}

	m := &Manager{
		db:         db,
		hmacSecret: []byte(hmacSecret),
		staticKeys: staticKeys,
	}

	// Initial snapshot load
	if err := m.reloadSnapshot(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to load initial key snapshot: %w", err)
	}

	return m, nil
}

func (m *Manager) Close() error {
	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *Manager) HashKey(rawKey string) string {
	mac := hmac.New(sha256.New, m.hmacSecret)
	mac.Write([]byte(rawKey))
	return hex.EncodeToString(mac.Sum(nil))
}

func (m *Manager) reloadSnapshot() error {
	byHash := make(map[string]*KeyInfo)
	byID := make(map[string]*KeyInfo)
	var all []*KeyInfo

	// 1. Process static keys first
	for _, sk := range m.staticKeys {
		hash := m.HashKey(sk.Key)
		prefix := sk.Key
		if len(prefix) > 12 {
			prefix = prefix[:12] + "..."
		}
		info := &KeyInfo{
			ID:            sk.ID,
			KeyPrefix:     prefix,
			HMACHash:      hash,
			Name:          sk.Name,
			AllowedModels: sk.AllowedModels,
			CreatedAt:     time.Now().Unix(),
			ExpiresAt:     0,
			Status:        "active",
			IsStatic:      true,
		}
		if _, exists := byID[info.ID]; exists {
			return fmt.Errorf("duplicate static key ID: %q", info.ID)
		}
		byHash[hash] = info
		byID[info.ID] = info
		all = append(all, info)
	}

	// 2. Read dynamic keys from DB
	rows, err := m.db.Query(`SELECT id, key_prefix, hmac_hash, name, allowed_models, created_at, expires_at, revoked_at, status FROM downstream_keys`)
	if err != nil {
		return err
	}
	defer rows.Close()

	now := time.Now().Unix()
	for rows.Next() {
		var id, keyPrefix, hmacHash, name, status string
		var allowedModelsJSON sql.NullString
		var createdAt int64
		var expiresAt, revokedAt sql.NullInt64

		if err := rows.Scan(&id, &keyPrefix, &hmacHash, &name, &allowedModelsJSON, &createdAt, &expiresAt, &revokedAt, &status); err != nil {
			return err
		}

		var allowedModels []string
		if allowedModelsJSON.Valid && allowedModelsJSON.String != "" {
			_ = json.Unmarshal([]byte(allowedModelsJSON.String), &allowedModels)
		}

		var exp int64
		if expiresAt.Valid {
			exp = expiresAt.Int64
			if status == "active" && exp > 0 && now > exp {
				status = "expired"
			}
		}

		var rev int64
		if revokedAt.Valid {
			rev = revokedAt.Int64
		}

		// Check conflict with static key
		if _, exists := byID[id]; exists {
			return fmt.Errorf("dynamic key ID %q conflicts with static key", id)
		}

		info := &KeyInfo{
			ID:            id,
			KeyPrefix:     keyPrefix,
			HMACHash:      hmacHash,
			Name:          name,
			AllowedModels: allowedModels,
			CreatedAt:     createdAt,
			ExpiresAt:     exp,
			RevokedAt:     rev,
			Status:        status,
			IsStatic:      false,
		}

		byHash[hmacHash] = info
		byID[id] = info
		all = append(all, info)
	}

	if err := rows.Err(); err != nil {
		return err
	}

	snap := &KeySnapshot{
		byHash: byHash,
		byID:   byID,
		all:    all,
	}
	m.snapshot.Store(snap)
	return nil
}

// Authenticate checks a raw Bearer key in O(1) memory lookup without touching SQLite.
func (m *Manager) Authenticate(rawKey string) (*KeyInfo, error) {
	rawKey = strings.TrimSpace(rawKey)
	if rawKey == "" {
		return nil, ErrInvalidKey
	}

	hash := m.HashKey(rawKey)
	snap := m.snapshot.Load()
	if snap == nil {
		return nil, ErrInvalidKey
	}

	info, exists := snap.byHash[hash]
	if !exists {
		return nil, ErrInvalidKey
	}

	// Constant-time compare on hash bytes
	if subtle.ConstantTimeCompare([]byte(info.HMACHash), []byte(hash)) != 1 {
		return nil, ErrInvalidKey
	}

	if info.Status == "revoked" || info.RevokedAt > 0 {
		return nil, ErrKeyRevoked
	}

	if info.ExpiresAt > 0 && time.Now().Unix() > info.ExpiresAt {
		return nil, ErrKeyExpired
	}

	return info, nil
}

func (m *Manager) IsModelAllowed(key *KeyInfo, model string) bool {
	if key == nil || len(key.AllowedModels) == 0 {
		return true
	}
	for _, allowed := range key.AllowedModels {
		if allowed == model {
			return true
		}
	}
	return false
}

func (m *Manager) CreateKey(name string, expiresAt int64, allowedModels []string) (*CreateKeyResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}

	// Generate key ID (12 hex chars) and secret (32 bytes = 256 bits)
	idBytes := make([]byte, 6)
	if _, err := rand.Read(idBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random id: %w", err)
	}
	keyID := hex.EncodeToString(idBytes)

	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return nil, fmt.Errorf("failed to generate random secret: %w", err)
	}
	secret := base64.RawURLEncoding.EncodeToString(secretBytes)

	fullKey := fmt.Sprintf("agw_sk_%s_%s", keyID, secret)
	keyPrefix := fmt.Sprintf("agw_sk_%s...", keyID)
	hmacHash := m.HashKey(fullKey)
	now := time.Now().Unix()

	var allowedModelsJSON sql.NullString
	if len(allowedModels) > 0 {
		b, _ := json.Marshal(allowedModels)
		allowedModelsJSON = sql.NullString{String: string(b), Valid: true}
	}

	var expSQL sql.NullInt64
	if expiresAt > 0 {
		expSQL = sql.NullInt64{Int64: expiresAt, Valid: true}
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	_, err := m.db.Exec(`
		INSERT INTO downstream_keys (id, key_prefix, hmac_hash, name, allowed_models, created_at, expires_at, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'active')
	`, keyID, keyPrefix, hmacHash, name, allowedModelsJSON, now, expSQL)
	if err != nil {
		return nil, fmt.Errorf("failed to insert key into db: %w", err)
	}

	if err := m.reloadSnapshot(); err != nil {
		return nil, fmt.Errorf("failed to update key snapshot: %w", err)
	}

	return &CreateKeyResult{
		ID:            keyID,
		Key:           fullKey,
		KeyPrefix:     keyPrefix,
		Name:          name,
		AllowedModels: allowedModels,
		CreatedAt:     now,
		ExpiresAt:     expiresAt,
		Status:        "active",
	}, nil
}

func (m *Manager) ListKeys() []*KeyInfo {
	snap := m.snapshot.Load()
	if snap == nil {
		return []*KeyInfo{}
	}
	result := make([]*KeyInfo, len(snap.all))
	for i, k := range snap.all {
		result[i] = &KeyInfo{
			ID:            k.ID,
			KeyPrefix:     k.KeyPrefix,
			HMACHash:      "", // never expose in public struct
			Name:          k.Name,
			AllowedModels: k.AllowedModels,
			CreatedAt:     k.CreatedAt,
			ExpiresAt:     k.ExpiresAt,
			RevokedAt:     k.RevokedAt,
			Status:        k.Status,
			IsStatic:      k.IsStatic,
		}
	}
	return result
}

func (m *Manager) RevokeKey(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrKeyNotFound
	}

	snap := m.snapshot.Load()
	if snap != nil {
		if k, exists := snap.byID[id]; exists && k.IsStatic {
			return ErrStaticKey
		}
	}

	m.writeMu.Lock()
	defer m.writeMu.Unlock()

	now := time.Now().Unix()
	res, err := m.db.Exec(`
		UPDATE downstream_keys
		SET status = 'revoked', revoked_at = ?
		WHERE id = ?
	`, now, id)
	if err != nil {
		return fmt.Errorf("failed to revoke key in db: %w", err)
	}
	rowsAff, err := res.RowsAffected()
	if err != nil || rowsAff == 0 {
		return ErrKeyNotFound
	}

	return m.reloadSnapshot()
}
