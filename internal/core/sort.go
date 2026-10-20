package core

import (
	"math/big"
	"sort"
	"strings"
	"time"
)

// compareSQLValues compares SQL returned values for UI sorting. All computed
// columns must be projected in SQL; this function only orders existing rows.
func compareSQLValues(a, b any) int {
	if a == nil && b == nil {
		return 0
	}
	if a == nil {
		return -1
	}
	if b == nil {
		return 1
	}
	at, isDateA := a.(time.Time)
	bt, isDateB := b.(time.Time)
	if isDateA && isDateB {
		if at.Before(bt) {
			return -1
		}
		if at.After(bt) {
			return 1
		}
		return 0
	}
	// Use big.Rat so SQL money/decimal and 64-bit integers sort exactly.
	as, bs := sqlText(a), sqlText(b)
	ar, okA := new(big.Rat).SetString(strings.TrimSpace(as))
	br, okB := new(big.Rat).SetString(strings.TrimSpace(bs))
	if okA && okB {
		return ar.Cmp(br)
	}
	return strings.Compare(strings.ToLower(as), strings.ToLower(bs))
}

// SortListItems sorts just the fetched rows, not the SQL result set on server.
// The original ORDER BY in list_sql remains in force until a column is clicked.
func SortListItems(rows []ListItem, sortField string, descending bool) {
	if strings.TrimSpace(sortField) == "" {
		return
	}
	sort.SliceStable(rows, func(i, j int) bool {
		cmp := compareSQLValues(rows[i].SQLValue(sortField), rows[j].SQLValue(sortField))
		if descending {
			return cmp > 0
		}
		return cmp < 0
	})
}