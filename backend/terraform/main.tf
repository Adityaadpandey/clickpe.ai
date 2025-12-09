terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# S3 Bucket for CSV uploads
resource "aws_s3_bucket" "csv_uploads" {
  bucket_prefix = "loan-eligibility-engine-"
  force_destroy = var.environment == "dev" ? true : false

  tags = {
    Name        = "loan-eligibility-engine-${var.environment}"
    Environment = var.environment
  }
}

resource "aws_s3_bucket_cors_configuration" "csv_uploads" {
  bucket = aws_s3_bucket.csv_uploads.id

  cors_rule {
    allowed_headers = ["*"]
    allowed_methods = ["GET", "PUT", "POST", "DELETE", "HEAD"]
    allowed_origins = ["*"]
    expose_headers  = ["ETag"]
    max_age_seconds = 3000
  }
}

resource "aws_s3_bucket_versioning" "csv_uploads" {
  bucket = aws_s3_bucket.csv_uploads.id

  versioning_configuration {
    status = "Enabled"
  }
}

# RDS PostgreSQL Database
resource "aws_db_subnet_group" "postgres" {
  name       = "loan-eligibility-${var.environment}"
  subnet_ids = var.subnet_ids

  tags = {
    Name        = "loan-eligibility-${var.environment}"
    Environment = var.environment
  }
}

resource "aws_security_group" "postgres" {
  name_prefix = "loan-eligibility-db-${var.environment}-"
  vpc_id      = var.vpc_id

# don't forget to chage this before going on prod => just the s3 havging the access

  ingress {
    from_port   = 5432
    to_port     = 5432
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  tags = {
    Name        = "loan-eligibility-db-${var.environment}"
    Environment = var.environment
  }
}

resource "aws_db_instance" "postgres" {
  identifier     = "loan-eligibility-${var.environment}"
  engine         = "postgres"
  engine_version = var.postgres_version
  instance_class = var.db_instance_class

  allocated_storage     = var.db_allocated_storage
  max_allocated_storage = var.db_max_allocated_storage
  storage_encrypted     = true

  db_name  = var.db_name
  username = var.db_username
  password = var.db_password

  db_subnet_group_name   = aws_db_subnet_group.postgres.name
  vpc_security_group_ids = [aws_security_group.postgres.id]

  publicly_accessible = var.db_publicly_accessible
  skip_final_snapshot = var.environment == "dev" ? true : false

  backup_retention_period = var.environment == "prod" ? 7 : 1
  backup_window           = "03:00-04:00"
  maintenance_window      = "mon:04:00-mon:05:00"

  tags = {
    Name        = "loan-eligibility-${var.environment}"
    Environment = var.environment
  }
}
