package store

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Tenant struct {
	APIKey        string   `dynamodbav:"api_key"`
	TenantID      string   `dynamodbav:"tenant_id"`
	Name          string   `dynamodbav:"name"`
	RPMLimit      int      `dynamodbav:"rpm_limit"`
	TPMLimit      int      `dynamodbav:"tpm_limit"`
	AllowedModels []string `dynamodbav:"allowed_models"`
	IsActive      bool     `dynamodbav:"is_active"`
}

type TenantStore interface {
	GetTenant(ctx context.Context, apiKey string) (*Tenant, error)
	CreateTenant(ctx context.Context, tenant *Tenant) error
}

type cachedTenant struct {
	tenant    *Tenant
	expiresAt time.Time
}

type DynamoDBTenantStore struct {
	client    *dynamodb.Client
	tableName string
	cache     map[string]cachedTenant
	mu        sync.RWMutex
}

func NewDynamoDBTenantStore(ctx context.Context, region, tableName string) (*DynamoDBTenantStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	return &DynamoDBTenantStore{
		client:    dynamodb.NewFromConfig(cfg),
		tableName: tableName,
		cache:     make(map[string]cachedTenant),
	}, nil
}

func (s *DynamoDBTenantStore) GetTenant(ctx context.Context, apiKey string) (*Tenant, error) {
	// 1. Check Cache
	s.mu.RLock()
	entry, found := s.cache[apiKey]
	s.mu.RUnlock()

	if found && time.Now().Before(entry.expiresAt) {
		return entry.tenant, nil
	}

	// 2. Fetch from DynamoDB
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"api_key": &types.AttributeValueMemberS{Value: apiKey},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get item from DynamoDB: %w", err)
	}

	if out.Item == nil {
		return nil, nil // Not found
	}

	var tenant Tenant
	err = attributevalue.UnmarshalMap(out.Item, &tenant)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal tenant: %w", err)
	}

	if !tenant.IsActive {
		return nil, fmt.Errorf("tenant is not active")
	}

	// 3. Apply Defaults if missing
	if tenant.RPMLimit == 0 {
		tenant.RPMLimit = 100 // Default RPM
	}
	if tenant.TPMLimit == 0 {
		tenant.TPMLimit = 100000 // Default (100k TPM)
	}

	// 4. Update Cache (60m TTL)
	s.mu.Lock()
	s.cache[apiKey] = cachedTenant{
		tenant:    &tenant,
		expiresAt: time.Now().Add(60 * time.Minute),
	}
	s.mu.Unlock()

	return &tenant, nil
}

func (s *DynamoDBTenantStore) CreateTenant(ctx context.Context, tenant *Tenant) error {
	item, err := attributevalue.MarshalMap(tenant)
	if err != nil {
		return fmt.Errorf("failed to marshal tenant: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to put item to DynamoDB: %w", err)
	}
	return nil
}
