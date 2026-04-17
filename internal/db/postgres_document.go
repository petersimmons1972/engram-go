package db

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StoreDocument inserts raw document content into the documents table and
// returns the generated UUID. Used by Tier-2 (>8 MB) ingestion in A4 so the
// full body survives even though the memory itself only carries a synopsis.
func (b *PostgresBackend) StoreDocument(ctx context.Context, project, content string) (string, error) {
	if content == "" {
		return "", fmt.Errorf("StoreDocument: content is empty")
	}
	id := uuid.New().String()
	// Hash via streaming writer so we don't allocate a second full copy of
	// content — matters for Tier-2 bodies up to 50 MB.
	h := sha256.New()
	_, _ = io.WriteString(h, content)
	hash := hex.EncodeToString(h.Sum(nil))
	_, err := b.pool.Exec(ctx, `
		INSERT INTO documents (id, project, content, sha256, size_bytes)
		VALUES ($1, $2, $3, $4, $5)`,
		id, project, content, hash, len(content),
	)
	if err != nil {
		return "", fmt.Errorf("insert document: %w", err)
	}
	return id, nil
}

// GetDocument fetches the raw content for a document ID. Returns "" with no
// error when the document does not exist (callers distinguish via empty value).
func (b *PostgresBackend) GetDocument(ctx context.Context, id string) (string, error) {
	var content string
	err := b.pool.QueryRow(ctx,
		`SELECT content FROM documents WHERE id = $1`, id,
	).Scan(&content)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get document: %w", err)
	}
	return content, nil
}

// SetMemoryDocumentID links a memory to a previously stored document.
// Separated from StoreMemory so a Tier-2 write can run: StoreDocument →
// Engine.Store(memory) → SetMemoryDocumentID(link), keeping the existing
// memory-store transaction simple.
func (b *PostgresBackend) SetMemoryDocumentID(ctx context.Context, memoryID, documentID string) error {
	_, err := b.pool.Exec(ctx,
		`UPDATE memories SET document_id = $1 WHERE id = $2`,
		documentID, memoryID,
	)
	if err != nil {
		return fmt.Errorf("set memory document_id: %w", err)
	}
	return nil
}
