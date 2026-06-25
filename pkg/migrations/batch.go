// SPDX-License-Identifier: Apache-2.0

package migrations

import (
	"encoding/json"
	"fmt"
)

// BatchFormatV1 is the format identifier for the batch envelope stored in the migrations table.
const BatchFormatV1 = "batch/v1"

// Batch groups multiple migrations into a single expand/contract transition.
type Batch struct {
	Members []*RawMigration
}

// NewBatch creates a Batch from the given raw migrations after validation.
// At least 2 members are required; single migrations should use the normal path.
func NewBatch(members []*RawMigration) (*Batch, error) {
	if len(members) < 2 {
		return nil, fmt.Errorf("batch requires at least 2 migrations, got %d", len(members))
	}

	for _, m := range members {
		parsed, err := ParseMigration(m)
		if err != nil {
			return nil, fmt.Errorf("batch member %q: %w", m.Name, err)
		}
		for _, op := range parsed.Operations {
			if _, ok := op.(*OpRawSQL); ok {
				return nil, fmt.Errorf("batch member %q contains a raw SQL operation, which is not supported in batch migrations", m.Name)
			}
		}
	}

	return &Batch{Members: members}, nil
}

// Name returns the last member's name, representing the version the batch brings the database to.
func (b *Batch) Name() string {
	return b.Members[len(b.Members)-1].Name
}

// VersionSchemaName returns the version schema name for the batch.
func (b *Batch) VersionSchemaName() string {
	last := b.Members[len(b.Members)-1]
	if last.VersionSchema != "" {
		return last.VersionSchema
	}
	return last.Name
}

// MemberNames returns the ordered list of member migration names.
func (b *Batch) MemberNames() []string {
	names := make([]string, len(b.Members))
	for i, m := range b.Members {
		names[i] = m.Name
	}
	return names
}

// CompositeOperations parses all members and returns a flat slice of all operations.
func (b *Batch) CompositeOperations() (Operations, error) {
	var ops Operations
	for _, m := range b.Members {
		parsed, err := ParseMigration(m)
		if err != nil {
			return nil, fmt.Errorf("batch member %q: %w", m.Name, err)
		}
		ops = append(ops, parsed.Operations...)
	}
	return ops, nil
}

// CompositeMigration builds a single Migration from all batch members.
func (b *Batch) CompositeMigration() (*Migration, error) {
	ops, err := b.CompositeOperations()
	if err != nil {
		return nil, err
	}
	return &Migration{
		Name:          b.Name(),
		VersionSchema: b.VersionSchemaName(),
		Operations:    ops,
	}, nil
}

// IsBatchJSON returns true if the raw JSON represents a batch envelope.
func IsBatchJSON(raw []byte) bool {
	var header struct {
		Format string `json:"format"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return false
	}
	return header.Format == BatchFormatV1
}

// DecodeBatchMemberNames extracts the ordered member names from a batch envelope JSON.
// Returns nil if the JSON is not a batch.
func DecodeBatchMemberNames(raw []byte) ([]string, error) {
	var header struct {
		Format  string `json:"format"`
		Members []struct {
			Name string `json:"name"`
		} `json:"members"`
	}
	if err := json.Unmarshal(raw, &header); err != nil {
		return nil, fmt.Errorf("decoding batch header: %w", err)
	}
	if header.Format != BatchFormatV1 {
		return nil, nil
	}
	names := make([]string, len(header.Members))
	for i, m := range header.Members {
		names[i] = m.Name
	}
	return names, nil
}

// DecodeBatchRawMembers extracts the individual RawMigration objects from a
// batch envelope JSON. Returns nil if the JSON is not a batch.
func DecodeBatchRawMembers(raw []byte) ([]*RawMigration, error) {
	var envelope struct {
		Format  string `json:"format"`
		Members []struct {
			Name          string          `json:"name"`
			VersionSchema string          `json:"version_schema,omitempty"`
			Operations    json.RawMessage `json:"operations"`
		} `json:"members"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decoding batch envelope: %w", err)
	}
	if envelope.Format != BatchFormatV1 {
		return nil, nil
	}
	result := make([]*RawMigration, len(envelope.Members))
	for i, m := range envelope.Members {
		result[i] = &RawMigration{
			Name:          m.Name,
			VersionSchema: m.VersionSchema,
			Operations:    m.Operations,
		}
	}
	return result, nil
}
