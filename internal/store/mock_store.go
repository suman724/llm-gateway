package store

import (
	"context"
	"errors"
)

// MockTenantStore
type MockTenantStore struct {
	Tenants map[string]*Tenant
}

func (m *MockTenantStore) GetTenant(ctx context.Context, apiKey string) (*Tenant, error) {
	if t, ok := m.Tenants[apiKey]; ok {
		if !t.IsActive {
			return nil, errors.New("tenant is not active")
		}
		return t, nil
	}
	return nil, nil // Not found
}

func (m *MockTenantStore) CreateTenant(ctx context.Context, tenant *Tenant) error {
	m.Tenants[tenant.APIKey] = tenant
	return nil
}

// MockRateLimitStore
type MockRateLimitStore struct {
	RPM map[string]int64
	TPM map[string]int64
	// Allow forcing errors for testing
	Err error
}

func (m *MockRateLimitStore) IncrementRPM(ctx context.Context, tenantID string) (int64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	m.RPM[tenantID]++
	return m.RPM[tenantID], nil
}

func (m *MockRateLimitStore) IncrementTPM(ctx context.Context, tenantID string, tokens int) (int64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	// Note: We don't have separate input/output maps here, just total per tenant
	// If needed we can expand. For basic TPM limit test, this is enough.
	m.TPM[tenantID] += int64(tokens)
	return m.TPM[tenantID], nil
}

func (m *MockRateLimitStore) GetTPM(ctx context.Context, tenantID string) (int64, error) {
	if m.Err != nil {
		return 0, m.Err
	}
	return m.TPM[tenantID], nil
}

// Helper to easy init
func NewMockTenantStore() *MockTenantStore {
	return &MockTenantStore{Tenants: make(map[string]*Tenant)}
}

func NewMockRateLimitStore() *MockRateLimitStore {
	return &MockRateLimitStore{
		RPM: make(map[string]int64),
		TPM: make(map[string]int64),
	}
}

// MockUsageStore
type MockUsageStore struct {
	Records []*UsageRecord
}

func (m *MockUsageStore) LogUsage(ctx context.Context, record *UsageRecord) error {
	m.Records = append(m.Records, record)
	return nil
}

// MockModelStore
type MockModelStore struct {
	Models map[string]*Model
}

func (m *MockModelStore) GetModel(ctx context.Context, modelID string) (*Model, error) {
	if model, ok := m.Models[modelID]; ok {
		return model, nil
	}
	return nil, errors.New("model not found")
}
