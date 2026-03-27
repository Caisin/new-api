package common

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateSQLiteToPostgresMigrationConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     SQLiteToPostgresMigrationConfig
		wantErr string
	}{
		{
			name: "disabled migration ignores empty fields",
			cfg:  SQLiteToPostgresMigrationConfig{},
		},
		{
			name: "requires sqlite path when enabled",
			cfg: SQLiteToPostgresMigrationConfig{
				Enabled:     true,
				PostgresDSN: "postgres://user:pass@localhost:5432/newapi",
				BatchSize:   1000,
			},
			wantErr: "sqlite-path",
		},
		{
			name: "requires postgres dsn when enabled",
			cfg: SQLiteToPostgresMigrationConfig{
				Enabled:    true,
				SQLitePath: "one-api.db",
				BatchSize:  1000,
			},
			wantErr: "postgres-dsn",
		},
		{
			name: "requires postgres scheme",
			cfg: SQLiteToPostgresMigrationConfig{
				Enabled:     true,
				SQLitePath:  "one-api.db",
				PostgresDSN: "mysql://user:pass@localhost:3306/newapi",
				BatchSize:   1000,
			},
			wantErr: "PostgreSQL DSN",
		},
		{
			name: "requires positive batch size",
			cfg: SQLiteToPostgresMigrationConfig{
				Enabled:     true,
				SQLitePath:  "one-api.db",
				PostgresDSN: "postgres://user:pass@localhost:5432/newapi",
				BatchSize:   0,
			},
			wantErr: "batch-size",
		},
		{
			name: "accepts valid config",
			cfg: SQLiteToPostgresMigrationConfig{
				Enabled:     true,
				SQLitePath:  "one-api.db",
				PostgresDSN: "postgres://user:pass@localhost:5432/newapi?sslmode=disable",
				BatchSize:   1000,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateSQLiteToPostgresMigrationConfig(tt.cfg)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestGetSQLiteToPostgresMigrationConfigFromFlags(t *testing.T) {
	originalEnabled := *SQLiteToPostgres
	originalSQLitePath := *SQLitePathFlag
	originalPostgresDSN := *PostgresDSNFlag
	originalBatchSize := *BatchSizeFlag
	t.Cleanup(func() {
		*SQLiteToPostgres = originalEnabled
		*SQLitePathFlag = originalSQLitePath
		*PostgresDSNFlag = originalPostgresDSN
		*BatchSizeFlag = originalBatchSize
	})

	*SQLiteToPostgres = true
	*SQLitePathFlag = "source.db"
	*PostgresDSNFlag = "postgres://user:pass@localhost:5432/newapi"
	*BatchSizeFlag = 250

	cfg, err := GetSQLiteToPostgresMigrationConfig()
	require.NoError(t, err)
	require.True(t, cfg.Enabled)
	require.Equal(t, "source.db", cfg.SQLitePath)
	require.Equal(t, "postgres://user:pass@localhost:5432/newapi", cfg.PostgresDSN)
	require.Equal(t, 250, cfg.BatchSize)
}
