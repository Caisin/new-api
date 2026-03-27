package model

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func openTestSQLiteDB(t *testing.T, name string) *gorm.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	return db
}

func TestSQLiteToPostgresBusinessModelsExcludeLog(t *testing.T) {
	tables := sqliteToPostgresBusinessTables()
	require.NotEmpty(t, tables)

	modelNames := make(map[string]struct{}, len(tables))
	for _, table := range tables {
		modelNames[reflect.TypeOf(table.model).Elem().Name()] = struct{}{}
	}

	_, hasLog := modelNames["Log"]
	_, hasOption := modelNames["Option"]
	_, hasSubscriptionPlan := modelNames["SubscriptionPlan"]
	require.False(t, hasLog)
	require.True(t, hasOption)
	require.True(t, hasSubscriptionPlan)
}

func TestEnsureTablesEmptyReturnsTableName(t *testing.T) {
	db := openTestSQLiteDB(t, "target.db")
	require.NoError(t, db.AutoMigrate(&Option{}))
	require.NoError(t, db.Create(&Option{Key: "site_name", Value: "new-api"}).Error)

	err := ensureTablesEmpty(db, []sqliteToPostgresTable{
		{
			name:  "options",
			model: &Option{},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "options")
}

func TestMigrateTablePreservesIDsAndDeletedRows(t *testing.T) {
	source := openTestSQLiteDB(t, "source.db")
	target := openTestSQLiteDB(t, "target.db")
	require.NoError(t, source.AutoMigrate(&User{}))
	require.NoError(t, target.AutoMigrate(&User{}))

	user := User{
		Id:          42,
		Username:    "deleted-user",
		Password:    "12345678",
		DisplayName: "Deleted User",
		AccessToken: nil,
	}
	require.NoError(t, source.Session(&gorm.Session{SkipHooks: true}).Create(&user).Error)
	require.NoError(t, source.Delete(&user).Error)

	rows, err := migrateTableData(source, target, sqliteToPostgresTable{
		name:  "users",
		model: &User{},
	}, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var got User
	require.NoError(t, target.Unscoped().First(&got, 42).Error)
	require.Equal(t, 42, got.Id)
	require.True(t, got.DeletedAt.Valid)
}

func TestMigrateTableSkipsHooksToPreserveTimestamps(t *testing.T) {
	source := openTestSQLiteDB(t, "source.db")
	target := openTestSQLiteDB(t, "target.db")
	require.NoError(t, source.AutoMigrate(&SubscriptionPlan{}))
	require.NoError(t, target.AutoMigrate(&SubscriptionPlan{}))

	plan := SubscriptionPlan{
		Id:        7,
		Title:     "Starter",
		Subtitle:  "Monthly",
		CreatedAt: 111,
		UpdatedAt: 222,
	}
	require.NoError(t, source.Session(&gorm.Session{SkipHooks: true}).Create(&plan).Error)

	rows, err := migrateTableData(source, target, sqliteToPostgresTable{
		name:  "subscription_plans",
		model: &SubscriptionPlan{},
	}, 10)
	require.NoError(t, err)
	require.EqualValues(t, 1, rows)

	var got SubscriptionPlan
	require.NoError(t, target.First(&got, 7).Error)
	require.EqualValues(t, 111, got.CreatedAt)
	require.EqualValues(t, 222, got.UpdatedAt)
}
