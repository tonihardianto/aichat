package migrations

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
)

//go:embed 000001_init.sql
var initSQL string

func Run(ctx context.Context, db *sql.DB) error {
	queries := strings.Split(initSQL, ";")
	for _, query := range queries {
		query = strings.TrimSpace(query)
		if query == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, query); err != nil {
			return fmt.Errorf("failed executing migration statement: %w\nStatement: %s", err, query)
		}
	}
	return nil
}
