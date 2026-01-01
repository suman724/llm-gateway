package config

import (
	"os"
	"time"
)

type Config struct {
	ServerPort        string
	AWSRegion         string
	DynamoDBTableName string
	RedisAddr         string
	RedisPassword     string
	LLMTimeout        time.Duration
}

func LoadConfig() *Config {
	timeoutStr := getEnv("LLM_TIMEOUT", "60s")
	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		timeout = 60 * time.Second
	}

	return &Config{
		ServerPort:        getEnv("SERVER_PORT", "8080"),
		AWSRegion:         getEnv("AWS_REGION", "us-east-1"),
		DynamoDBTableName: getEnv("DYNAMODB_TABLE_NAME", "LLMGateway_Tenants"),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		RedisPassword:     getEnv("REDIS_PASSWORD", ""),
		LLMTimeout:        timeout,
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
