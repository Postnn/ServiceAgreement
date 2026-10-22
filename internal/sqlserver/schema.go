package sqlserver

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Postnn/ServiceAgreement/internal/config"
	"github.com/Postnn/ServiceAgreement/internal/readmodel"
	"github.com/Postnn/ServiceAgreement/internal/sqlquery"
)

// QueryListSchema reads metadata only from list_sql. The word search is an
// outer SELECT, so no secondary search_sql or cross-query comparison exists.
func QueryListSchema(ctx context.Context, db *sql.DB, cfg config.Config) ([]string, error) {
	query, args, err := sqlquery.BuildListSearchQuery(cfg.ListSQL, "", 1, cfg.ListOrderBy)
	if err != nil {
		return nil, fmt.Errorf("list_sql: %w", err)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list_sql: %w", err)
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("list_sql: %w", err)
	}
	if _, err := readmodel.ColumnNames(names); err != nil {
		return nil, fmt.Errorf("list_sql: %w", err)
	}
	return names, nil
}
