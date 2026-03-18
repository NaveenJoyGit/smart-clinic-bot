package db

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgxvector "github.com/pgvector/pgvector-go/pgx"
)

func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	// Bootstrap: ensure the vector extension exists before creating the pool.
	// pgxvector.RegisterTypes (called in AfterConnect) requires the extension
	// to already be installed, but migrations run after Connect returns.
	boot, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, err
	}
	_, err = boot.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS vector")
	boot.Close(ctx)
	if err != nil {
		return nil, err
	}

	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	cfg.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		return pgxvector.RegisterTypes(ctx, conn)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
