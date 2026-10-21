package core

import (
    "fmt"
    "math/big"
    "strings"
    "time"
)

// ColumnPresentation is determined by SQL Server DatabaseTypeName, never by
// a column alias, its label, or a textual representation of its values.
// MONEY alone has the special N2 display and right-alignment policy.
// Other SQL numeric types still use numeric filter operators.
type ColumnPresentation struct {
    Kind  FilterKind
    Money bool
}

func isMoneySQLType(sqlType string) bool {
    return strings.EqualFold(strings.TrimSpace(sqlType), "MONEY")
}

func isNumericSQLType(sqlType string) bool {
    switch strings.ToUpper(strings.TrimSpace(sqlType)) {
    case "MONEY", "SMALLMONEY", "DECIMAL", "NUMERIC", "FLOAT", "REAL",
        "DOUBLE", "DOUBLE PRECISION", "INTEGER", "TINYINT", "SMALLINT", "INT", "BIGINT", "BIT":
        return true
    }
    return false
}

func isDateSQLType(sqlType string) bool {
    switch strings.ToUpper(strings.TrimSpace(sqlType)) {
    case "DATE", "DATETIME", "DATETIME2", "SMALLDATETIME", "DATETIMEOFFSET":
        return true
    }
    return false
}

func sqlResultFieldType(rows []ListItem, field string) string {
    for _, row := range rows {
        for name, sqlType := range row.ColumnTypes {
            if strings.EqualFold(name, field) && sqlType != "" {
                return sqlType
            }
        }
    }
    return ""
}

// InferColumnPresentation uses SQL metadata as the source of truth.
// In particular, NVARCHAR with an alias such as "Абон.плата" or "Amount"
// remains text even if all values happen to contain four decimal places.
// Only if SQL type metadata is entirely absent do we inspect the Go type of
// the raw value; we never parse its textual contents as a heuristic.
func InferColumnPresentation(rows []ListItem, field string) ColumnPresentation {
    dbType := sqlResultFieldType(rows, field)
    if dbType != "" {
        if isMoneySQLType(dbType) {
            return ColumnPresentation{Kind: FilterNumber, Money: true}
        }
        if isNumericSQLType(dbType) {
            return ColumnPresentation{Kind: FilterNumber}
        }
        if isDateSQLType(dbType) {
            return ColumnPresentation{Kind: FilterDate}
        }
        return ColumnPresentation{Kind: FilterText}
    }
    for _, row := range rows {
        for name, value := range row.Raw {
            if !strings.EqualFold(name, field) || value == nil {
                continue
            }
            switch value.(type) {
            case time.Time:
                return ColumnPresentation{Kind: FilterDate}
            case int, int8, int16, int32, int64,
                uint, uint8, uint16, uint32, uint64,
                float32, float64, bool, *big.Rat, *big.Float:
                return ColumnPresentation{Kind: FilterNumber}
            }
        }
    }
    return ColumnPresentation{Kind: FilterText}
}

// FormatListMoneyN2 remains useful for callers that construct list items
// themselves, but applies ONLY to source columns whose SQL type is MONEY.
// Values not backed by MONEY metadata are never reformatted.
func FormatListMoneyN2(rows []ListItem) {
    for i := range rows {
        for field, dbType := range rows[i].ColumnTypes {
            if !isMoneySQLType(dbType) {
                continue
            }
            var raw any
            present := false
            for name, value := range rows[i].Raw {
                if strings.EqualFold(name, field) {
                    raw, present = value, true
                    break
                }
            }
            if !present {
                continue
            }
            formatted, ok := MoneyDisplayN2(raw)
            if !ok {
                continue
            }
            for name := range rows[i].Values {
                if strings.EqualFold(name, field) {
                    rows[i].Values[name] = formatted
                    break
                }
            }
        }
    }
}

func ValidateColumnPresentation(rows []ListItem, field string) error {
    presentation := InferColumnPresentation(rows, field)
    if presentation.Money && presentation.Kind != FilterNumber {
        return fmt.Errorf("поле MONEY %q должно использовать числовой тип", field)
    }
    return nil
}
