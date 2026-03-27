package model

import (
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type sqliteToPostgresTable struct {
	name  string
	model any
}

func sqliteToPostgresBusinessTables() []sqliteToPostgresTable {
	return []sqliteToPostgresTable{
		{name: "channels", model: &Channel{}},
		{name: "tokens", model: &Token{}},
		{name: "users", model: &User{}},
		{name: "passkey_credentials", model: &PasskeyCredential{}},
		{name: "options", model: &Option{}},
		{name: "redemptions", model: &Redemption{}},
		{name: "abilities", model: &Ability{}},
		{name: "midjourneys", model: &Midjourney{}},
		{name: "top_ups", model: &TopUp{}},
		{name: "quota_data", model: &QuotaData{}},
		{name: "tasks", model: &Task{}},
		{name: "models", model: &Model{}},
		{name: "vendors", model: &Vendor{}},
		{name: "model_channel_policies", model: &ModelChannelPolicy{}},
		{name: "model_channel_states", model: &ModelChannelState{}},
		{name: "prefill_groups", model: &PrefillGroup{}},
		{name: "setups", model: &Setup{}},
		{name: "two_fas", model: &TwoFA{}},
		{name: "two_fa_backup_codes", model: &TwoFABackupCode{}},
		{name: "checkins", model: &Checkin{}},
		{name: "subscription_orders", model: &SubscriptionOrder{}},
		{name: "user_subscriptions", model: &UserSubscription{}},
		{name: "subscription_pre_consume_records", model: &SubscriptionPreConsumeRecord{}},
		{name: "custom_oauth_providers", model: &CustomOAuthProvider{}},
		{name: "user_oauth_bindings", model: &UserOAuthBinding{}},
		{name: "subscription_plans", model: &SubscriptionPlan{}},
	}
}

func RunSQLiteToPostgresMigration(sqlitePath string, postgresDSN string, batchSize int) error {
	cfg := common.SQLiteToPostgresMigrationConfig{
		Enabled:     true,
		SQLitePath:  sqlitePath,
		PostgresDSN: postgresDSN,
		BatchSize:   batchSize,
	}
	if err := common.ValidateSQLiteToPostgresMigrationConfig(cfg); err != nil {
		return err
	}

	sourceDB, err := openSQLiteMigrationDB(sqlitePath)
	if err != nil {
		return err
	}
	defer func() {
		_ = closeDB(sourceDB)
	}()

	targetDB, err := openPostgresMigrationDB(postgresDSN)
	if err != nil {
		return err
	}
	defer func() {
		_ = closeDB(targetDB)
	}()

	tables := sqliteToPostgresBusinessTables()
	if err := autoMigrateSQLiteToPostgresBusinessTables(targetDB, tables); err != nil {
		return err
	}
	if err := ensureTablesEmpty(targetDB, tables); err != nil {
		return err
	}

	for _, table := range tables {
		startedAt := time.Now()
		common.SysLog(fmt.Sprintf("start migrating table %s", table.name))
		rows, err := migrateTableData(sourceDB, targetDB, table, batchSize)
		if err != nil {
			return fmt.Errorf("migrate table %s: %w", table.name, err)
		}
		if err := resetTableSequence(targetDB, table); err != nil {
			return fmt.Errorf("reset sequence for %s: %w", table.name, err)
		}
		common.SysLog(fmt.Sprintf("table %s migrated: rows=%d elapsed=%s", table.name, rows, time.Since(startedAt).Round(time.Millisecond)))
	}
	return nil
}

func openSQLiteMigrationDB(sqlitePath string) (*gorm.DB, error) {
	sqliteFilePath := sqlitePath
	if idx := strings.Index(sqliteFilePath, "?"); idx >= 0 {
		sqliteFilePath = sqliteFilePath[:idx]
	}
	if sqliteFilePath != "" {
		if _, err := os.Stat(sqliteFilePath); err != nil {
			return nil, fmt.Errorf("open sqlite source %s: %w", sqliteFilePath, err)
		}
	}
	db, err := gorm.Open(sqlite.Open(sqlitePath), &gorm.Config{
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite source: %w", err)
	}
	return db, nil
}

func openPostgresMigrationDB(postgresDSN string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN:                  postgresDSN,
		PreferSimpleProtocol: true,
	}), &gorm.Config{
		PrepareStmt: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open postgres target: %w", err)
	}
	return db, nil
}

func autoMigrateSQLiteToPostgresBusinessTables(db *gorm.DB, tables []sqliteToPostgresTable) error {
	for _, table := range tables {
		if err := db.AutoMigrate(table.model); err != nil {
			return fmt.Errorf("auto migrate %s: %w", table.name, err)
		}
	}
	return nil
}

func ensureTablesEmpty(db *gorm.DB, tables []sqliteToPostgresTable) error {
	for _, table := range tables {
		var count int64
		if err := db.Unscoped().Model(table.model).Count(&count).Error; err != nil {
			return fmt.Errorf("count target table %s: %w", table.name, err)
		}
		if count > 0 {
			return fmt.Errorf("target table %s is not empty", table.name)
		}
	}
	return nil
}

func migrateTableData(source *gorm.DB, target *gorm.DB, table sqliteToPostgresTable, batchSize int) (int64, error) {
	primaryFieldName, primaryColumnName, primaryKind, err := getPrimaryFieldMetadata(source, table.model)
	if err != nil {
		return 0, fmt.Errorf("resolve primary key for %s: %w", table.name, err)
	}

	var totalRows int64
	var lastPrimaryValue any
	err = target.Transaction(func(tx *gorm.DB) error {
		for {
			batchPtr := newModelSlicePtr(table.model)
			query := source.Unscoped().Model(table.model).
				Order(clause.OrderByColumn{Column: clause.Column{Name: primaryColumnName}}).
				Limit(batchSize)
			if lastPrimaryValue != nil {
				query = query.Where(clause.Gt{Column: clause.Column{Name: primaryColumnName}, Value: lastPrimaryValue})
			}
			if err := query.Find(batchPtr).Error; err != nil {
				return err
			}

			batchLen := reflect.ValueOf(batchPtr).Elem().Len()
			if batchLen == 0 {
				return nil
			}

			records := reflect.ValueOf(batchPtr).Elem().Interface()
			if err := tx.Session(&gorm.Session{SkipHooks: true}).CreateInBatches(records, batchSize).Error; err != nil {
				return err
			}
			totalRows += int64(batchLen)

			lastPrimaryValue, err = getLastPrimaryValue(batchPtr, primaryFieldName, primaryKind)
			if err != nil {
				return err
			}
		}
	})
	if err != nil {
		return 0, err
	}
	return totalRows, nil
}

func resetTableSequence(db *gorm.DB, table sqliteToPostgresTable) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}

	_, primaryColumnName, primaryKind, err := getPrimaryFieldMetadata(db, table.model)
	if err != nil {
		return err
	}
	if !isIntegerKind(primaryKind) {
		return nil
	}

	var sequenceName sql.NullString
	if err := db.Raw("SELECT pg_get_serial_sequence(?, ?)", table.name, primaryColumnName).Scan(&sequenceName).Error; err != nil {
		return err
	}
	if !sequenceName.Valid || sequenceName.String == "" {
		return nil
	}

	var maxPrimaryKey sql.NullInt64
	maxExpr := fmt.Sprintf("MAX(%s)", primaryColumnName)
	if err := db.Model(table.model).Select(maxExpr).Scan(&maxPrimaryKey).Error; err != nil {
		return err
	}
	if maxPrimaryKey.Valid {
		return db.Exec("SELECT setval(?::regclass, ?, true)", sequenceName.String, maxPrimaryKey.Int64).Error
	}
	return db.Exec("SELECT setval(?::regclass, ?, false)", sequenceName.String, 1).Error
}

func getPrimaryFieldMetadata(db *gorm.DB, model any) (fieldName string, columnName string, kind reflect.Kind, err error) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		return "", "", reflect.Invalid, err
	}
	if stmt.Schema == nil || len(stmt.Schema.PrimaryFields) != 1 {
		return "", "", reflect.Invalid, fmt.Errorf("expected exactly one primary field")
	}
	primaryField := stmt.Schema.PrimaryFields[0]
	return primaryField.Name, primaryField.DBName, primaryField.FieldType.Kind(), nil
}

func newModelSlicePtr(model any) any {
	modelType := reflect.TypeOf(model)
	return reflect.New(reflect.SliceOf(modelType.Elem())).Interface()
}

func getLastPrimaryValue(batchPtr any, fieldName string, kind reflect.Kind) (any, error) {
	batchValue := reflect.ValueOf(batchPtr).Elem()
	if batchValue.Len() == 0 {
		return nil, nil
	}
	lastRow := batchValue.Index(batchValue.Len() - 1)
	fieldValue := lastRow.FieldByName(fieldName)
	if !fieldValue.IsValid() {
		return nil, fmt.Errorf("primary field %s not found", fieldName)
	}
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fieldValue.Int(), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int64(fieldValue.Uint()), nil
	case reflect.String:
		return fieldValue.String(), nil
	default:
		return fieldValue.Interface(), nil
	}
}

func isIntegerKind(kind reflect.Kind) bool {
	switch kind {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	default:
		return false
	}
}
