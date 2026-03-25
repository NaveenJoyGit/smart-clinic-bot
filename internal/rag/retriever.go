package rag

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// Document is a retrieved knowledge-base chunk.
type Document struct {
	ID          string
	SourceType  string
	Content     string
	Metadata    []byte // raw JSONB
}

// Retriever performs vector similarity search against clinic_knowledge_chunks.
type Retriever struct {
	pool     *pgxpool.Pool
	embedder Embedder
	logger   *slog.Logger
}

// NewRetriever constructs a Retriever.
func NewRetriever(pool *pgxpool.Pool, embedder Embedder, logger *slog.Logger) *Retriever {
	return &Retriever{pool: pool, embedder: embedder, logger: logger}
}

// Search returns the topK most similar knowledge chunks for the given clinic.
func (r *Retriever) Search(ctx context.Context, clinicID, query string, topK int) ([]Document, error) {
	r.logger.DebugContext(ctx, "rag search", "clinic_id", clinicID, "top_k", topK)
	embedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
		r.logger.WarnContext(ctx, "rag embed failed", "clinic_id", clinicID, "error", err)
		return nil, err
	}

	const q = `
SELECT id::text, source_type, content, metadata
FROM clinic_knowledge_chunks
WHERE clinic_id = $1
  AND embedding IS NOT NULL
ORDER BY embedding <=> $2
LIMIT $3`

	rows, err := r.pool.Query(ctx, q, clinicID, pgvector.NewVector(embedding), topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var docs []Document
	for rows.Next() {
		var d Document
		if err := rows.Scan(&d.ID, &d.SourceType, &d.Content, &d.Metadata); err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	r.logger.DebugContext(ctx, "rag search complete", "clinic_id", clinicID, "docs_found", len(docs))
	return docs, nil
}
