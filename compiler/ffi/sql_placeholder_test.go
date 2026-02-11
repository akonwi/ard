package ffi

import "testing"

func TestNormalizePlaceholders(t *testing.T) {
	tests := []struct {
		name   string
		driver string
		input  string
		want   string
	}{
		{
			name:   "pgx rewrites placeholders to positional",
			driver: "pgx",
			input:  "INSERT INTO users (id, name) VALUES (?, ?)",
			want:   "INSERT INTO users (id, name) VALUES ($1, $2)",
		},
		{
			name:   "pgx rewrites named params to positional",
			driver: "pgx",
			input:  "INSERT INTO users (id, name) VALUES (@id, @name)",
			want:   "INSERT INTO users (id, name) VALUES ($1, $2)",
		},
		{
			name:   "pgx rewrites repeated placeholders",
			driver: "pgx",
			input:  "SELECT * FROM users WHERE id = ? OR id = ?",
			want:   "SELECT * FROM users WHERE id = $1 OR id = $2",
		},
		{
			name:   "pgx rewrites repeated named params by occurrence",
			driver: "pgx",
			input:  "SELECT * FROM users WHERE id = @id OR id = @id",
			want:   "SELECT * FROM users WHERE id = $1 OR id = $2",
		},
		{
			name:   "sqlite leaves placeholders unchanged",
			driver: "sqlite3",
			input:  "INSERT INTO users (id, name) VALUES (?, ?)",
			want:   "INSERT INTO users (id, name) VALUES (?, ?)",
		},
		{
			name:   "mysql leaves placeholders unchanged",
			driver: "mysql",
			input:  "INSERT INTO users (id, name) VALUES (?, ?)",
			want:   "INSERT INTO users (id, name) VALUES (?, ?)",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := normalizePlaceholders(test.input, test.driver)
			if got != test.want {
				t.Fatalf("expected %q, got %q", test.want, got)
			}
		})
	}
}
