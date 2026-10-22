package sqlserver

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/readmodel"
	"github.com/Postnn/ServiceAgreement/internal/sqlquery"
)

// List returns raw values and metadata from one query. Appearance settings are
// reconciled by the caller after loading; the repository does not format cells.
func (s Store) List(ctx context.Context, search string) (readmodel.ListResult, error) {
	result, err := s.queryList(ctx, s.Config.ListSQL, search, s.Config.ListOrderBy)
	if err == nil {
		_, err = ValidateResultColumns(result.ColumnNames(), s.Config.ListColumns)
	}
	return result, err
}

func (s Store) ListUnconfigured(ctx context.Context, search string) (readmodel.ListResult, error) {
	return s.queryList(ctx, s.Config.WithoutParametersSQL, search, s.Config.WithoutParametersOrderBy)
}

func (s Store) queryList(ctx context.Context, source, search, order string) (readmodel.ListResult, error) {
	result := readmodel.ListResult{Items: make([]readmodel.ListItem, 0)}
	if s.DB == nil {
		return result, fmt.Errorf("не задано соединение с SQL Server")
	}
	query, args, err := sqlquery.BuildListSearchQuery(source, search, s.Config.ListLimit, order)
	if err != nil {
		return result, err
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return result, err
	}
	if _, err = readmodel.ColumnNames(names); err != nil {
		return result, err
	}
	idColumn, err := ValidateResultColumns(names, nil)
	if err != nil {
		return result, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return result, err
	}
	result.Columns = make([]readmodel.ColumnMetadata, len(names))
	for i, name := range names {
		c := readmodel.ColumnMetadata{Name: name, DatabaseType: types[i].DatabaseTypeName()}
		c.Nullable, c.NullableKnown = types[i].Nullable()
		c.Precision, c.Scale, c.DecimalSizeKnown = types[i].DecimalSize()
		result.Columns[i] = c
	}
	for rows.Next() {
		values := make([]any, len(names))
		dest := make([]any, len(names))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return result, err
		}
		var idText string
		if b, ok := values[idColumn].([]byte); ok {
			idText = string(b)
		} else {
			idText = fmt.Sprint(values[idColumn])
		}
		id, err := strconv.Atoi(strings.TrimSpace(idText))
		if err != nil || id <= 0 {
			return result, fmt.Errorf("неверный ContractID %q", idText)
		}
		item := readmodel.ListItem{ID: id, Raw: make(map[string]any, len(names))}
		for i, name := range names {
			if b, ok := values[i].([]byte); ok {
				item.Raw[name] = append([]byte(nil), b...)
			} else {
				item.Raw[name] = values[i]
			}
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}
