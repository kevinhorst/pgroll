// SPDX-License-Identifier: Apache-2.0

package roll_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xataio/pgroll/internal/testutils"
	"github.com/xataio/pgroll/pkg/backfill"
	"github.com/xataio/pgroll/pkg/migrations"
	"github.com/xataio/pgroll/pkg/roll"
)

func TestBatchStartAndComplete(t *testing.T) {
	t.Parallel()

	t.Run("batch creates tables from multiple migrations", func(t *testing.T) {
		testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
			ctx := context.Background()

			batch, err := migrations.NewBatch([]*migrations.RawMigration{
				rawMigration("01_create_users",
					`[{"create_table":{"name":"users","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
				rawMigration("02_create_posts",
					`[{"create_table":{"name":"posts","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
			})
			require.NoError(t, err)

			err = mig.StartBatch(ctx, batch, backfill.NewConfig())
			require.NoError(t, err)

			// Version schema should be named after the last member
			assert.True(t, schemaExists(t, db, roll.VersionedSchemaName(cSchema, "02_create_posts")))

			// Both tables should exist
			assert.True(t, tableExists(t, db, cSchema, "users"))
			assert.True(t, tableExists(t, db, cSchema, "posts"))

			// Complete the batch
			err = mig.Complete(ctx)
			require.NoError(t, err)

			// Tables should still exist after completion
			assert.True(t, tableExists(t, db, cSchema, "users"))
			assert.True(t, tableExists(t, db, cSchema, "posts"))
		})
	})

	t.Run("batch with version_schema on last member", func(t *testing.T) {
		testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
			ctx := context.Background()

			lastMember := rawMigration("02_create_posts",
				`[{"create_table":{"name":"posts","columns":[{"name":"id","type":"integer","pk":true}]}}]`)
			lastMember.VersionSchema = "custom_version"

			batch, err := migrations.NewBatch([]*migrations.RawMigration{
				rawMigration("01_create_users",
					`[{"create_table":{"name":"users","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
				lastMember,
			})
			require.NoError(t, err)

			err = mig.StartBatch(ctx, batch, backfill.NewConfig())
			require.NoError(t, err)

			assert.True(t, schemaExists(t, db, roll.VersionedSchemaName(cSchema, "custom_version")))

			err = mig.Complete(ctx)
			require.NoError(t, err)
		})
	})
}

func TestBatchRollback(t *testing.T) {
	t.Parallel()

	testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()

		batch, err := migrations.NewBatch([]*migrations.RawMigration{
			rawMigration("01_create_users",
				`[{"create_table":{"name":"users","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
			rawMigration("02_create_posts",
				`[{"create_table":{"name":"posts","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
		})
		require.NoError(t, err)

		err = mig.StartBatch(ctx, batch, backfill.NewConfig())
		require.NoError(t, err)

		// Rollback the batch
		err = mig.Rollback(ctx)
		require.NoError(t, err)

		// Version schema should be gone
		assert.False(t, schemaExists(t, db, roll.VersionedSchemaName(cSchema, "02_create_posts")))

		// No active migration should remain
		active, err := mig.State().IsActiveMigrationPeriod(ctx, cSchema)
		require.NoError(t, err)
		assert.False(t, active)
	})
}

func TestBatchRejectsActiveConflict(t *testing.T) {
	t.Parallel()

	testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()

		// Start a regular migration
		err := mig.Start(ctx, &migrations.Migration{
			Name:       "01_first",
			Operations: migrations.Operations{createTableOp("first_table")},
		}, backfill.NewConfig())
		require.NoError(t, err)

		// Attempting a batch should fail
		batch, err := migrations.NewBatch([]*migrations.RawMigration{
			rawMigration("02_create_users",
				`[{"create_table":{"name":"users","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
			rawMigration("03_create_posts",
				`[{"create_table":{"name":"posts","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
		})
		require.NoError(t, err)

		err = mig.StartBatch(ctx, batch, backfill.NewConfig())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "already in progress")
	})
}

func TestBatchUnappliedMigrations(t *testing.T) {
	t.Parallel()

	t.Run("migrations after batch are unapplied", func(t *testing.T) {
		fs := fstest.MapFS{
			"01_migration_1.json": &fstest.MapFile{Data: createTableMigration(t, "01_migration_1", "t1")},
			"02_migration_2.json": &fstest.MapFile{Data: createTableMigration(t, "02_migration_2", "t2")},
			"03_migration_3.json": &fstest.MapFile{Data: createTableMigration(t, "03_migration_3", "t3")},
			"04_migration_4.json": &fstest.MapFile{Data: createTableMigration(t, "04_migration_4", "t4")},
		}

		testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, _ *sql.DB) {
			ctx := context.Background()

			// Apply first migration individually
			m, err := migrations.ReadMigration(fs, "01_migration_1.json")
			require.NoError(t, err)
			err = mig.Start(ctx, m, backfill.NewConfig())
			require.NoError(t, err)
			err = mig.Complete(ctx)
			require.NoError(t, err)

			// Apply second and third as a batch
			batch, err := migrations.NewBatch([]*migrations.RawMigration{
				rawMigFromFile(t, fs, "02_migration_2.json"),
				rawMigFromFile(t, fs, "03_migration_3.json"),
			})
			require.NoError(t, err)
			err = mig.StartBatch(ctx, batch, backfill.NewConfig())
			require.NoError(t, err)
			err = mig.Complete(ctx)
			require.NoError(t, err)

			// Check unapplied
			unapplied, err := mig.UnappliedMigrations(ctx, fs)
			require.NoError(t, err)
			require.Len(t, unapplied, 1)
			assert.Equal(t, "04_migration_4", unapplied[0].Name)
		})
	})

	t.Run("all batch members applied leaves nothing unapplied", func(t *testing.T) {
		fs := fstest.MapFS{
			"01_migration_1.json": &fstest.MapFile{Data: createTableMigration(t, "01_migration_1", "t1")},
			"02_migration_2.json": &fstest.MapFile{Data: createTableMigration(t, "02_migration_2", "t2")},
		}

		testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, _ *sql.DB) {
			ctx := context.Background()

			batch, err := migrations.NewBatch([]*migrations.RawMigration{
				rawMigFromFile(t, fs, "01_migration_1.json"),
				rawMigFromFile(t, fs, "02_migration_2.json"),
			})
			require.NoError(t, err)
			err = mig.StartBatch(ctx, batch, backfill.NewConfig())
			require.NoError(t, err)
			err = mig.Complete(ctx)
			require.NoError(t, err)

			unapplied, err := mig.UnappliedMigrations(ctx, fs)
			require.NoError(t, err)
			require.Len(t, unapplied, 0)
		})
	})
}

func TestBatchAfterIndividualMigration(t *testing.T) {
	t.Parallel()

	testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()

		// Apply first migration individually
		err := mig.Start(ctx, &migrations.Migration{
			Name:       "01_create_users",
			Operations: migrations.Operations{createTableOp("users")},
		}, backfill.NewConfig())
		require.NoError(t, err)
		err = mig.Complete(ctx)
		require.NoError(t, err)

		// Apply batch of next two
		batch, err := migrations.NewBatch([]*migrations.RawMigration{
			rawMigration("02_create_posts",
				`[{"create_table":{"name":"posts","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
			rawMigration("03_create_comments",
				`[{"create_table":{"name":"comments","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
		})
		require.NoError(t, err)

		err = mig.StartBatch(ctx, batch, backfill.NewConfig())
		require.NoError(t, err)

		err = mig.Complete(ctx)
		require.NoError(t, err)

		// All three tables should exist
		assert.True(t, tableExists(t, db, cSchema, "users"))
		assert.True(t, tableExists(t, db, cSchema, "posts"))
		assert.True(t, tableExists(t, db, cSchema, "comments"))

		// Previous version schema (01_create_users) should be dropped
		assert.False(t, schemaExists(t, db, roll.VersionedSchemaName(cSchema, "01_create_users")))
	})
}

func TestBatchWithCrossMemberDependency(t *testing.T) {
	t.Parallel()

	testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()

		batch, err := migrations.NewBatch([]*migrations.RawMigration{
			rawMigration("01_create_users",
				`[{"create_table":{"name":"users","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
			rawMigration("02_add_email",
				`[{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}]`),
		})
		require.NoError(t, err)

		err = mig.StartBatch(ctx, batch, backfill.NewConfig())
		require.NoError(t, err)

		err = mig.Complete(ctx)
		require.NoError(t, err)

		assert.True(t, tableExists(t, db, cSchema, "users"))
		assert.True(t, columnExists(t, db, cSchema, "users", "email"))
	})
}

func TestBatchValidationRejectsInvalidMember(t *testing.T) {
	t.Parallel()

	testutils.WithMigratorAndConnectionToContainer(t, func(mig *roll.Roll, db *sql.DB) {
		ctx := context.Background()

		batch, err := migrations.NewBatch([]*migrations.RawMigration{
			rawMigration("01_create_users",
				`[{"create_table":{"name":"users","columns":[{"name":"id","type":"integer","pk":true}]}}]`),
			rawMigration("02_add_col_nonexistent",
				`[{"add_column":{"table":"nonexistent","up":"''","column":{"name":"col","type":"text","nullable":true}}}]`),
		})
		require.NoError(t, err)

		err = mig.StartBatch(ctx, batch, backfill.NewConfig())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "02_add_col_nonexistent")
		assert.Contains(t, err.Error(), "invalid")
	})
}

func columnExists(t *testing.T, db *sql.DB, schema, table, column string) bool {
	t.Helper()
	var exists bool
	err := db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = $1 AND table_name = $2 AND column_name = $3
		)`, schema, table, column).Scan(&exists)
	require.NoError(t, err)
	return exists
}

func rawMigration(name, opsJSON string) *migrations.RawMigration {
	return &migrations.RawMigration{
		Name:       name,
		Operations: json.RawMessage(opsJSON),
	}
}

func rawMigFromFile(t *testing.T, fs fstest.MapFS, filename string) *migrations.RawMigration {
	t.Helper()
	m, err := migrations.ReadRawMigration(fs, filename)
	require.NoError(t, err)
	return m
}

func createTableMigration(t *testing.T, name, tableName string) []byte {
	t.Helper()
	mig := &migrations.Migration{
		Name:       name,
		Operations: migrations.Operations{createTableOp(tableName)},
	}
	bytes, err := json.Marshal(mig)
	require.NoError(t, err)
	return bytes
}
