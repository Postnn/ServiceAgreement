package readmodel

import "strings"

// ColumnMetadata describes the SQL result independently of its rows and UI settings.
// The Known flags distinguish unavailable driver metadata from false/zero values.
type ColumnMetadata struct {
	Name             string
	DatabaseType     string
	Nullable         bool
	NullableKnown    bool
	Precision        int64
	Scale            int64
	DecimalSizeKnown bool
}

// ListResult is shared by both agreement lists, including empty results.
type ListResult struct {
	Columns []ColumnMetadata
	Items   []ListItem
}

func (r ListResult) ColumnNames() []string {
	names := make([]string, len(r.Columns))
	for i, c := range r.Columns {
		names[i] = c.Name
	}
	return names
}

func (r ListResult) ColumnType(field string) string {
	for _, c := range r.Columns {
		if strings.EqualFold(c.Name, field) {
			return c.DatabaseType
		}
	}
	return ""
}
