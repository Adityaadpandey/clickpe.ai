package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type Response struct {
	UploadURL string `json:"uploadUrl"`
	FileName  string `json:"fileName"`
	ExpiresIn int64  `json:"expiresIn"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

var corsHeaders = map[string]string{
	"Access-Control-Allow-Origin":  "*",
	"Access-Control-Allow-Headers": "*",
	"Access-Control-Allow-Methods": "POST,OPTIONS",
	"Content-Type":                 "application/json",
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {

	// ---- Handle OPTIONS (CORS Preflight) ----
	if request.HTTPMethod == "OPTIONS" {
		return events.APIGatewayProxyResponse{
			StatusCode: 200,
			Headers:    corsHeaders,
			Body:       "",
		}, nil
	}

	// Get bucket name from environment
	bucketName := os.Getenv("S3_BUCKET")
	if bucketName == "" {
		return errorResponse(500, "S3_BUCKET environment variable not set")
	}

	// Generate unique filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	fileName := fmt.Sprintf("uploads/%s.csv", timestamp)

	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return errorResponse(500, fmt.Sprintf("Failed to create AWS session: %v", err))
	}

	// Create S3 client
	svc := s3.New(sess)

	// Generate presigned URL
	req, _ := svc.PutObjectRequest(&s3.PutObjectInput{
		Bucket:      aws.String(bucketName),
		Key:         aws.String(fileName),
		ContentType: aws.String("text/csv"),
	})

	expiresIn := time.Hour
	urlStr, err := req.Presign(expiresIn)
	if err != nil {
		return errorResponse(500, fmt.Sprintf("Failed to generate presigned URL: %v", err))
	}

	response := Response{
		UploadURL: urlStr,
		FileName:  fileName,
		ExpiresIn: int64(expiresIn.Seconds()),
	}

	responseBody, _ := json.Marshal(response)

	return events.APIGatewayProxyResponse{
		StatusCode: 200,
		Headers:    corsHeaders,
		Body:       string(responseBody),
	}, nil
}

func errorResponse(statusCode int, message string) (events.APIGatewayProxyResponse, error) {
	body, _ := json.Marshal(ErrorResponse{
		Error:   "Error",
		Message: message,
	})

	return events.APIGatewayProxyResponse{
		StatusCode: statusCode,
		Headers:    corsHeaders,
		Body:       string(body),
	}, nil
}

func main() {
	lambda.Start(handler)
}
