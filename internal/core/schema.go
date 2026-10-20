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

// QueryListSchema retrieves SELECT result metadata from SQL Server, even if no
// records match. Both queries must expose exactly the same set of aliases, so
// no column disappears when the user switches between list and search.
func QueryListSchema(ctx context.Context, db *sql.DB, cfg Config) ([]string, error) {
	fetch := func(label, query, search string) ([]string, map[string]string, error) {
		rows, err := db.QueryContext(ctx, query,
			sql.Named("limit", 1), sql.Named("search", search))
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", label, err)
		}
		defer rows.Close()
		names, err := rows.Columns()
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", label, err)
		}
		known, err := columnNames(names)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", label, err)
		}
		return names, known, nil
	}
	list, listNames, err := fetch("list_sql", cfg.ListSQL, "")
	if err != nil {
		return nil, err
	}
	_, searchNames, err := fetch("search_sql", cfg.SearchSQL, "__ServiceAgreement_schema_check__")
	if err != nil {
		return nil, err
	}
	var onlyList, onlySearch []string
	for key, name := range listNames {
		if _, ok := searchNames[key]; !ok {
			onlyList = append(onlyList, name)
		}
	}
	for key, name := range searchNames {
		if _, ok := listNames[key]; !ok {
			onlySearch = append(onlySearch, name)
		}
	}
	if len(onlyList) > 0 || len(onlySearch) > 0 {
		// Put differences in SELECT order rather than random map iteration.
		orderedList, orderedSearch := make([]string, 0), make([]string, 0)
		for _, name := range list {
			if !containsName(searchNames, name) {
				orderedList = append(orderedList, name)
			}
		}
		// Sorting the second list matters only for a consistent error message.
		// SQL may use different column ordering between list and search.
		for _, name := range onlySearch {
			orderedSearch = append(orderedSearch, name)
		}
		return nil, fmt.Errorf("list_sql и search_sql возвращают разные поля. Только в list_sql: %s; только в search_sql: %s. Приведите SELECT к одинаковому составу колонок", strings.Join(orderedList, ", "), strings.Join(orderedSearch, ", "))
	}
	return list, nil
}

func containsName(known map[string]string, field string) bool {
	_, ok := known[strings.ToLower(strings.TrimSpace(field))]
	return ok
}