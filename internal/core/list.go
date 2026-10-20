package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// sqlText formats the value returned by SQL Server; it never derives extra fields.
func sqlText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 {
			return x.Format("02.01.2006")
		}
		return x.Format("02.01.2006 15:04:05")
	default:
		return fmt.Sprint(x)
	}
}

func (r ListItem) Field(name string) string {
	for k, v := range r.Values {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// SQLValue returns the unformatted SQL value for type-aware sorting.
func (r ListItem) SQLValue(name string) any {
	for k, v := range r.Raw {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return r.Field(name)
}

// ValidateResultColumns rejects missing and duplicate SQL aliases even for an empty list.
// No virtual columns are computed by Go. field and sortField must come from SQL.
func ValidateResultColumns(names []string, columns []ListColumn) (int, error) {
	known := make(map[string]bool, len(names))
	idColumn := -1
	for i, name := range names {
		k := strings.ToLower(strings.TrimSpace(name))
		if k == "" || known[k] {
			return -1, fmt.Errorf("дублирующийся или пустой SQL-алиас %q", name)
		}
		known[k] = true
		if k == "contractid" {
			idColumn = i
		}
	}
	if idColumn < 0 {
		return -1, fmt.Errorf("SQL списка должен возвращать ContractID (даже если колонка скрыта)")
	}
	if !known["searchtext"] {
		return -1, fmt.Errorf("SQL списка должен возвращать обязательное поле SearchText для поиска")
	}
	for _, col := range columns {
		if !known[strings.ToLower(strings.TrimSpace(col.Field))] {
			return -1, fmt.Errorf("поле %q из list_columns отсутствует в SQL результате; добавьте его в list_sql либо уберите из list_columns", col.Field)
		}
		if !known[strings.ToLower(strings.TrimSpace(col.SortField))] {
			return -1, fmt.Errorf("поле сортировки %q для колонки %q отсутствует в SQL результате", col.SortField, col.Field)
		}
	}
	return idColumn, nil
}

func (s Store) List(ctx context.Context, search string) ([]ListItem, error) {
    query, args, err := BuildListSearchQuery(s.Config.ListSQL, search, s.Config.ListLimit, s.Config.ListOrderBy)
    if err != nil {
        return nil, fmt.Errorf("список договоров с параметрами: %w", err)
    }
    rows, err := s.DB.QueryContext(ctx, query, args...)
    if err != nil {
        return nil, fmt.Errorf("список договоров: %w", err)
    }
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
    moneyType := make([]string, len(names))
    if types, err := rows.ColumnTypes(); err == nil {
        for i, typ := range types {
            if i < len(moneyType) {
                moneyType[i] = typ.DatabaseTypeName()
            }
        }
    }
	idColumn, err := ValidateResultColumns(names, s.Config.ListColumns)
	if err != nil {
		return nil, err
	}

	result := make([]ListItem, 0)
	for rows.Next() {
		values := make([]any, len(names))
		dest := make([]any, len(names))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("прочитать столбцы списка: %w", err)
		}
		idText := sqlText(values[idColumn])
		id, err := strconv.Atoi(strings.TrimSpace(idText))
		if err != nil || id <= 0 {
			return nil, fmt.Errorf("неверный ContractID %q", idText)
		}
		item := ListItem{ID: id, Values: make(map[string]string, len(names)), Raw: make(map[string]any, len(names))}
		for i, name := range names {
			item.Values[name] = SQLDisplayValue(values[i], moneyType[i], name)
			// The data stays fully SQL-sourced; []byte is copied because sql.Rows may reuse it.
			if raw, ok := values[i].([]byte); ok {
				item.Raw[name] = append([]byte(nil), raw...)
			} else {
				item.Raw[name] = values[i]
			}
		}
		result = append(result, item)
	}
	return result, rows.Err()
}