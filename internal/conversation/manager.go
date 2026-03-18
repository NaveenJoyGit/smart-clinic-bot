package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/naveenjoy/smart-clinic-bot/internal/ai"
	"github.com/redis/go-redis/v9"
)

const (
	cacheTTL  = 24 * time.Hour
	maxTurns  = 10
)

// Manager handles conversation persistence across Redis and PostgreSQL.
type Manager struct {
	pool  *pgxpool.Pool
	redis *redis.Client
}

// NewManager constructs a Manager.
func NewManager(pool *pgxpool.Pool, rdb *redis.Client) *Manager {
	return &Manager{pool: pool, redis: rdb}
}

func cacheKey(tenantID, platform, senderID string) string {
	return fmt.Sprintf("conv:%s:%s:%s", tenantID, platform, senderID)
}

// GetHistory returns the conversation history (oldest first).
// It first checks Redis; on a miss it falls back to PostgreSQL.
func (m *Manager) GetHistory(ctx context.Context, tenantID, platform, senderID string) ([]ai.Message, error) {
	key := cacheKey(tenantID, platform, senderID)

	data, err := m.redis.Get(ctx, key).Bytes()
	if err == nil {
		var msgs []ai.Message
		if err := json.Unmarshal(data, &msgs); err == nil {
			return msgs, nil
		}
	}

	// Cache miss — load from DB.
	const q = `
SELECT m.role, m.content
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE c.clinic_id = $1 AND c.platform = $2 AND c.external_id = $3
ORDER BY m.created_at DESC
LIMIT $4`

	rows, err := m.pool.Query(ctx, q, tenantID, platform, senderID, maxTurns*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ai.Message
	for rows.Next() {
		var msg ai.Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Reverse to get oldest-first order.
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}

	_ = m.writeCache(ctx, key, msgs)
	return msgs, nil
}

// AppendMessage upserts the conversation and inserts a new message, then refreshes the cache.
func (m *Manager) AppendMessage(ctx context.Context, tenantID, platform, senderID, role, content string) error {
	const upsertConv = `
INSERT INTO conversations (clinic_id, platform, external_id)
VALUES ($1, $2, $3)
ON CONFLICT (clinic_id, platform, external_id) DO UPDATE SET clinic_id = EXCLUDED.clinic_id
RETURNING id`

	var convID string
	if err := m.pool.QueryRow(ctx, upsertConv, tenantID, platform, senderID).Scan(&convID); err != nil {
		return err
	}

	const insertMsg = `INSERT INTO messages (conversation_id, role, content) VALUES ($1, $2, $3)`
	if _, err := m.pool.Exec(ctx, insertMsg, convID, role, content); err != nil {
		return err
	}

	// Refresh cache.
	history, err := m.loadFromDB(ctx, tenantID, platform, senderID)
	if err != nil {
		return err
	}
	return m.writeCache(ctx, cacheKey(tenantID, platform, senderID), history)
}

func (m *Manager) loadFromDB(ctx context.Context, tenantID, platform, senderID string) ([]ai.Message, error) {
	const q = `
SELECT m.role, m.content
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE c.clinic_id = $1 AND c.platform = $2 AND c.external_id = $3
ORDER BY m.created_at DESC
LIMIT $4`

	rows, err := m.pool.Query(ctx, q, tenantID, platform, senderID, maxTurns*2)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ai.Message
	for rows.Next() {
		var msg ai.Message
		if err := rows.Scan(&msg.Role, &msg.Content); err != nil {
			return nil, err
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (m *Manager) writeCache(ctx context.Context, key string, msgs []ai.Message) error {
	data, err := json.Marshal(msgs)
	if err != nil {
		return err
	}
	return m.redis.Set(ctx, key, data, cacheTTL).Err()
}
