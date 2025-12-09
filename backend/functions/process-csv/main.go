package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	awslambda "github.com/aws/aws-sdk-go/service/lambda"
	_ "github.com/lib/pq"
)

type User struct {
	UserID           string
	Email            string
	MonthlyIncome    float64
	CreditScore      int
	EmploymentStatus string
	Age              int
}

func handler(ctx context.Context, s3Event events.S3Event) error {
	// Process each S3 event record
	for _, record := range s3Event.Records {
		bucket := record.S3.Bucket.Name
		key := record.S3.Object.Key

		log.Printf("Processing file: s3://%s/%s", bucket, key)

		// Download CSV from S3
		csvData, err := downloadFromS3(bucket, key)
		if err != nil {
			log.Printf("Error downloading from S3: %v", err)
			return err
		}

		// Parse CSV
		users, err := parseCSV(csvData)
		if err != nil {
			log.Printf("Error parsing CSV: %v", err)
			return err
		}

		log.Printf("Parsed %d users from CSV", len(users))

		// Save to database
		userCount, err := saveToDatabase(users)
		if err != nil {
			log.Printf("Error saving to database: %v", err)
			return err
		}

		log.Printf("Successfully saved %d users to database", userCount)

		// Trigger matching workflow
		err = triggerMatchingWorkflow(userCount)
		if err != nil {
			log.Printf("Error triggering matching workflow: %v", err)
			// Don't return error - this is a non-critical failure
		}
	}

	return nil
}

func downloadFromS3(bucket, key string) ([]byte, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	svc := s3.New(sess)

	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read object body: %w", err)
	}

	return buf.Bytes(), nil
}

func parseCSV(data []byte) ([]User, error) {
	reader := csv.NewReader(bytes.NewReader(data))

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Create column index map
	colIndex := make(map[string]int)
	for i, col := range header {
		colIndex[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Validate required columns
	requiredCols := []string{"user_id", "email", "monthly_income", "credit_score", "employment_status", "age"}
	for _, col := range requiredCols {
		if _, exists := colIndex[col]; !exists {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	// Parse rows
	var users []User
	lineNum := 1

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading CSV line %d: %v", lineNum, err)
			lineNum++
			continue
		}

		lineNum++

		// Parse user data
		monthlyIncome, err := strconv.ParseFloat(strings.TrimSpace(record[colIndex["monthly_income"]]), 64)
		if err != nil {
			log.Printf("Invalid monthly_income on line %d: %v", lineNum, err)
			continue
		}

		creditScore, err := strconv.Atoi(strings.TrimSpace(record[colIndex["credit_score"]]))
		if err != nil {
			log.Printf("Invalid credit_score on line %d: %v", lineNum, err)
			continue
		}

		age, err := strconv.Atoi(strings.TrimSpace(record[colIndex["age"]]))
		if err != nil {
			log.Printf("Invalid age on line %d: %v", lineNum, err)
			continue
		}

		user := User{
			UserID:           strings.TrimSpace(record[colIndex["user_id"]]),
			Email:            strings.TrimSpace(record[colIndex["email"]]),
			MonthlyIncome:    monthlyIncome,
			CreditScore:      creditScore,
			EmploymentStatus: strings.TrimSpace(record[colIndex["employment_status"]]),
			Age:              age,
		}

		users = append(users, user)
	}

	return users, nil
}

func saveToDatabase(users []User) (int, error) {
	// Build connection string
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	// Connect to database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return 0, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Test connection
	err = db.Ping()
	if err != nil {
		return 0, fmt.Errorf("failed to ping database: %w", err)
	}

	// Begin transaction
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Prepare statement
	stmt, err := tx.Prepare(`
		INSERT INTO users (user_id, email, monthly_income, credit_score, employment_status, age)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id) DO UPDATE SET
			email = EXCLUDED.email,
			monthly_income = EXCLUDED.monthly_income,
			credit_score = EXCLUDED.credit_score,
			employment_status = EXCLUDED.employment_status,
			age = EXCLUDED.age,
			created_at = CURRENT_TIMESTAMP
	`)
	if err != nil {
		return 0, fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	// Insert users
	successCount := 0
	for _, user := range users {
		_, err := stmt.Exec(
			user.UserID,
			user.Email,
			user.MonthlyIncome,
			user.CreditScore,
			user.EmploymentStatus,
			user.Age,
		)
		if err != nil {
			log.Printf("Failed to insert user %s: %v", user.UserID, err)
			continue
		}
		successCount++
	}

	// Commit transaction
	err = tx.Commit()
	if err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return successCount, nil
}

func triggerMatchingWorkflow(userCount int) error {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	lambdaSvc := awslambda.New(sess)

	// Get function name from environment or construct it
	functionName := os.Getenv("TRIGGER_MATCHING_FUNCTION_NAME")
	if functionName == "" {
		// Construct function name based on current function name
		currentFn := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
		parts := strings.Split(currentFn, "-")
		if len(parts) >= 2 {
			functionName = strings.Join(parts[:len(parts)-1], "-") + "-triggerMatching"
		}
	}

	payload := map[string]interface{}{
		"user_count": userCount,
		"timestamp":  "now",
	}
	payloadBytes, _ := json.Marshal(payload)

	_, err = lambdaSvc.Invoke(&awslambda.InvokeInput{
		FunctionName:   aws.String(functionName),
		InvocationType: aws.String("Event"), // Async invocation
		Payload:        payloadBytes,
	})

	if err != nil {
		return fmt.Errorf("failed to invoke matching function: %w", err)
	}

	log.Printf("Successfully triggered matching workflow")
	return nil
}

func main() {
	lambda.Start(handler)
}
