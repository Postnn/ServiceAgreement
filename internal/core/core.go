// Package core contains SQL Server operations and rules of the service agreements editor.
// It is independent of the Windows UI, making it testable on any operating system.
package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ListItem struct {
	ID     int
	Values map[string]string
}

type Option struct {
	ID    int
	Label string
}

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
	if r.Period != 1 && r.Period != 3 && r.Period != 12 {
		return errors.New("период оказания услуги: только 1, 3 или 12 месяцев")
	}
	if r.AutoPay < 0 || r.AutoPay > 1 || r.Notify < 0 || r.Notify > 1 {
		return errors.New("автооплата и оповещение куратора должны быть 0 или 1")
	}
	if r.StartDay < 1 || r.StartDay > 31 {
		return errors.New("день начала цикла должен быть 1..31")
	}
	if r.StartMonth < 1 || r.StartMonth > 12 {
		return errors.New("месяц начала цикла должен быть 1..12")
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

type Store struct {
	DB     *sql.DB
	Config Config
}

func (s Store) Lookup(ctx context.Context, query, search string, selectedID int) ([]Option, error) {
	rows, err := s.DB.QueryContext(ctx, query, sql.Named("search", strings.TrimSpace(search)), sql.Named("limit", s.Config.LookupLimit), sql.Named("selected", selectedID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Option, 0)
	for rows.Next() {
		var opt Option
		if err := rows.Scan(&opt.ID, &opt.Label); err != nil {
			return nil, fmt.Errorf("столбцы справочника: %w", err)
		}
		result = append(result, opt)
	}
	return result, rows.Err()
}

func (s Store) Get(ctx context.Context, id int) (Record, error) {
	var r Record
	err := s.DB.QueryRowContext(ctx, s.Config.GetSQL, sql.Named("id", id)).Scan(
		&r.ContractID, &r.TypeID, &r.Period, &r.AutoPay, &r.Amount,
		&r.StartDay, &r.StartMonth, &r.DocumentsCode, &r.PaymentCode, &r.DisconnectCode, &r.Notify)
	return r, err
}

func recordArgs(r Record) []interface{} {
	return []interface{}{sql.Named("id", r.ContractID), sql.Named("type", r.TypeID), sql.Named("period", r.Period),
		sql.Named("auto", r.AutoPay), sql.Named("amount", r.Amount), sql.Named("day", r.StartDay),
		sql.Named("month", r.StartMonth), sql.Named("docs", r.DocumentsCode), sql.Named("pay", r.PaymentCode),
		sql.Named("off", r.DisconnectCode), sql.Named("notify", r.Notify)}
}
func (s Store) Insert(ctx context.Context, r Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	_, err := s.DB.ExecContext(ctx, s.Config.InsertSQL, recordArgs(r)...)
	return err
}

var ErrConflict = errors.New("запись изменена или удалена другим пользователем; обновите список")

func (s Store) Update(ctx context.Context, original, updated Record) error {
	if updated.ContractID != original.ContractID {
		return errors.New("ключ договора изменять нельзя")
	}
	if err := updated.Validate(); err != nil {
		return err
	}
	// Preserve exact original SQL money scale for optimistic concurrency checks.
	oldAmount, err := NormalizeMoney(original.Amount)
	if err != nil {
		return err
	}
	args := append(recordArgs(updated),
		sql.Named("old_type", original.TypeID), sql.Named("old_period", original.Period),
		sql.Named("old_auto", original.AutoPay), sql.Named("old_amount", oldAmount),
		sql.Named("old_day", original.StartDay), sql.Named("old_month", original.StartMonth),
		sql.Named("old_docs", original.DocumentsCode), sql.Named("old_pay", original.PaymentCode),
		sql.Named("old_off", original.DisconnectCode), sql.Named("old_notify", original.Notify))
	res, err := s.DB.ExecContext(ctx, s.Config.UpdateSQL, args...)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrConflict
	}
	return nil
}

func (s Store) Delete(ctx context.Context, id int) error {
	res, err := s.DB.ExecContext(ctx, s.Config.DeleteSQL, sql.Named("id", id))
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrConflict
	}
	return nil
}

// QueryContext helps UI calls finish instead of freezing on unreachable database.
func QueryContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 12*time.Second)
}

// ParseIntField is shared by UI and unit tests.
func ParseIntField(label, input string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return 0, fmt.Errorf("%s: требуется целое число", label)
	}
	return n, nil
}
