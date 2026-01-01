package store

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type UsageRecord struct {
	TenantID     string `dynamodbav:"tenant_id"`
	Timestamp    string `dynamodbav:"timestamp"` // ISO8601
	RequestID    string `dynamodbav:"request_id"`
	ModelID      string `dynamodbav:"model_id"`
	InputTokens  int    `dynamodbav:"input_tokens"`
	OutputTokens int    `dynamodbav:"output_tokens"`
}

type UsageStore interface {
	LogUsage(ctx context.Context, record *UsageRecord) error
}

type DynamoDBUsageStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoDBUsageStore(ctx context.Context, region, tableName string) (*DynamoDBUsageStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	return &DynamoDBUsageStore{
		client:    dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}, nil
}

func (s *DynamoDBUsageStore) LogUsage(ctx context.Context, record *UsageRecord) error {
	// Ensure timestamp is set
	if record.Timestamp == "" {
		record.Timestamp = time.Now().Format(time.RFC3339)
	}

	item, err := attributevalue.MarshalMap(record)
	if err != nil {
		return fmt.Errorf("failed to marshal usage record: %w", err)
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
