// SPDX-License-Identifier: Apache-2.0

package migrations

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBatch(t *testing.T) {
	t.Run("requires at least 2 members", func(t *testing.T) {
		_, err := NewBatch([]*RawMigration{
			rawMig("01_one", `[{"create_table":{"name":"t1","columns":[{"name":"id","type":"int","pk":true}]}}]`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "at least 2 migrations")
	})

	t.Run("rejects unparseable members", func(t *testing.T) {
		_, err := NewBatch([]*RawMigration{
			rawMig("01_one", `[{"create_table":{"name":"t1","columns":[{"name":"id","type":"int","pk":true}]}}]`),
			rawMig("02_two", `[{"bogus_op":{}}]`),
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "02_two")
	})

	t.Run("valid batch", func(t *testing.T) {
		b, err := NewBatch([]*RawMigration{
			rawMig("01_create_users", `[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]`),
			rawMig("02_add_email", `[{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}]`),
		})
		require.NoError(t, err)
		assert.Equal(t, "02_add_email", b.Name())
		assert.Equal(t, "02_add_email", b.VersionSchemaName())
		assert.Equal(t, []string{"01_create_users", "02_add_email"}, b.MemberNames())
	})

	t.Run("version schema from last member", func(t *testing.T) {
		m := rawMig("02_add_email", `[{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}]`)
		m.VersionSchema = "custom_schema"
		b, err := NewBatch([]*RawMigration{
			rawMig("01_create_users", `[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]`),
			m,
		})
		require.NoError(t, err)
		assert.Equal(t, "custom_schema", b.VersionSchemaName())
	})
}

func TestCompositeOperations(t *testing.T) {
	b, err := NewBatch([]*RawMigration{
		rawMig("01_create_users", `[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]`),
		rawMig("02_create_posts", `[{"create_table":{"name":"posts","columns":[{"name":"id","type":"int","pk":true}]}}]`),
	})
	require.NoError(t, err)

	ops, err := b.CompositeOperations()
	require.NoError(t, err)
	assert.Len(t, ops, 2)
}

func TestCompositeMigration(t *testing.T) {
	b, err := NewBatch([]*RawMigration{
		rawMig("01_create_users", `[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]`),
		rawMig("02_create_posts", `[{"create_table":{"name":"posts","columns":[{"name":"id","type":"int","pk":true}]}}]`),
	})
	require.NoError(t, err)

	mig, err := b.CompositeMigration()
	require.NoError(t, err)
	assert.Equal(t, "02_create_posts", mig.Name)
	assert.Equal(t, "02_create_posts", mig.VersionSchemaName())
	assert.Len(t, mig.Operations, 2)
}

func TestIsBatchJSON(t *testing.T) {
	t.Run("batch envelope", func(t *testing.T) {
		envelope := `{"format":"batch/v1","version_schema":"v","operations":[],"members":[]}`
		assert.True(t, IsBatchJSON([]byte(envelope)))
	})

	t.Run("regular migration", func(t *testing.T) {
		regular := `{"version_schema":"v","operations":[]}`
		assert.False(t, IsBatchJSON([]byte(regular)))
	})

	t.Run("invalid JSON", func(t *testing.T) {
		assert.False(t, IsBatchJSON([]byte("not json")))
	})
}

func TestDecodeBatchMemberNames(t *testing.T) {
	t.Run("batch envelope", func(t *testing.T) {
		envelope := `{"format":"batch/v1","members":[{"name":"a"},{"name":"b"},{"name":"c"}]}`
		names, err := DecodeBatchMemberNames([]byte(envelope))
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b", "c"}, names)
	})

	t.Run("regular migration returns nil", func(t *testing.T) {
		regular := `{"version_schema":"v","operations":[]}`
		names, err := DecodeBatchMemberNames([]byte(regular))
		require.NoError(t, err)
		assert.Nil(t, names)
	})

	t.Run("round trip through Migration unmarshal", func(t *testing.T) {
		envelope := `{
			"format":"batch/v1",
			"version_schema":"02_add_email",
			"operations":[
				{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}},
				{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}
			],
			"members":[
				{"name":"01_create_users","operations":[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]},
				{"name":"02_add_email","operations":[{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}]}
			]
		}`

		// Verify it's detected as a batch
		assert.True(t, IsBatchJSON([]byte(envelope)))

		// Verify member names
		names, err := DecodeBatchMemberNames([]byte(envelope))
		require.NoError(t, err)
		assert.Equal(t, []string{"01_create_users", "02_add_email"}, names)

		// Verify plain unmarshal into Migration produces correct composite
		var mig Migration
		err = json.Unmarshal([]byte(envelope), &mig)
		require.NoError(t, err)
		assert.Equal(t, "02_add_email", mig.VersionSchemaName())
		assert.Len(t, mig.Operations, 2)
	})
}

func TestDecodeBatchRawMembers(t *testing.T) {
	t.Run("extracts members with operations", func(t *testing.T) {
		envelope := `{
			"format":"batch/v1",
			"version_schema":"02_add_email",
			"operations":[
				{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}},
				{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}
			],
			"members":[
				{"name":"01_create_users","operations":[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]},
				{"name":"02_add_email","version_schema":"02_add_email","operations":[{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}]}
			]
		}`

		members, err := DecodeBatchRawMembers([]byte(envelope))
		require.NoError(t, err)
		require.Len(t, members, 2)

		assert.Equal(t, "01_create_users", members[0].Name)
		assert.Equal(t, "", members[0].VersionSchema)
		assert.Contains(t, string(members[0].Operations), "create_table")

		assert.Equal(t, "02_add_email", members[1].Name)
		assert.Equal(t, "02_add_email", members[1].VersionSchema)
		assert.Contains(t, string(members[1].Operations), "add_column")
	})

	t.Run("returns nil for non-batch JSON", func(t *testing.T) {
		regular := `{"version_schema":"v","operations":[]}`
		members, err := DecodeBatchRawMembers([]byte(regular))
		require.NoError(t, err)
		assert.Nil(t, members)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		_, err := DecodeBatchRawMembers([]byte("not json"))
		require.Error(t, err)
	})
}

func TestNewBatchMultiOps(t *testing.T) {
	b, err := NewBatch([]*RawMigration{
		rawMig("01_setup", `[
			{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}},
			{"create_table":{"name":"posts","columns":[{"name":"id","type":"int","pk":true}]}}
		]`),
		rawMig("02_more", `[
			{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}},
			{"add_column":{"table":"posts","up":"''","column":{"name":"title","type":"text","nullable":true}}},
			{"add_column":{"table":"posts","up":"''","column":{"name":"body","type":"text","nullable":true}}}
		]`),
	})
	require.NoError(t, err)

	ops, err := b.CompositeOperations()
	require.NoError(t, err)
	assert.Len(t, ops, 5)
}

func rawMig(name, opsJSON string) *RawMigration {
	return &RawMigration{
		Name:       name,
		Operations: json.RawMessage(opsJSON),
	}
}
