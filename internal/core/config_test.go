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
	if err := ValidateColumns([]ListColumn{{"ContractID", "Номер", 120}, {"contractid", "Дубль", 80}}); err == nil {
		t.Fatal("duplicate accepted")
	}
	if err := ValidateColumns([]ListColumn{{"SomeNewField", "Новое поле", 120}}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateColumns([]ListColumn{{"SomeNewField", "Новое поле", 40}}); err == nil {
		t.Fatal("narrow width accepted")
	}
}

func TestSaveLoadConfigAndBackwardCompatibility(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	cfg := DefaultConfig()
	cfg.ConnectionString = "sqlserver://testing?database=Договоры"
	cfg.ListColumns = append(cfg.ListColumns, ListColumn{Field: "CustomContractNumber", Title: "Наш №", Width: 180})
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

func TestDerivedListFields(t *testing.T) {
	r := ListItem{ID: 13068, Values: map[string]string{
		"ContractID": "13068", "ServicePeriod": "3", "AutoPay": "1", "StartDay": "7", "StartMonth": "1",
		"DocumentsCode": "-99", "PaymentCode": "306", "DisconnectCode": "320", "ContractCaption": "Контракт 13068",
	}}
	deriveListFields(&r)
	if r.Field("ServicePeriodText") != "Ежеквартально" || r.Field("AutoPayText") != "Да" {
		t.Fatal(r.Values)
	}
	if !strings.Contains(r.Field("PaymentText"), "06.05.") {
		t.Fatalf("XDD 306: %q", r.Field("PaymentText"))
	}
	if !strings.Contains(r.Field("DisconnectText"), "20.04.") {
		t.Fatalf("XDD 320: %q", r.Field("DisconnectText"))
	}
	if !strings.Contains(r.Field("DocumentsText"), "последний день") {
		t.Fatalf("-99 %q", r.Field("DocumentsText"))
	}
	if r.Field("contractcaption") != "Контракт 13068" {
		t.Fatal("case-insensitive aliases should work")
	}
}
