package domain

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/yugadn/salesflow/pkg/apperr"
)

func TestCustomerValidate(t *testing.T) {
	tests := []struct {
		name    string
		in      Customer
		wantErr bool
	}{
		{"valid", Customer{Code: " c-001 ", Name: "Toko Maju", RegionID: 1, PaymentTermDays: 30}, false},
		{"cash customer", Customer{Code: "C2", Name: "Toko", RegionID: 1, PaymentTermDays: 0}, false},
		{"missing name", Customer{Code: "C3", RegionID: 1}, true},
		{"missing region", Customer{Code: "C4", Name: "Toko"}, true},
		{"negative term", Customer{Code: "C5", Name: "Toko", RegionID: 1, PaymentTermDays: -1}, true},
		{"term over a year", Customer{Code: "C6", Name: "Toko", RegionID: 1, PaymentTermDays: 366}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()
			if tt.wantErr {
				ae, ok := apperr.As(err)
				require.True(t, ok, "want apperr, got %v", err)
				assert.Equal(t, apperr.KindInvalid, ae.Kind)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestCustomerValidateNormalizesCode(t *testing.T) {
	c := Customer{Code: " c-001 ", Name: " Toko Maju ", RegionID: 1}
	require.NoError(t, c.Validate())
	assert.Equal(t, "C-001", c.Code)
	assert.Equal(t, "Toko Maju", c.Name)
}

func TestProductValidateConversion(t *testing.T) {
	p := Product{SKU: "SKU1", Name: "Teh Botol", BaseUoM: "PCS"}
	tests := []struct {
		name    string
		in      ProductUoM
		wantErr bool
	}{
		{"carton holds 24 pieces", ProductUoM{UoM: "dus", Factor: 24}, false},
		{"base uom keeps factor 1", ProductUoM{UoM: "PCS", Factor: 1}, false},
		{"base uom cannot change factor", ProductUoM{UoM: "pcs", Factor: 12}, true},
		{"zero factor", ProductUoM{UoM: "DUS", Factor: 0}, true},
		{"negative factor", ProductUoM{UoM: "DUS", Factor: -24}, true},
		{"missing uom", ProductUoM{UoM: " ", Factor: 24}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.ValidateConversion(&tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestPriceValidate(t *testing.T) {
	tests := []struct {
		name    string
		price   string
		wantErr bool
	}{
		{"whole rupiah", "15000", false},
		{"two decimals", "15000.50", false},
		{"zero is allowed for promos", "0", false},
		{"negative", "-1", true},
		{"three decimals", "15000.505", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pr := Price{ProductID: 1, RegionID: 1, UoM: "pcs", Price: decimal.RequireFromString(tt.price)}
			err := pr.Validate()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "PCS", pr.UoM)
		})
	}
}
