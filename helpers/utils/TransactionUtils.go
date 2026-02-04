package utils

import (
	"context"
	"database/sql"
)


type TxFn func(*sql.Tx) (interface{}, error)


func WithTransaction(db *sql.DB, fn TxFn) (interface{}, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, NewInternalServerError("Failed to begin transaction", err)
	}

	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p)
		}
	}()

	result, err := fn(tx)
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, NewInternalServerError("Failed to commit transaction", err)
	}

	return result, nil
}


type ContextKey string

const (

	TxKey ContextKey = "transaction"
)


func GetTxFromContext(ctx context.Context) (*sql.Tx, bool) {
	tx, ok := ctx.Value(TxKey).(*sql.Tx)
	return tx, ok
}


func WithTxContext(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, TxKey, tx)
}
