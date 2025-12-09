
1. Copy the example variables file:
```bash
cp terraform.tfvars.example terraform.tfvars
```

2. Edit `terraform.tfvars` with your values:
   - Get VPC ID: `aws ec2 describe-vpcs --region us-east-1`
   - Get Subnet IDs: `aws ec2 describe-subnets --region us-east-1`
   - Set a strong database password

3. Initialize Terraform:
```bash
terraform init
```

4. Review the plan:
```bash
terraform plan
```

5. Apply the configuration:
```bash
terraform apply
```

6. Get the outputs:
```bash
terraform output
```

## Update .env file

After applying, update your `.env` file with the outputs:

```bash
# Get outputs
terraform output -raw db_host > /tmp/db_host.txt
terraform output -raw s3_bucket_name > /tmp/s3_bucket.txt

# Update .env
DB_HOST=$(terraform output -raw db_host)
S3_BUCKET=$(terraform output -raw s3_bucket_name)
```

## Update serverless.yml

Update the `bucketName` in `serverless.yml`:
```yaml
custom:
  bucketName: <output from terraform output s3_bucket_name>
```

## Destroy Infrastructure

To tear down everything:
```bash
terraform destroy
```

## Production Setup

For production:
1. Change `environment = "prod"` in terraform.tfvars
2. Set `db_publicly_accessible = false`
3. Use a larger instance class like `db.t3.small` or `db.t3.medium`
4. Enable automated backups with longer retention
