package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfigurationAndColumns(t *testing.T) {
	c := DefaultConfig()
	c.ConnectionString = "sqlserver://localhost?database=Test"
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(SQLFields) != 9 {
		t.Fatalf("want 9 editable SQL queries; got %d", len(SQLFields))
	}
	for i, field := range SQLFields {
		if ptr := c.SQLAt(i); ptr == nil || strings.TrimSpace(*ptr) == "" {
			t.Errorf("missing %s", field.Key)
		}
	}
	if err := ValidateColumns([]ListColumn{{Field: "ContractID", Title: "Номер", Width: 120, Order: 1, SortField: "ContractID", Visible: true}, {Field: "contractid", Title: "Дубль", Width: 80, Order: 2, SortField: "contractid", Visible: true}}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := ValidateColumns([]ListColumn{{Field: "SomeNewField", Title: "Новое поле", Width: 120, Order: 1, SortField: "SomeNewField", Visible: true}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateColumns([]ListColumn{{Field: "SomeNewField", Title: "Новое поле", Width: 40, Order: 1, SortField: "SomeNewField", Visible: true}}); err == nil {
		t.Fatal("narrow width accepted")
	}
}

func TestSaveLoadConfigAndBackwardCompatibility(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	cfg := DefaultConfig()
	cfg.ConnectionString = "sqlserver://testing?database=Договоры"
	cfg.ListColumns = append(cfg.ListColumns, ListColumn{Field: "CustomContractNumber", Title: "Наш №", Width: 180, Order: len(cfg.ListColumns) + 1, SortField: "ContractID", Visible: true})
	cfg.UpdateSQL = "UPDATE t SET [Name]=@type\nWHERE id=@id"
	if err := SaveConfig(p, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.UpdateSQL != cfg.UpdateSQL || len(got.ListColumns) != len(cfg.ListColumns) {
		t.Fatalf("config lost after serialization: %+v", got)
	}
	cfg.DeleteSQL = "DELETE FROM dbo.t WHERE id=@id"
	if err := SaveConfig(p, cfg); err != nil {
		t.Fatal(err)
	}
	var previous Config
	bytes, err := os.ReadFile(p + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(bytes, &previous); err != nil {
		t.Fatal(err)
	}
	if previous.DeleteSQL == cfg.DeleteSQL {
		t.Fatal("backup was not saved")
	}
	old := `{"connection_string":"sqlserver://old", "list_sql":"SELECT 1", "contracts_sql":"SELECT 2", "document_types_sql":"SELECT 3"}`
	if err := os.WriteFile(p, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	oldConfig, err := LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if oldConfig.ListSQL != "SELECT 1" || oldConfig.SearchSQL == "" || oldConfig.WithoutParametersSQL == "" || oldConfig.GetSQL == "" || len(oldConfig.ListColumns) == 0 {
		t.Fatalf("backwards migration: %+v", oldConfig)
	}
}

func TestColumnsSortAndVisibility(t *testing.T) {
	cols := []ListColumn{
		{Field: "Hidden", Title: "Невидимая", Width: 120, Order: 3, SortField: "SortID", Visible: false},
		{Field: "Name", Title: "Имя", Width: 150, Order: 2, SortField: "SortID", Visible: true},
		{Field: "ContractID", Title: "ID", Width: 80, Order: 1, SortField: "ContractID", Visible: true},
	}
	if err := ValidateColumns(cols); err != nil {
		t.Fatal(err)
	}
	got := VisibleColumns(cols)
	if len(got) != 2 || got[0].Field != "ContractID" || got[1].Field != "Name" {
		t.Fatalf("visibility/order: %+v", got)
	}
	bad := append([]ListColumn(nil), cols...)
	bad[0].Order = 1
	if ValidateColumns(bad) == nil {
		t.Fatal("duplicate order accepted")
	}
	for i := range bad {
		bad[i].Order = i + 1
		bad[i].Visible = false
	}
	if ValidateColumns(bad) == nil {
		t.Fatal("all-hidden columns accepted")
	}
	old := `[{"field":"ContractID","title":"Номер","width":100},{"field":"Name","title":"Имя","width":200}]`
	var upgraded []ListColumn
	if err := json.Unmarshal([]byte(old), &upgraded); err != nil {
		t.Fatal(err)
	}
	if !upgraded[0].Visible || upgraded[0].SortField != "ContractID" || upgraded[0].Order != -1 {
		t.Fatalf("legacy flags not upgraded: %+v", upgraded)
	}
	for i := range upgraded {
		if upgraded[i].Order == -1 {
			upgraded[i].Order = i + 1
		}
	}
	if err := ValidateColumns(upgraded); err != nil {
		t.Fatal(err)
	}
	explicit := `{"field":"Name","title":"Имя","width":123,"order":4,"sortField":"SortID","visible":false}`
	var configured ListColumn
	if err := json.Unmarshal([]byte(explicit), &configured); err != nil {
		t.Fatal(err)
	}
	if configured.Visible || configured.SortField != "SortID" || configured.Order != 4 {
		t.Fatal(configured)
	}
}

func TestDefaultQueriesProjectAllDisplayedColumns(t *testing.T) {
	cfg := DefaultConfig()
	for _, col := range cfg.ListColumns {
		for _, field := range []string{col.Field, col.SortField} {
			// The default SQL must project the aliases directly, never via Go.
			if !strings.Contains(cfg.ListSQL, " AS "+field) || !strings.Contains(cfg.SearchSQL, " AS "+field) {
				t.Fatalf("default SQL does not project %q", field)
			}
		}
	}
}

func TestUnconfiguredSQLSettings(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ConnectionString = "sqlserver://test"
	if !strings.Contains(cfg.WithoutParametersSQL, "NOT EXISTS") ||
		!strings.Contains(cfg.WithoutParametersSQL, "@search") ||
		!strings.Contains(cfg.WithoutParametersSQL, "ContractID") {
		t.Fatalf("default unconfigured SQL is incomplete: %s", cfg.WithoutParametersSQL)
	}
	if len(SQLFields) != 9 || SQLFields[2].Key != "without_parameters_sql" ||
		cfg.SQLAt(2) == nil || *cfg.SQLAt(2) != cfg.WithoutParametersSQL {
		t.Fatal("new query must appear as a separate editable SQL setting")
	}
	cfg.WithoutParametersSQL = ""
	if err := cfg.Validate(); err == nil {
		t.Fatal("missing without_parameters_sql was accepted")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	cfg = DefaultConfig()
	cfg.ConnectionString = "sqlserver://test"
	cfg.WithoutParametersSQL = "SELECT\n ContractID, ContractName FROM MyAgreements\nWHERE SearchText = @search"
	if err := SaveConfig(path, cfg); err != nil { t.Fatal(err) }
	roundTrip, err := LoadConfig(path)
	if err != nil || roundTrip.WithoutParametersSQL != cfg.WithoutParametersSQL {
		t.Fatalf("SQL query text lost during save/load: %v, %q", err, roundTrip.WithoutParametersSQL)
	}
}

func TestAgreementNameQueriesAndFactoryMigration(t *testing.T) {
    cfg := DefaultConfig()
    cfg.ConnectionString = "sqlserver://example?database=db"
    if !strings.Contains(cfg.WithoutParametersSQL, "d.[КодДокумента] AS ContractID") ||
        !strings.Contains(cfg.WithoutParametersSQL, "d.[Договор]) AS [Договор]") ||
        strings.Count(cfg.WithoutParametersSQL, "d.[Договор]") < 2 ||
        !strings.Contains(cfg.WithoutParametersSQL, "NOT EXISTS") {
        t.Fatalf("second list must return contract code/name and search by both: %s", cfg.WithoutParametersSQL)
    }
    if !strings.Contains(cfg.ContractsSQL, "d.[Договор]) AS ContractLabel") ||
       !strings.Contains(cfg.ContractsSQL, "@selected") ||
       !strings.Contains(cfg.ContractsSQL, "d.[Договор]) LIKE") {
       t.Fatalf("contract dropdown must show/search true contract name: %s", cfg.ContractsSQL)
    }
    if !strings.Contains(cfg.ListSQL, "d.[Договор]) AS AgreementLabel") ||
       !strings.Contains(cfg.ListSQL, "LEFT JOIN dbo.[ДокументыДанные] AS d") ||
       !strings.Contains(cfg.SearchSQL, "d.[Договор]) LIKE") {
       t.Fatal("configured list must show and search the name")
    }
    path := filepath.Join(t.TempDir(), "config.json")
    old := cfg
    old.WithoutParametersSQL = legacyWithoutParametersSQL
    old.ContractsSQL = legacyContractsSQL
    oldProjection := strings.Replace(listProjection,
        "CONVERT(nvarchar(max), d.[Договор]) AS AgreementLabel,",
        "CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]) AS AgreementLabel,", 1)
    oldProjection = strings.Replace(oldProjection,
        "\nLEFT JOIN dbo.[ДокументыДанные] AS d\n    ON d.[КодДокумента] = t.[КодДокументаДоговораОказанияУслуг]", "", 1)
    old.ListSQL = oldProjection + "\nORDER BY t.[КодДокументаДоговораОказанияУслуг] DESC"
    old.SearchSQL = oldProjection + "\nWHERE CONVERT(nvarchar(30), t.[КодДокументаДоговораОказанияУслуг]) LIKE N'%' + @search + N'%'" + "\nORDER BY t.[КодДокументаДоговораОказанияУслуг] DESC"
    if err := SaveConfig(path, old); err != nil {t.Fatal(err)}
    newcfg, err := LoadConfig(path)
    if err != nil {t.Fatal(err)}
    if newcfg.WithoutParametersSQL != cfg.WithoutParametersSQL || 
       newcfg.ContractsSQL != cfg.ContractsSQL ||
       newcfg.ListSQL != cfg.ListSQL || newcfg.SearchSQL != cfg.SearchSQL {
       t.Fatal("old untouched factory queries were not migrated")
    }
    raw, err := os.ReadFile(path)
    if err != nil || !strings.Contains(string(raw), "CONVERT(nvarchar(30),d.[КодДокумента]) AS ContractLabel") {
       t.Fatal("migration must not rewrite disk config without Save")
    }
    custom := cfg
    custom.WithoutParametersSQL = "SELECT ContractID, Title FROM CustomView WHERE Title LIKE @search"
    custom.ContractsSQL = "SELECT Code, Name FROM CustomLookup"
    custom.ListSQL = "SELECT ContractID FROM CustomList"
    custom.SearchSQL = "SELECT ContractID FROM CustomList WHERE Title LIKE @search"
    if err := SaveConfig(path, custom); err != nil {t.Fatal(err)}
    loaded, err := LoadConfig(path)
    if err != nil {t.Fatal(err)}
    if loaded.WithoutParametersSQL != custom.WithoutParametersSQL ||
       loaded.ContractsSQL != custom.ContractsSQL ||
       loaded.ListSQL != custom.ListSQL || loaded.SearchSQL != custom.SearchSQL {
        t.Fatal("migration overwrote user SQL")
    }
}
