package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// UnconfiguredResult retains the actual SELECT projection so every returned
// column can be shown in the second list, including when there are no rows.
// No list_columns settings, virtual fields or hidden fields are used here.
type UnconfiguredResult struct {
	Columns []string
	Items   []ListItem
}

// UnconfiguredColumns displays all returned columns except ContractID and
// SearchText. Those service aliases remain in the SQL result for identity
// and searching, but are never added to the table UI.
func UnconfiguredColumns(names []string) []ListColumn {
	columns := make([]ListColumn, 0, len(names))
	for _, name := range names {
        if IsServiceField(name) {
            continue
        }
        width := 170
        if strings.EqualFold(name, "ContractName") || strings.EqualFold(name, "Договор") {
            width = 350
        }
        columns = append(columns, ListColumn{
            Field: name, Title: name, Width: width, Order: len(columns) + 1,
            SortField: name, Visible: true,
        })
	}
	return columns
}

// ListUnconfigured executes the single configured SQL query for both the
// unfiltered and filtered "contracts without parameters" views.
// The configured SELECT projects positive ContractID and SearchText; the
// same outer word filter is applied as for the first list.
func (s Store) ListUnconfigured(ctx context.Context, search string) (UnconfiguredResult, error) {
	if s.DB == nil {
		return UnconfiguredResult{}, fmt.Errorf("не задано соединение с SQL Server")
	}
	query, args, err := BuildListSearchQuery(s.Config.WithoutParametersSQL, search, s.Config.ListLimit, s.Config.WithoutParametersOrderBy)
	if err != nil {
		return UnconfiguredResult{}, fmt.Errorf("without_parameters_sql: %w", err)
	}
	rows, err := s.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return UnconfiguredResult{}, fmt.Errorf("договоры без параметров: %w", err)
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return UnconfiguredResult{}, err
	}
    moneyType := make([]string, len(names))
    if types, err := rows.ColumnTypes(); err == nil {
        for i, typ := range types {
            if i < len(moneyType) { moneyType[i] = typ.DatabaseTypeName() }
        }
    }
	if _, err := columnNames(names); err != nil {
		return UnconfiguredResult{}, fmt.Errorf("without_parameters_sql: %w", err)
	}
	idIndex, err := ValidateResultColumns(names, nil)
	if err != nil {
		return UnconfiguredResult{}, err
	}
	result := UnconfiguredResult{
		Columns: append([]string(nil), names...),
		Items: make([]ListItem, 0),
	}
	for rows.Next() {
		values := make([]any, len(names))
		toScan := make([]any, len(names))
		for i := range values {
			toScan[i] = &values[i]
		}
		if err := rows.Scan(toScan...); err != nil {
			return UnconfiguredResult{}, fmt.Errorf("чтение договора без параметров: %w", err)
		}
		id, err := strconv.Atoi(strings.TrimSpace(sqlText(values[idIndex])))
		if err != nil || id <= 0 {
			return UnconfiguredResult{}, fmt.Errorf("неверный ContractID %q в without_parameters_sql", sqlText(values[idIndex]))
		}
		item := ListItem{ID: id, Values: make(map[string]string, len(names)), Raw: make(map[string]any, len(names))}
		for i, name := range names {
			item.Values[name] = SQLDisplayValue(values[i], moneyType[i], name)
			if bytes, ok := values[i].([]byte); ok {
				item.Raw[name] = append([]byte(nil), bytes...)
			} else {
				item.Raw[name] = values[i]
			}
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return UnconfiguredResult{}, err
	}
	return result, nil
}
