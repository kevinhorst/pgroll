// SPDX-License-Identifier: Apache-2.0

package roll

import (
	"context"
	"fmt"
	"io/fs"

	"github.com/kevinhorst/pgroll/pkg/migrations"
)

// MissingMigrations returns the slice of migrations that have been applied to
// the target database but are missing from the local migrations directory
// `dir`.
func (m *Roll) MissingMigrations(ctx context.Context, dir fs.FS) ([]*migrations.RawMigration, error) {
	// Collect all migration files from the directory
	files, err := migrations.CollectFilesFromDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading migration files: %w", err)
	}

	// Create a set of local migration names for fast lookup
	localMigNames := make(map[string]struct{}, len(files))
	for _, file := range files {
		mig, err := migrations.ReadRawMigration(dir, file)
		if err != nil {
			return nil, fmt.Errorf("reading migration file %s: %w", file, err)
		}
		localMigNames[mig.Name] = struct{}{}
	}

	// Get the full schema history from the database
	history, err := m.State().SchemaHistory(ctx, m.Schema())
	if err != nil {
		return nil, fmt.Errorf("reading schema history: %w", err)
	}

	// Find all migrations that have been applied to the database but are missing
	// from the local directory. Batch entries are expanded into individual members.
	migs := make([]*migrations.RawMigration, 0, len(history))
	for _, h := range history {
		if migrations.IsBatchJSON(h.RawJSON) {
			members, err := migrations.DecodeBatchRawMembers(h.RawJSON)
			if err != nil {
				return nil, fmt.Errorf("decoding batch members: %w", err)
			}
			for _, member := range members {
				if _, ok := localMigNames[member.Name]; !ok {
					migs = append(migs, member)
				}
			}
		} else {
			if _, ok := localMigNames[h.Migration.Name]; ok {
				continue
			}
			migs = append(migs, &h.Migration)
		}
	}

	return migs, nil
}
