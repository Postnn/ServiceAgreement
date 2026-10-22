package readmodel

import "strings"

type ListItem struct {
	ID          int
	Values      map[string]string
	Raw         map[string]any    // original SQL values for sorting and typed filtering
	ColumnTypes map[string]string // SQL Server DatabaseTypeName by alias
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
