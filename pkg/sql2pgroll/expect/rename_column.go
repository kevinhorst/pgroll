// SPDX-License-Identifier: Apache-2.0

package expect

import "github.com/kevinhorst/pgroll/pkg/migrations"

var RenameColumnOp1 = &migrations.OpRenameColumn{
	Table: "foo",
	From:  "a",
	To:    "b",
}
