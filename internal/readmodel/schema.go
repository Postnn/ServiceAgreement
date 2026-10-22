package readmodel

import (
	"fmt"
	"strings"
)

func ColumnNames(names []string) (map[string]string, error) {
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
