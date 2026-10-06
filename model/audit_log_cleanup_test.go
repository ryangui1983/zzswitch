package model

import (
	"context"
	"fmt"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestCountAndDeleteOldAuditLogBatch(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{TablePrefix: "audit_cleanup_"},
	})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		require.NoError(t, sqlDB.Close())
	})

	previous := LOG_DB
	previousType := common.LogDatabaseType()
	LOG_DB = db
	common.SetLogDatabaseType(common.DatabaseTypeSQLite)
	t.Cleanup(func() {
		LOG_DB = previous
		common.SetLogDatabaseType(previousType)
	})
	require.NoError(t, db.AutoMigrate(&AuditLog{}))

	const cutoff int64 = 500
	for i := 0; i < 3; i++ {
		require.NoError(t, db.Create(&AuditLog{
			EventId:   fmt.Sprintf("old-%d", i),
			CreatedAt: cutoff - 1,
			Category:  AuditCategoryLogin,
			Action:    "login",
		}).Error)
	}
	require.NoError(t, db.Create(&AuditLog{
		EventId:   "keep",
		CreatedAt: cutoff + 1,
		Category:  AuditCategoryLogin,
		Action:    "login",
	}).Error)

	ctx := context.Background()
	total, err := CountOldAuditLog(ctx, cutoff)
	require.NoError(t, err)
	assert.EqualValues(t, 3, total)

	deleted, err := DeleteOldAuditLogBatch(ctx, cutoff, 2)
	require.NoError(t, err)
	assert.Greater(t, deleted, int64(0))

	for {
		remaining, err := CountOldAuditLog(ctx, cutoff)
		require.NoError(t, err)
		if remaining == 0 {
			break
		}
		deleted, err = DeleteOldAuditLogBatch(ctx, cutoff, 2)
		require.NoError(t, err)
		require.Greater(t, deleted, int64(0))
	}

	var kept int64
	require.NoError(t, db.Model(&AuditLog{}).Where("event_id = ?", "keep").Count(&kept).Error)
	assert.EqualValues(t, 1, kept)
}
