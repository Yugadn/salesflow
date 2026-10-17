// Package client calls master-service over HTTP.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yugadn/salesflow/pkg/apperr"
	"github.com/yugadn/salesflow/services/order/internal/domain"
)

type Master struct {
	baseURL string
	http    *http.Client
}

func NewMaster(baseURL string) *Master {
	return &Master{baseURL: baseURL, http: &http.Client{Timeout: 5 * time.Second}}
}

type quoteRequest struct {
	CustomerID    int64           `json:"customer_id"`
	SalespersonID int64           `json:"salesperson_id"`
	Lines         []quoteLineSpec `json:"lines"`
}

type quoteLineSpec struct {
	ProductID int64  `json:"product_id"`
	UoM       string `json:"uom"`
}

type response struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// Quote asks master-service to price lines for the customer's region.
// A 4xx from master means the order references bad data, which is the
// caller's fault, so it is surfaced as a validation error.
func (m *Master) Quote(ctx context.Context, customerID, salespersonID int64, lines []domain.LineRequest) (domain.Quote, error) {
	req := quoteRequest{CustomerID: customerID, SalespersonID: salespersonID}
	for _, l := range lines {
		req.Lines = append(req.Lines, quoteLineSpec{ProductID: l.ProductID, UoM: l.UoM})
	}
	body, err := json.Marshal(req)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("encode quote request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, m.baseURL+"/internal/quote", bytes.NewReader(body))
	if err != nil {
		return domain.Quote{}, fmt.Errorf("build quote request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := m.http.Do(httpReq)
	if err != nil {
		return domain.Quote{}, fmt.Errorf("call master-service: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var out response
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return domain.Quote{}, fmt.Errorf("decode master-service response (status %d): %w", resp.StatusCode, err)
	}

	switch {
	case resp.StatusCode == http.StatusOK:
		var q domain.Quote
		if err := json.Unmarshal(out.Data, &q); err != nil {
			return domain.Quote{}, fmt.Errorf("decode quote: %w", err)
		}
		return q, nil
	case resp.StatusCode >= 400 && resp.StatusCode < 500 && out.Error != nil:
		return domain.Quote{}, apperr.Invalid(out.Error.Code, "%s", out.Error.Message)
	default:
		return domain.Quote{}, fmt.Errorf("master-service returned status %d", resp.StatusCode)
	}
}
