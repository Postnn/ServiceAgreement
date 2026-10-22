package readmodel

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ListColumn maps a displayed column to a field returned by both SQL list queries.
// Order is one-based; SortField may reference another (even hidden) SQL field.
// Visible=false hides the column, but SQL must still return every configured alias.
type ListColumn struct {
	Field     string `json:"field"`
	Title     string `json:"title"`
	Width     int    `json:"width"`
	Order     int    `json:"order"`
	SortField string `json:"sortField"`
	Visible   bool   `json:"visible"`
}

// UnmarshalJSON upgrades pre-v4 column objects without losing the visible flag.
// Missing order is normalized to the legacy array position by LoadConfig.
func (c *ListColumn) UnmarshalJSON(data []byte) error {
	type wireColumn struct {
		Field     string  `json:"field"`
		Title     string  `json:"title"`
		Width     int     `json:"width"`
		Order     *int    `json:"order"`
		SortField *string `json:"sortField"`
		Visible   *bool   `json:"visible"`
	}
	var w wireColumn
	d := json.NewDecoder(strings.NewReader(string(data)))
	d.DisallowUnknownFields()
	if err := d.Decode(&w); err != nil {
		return err
	}
	*c = ListColumn{Field: w.Field, Title: w.Title, Width: w.Width,
		Order: -1, SortField: w.Field, Visible: true}
	if w.Order != nil {
		c.Order = *w.Order
	}
	if w.SortField != nil {
		c.SortField = *w.SortField
	}
	if w.Visible != nil {
		c.Visible = *w.Visible
	}
	return nil
}

// OrderedColumns returns a copy sorted by the explicit order property.
func OrderedColumns(cols []ListColumn) []ListColumn {
	result := append([]ListColumn(nil), cols...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].Order < result[j].Order })
	return result
}

// IsServiceField returns true for aliases needed for lookup and filtering,
// which must never be presented as ordinary columns in either table.
func IsServiceField(field string) bool {
	return strings.EqualFold(strings.TrimSpace(field), "ContractID") ||
		strings.EqualFold(strings.TrimSpace(field), "SearchText")
}

// VisibleColumns never displays service fields, even if an older config.json
// explicitly marks either of them visible.
func VisibleColumns(cols []ListColumn) []ListColumn {
	var visible []ListColumn
	for _, c := range OrderedColumns(cols) {
		if c.Visible && !IsServiceField(c.Field) {
			visible = append(visible, c)
		}
	}
	return visible
}

func ValidateColumns(cols []ListColumn) error {
	if len(cols) == 0 || len(cols) > 40 {
		return errors.New("задайте от 1 до 40 колонок списка")
	}
	seenFields, seenOrders := map[string]bool{}, map[int]bool{}
	visibleCount := 0
	for i, col := range cols {
		field := strings.TrimSpace(col.Field)
		if field == "" || strings.TrimSpace(col.Title) == "" || strings.TrimSpace(col.SortField) == "" {
			return fmt.Errorf("колонка %d: заполните field, title и sortField", i+1)
		}
		if col.Width < 45 || col.Width > 1600 {
			return fmt.Errorf("колонка %d: ширина должна быть 45..1600", i+1)
		}
		if col.Order < 1 || col.Order > 1000 || seenOrders[col.Order] {
			return fmt.Errorf("колонка %d: order должен быть уникальным числом от 1 до 1000", i+1)
		}
		seenOrders[col.Order] = true
		lower := strings.ToLower(field)
		if seenFields[lower] {
			return fmt.Errorf("колонка %d: повтор поля %s", i+1, field)
		}
		seenFields[lower] = true
		if col.Visible {
			visibleCount++
		}
	}
	if visibleCount == 0 {
		return errors.New("должна быть хотя бы одна видимая колонка списка")
	}
	return nil
}
