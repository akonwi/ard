package ffi

import "testing"

func TestDetectDriver(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		expects string
	}{
		{
			name:    "postgresql URL uses pgx driver",
			input:   "postgresql://user:password@localhost:5432/dbname?sslmode=require&channel_binding=require",
			expects: "pgx",
		},
		{
			name:    "postgres URL uses pgx driver",
			input:   "postgres://user:password@localhost:5432/dbname",
			expects: "pgx",
		},
		{
			name:    "mysql connection string uses mysql driver",
			input:   "user:password@tcp(localhost:3306)/dbname",
			expects: "mysql",
		},
		{
			name:    "sqlite file path uses sqlite driver",
			input:   "test.db",
			expects: "sqlite3",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detectDriver(test.input); got != test.expects {
				t.Fatalf("expected driver %q, got %q", test.expects, got)
			}
		})
	}
}
