.PHONY: build clean deploy deploy-prod

build:
	@echo "Building Go Lambda functions..."
	@mkdir -p dist

	# Generate Presigned URL
	@echo "-> generate-presigned-url"
	cd functions/generate-presigned-url && \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap main.go
	cd functions/generate-presigned-url && \
		zip ../../dist/generatePresignedUrl.zip bootstrap && \
		rm bootstrap

	# Process CSV
	@echo "-> process-csv"
	cd functions/process-csv && \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap main.go
	cd functions/process-csv && \
		zip ../../dist/processCsv.zip bootstrap && \
		rm bootstrap

	# Trigger Matching
	@echo "-> trigger-matching"
	cd functions/trigger-matching && \
		GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o bootstrap main.go
	cd functions/trigger-matching && \
		zip ../../dist/triggerMatching.zip bootstrap && \
		rm bootstrap

	@echo "Build complete!"

clean:
	@echo "Cleaning build artifacts..."
	rm -rf dist
	find functions -name "bootstrap" -type f -delete
	@echo "Clean complete!"

deploy: 
	@echo "Building functions..."
	@$(MAKE) build
	@echo "Deploying..."
	serverless deploy --stage dev
	@echo "Deployment complete!"

deploy-prod: build
	@echo "Deploying (prod)..."
	serverless deploy --stage prod
	@echo "Production deployment complete!"
