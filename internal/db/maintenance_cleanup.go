package db

import (
	"context"
	"database/sql"
)

const maintenanceCleanupBatchSize = 1000

// executeCleanupBatches deletes at most maintenanceCleanupBatchSize rows per
// statement so large cleanup backlogs make progress within the query timeout.
func executeCleanupBatches(ctx context.Context, conn *sql.DB, query string) error {
	for {
		result, err := conn.ExecContext(ctx, query, maintenanceCleanupBatchSize)
		if err != nil {
			return err
		}
		deleted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if deleted < maintenanceCleanupBatchSize {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}
