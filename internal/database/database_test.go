package database

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type stubPinger struct {
	err error
}

func (p stubPinger) Ping(context.Context) error {
	return p.err
}

func TestConnectRequiresDatabaseURL(t *testing.T) {
	pool, err := Connect(context.Background(), "")

	if !errors.Is(err, ErrMissingDatabaseURL) {
		t.Fatalf("expected ErrMissingDatabaseURL, got %v", err)
	}

	if pool != nil {
		t.Fatal("expected nil pool when database url is empty")
	}
}

func TestPingWrapsDatabaseError(t *testing.T) {
	databaseErr := errors.New("connection refused")

	err := Ping(context.Background(), stubPinger{err: databaseErr})

	if !errors.Is(err, databaseErr) {
		t.Fatalf("expected wrapped database error, got %v", err)
	}

	if !strings.Contains(err.Error(), "ping database") {
		t.Fatalf("expected ping context in error, got %q", err.Error())
	}
}

func TestPingReturnsNilWhenDatabaseResponds(t *testing.T) {
	if err := Ping(context.Background(), stubPinger{}); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}
