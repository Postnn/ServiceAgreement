package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ListColumn refers to either an alias returned by list_sql/search_sql or to
// one of the virtual aliases described in README.md. Order controls display.
type ListColumn struct {
	Field string `json:"field"`
	Title string `json:"title"`
	Width int    `json:"width"`
}

type Config struct {
	ConnectionString string       `json:"connection_string"`
	ListLimit        int          `json:"list_limit"`
	LookupLimit      int          `json:"lookup_limit"`
	ListSQL          string       `json:"list_sql"`
	SearchSQL        string       `json:"search_sql"`
	ListColumns      []ListColumn `json:"list_columns"`
	ContractsSQL     string       `json:"contracts_sql"`
	DocumentTypesSQL string       `json:"document_types_sql"`
	GetSQL           string       `json:"get_sql"`
	InsertSQL        string       `json:"insert_sql"`
	UpdateSQL        string       `json:"update_sql"`
	DeleteSQL        string       `json:"delete_sql"`
}

const listProjection = `SELECT TOP (@limit)
    t.[КодДокументаДоговораОказанияУслуг] AS ContractID,
    CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]) AS AgreementLabel,
    t.[ПериодОказанияУслуги] AS ServicePeriod,
    CONVERT(nvarchar(40), CONVERT(decimal(19,4), t.[СуммаАбонентскойПлаты])) AS Amount,
    t.[Автооплата] AS AutoPay,
    t.[ДеньМесяцаНачалаОказанияУслуги] AS StartDay,
    t.[МесяцНачалаОказанияУслуги] AS StartMonth,
    t.[ДеньМесяцаПолученияДокументов] AS DocumentsCode,
    t.[ДеньМесяцаОплатыПоДоговору] AS PaymentCode,
    t.[ДеньМесяцаОтключенияУслуги] AS DisconnectCode
FROM dbo.[ДокументыДоговораОказанияУслуг] AS t`

const defaultGetSQL = `SELECT
 [КодДокументаДоговораОказанияУслуг], [КодТипаДокументаОснованияАвтооплаты],
 [ПериодОказанияУслуги], [Автооплата],
 CONVERT(nvarchar(40),CONVERT(decimal(19,4),[СуммаАбонентскойПлаты])),
 [ДеньМесяцаНачалаОказанияУслуги], [МесяцНачалаОказанияУслуги],
 [ДеньМесяцаПолученияДокументов], [ДеньМесяцаОплатыПоДоговору],
 [ДеньМесяцаОтключенияУслуги], [ОповещениеКуратора]
FROM dbo.[ДокументыДоговораОказанияУслуг]
WHERE [КодДокументаДоговораОказанияУслуг] = @id`

const defaultInsertSQL = `INSERT INTO dbo.[ДокументыДоговораОказанияУслуг] (
 [КодДокументаДоговораОказанияУслуг],[КодТипаДокументаОснованияАвтооплаты],
 [ПериодОказанияУслуги],[Автооплата],[СуммаАбонентскойПлаты],
 [ДеньМесяцаНачалаОказанияУслуги],[МесяцНачалаОказанияУслуги],
 [ДеньМесяцаПолученияДокументов],[ДеньМесяцаОплатыПоДоговору],
 [ДеньМесяцаОтключенияУслуги],[ОповещениеКуратора]
) VALUES (@id,@type,@period,@auto,CONVERT(money,@amount),@day,@month,@docs,@pay,@off,@notify)`

const defaultUpdateSQL = `UPDATE dbo.[ДокументыДоговораОказанияУслуг] SET
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

const defaultDeleteSQL = `DELETE FROM dbo.[ДокументыДоговораОказанияУслуг]
WHERE [КодДокументаДоговораОказанияУслуг] = @id`

func DefaultConfig() Config {
	return Config{
		ConnectionString: "",
		ListLimit:        200, LookupLimit: 100,
		ListSQL: listProjection + `
ORDER BY t.[КодДокументаДоговораОказанияУслуг] DESC`,
		SearchSQL: listProjection + `
WHERE CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]) LIKE N'%' + @search + N'%'
ORDER BY t.[КодДокументаДоговораОказанияУслуг] DESC`,
		ListColumns: []ListColumn{
			{"ContractID", "Код договора", 120},
			{"AgreementLabel", "Договор", 220},
			{"ServicePeriodText", "Период", 120},
			{"Amount", "Абонентская плата", 140},
			{"DocumentsText", "Получение документов", 310},
			{"PaymentText", "Оплата по договору", 310},
			{"DisconnectText", "Отключение услуги", 310},
			{"AutoPayText", "Автооплата", 90},
		},
		ContractsSQL: `SELECT TOP (@limit)
 d.[КодДокумента] AS ContractID,
 CONVERT(nvarchar(30),d.[КодДокумента]) AS ContractLabel
FROM dbo.[ДокументыДанные] AS d
WHERE @search = N'' OR CONVERT(nvarchar(30),d.[КодДокумента]) LIKE N'%' + @search + N'%' OR d.[КодДокумента] = @selected
ORDER BY d.[КодДокумента] DESC`,
		DocumentTypesSQL: `SELECT TOP (@limit)
 t.[КодТипаДокумента] AS TypeID,
 CONVERT(nvarchar(30),t.[КодТипаДокумента]) AS TypeLabel
FROM dbo.[ТипыДокументов] AS t
WHERE @search = N'' OR CONVERT(nvarchar(30),t.[КодТипаДокумента]) LIKE N'%' + @search + N'%' OR t.[КодТипаДокумента] = @selected
ORDER BY t.[КодТипаДокумента]`,
		GetSQL: defaultGetSQL, InsertSQL: defaultInsertSQL, UpdateSQL: defaultUpdateSQL, DeleteSQL: defaultDeleteSQL,
	}
}

// SQLFields is ordered the same way as the configuration editor's query selector.
var SQLFields = []struct{ Key, Caption string }{
	{"list_sql", "Список договоров (без фильтра)"},
	{"search_sql", "Поиск по списку договоров"},
	{"contracts_sql", "Поиск и выбор договора"},
	{"document_types_sql", "Поиск типа документа основания автооплаты"},
	{"get_sql", "Чтение одной записи"},
	{"insert_sql", "Добавление записи"},
	{"update_sql", "Обновление записи"},
	{"delete_sql", "Удаление записи"},
}

func (c *Config) SQLAt(i int) *string {
	switch i {
	case 0:
		return &c.ListSQL
	case 1:
		return &c.SearchSQL
	case 2:
		return &c.ContractsSQL
	case 3:
		return &c.DocumentTypesSQL
	case 4:
		return &c.GetSQL
	case 5:
		return &c.InsertSQL
	case 6:
		return &c.UpdateSQL
	case 7:
		return &c.DeleteSQL
	}
	return nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ConnectionString) == "" {
		return errors.New("не задана строка подключения")
	}
	for i, item := range SQLFields {
		sql := c.SQLAt(i)
		if sql == nil || strings.TrimSpace(*sql) == "" {
			return fmt.Errorf("не задан запрос %s", item.Key)
		}
	}
	if c.ListLimit < 1 || c.ListLimit > 1000 || c.LookupLimit < 1 || c.LookupLimit > 500 {
		return errors.New("list_limit должен быть 1..1000, lookup_limit — 1..500")
	}
	return ValidateColumns(c.ListColumns)
}

func ValidateColumns(cols []ListColumn) error {
	if len(cols) == 0 || len(cols) > 40 {
		return errors.New("задайте от 1 до 40 колонок списка")
	}
	seen := map[string]bool{}
	for i, col := range cols {
		field := strings.TrimSpace(col.Field)
		if field == "" || strings.TrimSpace(col.Title) == "" {
			return fmt.Errorf("колонка %d: заполните поле и заголовок", i+1)
		}
		if col.Width < 45 || col.Width > 1600 {
			return fmt.Errorf("колонка %d: ширина должна быть 45..1600", i+1)
		}
		lower := strings.ToLower(field)
		if seen[lower] {
			return fmt.Errorf("колонка %d: повтор поля %s", i+1, field)
		}
		seen[lower] = true
	}
	return nil
}

// LoadConfig supplies defaults for new fields when reading an older config.json.
// Existing SQL connection and user-edited queries are never overwritten.
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("открыть конфигурацию: %w", err)
	}
	defer f.Close()
	c := DefaultConfig()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("формат конфигурации: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return Config{}, errors.New("в config.json более одного JSON-объекта")
		}
		return Config{}, fmt.Errorf("лишние данные после JSON: %w", err)
	}
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// SaveConfig stores a readable UTF-8 JSON file next to the executable.
// It keeps a .bak snapshot of an existing config before replacement.
func SaveConfig(path string, c Config) error {
	if err := c.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".serviceagreement-config-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	old, err := os.ReadFile(path)
	hadOld := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if hadOld {
		if err := os.WriteFile(path+".bak", old, 0600); err != nil {
			return fmt.Errorf("резервная копия конфигурации: %w", err)
		}
		// os.Rename does not replace an existing file reliably on Windows.
		if err := os.Remove(path); err != nil {
			return err
		}
	}
	if err := os.Rename(f.Name(), path); err != nil {
		if hadOld {
			_ = os.WriteFile(path, old, 0600)
		}
		return fmt.Errorf("замена config.json: %w", err)
	}
	return nil
}
