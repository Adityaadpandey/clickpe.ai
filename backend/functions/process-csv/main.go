package main

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	awslambda "github.com/aws/aws-sdk-go/service/lambda"
	_ "github.com/lib/pq"
)

const (
	// Batch size for database inserts - PostgreSQL can handle large batches
	BatchSize = 5000

	// Number of worker goroutines for parsing
	NumWorkers = 4

	// Channel buffer size
	ChannelBuffer = 1000
)

type User struct {
	UserID           string
	Email            string
	MonthlyIncome    float64
	CreditScore      int
	EmploymentStatus string
	Age              int
}

type ParsedBatch struct {
	Users []User
	Error error
}

func handler(ctx context.Context, s3Event events.S3Event) error {
	startTime := time.Now()

	for _, record := range s3Event.Records {
		bucket := record.S3.Bucket.Name
		key := record.S3.Object.Key

		log.Printf("Processing file: s3://%s/%s", bucket, key)

		// Process CSV with streaming approach
		userCount, err := processCSVStreaming(ctx, bucket, key)
		if err != nil {
			log.Printf("Error processing CSV: %v", err)
			return err
		}

		log.Printf("Successfully processed %d users in %v", userCount, time.Since(startTime))

		// Trigger matching workflow asynchronously
		go func() {
			if err := triggerMatchingWorkflow(userCount); err != nil {
				log.Printf("Error triggering matching workflow: %v", err)
			}
		}()
	}

	return nil
}

// processCSVStreaming streams CSV from S3, parses with workers, and batch inserts to DB
func processCSVStreaming(ctx context.Context, bucket, key string) (int, error) {
	// Create S3 session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to create AWS session: %w", err)
	}

	svc := s3.New(sess)

	// Stream object from S3
	result, err := svc.GetObject(&s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer result.Body.Close()

	// Create database connection pool
	db, err := createDBPool()
	if err != nil {
		return 0, err
	}
	defer db.Close()

	// Setup channels for pipeline
	rowsChan := make(chan []string, ChannelBuffer)
	usersChan := make(chan User, ChannelBuffer)
	errorsChan := make(chan error, NumWorkers)

	// Parse CSV header
	reader := csv.NewReader(bufio.NewReaderSize(result.Body, 256*1024)) // 256KB buffer
	header, err := reader.Read()
	if err != nil {
		return 0, fmt.Errorf("failed to read CSV header: %w", err)
	}

	colIndex := createColumnIndex(header)
	if err := validateColumns(colIndex); err != nil {
		return 0, err
	}

	// Start worker pool for parsing
	var wg sync.WaitGroup
	for i := 0; i < NumWorkers; i++ {
		wg.Add(1)
		go parseWorker(&wg, rowsChan, usersChan, errorsChan, colIndex)
	}

	// Start batch inserter
	insertDone := make(chan int)
	go batchInserter(ctx, db, usersChan, insertDone)

	// Read CSV rows and distribute to workers
	go func() {
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

			// Send to worker
			select {
			case rowsChan <- record:
			case <-ctx.Done():
				close(rowsChan)
				return
			}

			lineNum++
		}
		close(rowsChan)
	}()

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(usersChan)
		close(errorsChan)
	}()

	// Check for parsing errors
	for err := range errorsChan {
		if err != nil {
			log.Printf("Worker error: %v", err)
		}
	}

	// Wait for insertion to complete and get count
	totalInserted := <-insertDone

	return totalInserted, nil
}

func createColumnIndex(header []string) map[string]int {
	colIndex := make(map[string]int, len(header))
	for i, col := range header {
		colIndex[strings.ToLower(strings.TrimSpace(col))] = i
	}
	return colIndex
}

func validateColumns(colIndex map[string]int) error {
	requiredCols := []string{"user_id", "email", "monthly_income", "credit_score", "employment_status", "age"}
	for _, col := range requiredCols {
		if _, exists := colIndex[col]; !exists {
			return fmt.Errorf("missing required column: %s", col)
		}
	}
	return nil
}

// parseWorker processes CSV rows concurrently
func parseWorker(wg *sync.WaitGroup, rowsChan <-chan []string, usersChan chan<- User, errorsChan chan<- error, colIndex map[string]int) {
	defer wg.Done()

	for record := range rowsChan {
		user, err := parseUserRecord(record, colIndex)
		if err != nil {
			// Skip invalid records, don't block on errors
			continue
		}
		usersChan <- user
	}
}

func parseUserRecord(record []string, colIndex map[string]int) (User, error) {
	monthlyIncome, err := strconv.ParseFloat(strings.TrimSpace(record[colIndex["monthly_income"]]), 64)
	if err != nil {
		return User{}, err
	}

	creditScore, err := strconv.Atoi(strings.TrimSpace(record[colIndex["credit_score"]]))
	if err != nil {
		return User{}, err
	}

	age, err := strconv.Atoi(strings.TrimSpace(record[colIndex["age"]]))
	if err != nil {
		return User{}, err
	}

	return User{
		UserID:           strings.TrimSpace(record[colIndex["user_id"]]),
		Email:            strings.TrimSpace(record[colIndex["email"]]),
		MonthlyIncome:    monthlyIncome,
		CreditScore:      creditScore,
		EmploymentStatus: strings.TrimSpace(record[colIndex["employment_status"]]),
		Age:              age,
	}, nil
}

// batchInserter collects users and inserts in batches
func batchInserter(ctx context.Context, db *sql.DB, usersChan <-chan User, done chan<- int) {
	batch := make([]User, 0, BatchSize)
	totalInserted := 0

	insertBatch := func() {
		if len(batch) == 0 {
			return
		}

		count, err := bulkInsert(ctx, db, batch)
		if err != nil {
			log.Printf("Error inserting batch: %v", err)
		} else {
			totalInserted += count
		}

		batch = batch[:0] // Reset batch
	}

	for user := range usersChan {
		batch = append(batch, user)

		if len(batch) >= BatchSize {
			insertBatch()
		}
	}

	// Insert remaining users
	insertBatch()

	done <- totalInserted
}

// bulkInsert uses PostgreSQL COPY or multi-row INSERT for efficiency
func bulkInsert(ctx context.Context, db *sql.DB, users []User) (int, error) {
	if len(users) == 0 {
		return 0, nil
	}

	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Build multi-row INSERT statement
	var valueStrings []string
	var valueArgs []interface{}

	for i, user := range users {
		valueStrings = append(valueStrings, fmt.Sprintf("($%d, $%d, $%d, $%d, $%d, $%d)",
			i*6+1, i*6+2, i*6+3, i*6+4, i*6+5, i*6+6))

		valueArgs = append(valueArgs,
			user.UserID,
			user.Email,
			user.MonthlyIncome,
			user.CreditScore,
			user.EmploymentStatus,
			user.Age,
		)
	}

	query := fmt.Sprintf(`
		INSERT INTO users (user_id, email, monthly_income, credit_score, employment_status, age)
		VALUES %s
		ON CONFLICT (user_id) DO UPDATE SET
			email = EXCLUDED.email,
			monthly_income = EXCLUDED.monthly_income,
			credit_score = EXCLUDED.credit_score,
			employment_status = EXCLUDED.employment_status,
			age = EXCLUDED.age,
			updated_at = CURRENT_TIMESTAMP
	`, strings.Join(valueStrings, ","))

	_, err = tx.ExecContext(ctx, query, valueArgs...)
	if err != nil {
		return 0, fmt.Errorf("failed to execute bulk insert: %w", err)
	}

	// Commit transaction
	if err = tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return len(users), nil
}

func createDBPool() (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool for high throughput
	maxConns := runtime.NumCPU() * 2
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(1 * time.Minute)

	// Test connection
	if err = db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

func triggerMatchingWorkflow(userCount int) error {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	lambdaSvc := awslambda.New(sess)

	functionName := os.Getenv("TRIGGER_MATCHING_FUNCTION_NAME")
	if functionName == "" {
		currentFn := os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
		parts := strings.Split(currentFn, "-")
		if len(parts) >= 2 {
			functionName = strings.Join(parts[:len(parts)-1], "-") + "-triggerMatching"
		}
	}

	payload := map[string]interface{}{
		"user_count": userCount,
		"timestamp":  time.Now().UTC().Format(time.RFC3339),
	}
	payloadBytes, _ := json.Marshal(payload)

	_, err = lambdaSvc.Invoke(&awslambda.InvokeInput{
		FunctionName:   aws.String(functionName),
		InvocationType: aws.String("Event"),
		Payload:        payloadBytes,
	})

	if err != nil {
		return fmt.Errorf("failed to invoke matching function: %w", err)
	}

	log.Printf("Successfully triggered matching workflow for %d users", userCount)
	return nil
}

func main() {
	lambda.Start(handler)
}
