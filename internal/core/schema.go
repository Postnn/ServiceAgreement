package core

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// ColumnSync describes the difference between configured columns and the
// actual SQL SELECT projection. Callers must ask permission before discarding
// columns which disappeared from SQL; additions are always visible.
type ColumnSync struct {
	Columns      []ListColumn
	Added        []string
	Removed      []string
	RepairedSort []string
}

func (s ColumnSync) Changed() bool {
	return len(s.Added) > 0 || len(s.Removed) > 0 || len(s.RepairedSort) > 0
}

func columnNames(names []string) (map[string]string, error) {
	known := make(map[string]string, len(names))
	for _, name := range names {
		normalized := strings.ToLower(strings.TrimSpace(name))
		if normalized == "" {
			return nil, fmt.Errorf("SQL возвращает столбец без имени — укажите AS <имя>")
		}
		if _, exists := known[normalized]; exists {
			return nil, fmt.Errorf("SQL возвращает повторяющееся имя столбца: %s", name)
		}
		known[normalized] = name
	}
	if _, exists := known["contractid"]; !exists {
		return nil, fmt.Errorf("SQL-список должен возвращать колонку ContractID — ключ выбранного договора")
	}
	if _, exists := known["searchtext"]; !exists {
		return nil, fmt.Errorf("SQL-список должен возвращать обязательную колонку SearchText для поиска")
	}
	if len(names) > 40 {
		return nil, fmt.Errorf("SQL возвращает %d колонок, максимально допустимо 40", len(names))
	}
	return known, nil
}

// ReconcileListColumns preserves user appearance settings. New SQL aliases
// are appended in SELECT order, visible=true. Removed columns are returned
// separately so that the user can confirm their removal.
// Missing sortField is reset to field rather than losing a still-valid column.
func ReconcileListColumns(existing []ListColumn, names []string) (ColumnSync, error) {
	known, err := columnNames(names)
	if err != nil {
		return ColumnSync{}, err
	}
	result := ColumnSync{Columns: make([]ListColumn, 0, len(names))}
	existingNames := make(map[string]bool, len(existing))
	maxOrder := 0
	for _, col := range existing {
		if col.Order > maxOrder {
			maxOrder = col.Order
		}
		fieldKey := strings.ToLower(strings.TrimSpace(col.Field))
		actualField, exists := known[fieldKey]
		if !exists {
			result.Removed = append(result.Removed, col.Field)
			continue
		}
		col.Field = actualField
		sortKey := strings.ToLower(strings.TrimSpace(col.SortField))
		if actualSort, ok := known[sortKey]; ok {
			col.SortField = actualSort
		} else {
			result.RepairedSort = append(result.RepairedSort, col.Field)
			col.SortField = col.Field
		}
		result.Columns = append(result.Columns, col)
		existingNames[fieldKey] = true
	}
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if existingNames[key] {
			continue
		}
		maxOrder++
		result.Columns = append(result.Columns, ListColumn{
			Field: name, Title: name, Width: 170, Order: maxOrder,
			SortField: name, Visible: true,
		})
		result.Added = append(result.Added, name)
	}
	result.Columns = OrderedColumns(result.Columns)
	if err := ValidateColumns(result.Columns); err != nil {
		return ColumnSync{}, fmt.Errorf("настройки колонок после синхронизации: %w", err)
	}
	return result, nil
}

// QueryListSchema reads metadata only from list_sql. The word search is an
// outer SELECT, so no secondary search_sql or cross-query comparison exists.
func QueryListSchema(ctx context.Context, db *sql.DB, cfg Config) ([]string, error) {
    query, args, err := BuildListSearchQuery(cfg.ListSQL, "", 1)
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
    if _, err := columnNames(names); err != nil {
        return nil, fmt.Errorf("list_sql: %w", err)
    }
    return names, nil
}
