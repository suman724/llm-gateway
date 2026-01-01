resource "aws_dynamodb_table" "tenants" {
  name           = "LLMGateway_Tenants"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "api_key"

  attribute {
    name = "api_key"
    type = "S"
  }

  tags = {
    Environment = "prod"
  }
}

resource "aws_dynamodb_table" "models" {
  name           = "LLMGateway_Models"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "model_id"

  attribute {
    name = "model_id"
    type = "S"
  }

  tags = {
    Environment = "prod"
  }
}

resource "aws_dynamodb_table" "usage_logs" {
  name           = "LLMGateway_UsageLogs"
  billing_mode   = "PAY_PER_REQUEST"
  hash_key       = "tenant_id"
  range_key      = "timestamp"

  attribute {
    name = "tenant_id"
    type = "S"
  }

  attribute {
    name = "timestamp"
    type = "S"
  }

  tags = {
    Environment = "prod"
  }
}

resource "aws_elasticache_subnet_group" "redis" {
  name       = "${var.app_name}-redis-subnet"
  subnet_ids = module.vpc.private_subnets
}

resource "aws_elasticache_replication_group" "redis" {
  replication_group_id       = "${var.app_name}-redis"
  description                = "Redis for Rate Limiting"
  node_type                  = "cache.t3.micro"
  num_cache_clusters         = 1
  port                       = 6379
  parameter_group_name       = "default.redis7"
  automatic_failover_enabled = false
  subnet_group_name          = aws_elasticache_subnet_group.redis.name
  security_group_ids         = [aws_security_group.redis_sg.id]

  at_rest_encryption_enabled = true
  transit_encryption_enabled = false # Simplicity for internal VPC
}
