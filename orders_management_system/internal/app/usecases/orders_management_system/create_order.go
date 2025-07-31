package orders_management_system

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/moguchev/microservices_courcse/orders_management_system/internal/app/models"
	postgres_transaction_manager "github.com/moguchev/microservices_courcse/orders_management_system/internal/app/transaction_manager/postgres"
	pkgerrors "github.com/moguchev/microservices_courcse/orders_management_system/pkg/errors"
)

// CreateOrder - создание заказа
func (oms *usecase) CreateOrder(ctx context.Context, userID models.UserID, info CreateOrderInfo) (*models.Order, error) {
	const api = "orders_management_system.usecase.CreateOrder"

	// TODO: ключ идемпотентности

	// comunda, temporal
	//
	// workflow := NewWorkflow()
	// workflow.Add(func())
	// workflow.Add(func())
	// err := workflow.Do()

	// Формируем запись о заказе
	var (
		orderID = models.OrderID(uuid.New())
		order   = &models.Order{
			ID:                orderID,
			UserID:            userID,
			Items:             info.Items,
			DeliveryOrderInfo: info.DeliveryOrderInfo,
		}
	)

	// Варинт 0. - код в разных репозиториях (пакетах). Проблема: как сделать вызовы в одной транзакции?

	/*
		// Создаем заказ в БД
		if err := oms.OrdersStorage.CreateOrder(ctx, order); err != nil {
			return nil, err
		}

		// Удаляем товары из корзины
		if err := oms.CheckoutStorage.DeleteItems(ctx, userID, info.Items); err != nil {
			return nil, err
		}

	*/

	// Варинт 1. - не приемлимый
	/*
		OrdersStorage.CreateOrderAndDeleteItems(ctx, order) {
			tx := conn.Begin()

			tx.Exec("INSERT INTO ... ")
			tx.Exec("DELETE FROM ... ")

			tx.Commit()
		}()
	*/

	// Варинт 2. - компромиссный (допустимый но не жедательный)
	/*
		oms.OrdersStorage.CreateOrderTx(tx pgx.Transcation, args ...)
		oms.CheckoutStorage.DeleteItemsTx(tx pgx.Transcation,)

		tx := oms.Postgres.Begin()

		oms.OrdersStorage.CreateOrderTx(tx, ...)
		oms.CheckoutStorage.DeleteItemsTx(tx, ...)

		tx.Commit()
		//
	*/
	// Резервируем стоки на складах
	if err := oms.WarehouseManagementSystem.ReserveStocks(ctx, userID, info.Items); err != nil {
		return nil, pkgerrors.Wrap(api, err)
	}

	err := oms.TransactionManager.RunTransaction(ctx, func(txCtx context.Context) error { // TRANSANCTION SCOPE
		// Создаем заказ в БД
		if err := oms.OrdersStorage.CreateOrder(txCtx, order); err != nil {
			return err
		}

		// Публикуем сообщение в outbox табличке, которое будет обработона асинхронно позже
		if err := oms.OrdersStorage.CreateOutboxMessage(txCtx, order); err != nil {
			return err
		}

		// Удаляем товары из корзины
		if err := oms.CheckoutStorage.DeleteItems(txCtx, userID, info.Items); err != nil {
			return err
		}

		return nil
	},
		postgres_transaction_manager.WithAccessMode(pgx.ReadWrite),
		postgres_transaction_manager.WithIsoLevel(pgx.ReadCommitted),
		postgres_transaction_manager.WithDeferrableMode(pgx.NotDeferrable),
	)
	if err != nil {
		return nil, pkgerrors.Wrap(api, err)
	}

	// // outbox process
	// go func() {
	// 	for {
	// 		_ = oms.TransactionManager.RunTransaction(ctx, func(txCtx context.Context) error { // TRANSANCTION SCOP
	// 			order, err := oms.OrdersStorage.GetOutboxMessage(txCtx)
	// 			if err != nil {
	// 				return err
	// 			}

	// 			if err := kafka.Produce(order); err != nil {
	// 				return err
	// 			}

	// 			if err := oms.OrdersStorage.DeleteOutboxMessage(txCtx, order); err != nil {
	// 				return err
	// 			}
	// 			return nil
	// 		})
	// 	}
	// }()

	return order, nil
}
