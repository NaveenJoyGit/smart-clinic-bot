package rag

import (
	"context"

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
}

// NewRetriever constructs a Retriever.
func NewRetriever(pool *pgxpool.Pool, embedder Embedder) *Retriever {
	return &Retriever{pool: pool, embedder: embedder}
}

// Search returns the topK most similar knowledge chunks for the given clinic.
func (r *Retriever) Search(ctx context.Context, clinicID, query string, topK int) ([]Document, error) {
	embedding, err := r.embedder.Embed(ctx, query)
	if err != nil {
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
	return docs, rows.Err()
}
