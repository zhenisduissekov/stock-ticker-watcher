package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"stock-ticker-watcher/internal/models"

	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore handles database operations
type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// New creates a new store instance
func New(dbPath string, logger *slog.Logger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	s := &SQLiteStore{
		db:     db,
		logger: logger,
	}

	if err := s.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	return s, nil
}

// init creates tables and initializes demo user
func (s *SQLiteStore) init() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL
		);
		
		CREATE TABLE IF NOT EXISTS user_stocks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			ticker TEXT NOT NULL,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (user_id) REFERENCES users(id),
			UNIQUE(user_id, ticker)
		);
	`)
	if err != nil {
		return fmt.Errorf("failed to create tables: %w", err)
	}

	// Initialize demo user
	_, err = s.db.Exec("INSERT OR IGNORE INTO users (id, name) VALUES (1, 'Demo User')")
	if err != nil {
		return fmt.Errorf("failed to initialize demo user: %w", err)
	}

	s.logger.Info("Database initialized successfully")
	return nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}
	s.logger.Info("Database connection closed")
	return nil
}

// GetWatchlist retrieves all tickers for a user
func (s *SQLiteStore) GetWatchlist(ctx context.Context, userID int) ([]string, error) {
	query := `SELECT ticker FROM user_stocks WHERE user_id = ?`
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query watchlist: %w", err)
	}
	defer rows.Close()

	var tickers []string
	for rows.Next() {
		var ticker string
		if err := rows.Scan(&ticker); err != nil {
			return nil, fmt.Errorf("failed to scan ticker: %w", err)
		}
		tickers = append(tickers, ticker)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating watchlist: %w", err)
	}

	return tickers, nil
}

// AddTicker adds a ticker to a user's watchlist
func (s *SQLiteStore) AddTicker(ctx context.Context, userID int, ticker string) error {
	query := `INSERT INTO user_stocks (user_id, ticker) VALUES (?, ?)`
	result, err := s.db.ExecContext(ctx, query, userID, ticker)
	if err != nil {
		return fmt.Errorf("failed to add ticker: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticker already exists in watchlist")
	}

	s.logger.Info("Ticker added to watchlist", "user_id", userID, "ticker", ticker)
	return nil
}

// RemoveTicker removes a ticker from a user's watchlist
func (s *SQLiteStore) RemoveTicker(ctx context.Context, userID int, ticker string) error {
	query := `DELETE FROM user_stocks WHERE user_id = ? AND ticker = ?`
	result, err := s.db.ExecContext(ctx, query, userID, ticker)
	if err != nil {
		return fmt.Errorf("failed to remove ticker: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("ticker not found in watchlist")
	}

	s.logger.Info("Ticker removed from watchlist", "user_id", userID, "ticker", ticker)
	return nil
}

// GetUser retrieves a user by ID
func (s *SQLiteStore) GetUser(ctx context.Context, userID int) (*models.User, error) {
	query := `SELECT id, name FROM users WHERE id = ?`
	row := s.db.QueryRowContext(ctx, query, userID)

	var user models.User
	if err := row.Scan(&user.ID, &user.Name); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("user not found")
		}
		return nil, fmt.Errorf("failed to scan user: %w", err)
	}

	return &user, nil
}
