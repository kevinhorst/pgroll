// SPDX-License-Identifier: Apache-2.0

package state

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kevinhorst/pgroll/pkg/migrations"
)

func TestEncodeBatchEnvelopeRoundTrip(t *testing.T) {
	batch, err := migrations.NewBatch([]*migrations.RawMigration{
		{
			Name:       "01_create_users",
			Operations: json.RawMessage(`[{"create_table":{"name":"users","columns":[{"name":"id","type":"int","pk":true}]}}]`),
		},
		{
			Name:          "02_add_email",
			VersionSchema: "custom_v",
			Operations:    json.RawMessage(`[{"add_column":{"table":"users","up":"''","column":{"name":"email","type":"text","nullable":true}}}]`),
		},
	})
	require.NoError(t, err)

	encoded, err := encodeBatchEnvelope(batch)
	require.NoError(t, err)

	var envelope storedBatchEnvelope
	err = json.Unmarshal(encoded, &envelope)
	require.NoError(t, err)

	assert.Equal(t, migrations.BatchFormatV1, envelope.Format)
	assert.Equal(t, "custom_v", envelope.VersionSchema)
	assert.Len(t, envelope.Members, 2)
	assert.Equal(t, "01_create_users", envelope.Members[0].Name)
	assert.Equal(t, "02_add_email", envelope.Members[1].Name)
	assert.Equal(t, "custom_v", envelope.Members[1].VersionSchema)

	// Flattened operations should contain both ops
	var flatOps []json.RawMessage
	err = json.Unmarshal(envelope.Operations, &flatOps)
	require.NoError(t, err)
	assert.Len(t, flatOps, 2)

	// Verify IsBatchJSON detects the envelope
	assert.True(t, migrations.IsBatchJSON(encoded))

	// Verify DecodeBatchMemberNames extracts names correctly
	names, err := migrations.DecodeBatchMemberNames(encoded)
	require.NoError(t, err)
	assert.Equal(t, []string{"01_create_users", "02_add_email"}, names)
}
