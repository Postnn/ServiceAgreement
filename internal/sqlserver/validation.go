package sqlserver

import (
	"fmt"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

// ValidateResultColumns rejects missing and duplicate SQL aliases even for an empty list.
// No virtual columns are computed by Go. field and sortField must come from SQL.
func ValidateResultColumns(names []string, columns []readmodel.ListColumn) (int, error) {
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
