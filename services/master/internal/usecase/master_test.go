package usecase

import (
	"context"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/services/master/internal/domain"
)

// fixture seeds two regions with different prices for the same product, the
// situation the quote must resolve correctly.
type fixture struct {
	svc         *Service
	jakarta     domain.Region
	surabaya    domain.Region
	customer    domain.Customer // in Jakarta
	salesperson domain.Salesperson
	tea         domain.Product
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	svc := NewService(newFakeRepo())

	jkt, err := svc.CreateRegion(ctx, domain.Region{Code: "JKT", Name: "Jakarta"})
	require.NoError(t, err)
	sby, err := svc.CreateRegion(ctx, domain.Region{Code: "SBY", Name: "Surabaya"})
	require.NoError(t, err)
	cust, err := svc.CreateCustomer(ctx, domain.Customer{Code: "C1", Name: "Toko Maju", RegionID: jkt.ID, PaymentTermDays: 30})
	require.NoError(t, err)
	sp, err := svc.CreateSalesperson(ctx, domain.Salesperson{Code: "S1", Name: "Andi", RegionID: jkt.ID})
	require.NoError(t, err)
	tea, err := svc.CreateProduct(ctx, domain.Product{SKU: "TEA-350", Name: "Teh Botol 350ml", BaseUoM: "PCS"})
	require.NoError(t, err)
	_, err = svc.SetConversion(ctx, tea.ID, domain.ProductUoM{UoM: "DUS", Factor: 24})
	require.NoError(t, err)

	for _, p := range []domain.Price{
		{ProductID: tea.ID, RegionID: jkt.ID, UoM: "DUS", Price: decimal.RequireFromString("96000")},
		{ProductID: tea.ID, RegionID: sby.ID, UoM: "DUS", Price: decimal.RequireFromString("92000")},
		{ProductID: tea.ID, RegionID: jkt.ID, UoM: "PCS", Price: decimal.RequireFromString("4200")},
	} {
		_, err := svc.SetPrice(ctx, p)
		require.NoError(t, err)
	}
	return fixture{svc: svc, jakarta: jkt, surabaya: sby, customer: cust, salesperson: sp, tea: tea}
}

func TestQuote_UsesCustomerRegionPrice(t *testing.T) {
	f := newFixture(t)

	q, err := f.svc.Quote(context.Background(), domain.QuoteRequest{
		CustomerID:    f.customer.ID,
		SalespersonID: f.salesperson.ID,
		Lines:         []domain.QuoteLineSpec{{ProductID: f.tea.ID, UoM: "dus"}, {ProductID: f.tea.ID, UoM: "PCS"}},
	})

	require.NoError(t, err)
	require.Len(t, q.Lines, 2)
	assert.Equal(t, "96000", q.Lines[0].UnitPrice.String(), "Jakarta price, not Surabaya's 92000")
	assert.Equal(t, int64(24), q.Lines[0].ConversionFactor)
	assert.Equal(t, "DUS", q.Lines[0].UoM, "uom is normalized")
	assert.Equal(t, "4200", q.Lines[1].UnitPrice.String())
	assert.Equal(t, int64(1), q.Lines[1].ConversionFactor)
	assert.Equal(t, f.customer.RegionID, q.Customer.RegionID)
	assert.Equal(t, 30, q.Customer.PaymentTermDays)
}

func TestQuote_Errors(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(f fixture, r *domain.QuoteRequest)
		wantKind apperr.Kind
		wantCode string
	}{
		{
			name:     "no lines",
			mutate:   func(_ fixture, r *domain.QuoteRequest) { r.Lines = nil },
			wantKind: apperr.KindInvalid, wantCode: "empty_quote",
		},
		{
			name:     "unknown customer",
			mutate:   func(_ fixture, r *domain.QuoteRequest) { r.CustomerID = 999 },
			wantKind: apperr.KindNotFound, wantCode: "customer_not_found",
		},
		{
			name:     "unknown salesperson",
			mutate:   func(_ fixture, r *domain.QuoteRequest) { r.SalespersonID = 999 },
			wantKind: apperr.KindNotFound, wantCode: "salesperson_not_found",
		},
		{
			name: "product not sold in that uom",
			mutate: func(f fixture, r *domain.QuoteRequest) {
				r.Lines = []domain.QuoteLineSpec{{ProductID: f.tea.ID, UoM: "PALLET"}}
			},
			wantKind: apperr.KindInvalid, wantCode: "uom_not_configured",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			req := domain.QuoteRequest{
				CustomerID:    f.customer.ID,
				SalespersonID: f.salesperson.ID,
				Lines:         []domain.QuoteLineSpec{{ProductID: f.tea.ID, UoM: "DUS"}},
			}
			tt.mutate(f, &req)

			_, err := f.svc.Quote(context.Background(), req)

			ae, ok := apperr.As(err)
			require.True(t, ok, "want apperr, got %v", err)
			assert.Equal(t, tt.wantKind, ae.Kind)
			assert.Equal(t, tt.wantCode, ae.Code)
		})
	}
}

func TestQuote_MissingRegionPrice(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	sbyCustomer, err := f.svc.CreateCustomer(ctx, domain.Customer{Code: "C2", Name: "Toko Sby", RegionID: f.surabaya.ID})
	require.NoError(t, err)

	_, err = f.svc.Quote(ctx, domain.QuoteRequest{
		CustomerID:    sbyCustomer.ID,
		SalespersonID: f.salesperson.ID,
		Lines:         []domain.QuoteLineSpec{{ProductID: f.tea.ID, UoM: "PCS"}}, // only Jakarta has a PCS price
	})

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "price_not_set", ae.Code)
}

func TestSetPrice_RequiresConfiguredUoM(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.SetPrice(context.Background(), domain.Price{
		ProductID: f.tea.ID, RegionID: f.jakarta.ID, UoM: "PALLET", Price: decimal.NewFromInt(1),
	})

	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "uom_not_configured", ae.Code)
}

func TestSetPrice_RoundsToCents(t *testing.T) {
	f := newFixture(t)

	got, err := f.svc.SetPrice(context.Background(), domain.Price{
		ProductID: f.tea.ID, RegionID: f.jakarta.ID, UoM: "PCS", Price: decimal.RequireFromString("4200.50"),
	})

	require.NoError(t, err)
	assert.Equal(t, "4200.5", got.Price.String())
}

func TestSetConversion_RejectsChangingBaseFactor(t *testing.T) {
	f := newFixture(t)

	_, err := f.svc.SetConversion(context.Background(), f.tea.ID, domain.ProductUoM{UoM: "PCS", Factor: 12})

	require.Error(t, err)
}

func TestUpdateProduct_KeepsBaseUoM(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	got, err := f.svc.UpdateProduct(ctx, f.tea.ID, domain.Product{SKU: "tea-350b", Name: "Teh Botol 350 ml"})
	require.NoError(t, err)
	assert.Equal(t, "TEA-350B", got.SKU)
	assert.Equal(t, "PCS", got.BaseUoM, "omitted base uom keeps the current one")

	_, err = f.svc.UpdateProduct(ctx, f.tea.ID, domain.Product{SKU: "TEA-350", Name: "Teh", BaseUoM: "DUS"})
	ae, ok := apperr.As(err)
	require.True(t, ok)
	assert.Equal(t, "base_uom_immutable", ae.Code)
}

func TestUpdates_ValidateAndReportMissing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	c, err := f.svc.UpdateCustomer(ctx, f.customer.ID, domain.Customer{Code: "c1", Name: "Toko Maju Jaya", RegionID: f.surabaya.ID, PaymentTermDays: 45})
	require.NoError(t, err)
	assert.Equal(t, f.customer.ID, c.ID)
	assert.Equal(t, 45, c.PaymentTermDays)

	_, err = f.svc.UpdateCustomer(ctx, f.customer.ID, domain.Customer{Code: "C1", Name: "", RegionID: f.jakarta.ID})
	require.Error(t, err, "validation still applies on update")

	_, err = f.svc.UpdateSalesperson(ctx, 999, domain.Salesperson{Code: "S9", Name: "X", RegionID: f.jakarta.ID})
	assert.True(t, apperr.IsNotFound(err))

	_, err = f.svc.UpdateRegion(ctx, 999, domain.Region{Code: "X", Name: "X"})
	assert.True(t, apperr.IsNotFound(err))
}
