package ffi

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/mattn/go-sqlite3"
)

type sqlConnection struct {
	db     *sql.DB
	driver string
}

type sqlTransaction struct {
	tx     *sql.Tx
	driver string
}

// Generic SQL FFI functions supporting multiple database drivers
// (postgres, mysql, sqlite, etc.)

// SqlCreateConnection creates a new database connection from a connection string.
// Returns an opaque *sqlConnection handle.
func SqlCreateConnection(connectionString string) (any, error) {
	driver := detectDriver(connectionString)

	db, err := sql.Open(driver, connectionString)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("Failed to connect to database: %s", err)
	}

	return &sqlConnection{db: db, driver: driver}, nil
}

// detectDriver identifies the SQL driver based on the connection string
func detectDriver(connStr string) string {
	connStr = strings.TrimSpace(connStr)

	// Check for postgres
	if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		return "pgx"
	}

	// Check for mysql
	if strings.Contains(connStr, "@tcp(") || strings.Contains(connStr, "@unix(") {
		return "mysql"
	}

	// Default to sqlite for any other format (file paths, file: URIs, etc.)
	return "sqlite3"
}

// SqlClose closes a database connection.
// Takes an opaque *sqlConnection handle.
func SqlClose(handle any) error {
	conn, ok := handle.(*sqlConnection)
	if !ok {
		return fmt.Errorf("SQL Error: invalid connection object")
	}
	return conn.db.Close()
}

// Extract parameter names from a sql expression in the order they appear
func SqlExtractParams(sqlStr string) []string {
	// Split SQL into tokens by multiple delimiters
	delimiters := []string{" ", "(", ")", ",", ";", "=", "<", ">", "!", "\t", "\n", "\r"}
	tokens := splitSQLByMultipleDelimiters(sqlStr, delimiters)

	var paramNames []string

	for _, token := range tokens {
		if strings.HasPrefix(token, "@") && len(token) > 1 {
			// Extract parameter name, removing @ prefix and any trailing punctuation
			paramName := strings.TrimLeft(token[1:], "@")
			paramName = strings.TrimRight(paramName, ".,;:!?")

			if paramName != "" {
				paramNames = append(paramNames, paramName)
			}
		}
	}

	return paramNames
}

// splitSQLByMultipleDelimiters is a helper function to split string by multiple delimiters
// Used by both SQL and SQLite extract_params implementations
func splitSQLByMultipleDelimiters(s string, delimiters []string) []string {
	// Replace all delimiters with a single delimiter, then split
	result := s
	for _, delimiter := range delimiters {
		result = strings.ReplaceAll(result, delimiter, " ")
	}

	// Split by space and filter out empty strings
	tokens := strings.Split(result, " ")
	var nonEmpty []string
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			nonEmpty = append(nonEmpty, token)
		}
	}

	return nonEmpty
}

// sqlRunner interface abstracts both *sql.DB and *sql.Tx for query execution
type sqlRunner interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func normalizePlaceholders(sqlStr, driver string) string {
	if driver != "pgx" {
		return sqlStr
	}

	// PostgreSQL requires positional placeholders like $1, $2, ...
	// Support both Ard's @name placeholders and ? placeholders.
	var out strings.Builder
	out.Grow(len(sqlStr) + 16)
	index := 1
	for i := 0; i < len(sqlStr); i++ {
		ch := sqlStr[i]

		if ch == '@' {
			j := i + 1
			for j < len(sqlStr) {
				c := sqlStr[j]
				isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
				isDigit := c >= '0' && c <= '9'
				if isAlpha || isDigit || c == '_' {
					j++
					continue
				}
				break
			}

			if j > i+1 {
				out.WriteString(fmt.Sprintf("$%d", index))
				index++
				i = j - 1
				continue
			}
		}

		if ch == '?' {
			out.WriteString(fmt.Sprintf("$%d", index))
			index++
			continue
		}
		out.WriteByte(ch)
	}

	return out.String()
}

func resolveRunner(raw any) (sqlRunner, string, bool) {
	if conn, ok := raw.(*sqlConnection); ok {
		return conn.db, conn.driver, true
	}
	if tx, ok := raw.(*sqlTransaction); ok {
		return tx.tx, tx.driver, true
	}
	return nil, "", false
}

// executeQuery is a helper that works with both *sql.DB and *sql.Tx
func executeQuery(runner sqlRunner, sqlStr string, values []any) ([]any, error) {
	rows, err := runner.Query(sqlStr, values...)
	if err != nil {
		return nil, fmt.Errorf("failed to execute query: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	// Build result list - each row is a map[string]any
	var results []any

	for rows.Next() {
		// Create scan targets for each column
		scanValues := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range scanValues {
			scanTargets[i] = &scanValues[i]
		}

		// Scan the row
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create map for this row
		rowMap := make(map[string]any)
		for i, columnName := range columns {
			rowMap[columnName] = scanValues[i]
		}

		results = append(results, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return results, nil
}

// SqlQuery executes a query and returns the rows as a list of Dynamic maps.
func SqlQuery(conn any, sqlStr string, values []any) ([]any, error) {
	runner, driver, ok := resolveRunner(conn)
	if !ok {
		return nil, fmt.Errorf("SQL Error: invalid connection object")
	}

	sqlStr = normalizePlaceholders(sqlStr, driver)
	return executeQuery(runner, sqlStr, values)
}

// SqlExecute executes a query without returning rows.
func SqlExecute(conn any, sqlStr string, values []any) error {
	runner, driver, ok := resolveRunner(conn)
	if !ok {
		return fmt.Errorf("SQL Error: invalid connection object")
	}

	sqlStr = normalizePlaceholders(sqlStr, driver)
	_, err := runner.Exec(sqlStr, values...)
	return err
}

// SqlBeginTx begins a new transaction.
// Takes an opaque *sqlConnection handle, returns an opaque *sqlTransaction handle.
func SqlBeginTx(handle any) (any, error) {
	conn, ok := handle.(*sqlConnection)
	if !ok {
		return nil, fmt.Errorf("SQL Error: invalid connection object")
	}

	tx, err := conn.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %v", err)
	}

	return &sqlTransaction{tx: tx, driver: conn.driver}, nil
}

// SqlCommit commits a transaction.
// Takes an opaque *sqlTransaction handle.
func SqlCommit(handle any) error {
	wrappedTx, ok := handle.(*sqlTransaction)
	if !ok {
		return fmt.Errorf("SQL Error: invalid transaction object")
	}
	return wrappedTx.tx.Commit()
}

// SqlRollback rolls back a transaction.
// Takes an opaque *sqlTransaction handle.
func SqlRollback(handle any) error {
	wrappedTx, ok := handle.(*sqlTransaction)
	if !ok {
		return fmt.Errorf("SQL Error: invalid transaction object")
	}
	return wrappedTx.tx.Rollback()
}