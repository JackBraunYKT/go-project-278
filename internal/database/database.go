package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrMissingDatabaseURL = errors.New("database url is required")

type Pinger interface {
	Ping(context.Context) error
}

func Connect(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if databaseURL == "" {
		return nil, ErrMissingDatabaseURL
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	if err := Ping(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}

func Ping(ctx context.Context, database Pinger) error {
	if err := database.Ping(ctx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	return nil
}
