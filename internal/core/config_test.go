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
	if len(SQLFields) != 8 {
		t.Fatalf("want 8 editable SQL queries; got %d", len(SQLFields))
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
	if oldConfig.ListSQL != "SELECT 1" || oldConfig.SearchSQL == "" || oldConfig.GetSQL == "" || len(oldConfig.ListColumns) == 0 {
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