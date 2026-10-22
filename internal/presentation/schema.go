package presentation

import (
	"fmt"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

// ColumnSync describes the difference between configured columns and the
// actual SQL SELECT projection. Callers must ask permission before discarding
// columns which disappeared from SQL; additions are always visible.
type ColumnSync struct {
	Columns      []readmodel.ListColumn
	Added        []string
	Removed      []string
	RepairedSort []string
}

func (s ColumnSync) Changed() bool {
	return len(s.Added) > 0 || len(s.Removed) > 0 || len(s.RepairedSort) > 0
}

// ReconcileListColumns preserves user appearance settings. New SQL aliases
// are appended in SELECT order, visible=true. Removed columns are returned
// separately so that the user can confirm their removal.
// Missing sortField is reset to field rather than losing a still-valid column.
func ReconcileListColumns(existing []readmodel.ListColumn, names []string) (ColumnSync, error) {
	known, err := readmodel.ColumnNames(names)
	if err != nil {
		return ColumnSync{}, err
	}
	result := ColumnSync{Columns: make([]readmodel.ListColumn, 0, len(names))}
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
		result.Columns = append(result.Columns, readmodel.ListColumn{
			Field: name, Title: name, Width: 170, Order: maxOrder,
			SortField: name, Visible: !readmodel.IsServiceField(name),
		})
		result.Added = append(result.Added, name)
	}
	result.Columns = readmodel.OrderedColumns(result.Columns)
	if err := readmodel.ValidateColumns(result.Columns); err != nil {
		return ColumnSync{}, fmt.Errorf("настройки колонок после синхронизации: %w", err)
	}
	return result, nil
}
