package store

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

type Model struct {
	ModelID      string   `dynamodbav:"model_id"`
	ProviderName string   `dynamodbav:"provider_name"`
	BaseURLs     []string `dynamodbav:"base_urls"`
	APIKeyEnv    string   `dynamodbav:"api_key_env"`
}

type ModelStore interface {
	GetModel(ctx context.Context, modelID string) (*Model, error)
}

type DynamoDBModelStore struct {
	client    *dynamodb.Client
	tableName string
}

func NewDynamoDBModelStore(ctx context.Context, region, tableName string) (*DynamoDBModelStore, error) {
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, err
	}

	return &DynamoDBModelStore{
		client:    dynamodb.NewFromConfig(cfg),
		tableName: tableName,
	}, nil
}

func (s *DynamoDBModelStore) GetModel(ctx context.Context, modelID string) (*Model, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"model_id": &types.AttributeValueMemberS{Value: modelID},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get model from DynamoDB: %w", err)
	}

	if out.Item == nil {
		return nil, nil // Not found
	}

	var model Model
	err = attributevalue.UnmarshalMap(out.Item, &model)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal model: %w", err)
	}

	return &model, nil
}
