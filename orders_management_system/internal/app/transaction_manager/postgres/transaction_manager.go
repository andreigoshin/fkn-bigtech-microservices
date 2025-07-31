package transaction_manager

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/moguchev/microservices_courcse/orders_management_system/internal/app/transaction_manager"
	"github.com/moguchev/microservices_courcse/orders_management_system/pkg/postgres"
)

var (
	_ transaction_manager.TransactionManager = (*TransactionManager)(nil)
)

// TransactionManager - менеджер транзакций: позовляет выполнять функции разных репозиториев ходящих в одну БД в рамках транзакции
type TransactionManager struct {
	connection *postgres.Connection
}

// New constructs TransactionManager
func New(connection *postgres.Connection) *TransactionManager {
	return &TransactionManager{connection: connection}
}

type key string

const (
	txKey key = "tx"
)

func (m *TransactionManager) runTransaction(ctx context.Context, txOpts pgx.TxOptions, fn func(txCtx context.Context) error) (err error) {
	// If it's nested Transaction, skip initiating a new one and return func(ctx context.Context) error
	if _, ok := ctx.Value(txKey).(*postgres.Transaction); ok {
		return fn(ctx)
	}

	// Begin runTransaction
	tx, err := m.connection.BeginTx(ctx, txOpts)
	if err != nil {
		return fmt.Errorf("can't begin transaction: %v", err)
	}

	// Set txKey to context
	txCtx := context.WithValue(ctx, txKey, tx)

	// Set up a defer function for rolling back the runTransaction.
	defer func() {
		// recover from panic
		if r := recover(); r != nil {
			err = fmt.Errorf("panic recovered: %v", r)
		}

		// if func(ctx context.Context) error didn't return error - commit
		if err == nil {
			// if commit returns error -> rollback
			err = tx.Commit(ctx)
			if err != nil {
				err = fmt.Errorf("commit failed: %v", err)
			}
		}

		// rollback on any error
		if err != nil {
			if errRollback := tx.Rollback(ctx); errRollback != nil {
				err = fmt.Errorf("rollback failed: %v", errRollback)
			}
		}
	}()

	// Execute the code inside the runTransaction. If the function
	// fails, return the error and the defer function will roll back or commit otherwise.

	// return error without wrapping errors.Wrap
	err = fn(txCtx)

	return err
}

// GetQueryEngine provides QueryEngine
func (m *TransactionManager) GetQueryEngine(ctx context.Context) QueryEngine {
	// Transaction always runs on node with NodeRoleWrite role
	if tx, ok := ctx.Value(txKey).(QueryEngine); ok {
		return tx
	}

	return m.connection
}

func WithIsoLevel(lvl pgx.TxIsoLevel) transaction_manager.TransactionOption {
	return func(x any) {
		if opts, ok := x.(*pgx.TxOptions); ok {
			opts.IsoLevel = lvl
		}
	}
}

func WithAccessMode(mode pgx.TxAccessMode) transaction_manager.TransactionOption {
	return func(x any) {
		if opts, ok := x.(*pgx.TxOptions); ok {
			opts.AccessMode = mode
		}
	}
}

func WithDeferrableMode(mode pgx.TxDeferrableMode) transaction_manager.TransactionOption {
	return func(x any) {
		if opts, ok := x.(*pgx.TxOptions); ok {
			opts.DeferrableMode = mode
		}
	}
}

var defaultTxOptions = pgx.TxOptions{
	IsoLevel:       pgx.ReadCommitted,
	AccessMode:     ReadWrite,
	DeferrableMode: pgx.Deferrable,
}

// RunReadCommitted execs f func in runTransaction with LevelReadCommitted isolation level
func (m *TransactionManager) RunTransaction(
	ctx context.Context,
	fn func(txCtx context.Context) error,
	opts ...transaction_manager.TransactionOption,
) error {
	txOptions := defaultTxOptions
	for _, opt := range opts {
		opt(&txOptions)
	}

	return m.runTransaction(ctx, txOptions, fn)
}
