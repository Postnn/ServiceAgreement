package domain

import (
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

type Record struct {
	ContractID     int
	TypeID         int
	Period         int
	AutoPay        int
	Amount         string
	StartDay       int
	StartMonth     int
	DocumentsCode  int
	PaymentCode    int
	DisconnectCode int
	Notify         int
}

var moneyPattern = regexp.MustCompile(`^-?[0-9]{1,15}(?:\.[0-9]{1,4})?$`)
var maxMoney = big.NewInt(9223372036854775807)
var minMoney = new(big.Int).Neg(new(big.Int).Add(big.NewInt(9223372036854775807), big.NewInt(1)))

// NormalizeMoney avoids float64 and exactly preserves SQL Server money scale (4 digits).
// Supports the full signed SQL Server money range.
func NormalizeMoney(input string) (string, error) {
	s := strings.ReplaceAll(strings.TrimSpace(input), ",", ".")
	if !moneyPattern.MatchString(s) {
		return "", errors.New("сумма: укажите число с точностью до 4 знаков, без пробелов")
	}
	negative := strings.HasPrefix(s, "-")
	parts := strings.SplitN(strings.TrimPrefix(s, "-"), ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", 4-len(fraction))
	number := new(big.Int)
	number.SetString(parts[0]+fraction, 10)
	if negative {
		number.Neg(number)
	}
	if number.Cmp(maxMoney) > 0 || number.Cmp(minMoney) < 0 {
		return "", errors.New("сумма превышает диапазон SQL Server money")
	}
	sign := ""
	if number.Sign() < 0 {
		sign = "-"
	}
	abs := new(big.Int).Abs(number)
	whole := new(big.Int).Div(abs, big.NewInt(10000))
	rest := new(big.Int).Mod(abs, big.NewInt(10000))
	return fmt.Sprintf("%s%s.%04d", sign, whole.String(), rest.Int64()), nil
}

func (r *Record) Validate() error {
	if r.ContractID <= 0 {
		return errors.New("выберите договор")
	}
	if r.TypeID <= 0 {
		return errors.New("выберите тип документа автооплаты")
	}
	if r.Period < 1 || r.Period > 255 {
		return errors.New("период оказания услуги: укажите от 1 до 255 месяцев (SQL tinyint)")
	}
	if r.AutoPay < 0 || r.AutoPay > 1 || r.Notify < 0 || r.Notify > 1 {
		return errors.New("автооплата и оповещение куратора должны быть 0 или 1")
	}
	if r.StartDay < 1 || r.StartDay > 31 {
		return errors.New("день начала периода должен быть 1..31")
	}
	if r.StartMonth < 1 || r.StartMonth > 12 {
		return errors.New("месяц начала периода должен быть 1..12")
	}
	for _, code := range []struct {
		v int
		k string
	}{{r.DocumentsCode, "документы"}, {r.PaymentCode, "оплата"}, {r.DisconnectCode, "отключение"}} {
		if err := ValidateXDD(code.v, code.k); err != nil {
			return err
		}
	}
	amount, err := NormalizeMoney(r.Amount)
	if err != nil {
		return err
	}
	r.Amount = amount
	return nil
}
