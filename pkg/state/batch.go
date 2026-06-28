// SPDX-License-Identifier: Apache-2.0

package state

import (
	"encoding/json"
	"fmt"

	"github.com/kevinhorst/pgroll/pkg/migrations"
)

// storedBatchEnvelope is the JSON structure persisted in the migrations table's
// migration column when a batch is applied.
type storedBatchEnvelope struct {
	Format        string              `json:"format"`
	VersionSchema string              `json:"version_schema,omitempty"`
	Operations    json.RawMessage     `json:"operations"`
	Members       []storedBatchMember `json:"members"`
}

type storedBatchMember struct {
	Name          string          `json:"name"`
	VersionSchema string          `json:"version_schema,omitempty"`
	Operations    json.RawMessage `json:"operations"`
}

// encodeBatchEnvelope serializes a Batch into the JSON envelope format used for storage.
func encodeBatchEnvelope(batch *migrations.Batch) ([]byte, error) {
	var allOps []json.RawMessage
	members := make([]storedBatchMember, len(batch.Members))

	for i, m := range batch.Members {
		var ops []json.RawMessage
		if err := json.Unmarshal(m.Operations, &ops); err != nil {
			return nil, fmt.Errorf("parsing operations for member %q: %w", m.Name, err)
		}
		allOps = append(allOps, ops...)
		members[i] = storedBatchMember{
			Name:          m.Name,
			VersionSchema: m.VersionSchema,
			Operations:    m.Operations,
		}
	}

	flatOps, err := json.Marshal(allOps)
	if err != nil {
		return nil, fmt.Errorf("marshaling flattened operations: %w", err)
	}

	envelope := storedBatchEnvelope{
		Format:        migrations.BatchFormatV1,
		VersionSchema: batch.VersionSchemaName(),
		Operations:    flatOps,
		Members:       members,
	}

	return json.Marshal(envelope)
}
