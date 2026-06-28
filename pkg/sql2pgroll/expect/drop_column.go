// SPDX-License-Identifier: Apache-2.0

package expect

import (
	"github.com/kevinhorst/pgroll/pkg/migrations"
	"github.com/kevinhorst/pgroll/pkg/sql2pgroll"
)

var DropColumnOp1 = &migrations.OpDropColumn{
	Table:  "foo",
	Column: "bar",
	Down:   sql2pgroll.PlaceHolderSQL,
}
