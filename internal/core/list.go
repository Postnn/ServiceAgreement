package core

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"
)

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

func (s Store) List(ctx context.Context, search string) ([]ListItem, error) {
	search = strings.TrimSpace(search)
	query := s.Config.ListSQL
	if search != "" {
		query = s.Config.SearchSQL
	}
	rows, err := s.DB.QueryContext(ctx, query, sql.Named("search", search), sql.Named("limit", s.Config.ListLimit))
	if err != nil {
		return nil, fmt.Errorf("список договоров: %w", err)
	}
	defer rows.Close()
	names, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	idColumn := -1
	known := make(map[string]bool, len(names)+4)
	for i, name := range names {
		k := strings.ToLower(name)
		if known[k] {
			return nil, fmt.Errorf("дублирующийся псевдоним столбца: %s", name)
		}
		known[k] = true
		if strings.EqualFold(name, "ContractID") {
			idColumn = i
		}
	}
	if idColumn < 0 {
		return nil, fmt.Errorf("SQL списка должен возвращать колонку ContractID")
	}
	virtual := []string{"ServicePeriodText", "AutoPayText", "DocumentsText", "PaymentText", "DisconnectText"}
	for _, name := range virtual {
		known[strings.ToLower(name)] = true
	}
	for _, col := range s.Config.ListColumns {
		if !known[strings.ToLower(col.Field)] {
			return nil, fmt.Errorf("поле %q из list_columns отсутствует в SQL результате", col.Field)
		}
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
		item := ListItem{ID: id, Values: make(map[string]string, len(names)+len(virtual))}
		for i, name := range names {
			item.Values[name] = sqlText(values[i])
		}
		deriveListFields(&item)
		result = append(result, item)
	}
	return result, rows.Err()
}

func deriveListFields(r *ListItem) {
	switch r.Field("ServicePeriod") {
	case "1":
		r.Values["ServicePeriodText"] = "Ежемесячно"
	case "3":
		r.Values["ServicePeriodText"] = "Ежеквартально"
	case "12":
		r.Values["ServicePeriodText"] = "Ежегодно"
	default:
		r.Values["ServicePeriodText"] = r.Field("ServicePeriod")
	}
	switch r.Field("AutoPay") {
	case "1":
		r.Values["AutoPayText"] = "Да"
	case "0":
		r.Values["AutoPayText"] = "Нет"
	default:
		r.Values["AutoPayText"] = r.Field("AutoPay")
	}
	startDay, _ := strconv.Atoi(r.Field("StartDay"))
	startMonth, _ := strconv.Atoi(r.Field("StartMonth"))
	for _, f := range []struct{ Raw, Virtual string }{
		{"DocumentsCode", "DocumentsText"},
		{"PaymentCode", "PaymentText"},
		{"DisconnectCode", "DisconnectText"},
	} {
		if code, err := strconv.Atoi(r.Field(f.Raw)); err == nil {
			description := ExplainXDD(code)
			if startDay > 0 && startMonth > 0 {
				description += "  →  " + ExampleDate(code, startDay, startMonth, time.Now().Year())
			}
			r.Values[f.Virtual] = description
		} else {
			r.Values[f.Virtual] = r.Field(f.Raw)
		}
	}
}
