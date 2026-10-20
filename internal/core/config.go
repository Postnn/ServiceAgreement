package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// VisibleColumns returns the configured display subset, in explicit order.
func VisibleColumns(cols []ListColumn) []ListColumn {
	var visible []ListColumn
	for _, c := range OrderedColumns(cols) {
		if c.Visible {
			visible = append(visible, c)
		}
	}
	return visible
}

type Config struct {
	ConnectionString string       `json:"connection_string"`
	ListLimit        int          `json:"list_limit"`
	LookupLimit      int          `json:"lookup_limit"`
	ListSQL          string       `json:"list_sql"`
	WithoutParametersSQL string `json:"without_parameters_sql"`
	ListColumns      []ListColumn `json:"list_columns"`
	ContractsSQL     string       `json:"contracts_sql"`
	DocumentTypesSQL string       `json:"document_types_sql"`
	GetSQL           string       `json:"get_sql"`
	InsertSQL        string       `json:"insert_sql"`
	UpdateSQL        string       `json:"update_sql"`
	DeleteSQL        string       `json:"delete_sql"`
}

const listProjection = `SELECT
    t.[КодДокументаДоговораОказанияУслуг] AS ContractID,
    CONVERT(nvarchar(max), d.[Договор]) AS AgreementLabel,
    CONCAT(CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]),
        N' ', COALESCE(CONVERT(nvarchar(max), d.[Договор]), N'')) AS SearchText,
    t.[ПериодОказанияУслуги] AS ServicePeriod,
    CASE t.[ПериодОказанияУслуги]
         WHEN 1 THEN N'Один месяц (1)'
         WHEN 3 THEN N'Три месяца (3)'
         WHEN 12 THEN N'Двенадцать месяцев (12)'
         ELSE CONVERT(nvarchar(3), t.[ПериодОказанияУслуги]) + N' ' +
              CASE WHEN t.[ПериодОказанияУслуги] % 100 BETWEEN 11 AND 14 THEN N'месяцев'
                   WHEN t.[ПериодОказанияУслуги] % 10 = 1 THEN N'месяц'
                   WHEN t.[ПериодОказанияУслуги] % 10 BETWEEN 2 AND 4 THEN N'месяца'
                   ELSE N'месяцев' END END AS ServicePeriodText,
    CONVERT(nvarchar(40), CONVERT(decimal(19,4), t.[СуммаАбонентскойПлаты])) AS Amount,
    t.[Автооплата] AS AutoPay,
    CASE t.[Автооплата] WHEN 1 THEN N'Да' ELSE N'Нет' END AS AutoPayText,
    t.[ДеньМесяцаНачалаОказанияУслуги] AS StartDay,
    t.[МесяцНачалаОказанияУслуги] AS StartMonth,
    t.[ДеньМесяцаПолученияДокументов] AS DocumentsCode,
    t.[ДеньМесяцаОплатыПоДоговору] AS PaymentCode,
    t.[ДеньМесяцаОтключенияУслуги] AS DisconnectCode,
    CASE WHEN t.[ДеньМесяцаПолученияДокументов] < 0 THEN N'До начала периода' ELSE N'От начала периода' END + N': ' +
    CASE WHEN ABS(CONVERT(bigint, t.[ДеньМесяцаПолученияДокументов])) / 100 = 0 THEN N'без смещения по месяцам, '
         WHEN t.[ДеньМесяцаПолученияДокументов] < 0 THEN N'за ' + CONVERT(nvarchar(20), ABS(CONVERT(bigint, t.[ДеньМесяцаПолученияДокументов])) / 100) + N' мес., '
         ELSE N'через ' + CONVERT(nvarchar(20), ABS(CONVERT(bigint, t.[ДеньМесяцаПолученияДокументов])) / 100) + N' мес., ' END +
    CASE WHEN ABS(CONVERT(bigint, t.[ДеньМесяцаПолученияДокументов])) % 100 = 99 THEN N'последний день месяца'
         ELSE CONVERT(nvarchar(2), ABS(CONVERT(bigint, t.[ДеньМесяцаПолученияДокументов])) % 100) + N'-е число' END AS DocumentsText,
    CASE WHEN t.[ДеньМесяцаОплатыПоДоговору] < 0 THEN N'До начала периода' ELSE N'От начала периода' END + N': ' +
    CASE WHEN ABS(CONVERT(bigint, t.[ДеньМесяцаОплатыПоДоговору])) / 100 = 0 THEN N'без смещения по месяцам, '
         WHEN t.[ДеньМесяцаОплатыПоДоговору] < 0 THEN N'за ' + CONVERT(nvarchar(20), ABS(CONVERT(bigint, t.[ДеньМесяцаОплатыПоДоговору])) / 100) + N' мес., '
         ELSE N'через ' + CONVERT(nvarchar(20), ABS(CONVERT(bigint, t.[ДеньМесяцаОплатыПоДоговору])) / 100) + N' мес., ' END +
    CASE WHEN ABS(CONVERT(bigint, t.[ДеньМесяцаОплатыПоДоговору])) % 100 = 99 THEN N'последний день месяца'
         ELSE CONVERT(nvarchar(2), ABS(CONVERT(bigint, t.[ДеньМесяцаОплатыПоДоговору])) % 100) + N'-е число' END AS PaymentText,
    CASE WHEN t.[ДеньМесяцаОтключенияУслуги] < 0 THEN N'До начала периода' ELSE N'От начала периода' END + N': ' +
    CASE WHEN ABS(CONVERT(bigint, t.[ДеньМесяцаОтключенияУслуги])) / 100 = 0 THEN N'без смещения по месяцам, '
         WHEN t.[ДеньМесяцаОтключенияУслуги] < 0 THEN N'за ' + CONVERT(nvarchar(20), ABS(CONVERT(bigint, t.[ДеньМесяцаОтключенияУслуги])) / 100) + N' мес., '
         ELSE N'через ' + CONVERT(nvarchar(20), ABS(CONVERT(bigint, t.[ДеньМесяцаОтключенияУслуги])) / 100) + N' мес., ' END +
    CASE WHEN ABS(CONVERT(bigint, t.[ДеньМесяцаОтключенияУслуги])) % 100 = 99 THEN N'последний день месяца'
         ELSE CONVERT(nvarchar(2), ABS(CONVERT(bigint, t.[ДеньМесяцаОтключенияУслуги])) % 100) + N'-е число' END AS DisconnectText
FROM dbo.[ДокументыДоговораОказанияУслуг] AS t
LEFT JOIN dbo.[ДокументыДанные] AS d
    ON d.[КодДокумента] = t.[КодДокументаДоговораОказанияУслуг]`

// Default candidate list. Adapt dbo.ДокументыДанные to the actual
// service-agreement type and name schema in the SQL settings. ContractID is
// required, every other SELECT field is displayed by the application.
const defaultWithoutParametersSQL = `SELECT
    d.[КодДокумента] AS ContractID,
    CONVERT(nvarchar(max), d.[Договор]) AS [Договор],
    CONCAT(CONVERT(nvarchar(30), d.[КодДокумента]),
        N' ', COALESCE(CONVERT(nvarchar(max), d.[Договор]), N'')) AS SearchText
FROM dbo.[ДокументыДанные] AS d
WHERE NOT EXISTS (
    SELECT 1
    FROM dbo.[ДокументыДоговораОказанияУслуг] AS p
    WHERE p.[КодДокументаДоговораОказанияУслуг] = d.[КодДокумента]
)`

const legacyNamedWithoutParametersSQL = `SELECT TOP (@limit)
    d.[КодДокумента] AS ContractID,
    CONVERT(nvarchar(max), d.[Договор]) AS [Договор]
FROM dbo.[ДокументыДанные] AS d
WHERE NOT EXISTS (
    SELECT 1
    FROM dbo.[ДокументыДоговораОказанияУслуг] AS p
    WHERE p.[КодДокументаДоговораОказанияУслуг] = d.[КодДокумента]
)
AND (
    @search = N'' OR
    CONVERT(nvarchar(30), d.[КодДокумента]) LIKE N'%' + @search + N'%' OR
    CONVERT(nvarchar(max), d.[Договор]) LIKE N'%' + @search + N'%'
)
ORDER BY d.[КодДокумента] DESC`

// Previous factory SQL is kept for migrating unmodified templates only.
// SQL queries edited by users are never changed by migration.
const legacyWithoutParametersSQL = `SELECT TOP (@limit)
    d.[КодДокумента] AS ContractID,
    CONVERT(nvarchar(30), d.[КодДокумента]) AS ContractName
FROM dbo.[ДокументыДанные] AS d
WHERE NOT EXISTS (
    SELECT 1
    FROM dbo.[ДокументыДоговораОказанияУслуг] AS p
    WHERE p.[КодДокументаДоговораОказанияУслуг] = d.[КодДокумента]
)
AND (
    @search = N'' OR
    CONVERT(nvarchar(30), d.[КодДокумента]) LIKE N'%' + @search + N'%'
)
ORDER BY d.[КодДокумента] DESC`

const defaultContractsSQL = `SELECT TOP (@limit)
    d.[КодДокумента] AS ContractID,
    CONVERT(nvarchar(max), d.[Договор]) AS ContractLabel
FROM dbo.[ДокументыДанные] AS d
WHERE
    @search = N'' OR
    d.[КодДокумента] = @selected OR
    CONVERT(nvarchar(30), d.[КодДокумента]) LIKE N'%' + @search + N'%' OR
    CONVERT(nvarchar(max), d.[Договор]) LIKE N'%' + @search + N'%'
ORDER BY d.[КодДокумента] DESC`

const legacyContractsSQL = `SELECT TOP (@limit)
 d.[КодДокумента] AS ContractID,
 CONVERT(nvarchar(30),d.[КодДокумента]) AS ContractLabel
FROM dbo.[ДокументыДанные] AS d
WHERE @search = N'' OR CONVERT(nvarchar(30),d.[КодДокумента]) LIKE N'%' + @search + N'%' OR d.[КодДокумента] = @selected
ORDER BY d.[КодДокумента] DESC`

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
		WithoutParametersSQL: defaultWithoutParametersSQL,
		ListSQL: listProjection,
		ListColumns: []ListColumn{
			{Field: "ContractID", Title: "Код договора", Width: 120, Order: 1, SortField: "ContractID", Visible: true},
			{Field: "AgreementLabel", Title: "Договор", Width: 220, Order: 2, SortField: "ContractID", Visible: true},
			{Field: "ServicePeriodText", Title: "Период оказания услуги", Width: 200, Order: 3, SortField: "ServicePeriod", Visible: true},
			{Field: "Amount", Title: "Абонентская плата", Width: 140, Order: 4, SortField: "Amount", Visible: true},
			{Field: "DocumentsText", Title: "Получение документов", Width: 305, Order: 5, SortField: "DocumentsCode", Visible: true},
			{Field: "PaymentText", Title: "Оплата по договору", Width: 305, Order: 6, SortField: "PaymentCode", Visible: true},
			{Field: "DisconnectText", Title: "Отключение услуги", Width: 305, Order: 7, SortField: "DisconnectCode", Visible: true},
			{Field: "AutoPayText", Title: "Автооплата", Width: 90, Order: 8, SortField: "AutoPay", Visible: true},
			// Hidden SQL-only helpers remain available as sortField targets.
			// Newly discovered aliases NOT listed here are visible by default.
			{Field: "ServicePeriod", Title: "Период (число)", Width: 140, Order: 9, SortField: "ServicePeriod", Visible: false},
			{Field: "AutoPay", Title: "Автооплата (код)", Width: 120, Order: 10, SortField: "AutoPay", Visible: false},
			{Field: "StartDay", Title: "День начала периода", Width: 150, Order: 11, SortField: "StartDay", Visible: false},
			{Field: "StartMonth", Title: "Месяц начала периода", Width: 150, Order: 12, SortField: "StartMonth", Visible: false},
			{Field: "DocumentsCode", Title: "Получение документов (XDD)", Width: 180, Order: 13, SortField: "DocumentsCode", Visible: false},
			{Field: "PaymentCode", Title: "Оплата (XDD)", Width: 160, Order: 14, SortField: "PaymentCode", Visible: false},
			{Field: "DisconnectCode", Title: "Отключение (XDD)", Width: 170, Order: 15, SortField: "DisconnectCode", Visible: false},
            {Field: "SearchText", Title: "Поисковый текст", Width: 220, Order: 16, SortField: "SearchText", Visible: false},
		},
		ContractsSQL: defaultContractsSQL,
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
	{"without_parameters_sql", "Договоры без параметров (с поиском)"},
	{"contracts_sql", "Поиск и выбор договора"},
	{"document_types_sql", "Поиск типа документа основания автооплаты"},
	{"get_sql", "Чтение одной записи"},
	{"insert_sql", "Добавление записи"},
	{"update_sql", "Обновление записи"},
	{"delete_sql", "Удаление записи"},
}

func (c *Config) SQLAt(i int) *string {
    switch i {
    case 0: return &c.ListSQL
    case 1: return &c.WithoutParametersSQL
    case 2: return &c.ContractsSQL
    case 3: return &c.DocumentTypesSQL
    case 4: return &c.GetSQL
    case 5: return &c.InsertSQL
    case 6: return &c.UpdateSQL
    case 7: return &c.DeleteSQL
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

// LoadConfig supplies defaults for new fields when reading an older config.json.
// Existing SQL connection and user-edited queries are never overwritten.
// Replace only byte-identical factory SQL from earlier versions.
// The updated SQL stays in memory until the user saves settings.
// Upgrade only exact, untouched factory SQL texts. Never rewrite custom
// SQL queries, since their SELECT projections and formatting belong to users.
// Factory queries from previous releases included TOP and ORDER BY and would
// truncate the candidate set before word filtering if left unchanged.
func upgradeFactoryAgreementQueries(c *Config) {
    if c.WithoutParametersSQL == legacyWithoutParametersSQL ||
        c.WithoutParametersSQL == legacyNamedWithoutParametersSQL {
        c.WithoutParametersSQL = defaultWithoutParametersSQL
    }
    if c.ContractsSQL == legacyContractsSQL {
        c.ContractsSQL = defaultContractsSQL
    }
    // Factory query from the immediately previous release (real title, TOP).
    previous := strings.Replace(listProjection, "SELECT\n", "SELECT TOP (@limit)\n", 1)
    previous = strings.Replace(previous, "    CONCAT(CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]),\n        N' ', COALESCE(CONVERT(nvarchar(max), d.[Договор]), N'')) AS SearchText,\n", "", 1)
    const previousOrder = "\nORDER BY t.[КодДокументаДоговораОказанияУслуг] DESC"
    if c.ListSQL == previous + previousOrder {
        c.ListSQL = listProjection
    }
    // Even older factory version projected numeric contract ID as the label.
    older := strings.Replace(previous,
        "CONVERT(nvarchar(max), d.[Договор]) AS AgreementLabel,",
        "CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]) AS AgreementLabel,", 1)
    older = strings.Replace(older,
        "\nLEFT JOIN dbo.[ДокументыДанные] AS d\n    ON d.[КодДокумента] = t.[КодДокументаДоговораОказанияУслуг]", "", 1)
    if c.ListSQL == older + previousOrder {
        c.ListSQL = listProjection
    }
}

// LoadConfig drops only the deprecated search_sql key from old JSON files.
// Other unknown keys still raise an error. SaveConfig never writes search_sql.
func LoadConfig(path string) (Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return Config{}, fmt.Errorf("открыть конфигурацию: %w", err)
    }
    decoder := json.NewDecoder(strings.NewReader(string(data)))
    var fields map[string]json.RawMessage
    if err := decoder.Decode(&fields); err != nil {
        return Config{}, fmt.Errorf("формат конфигурации: %w", err)
    }
    if fields == nil {
        return Config{}, errors.New("конфигурация должна быть JSON-объектом")
    }
    var extra any
    if err := decoder.Decode(&extra); err != io.EOF {
        if err == nil {
            return Config{}, errors.New("в config.json более одного JSON-объекта")
        }
        return Config{}, fmt.Errorf("лишние данные после JSON: %w", err)
    }
    delete(fields, "search_sql")
    normalized, err := json.Marshal(fields)
    if err != nil {
        return Config{}, err
    }
    c := DefaultConfig()
    strict := json.NewDecoder(strings.NewReader(string(normalized)))
    strict.DisallowUnknownFields()
    if err := strict.Decode(&c); err != nil {
        return Config{}, fmt.Errorf("формат конфигурации: %w", err)
    }
    upgradeFactoryAgreementQueries(&c)
    for i := range c.ListColumns {
        if c.ListColumns[i].Order == -1 {
            c.ListColumns[i].Order = i + 1
        }
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