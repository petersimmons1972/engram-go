package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/petersimmons1972/engram/internal/atom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type atomExecerStub struct {
	tag pgconn.CommandTag
}

func (s atomExecerStub) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return s.tag, nil
}

func TestInsertAtomReportsIDConflict(t *testing.T) {
	candidate := atom.Atom{ID: "candidate"}

	inserted, err := insertAtom(
		context.Background(),
		atomExecerStub{tag: pgconn.NewCommandTag("INSERT 0 0")},
		&candidate,
	)

	require.NoError(t, err)
	assert.False(t, inserted, "supersession must distinguish a conflict from a new linked row")
}
