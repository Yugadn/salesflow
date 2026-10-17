//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/db"
	"github.com/yugadn/salesflow/pkg/events"
	"github.com/yugadn/salesflow/pkg/testinfra"
	"github.com/yugadn/salesflow/services/warehouse/internal/domain"
	"github.com/yugadn/salesflow/services/warehouse/internal/repository"
	"github.com/yugadn/salesflow/services/warehouse/internal/usecase"
	"github.com/yugadn/salesflow/services/warehouse/migrations"
)

// TestConcurrentReservationsNeverOversell fires 20 confirmed orders at once
// against stock that covers exactly 10 of them. Half the orders list their
// products in reverse, so if rows were not locked in a fixed order PostgreSQL
// would detect deadlocks and some calls would fail.
func TestConcurrentReservationsNeverOversell(t *testing.T) {
	ctx := context.Background()
	url := testinfra.Postgres(t)
	pool, err := db.Connect(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	require.NoError(t, db.Migrate(url, migrations.FS))

	svc := usecase.NewService(repository.NewPostgres(pool))
	wh, err := svc.CreateWarehouse(ctx, domain.Warehouse{Code: "WH-JKT", Name: "Jakarta", RegionID: 1})
	require.NoError(t, err)
	const productA, productB = 1, 2
	for _, p := range []int64{productA, productB} {
		_, err := svc.ReceiveStock(ctx, wh.ID, p, 100)
		require.NoError(t, err)
	}

	const orders = 20
	var wg sync.WaitGroup
	errs := make(chan error, orders)
	for i := 1; i <= orders; i++ {
		lineA := events.OrderLine{LineNo: 1, ProductID: productA, SKU: "A", UoM: "PCS", ConversionFactor: 1, Qty: 10, BaseQty: 10, UnitPrice: decimal.NewFromInt(1)}
		lineB := events.OrderLine{LineNo: 2, ProductID: productB, SKU: "B", UoM: "PCS", ConversionFactor: 1, Qty: 10, BaseQty: 10, UnitPrice: decimal.NewFromInt(1)}
		lines := []events.OrderLine{lineA, lineB}
		if i%2 == 0 {
			lines = []events.OrderLine{lineB, lineA}
		}
		env, err := events.New("test", events.OrderConfirmed, events.OrderConfirmedPayload{
			OrderID: int64(i), OrderNumber: "SO", CustomerName: "Toko", RegionID: 1, Lines: lines,
		})
		require.NoError(t, err)

		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- svc.HandleEvent(ctx, env)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err, "no reservation may fail with a deadlock or constraint error")
	}

	stocks, err := svc.ListStocks(ctx, wh.ID)
	require.NoError(t, err)
	for _, s := range stocks {
		assert.Equal(t, int64(100), s.OnHand)
		assert.Equal(t, int64(100), s.Reserved, "product %d: reserved exactly what is on hand, never more", s.ProductID)
	}

	deliveries, err := svc.ListDeliveries(ctx, domain.DeliveryFilter{Limit: 200})
	require.NoError(t, err)
	assert.Len(t, deliveries, 10)

	var created, failed int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE event_type = 'delivery.created'),
		        count(*) FILTER (WHERE event_type = 'stock.reservation_failed')
		 FROM outbox`).Scan(&created, &failed))
	assert.Equal(t, 10, created)
	assert.Equal(t, 10, failed)
}
