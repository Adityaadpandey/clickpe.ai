-- Users table
CREATE TABLE users (
    user_id VARCHAR(50) PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    monthly_income DECIMAL(10,2),
    credit_score INTEGER,
    employment_status VARCHAR(50),
    age INTEGER,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Loan products table
CREATE TABLE loan_products (
    product_id SERIAL PRIMARY KEY,
    product_name VARCHAR(255) NOT NULL,
    provider VARCHAR(255),
    interest_rate DECIMAL(5,2),
    min_income DECIMAL(10,2),
    min_credit_score INTEGER,
    max_credit_score INTEGER,
    min_age INTEGER,
    max_age INTEGER,
    employment_required BOOLEAN,
    source_url TEXT,
    crawled_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(product_name, provider)
);

-- Matches table
CREATE TABLE matches (
    match_id SERIAL PRIMARY KEY,
    user_id VARCHAR(50) REFERENCES users(user_id),
    product_id INTEGER REFERENCES loan_products(product_id),
    match_score DECIMAL(5,2),
    matched_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    notified BOOLEAN DEFAULT FALSE,
    UNIQUE(user_id, product_id)
);

CREATE INDEX idx_users_income ON users(monthly_income);
CREATE INDEX idx_users_credit ON users(credit_score);
CREATE INDEX idx_products_criteria ON loan_products(min_income, min_credit_score);
CREATE INDEX idx_matches_notified ON matches(notified);
