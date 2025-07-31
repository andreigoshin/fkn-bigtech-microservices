package orders_storage

import (
	transaction_manager "github.com/moguchev/microservices_courcse/orders_management_system/internal/app/transaction_manager/postgres"
	oms "github.com/moguchev/microservices_courcse/orders_management_system/internal/app/usecases/orders_management_system"
)

// Check that we implemet contract for usecase
var (
	_ oms.OrdersStorage = (*OrdersStorage)(nil)
)

type OrdersStorage struct {
	// connection *postgres.Connection // если тестируте только интеграционными
	// connection Connection // если мокаете базу данных

	driver transaction_manager.QueryEngineProvider
}

// New - returns OrdersStorage
func New( /*connection *postgres.Connection*/ driver transaction_manager.QueryEngineProvider) *OrdersStorage {
	return &OrdersStorage{
		// connection: connection, // было
		driver: driver, // стало
	}
}

const (
	tableOrdersName = "orders"
)
