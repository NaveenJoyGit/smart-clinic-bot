package rag

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// Indexer embeds content and stores it in clinic_knowledge_chunks.
type Indexer struct {
	pool     *pgxpool.Pool
	embedder Embedder
}

func NewIndexer(pool *pgxpool.Pool, embedder Embedder) *Indexer {
	return &Indexer{pool: pool, embedder: embedder}
}

// IndexDocument generates an embedding for content and upserts the chunk.
func (idx *Indexer) IndexDocument(ctx context.Context, clinicID, sourceType, sourceID, content string, metadata map[string]any) error {
	vec, err := idx.embedder.Embed(ctx, content)
	if err != nil {
		return err
	}
	meta, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	const q = `
INSERT INTO clinic_knowledge_chunks (clinic_id, source_type, source_id, content, metadata, embedding)
VALUES ($1, $2, $3::uuid, $4, $5, $6)
ON CONFLICT DO NOTHING`
	_, err = idx.pool.Exec(ctx, q, clinicID, sourceType, sourceID, content, meta, pgvector.NewVector(vec))
	return err
}
