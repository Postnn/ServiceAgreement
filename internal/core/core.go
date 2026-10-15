// Package core contains SQL Server operations and rules of the service agreements editor.
// It is independent of the Windows UI, making it testable on any operating system.
package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const target = "dbo.ДокументыДоговораОказанияУслуг"

// Config is intentionally limited to connection details and read-only SQL queries.
// Queries must return columns in the documented order and use named parameters.
type Config struct {
	ConnectionString string `json:"connection_string"`
	ListSQL          string `json:"list_sql"`
	ContractsSQL     string `json:"contracts_sql"`
	DocumentTypesSQL string `json:"document_types_sql"`
	ListLimit        int    `json:"list_limit"`
	LookupLimit      int    `json:"lookup_limit"`
}

func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("открыть конфигурацию: %w", err)
	}
	defer f.Close()
	var c Config
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("формат конфигурации: %w", err)
	}
	var extra interface{}
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, errors.New("в config.json более одного JSON-объекта")
		}
		return Config{}, fmt.Errorf("лишние данные после JSON: %w", err)
	}
	if strings.TrimSpace(c.ConnectionString) == "" {
		return Config{}, errors.New("connection_string не задан")
	}
	for field, val := range map[string]string{"list_sql": c.ListSQL, "contracts_sql": c.ContractsSQL, "document_types_sql": c.DocumentTypesSQL} {
		if strings.TrimSpace(val) == "" {
			return Config{}, fmt.Errorf("%s не задан", field)
		}
	}
	if c.ListLimit == 0 {
		c.ListLimit = 200
	}
	if c.LookupLimit == 0 {
		c.LookupLimit = 100
	}
	if c.ListLimit < 1 || c.ListLimit > 1000 || c.LookupLimit < 1 || c.LookupLimit > 500 {
		return Config{}, errors.New("list_limit должен быть 1..1000, lookup_limit — 1..500")
	}
	return c, nil
}

type ListItem struct {
	ID             int
	Agreement      string
	Period         int
	Amount         string
	AutoPay        int
	StartDay       int
	StartMonth     int
	DocumentsCode  int
	PaymentCode    int
	DisconnectCode int
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

func (s Store) List(ctx context.Context, search string) ([]ListItem, error) {
	rows, err := s.DB.QueryContext(ctx, s.Config.ListSQL, sql.Named("search", strings.TrimSpace(search)), sql.Named("limit", s.Config.ListLimit))
	if err != nil {
		return nil, fmt.Errorf("список договоров: %w", err)
	}
	defer rows.Close()
	result := make([]ListItem, 0)
	for rows.Next() {
		var r ListItem
		if err := rows.Scan(&r.ID, &r.Agreement, &r.Period, &r.Amount, &r.AutoPay, &r.StartDay, &r.StartMonth, &r.DocumentsCode, &r.PaymentCode, &r.DisconnectCode); err != nil {
			return nil, fmt.Errorf("столбцы list_sql: %w", err)
		}
		result = append(result, r)
	}
	return result, rows.Err()
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

const selectSQL = `SELECT
 [КодДокументаДоговораОказанияУслуг], [КодТипаДокументаОснованияАвтооплаты],
 [ПериодОказанияУслуги], [Автооплата],
 CONVERT(nvarchar(40),CONVERT(decimal(19,4),[СуммаАбонентскойПлаты])),
 [ДеньМесяцаНачалаОказанияУслуги], [МесяцНачалаОказанияУслуги],
 [ДеньМесяцаПолученияДокументов], [ДеньМесяцаОплатыПоДоговору],
 [ДеньМесяцаОтключенияУслуги], [ОповещениеКуратора]
FROM ` + target + ` WHERE [КодДокументаДоговораОказанияУслуг]=@id`

func (s Store) Get(ctx context.Context, id int) (Record, error) {
	var r Record
	err := s.DB.QueryRowContext(ctx, selectSQL, sql.Named("id", id)).Scan(
		&r.ContractID, &r.TypeID, &r.Period, &r.AutoPay, &r.Amount,
		&r.StartDay, &r.StartMonth, &r.DocumentsCode, &r.PaymentCode, &r.DisconnectCode, &r.Notify)
	return r, err
}

const insertSQL = `INSERT INTO ` + target + ` (
 [КодДокументаДоговораОказанияУслуг],[КодТипаДокументаОснованияАвтооплаты],
 [ПериодОказанияУслуги],[Автооплата],[СуммаАбонентскойПлаты],
 [ДеньМесяцаНачалаОказанияУслуги],[МесяцНачалаОказанияУслуги],
 [ДеньМесяцаПолученияДокументов],[ДеньМесяцаОплатыПоДоговору],
 [ДеньМесяцаОтключенияУслуги],[ОповещениеКуратора]
) VALUES (@id,@type,@period,@auto,CONVERT(money,@amount),@day,@month,@docs,@pay,@off,@notify)`

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
	_, err := s.DB.ExecContext(ctx, insertSQL, recordArgs(r)...)
	return err
}

const updateSQL = `UPDATE ` + target + ` SET
 [КодТипаДокументаОснованияАвтооплаты]=@type,
 [ПериодОказанияУслуги]=@period,[Автооплата]=@auto,
 [СуммаАбонентскойПлаты]=CONVERT(money,@amount),
 [ДеньМесяцаНачалаОказанияУслуги]=@day,
 [МесяцНачалаОказанияУслуги]=@month,
 [ДеньМесяцаПолученияДокументов]=@docs,
 [ДеньМесяцаОплатыПоДоговору]=@pay,
 [ДеньМесяцаОтключенияУслуги]=@off,
 [ОповещениеКуратора]=@notify
WHERE [КодДокументаДоговораОказанияУслуг]=@id
 AND [КодТипаДокументаОснованияАвтооплаты]=@old_type
 AND [ПериодОказанияУслуги]=@old_period AND [Автооплата]=@old_auto
 AND [СуммаАбонентскойПлаты]=CONVERT(money,@old_amount)
 AND [ДеньМесяцаНачалаОказанияУслуги]=@old_day
 AND [МесяцНачалаОказанияУслуги]=@old_month
 AND [ДеньМесяцаПолученияДокументов]=@old_docs
 AND [ДеньМесяцаОплатыПоДоговору]=@old_pay
 AND [ДеньМесяцаОтключенияУслуги]=@old_off
 AND [ОповещениеКуратора]=@old_notify`

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
	res, err := s.DB.ExecContext(ctx, updateSQL, args...)
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
	res, err := s.DB.ExecContext(ctx, `DELETE FROM `+target+` WHERE [КодДокументаДоговораОказанияУслуг]=@id`, sql.Named("id", id))
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