package presentation

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/domain"
)

// MoneyDisplayN2 renders numeric MONEY/SMALLMONEY values in Russian N2 form:
// a non-breaking space between thousands and a comma before two decimals.
// A big.Rat avoids float conversions and preserves SQL's 4-digit money scale.
// Exact half-cent cases use midpoint-to-even rounding.
func MoneyDisplayN2(value any) (string, bool) {
	if value == nil {
		return "", true
	}
	var source string
	switch n := value.(type) {
	case string:
		source = strings.TrimSpace(n)
	case []byte:
		source = strings.TrimSpace(string(n))
	case float32:
		source = strconv.FormatFloat(float64(n), 'f', 4, 32)
	case float64:
		source = strconv.FormatFloat(n, 'f', 4, 64)
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		source = fmt.Sprint(n)
	default:
		source = strings.TrimSpace(fmt.Sprint(value))
	}
	if source == "" {
		return "", true
	}
	if strings.Contains(source, ",") && !strings.Contains(source, ".") {
		source = strings.ReplaceAll(source, ",", ".")
	}
	money, ok := new(big.Rat).SetString(source)
	if !ok {
		return "", false
	}
	negative := money.Sign() < 0
	if negative {
		money.Abs(money)
	}
	money.Mul(money, big.NewRat(100, 1))
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(money.Num(), money.Denom(), remainder)
	twice := new(big.Int).Mul(remainder, big.NewInt(2))
	cmp := twice.Cmp(money.Denom())
	if cmp > 0 || cmp == 0 && quotient.Bit(0) != 0 {
		quotient.Add(quotient, big.NewInt(1))
	}
	if quotient.Sign() == 0 {
		negative = false
	}
	whole := new(big.Int)
	hundredths := new(big.Int)
	whole.QuoRem(quotient, big.NewInt(100), hundredths)
	digits := whole.String()
	if len(digits) > 3 {
		var grouped strings.Builder
		for i, r := range digits {
			if i > 0 && (len(digits)-i)%3 == 0 {
				grouped.WriteRune('\u00a0')
			}
			grouped.WriteRune(r)
		}
		digits = grouped.String()
	}
	prefix := ""
	if negative {
		prefix = "-"
	}
	return fmt.Sprintf("%s%s,%02d", prefix, digits, hundredths.Int64()), true
}

// SQLDisplayValue formats N2 ONLY for SQL Server MONEY columns.
// Alias, display name, precision or textual values cannot trigger N2.
// The original unrounded SQL value is retained separately in ListItem.Raw.
func SQLDisplayValue(value any, dbType, _ string) string {
	if isMoneySQLType(dbType) {
		if formatted, ok := MoneyDisplayN2(value); ok {
			return formatted
		}
	}
	return SQLText(value)
}

// ParseMoneyEditorText accepts Russian N2 input ("1 234,56", with nonbreaking
// spaces) as well as SQL-style decimal input. The database still receives
// canonical money(4) values, never a locale-formatted string.
func ParseMoneyEditorText(input string) (string, error) {
	s := strings.TrimSpace(input)
	s = strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "", "\t", "", ",", ".").Replace(s)
	return domain.NormalizeMoney(s)
}

// MoneyValueFromEditor preserves SQL money's four-decimal precision when the
// user changes only unrelated fields. Merely displaying the original as N2
// must not round its stored value on Save.
func MoneyValueFromEditor(input, displayedOriginal, exactOriginal string) (string, error) {
	if strings.TrimSpace(input) == displayedOriginal {
		return domain.NormalizeMoney(exactOriginal)
	}
	return ParseMoneyEditorText(input)
}
