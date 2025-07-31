package transaction_manager

import "context"

type (
	// TransactionOption
	TransactionOption func(x any)

	// TransactionManager
	TransactionManager interface {
		RunTransaction(ctx context.Context, f func(txCtx context.Context) error, opts ...TransactionOption) error
	}
)
