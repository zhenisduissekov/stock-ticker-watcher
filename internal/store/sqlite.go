package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
)

// SQLiteStore handles database operations
type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// New creates a new store instance
func New(dbPath string, logger *slog.Logger) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", buildDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Connection pool settings. SQLite serializes writes, so we keep the pool
	// small; WAL (set via DSN) allows reads to proceed concurrently with a
	// writer, and busy_timeout lets contending writers wait rather than fail.
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxIdleTime(5 * time.Minute)

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

	logger.Info("SQLite configured", "journal_mode", "WAL", "busy_timeout_ms", 5000, "max_open_conns", 10)
	return s, nil
}

// buildDSN appends reliability pragmas to the database path so they are applied
// to every pooled connection (mattn/go-sqlite3 reads these from the DSN):
//   - _journal_mode=WAL   : concurrent readers alongside a single writer
//   - _busy_timeout=5000  : wait up to 5s on a locked DB instead of erroring
//   - _foreign_keys=on    : enforce the user_stocks -> users FK
func buildDSN(dbPath string) string {
	params := "_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"
	if strings.Contains(dbPath, "?") {
		return dbPath + "&" + params
	}
	return dbPath + "?" + params
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

// Ping verifies the database connection is alive (used by the readiness probe).
func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
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

// GetAllTickers returns the distinct set of tickers across all users'
// watchlists. Used by the price simulator to drive live updates for exactly
// the tickers someone is watching, rather than a fixed hardcoded list.
func (s *SQLiteStore) GetAllTickers(ctx context.Context) ([]string, error) {
	query := `SELECT DISTINCT ticker FROM user_stocks`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tickers: %w", err)
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
		return nil, fmt.Errorf("error iterating tickers: %w", err)
	}

	return tickers, nil
}

// AddTicker adds a ticker to a user's watchlist
func (s *SQLiteStore) AddTicker(ctx context.Context, userID int, ticker string) error {
	query := `INSERT INTO user_stocks (user_id, ticker) VALUES (?, ?)`
	result, err := s.db.ExecContext(ctx, query, userID, ticker)
	if err != nil {
		// A duplicate (user_id, ticker) trips the UNIQUE constraint; surface it
		// as a typed error so callers don't have to match on the message.
		var sqliteErr sqlite3.Error
		if errors.As(err, &sqliteErr) && sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique {
			return ErrTickerExists
		}
		return fmt.Errorf("failed to add ticker: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return ErrTickerExists
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
		return ErrTickerNotFound
	}

	s.logger.Info("Ticker removed from watchlist", "user_id", userID, "ticker", ticker)
	return nil
}
