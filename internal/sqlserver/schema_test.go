package sqlserver

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Postnn/ServiceAgreement/internal/config"
	"github.com/Postnn/ServiceAgreement/internal/presentation"
	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

func TestReconcileColumnsAddsVisibleAndPreservesSettings(t *testing.T) {
	original := []readmodel.ListColumn{
		{Field: "ContractID", Title: "Договор", Width: 110, Order: 3, SortField: "SortID", Visible: true},
		{Field: "OldAlias", Title: "Удалённое", Width: 180, Order: 4, SortField: "OldAlias", Visible: false},
		{Field: "Title", Title: "Название договора", Width: 450, Order: 1, SortField: "Title", Visible: false},
		{Field: "SearchText", Title: "Текст поиска", Width: 300, Order: 5, SortField: "SearchText", Visible: false},
	}
	names := []string{"ContractID", "TITLE", "SearchText", "AddedNumber", "AnotherColumn"}
	sync, err := presentation.ReconcileListColumns(original, names)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sync.Added, []string{"AddedNumber", "AnotherColumn"}) || !reflect.DeepEqual(sync.Removed, []string{"OldAlias"}) || !reflect.DeepEqual(sync.RepairedSort, []string{"ContractID"}) {
		t.Fatalf("unexpected synchronization report: %+v", sync)
	}
	if len(sync.Columns) != 5 || sync.Columns[0].Title != "Название договора" || sync.Columns[0].Visible || sync.Columns[0].Field != "TITLE" {
		t.Fatalf("lost user settings: %+v", sync.Columns)
	}
	if sync.Columns[1].SortField != "ContractID" || sync.Columns[1].Width != 110 {
		t.Fatalf("sort fallback lost settings: %+v", sync.Columns[1])
	}
	for _, c := range sync.Columns[3:] {
		if !c.Visible || c.SortField != c.Field || c.Width != 170 || c.Order < 6 {
			t.Fatalf("new column must be VISIBLE by default: %+v", c)
		}
	}
	again, err := presentation.ReconcileListColumns(sync.Columns, names)
	if err != nil || again.Changed() {
		t.Fatalf("not idempotent: %+v / %v", again, err)
	}
	// Original settings are not mutated until the user accepts the proposal.
	if original[0].SortField != "SortID" || original[2].Field != "Title" {
		t.Fatal("mutated original")
	}
}

func TestColumnProjectionValidation(t *testing.T) {
	cases := [][]string{
		{"Missing"}, {"ContractID", "contractid"}, {"ContractID", ""},
	}
	for _, names := range cases {
		if _, err := presentation.ReconcileListColumns(nil, names); err == nil {
			t.Errorf("accepted bad projection %v", names)
		}
	}
}

type sqlSchemaDriver struct{ missing bool }
type schemaConn struct{ missing bool }

func (d sqlSchemaDriver) Open(_ string) (driver.Conn, error) {
	return &schemaConn{missing: d.missing}, nil
}
func (*schemaConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (*schemaConn) Close() error                        { return nil }
func (*schemaConn) Begin() (driver.Tx, error)           { return nil, errors.New("unused") }
func (c *schemaConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 2 || !strings.Contains(query, "FROM (\nlist\n)") ||
		!strings.Contains(query, "TOP (@limit)") {
		return nil, errors.New("expected only wrapped list_sql and @limit; no search_sql")
	}
	if c.missing {
		return &fakeSQLRows{names: []string{"ContractID", "Alpha", "SortID"}}, nil
	}
	return &fakeSQLRows{names: []string{"ContractID", "Alpha", "SearchText", "SortID"}}, nil
}
func TestQueryListSchemaUsesSingleSource(t *testing.T) {
	for _, missing := range []bool{false, true} {
		name := "schemaTestWithSearchText"
		if missing {
			name = "schemaTestWithoutSearchText"
		}
		sql.Register(name, sqlSchemaDriver{missing: missing})
		db, err := sql.Open(name, "")
		if err != nil {
			t.Fatal(err)
		}
		c := config.DefaultConfig()
		c.ListSQL = "list"
		cols, err := QueryListSchema(context.Background(), db, c)
		db.Close()
		if missing && err == nil {
			t.Fatal("required SearchText was missing but schema accepted it")
		}
		expected := []string{"ContractID", "Alpha", "SearchText", "SortID"}
		if !missing && (err != nil || !reflect.DeepEqual(cols, expected)) {
			t.Fatalf("wrong single-query schema: %v columns %v", err, cols)
		}
	}
}

func TestReconcileColumnsKeepsServiceAliasesHidden(t *testing.T) {
	src := []readmodel.ListColumn{{Field: "AgreementLabel", Title: "Договор", Width: 200, Order: 1, SortField: "ContractID", Visible: true}}
	result, err := presentation.ReconcileListColumns(src, []string{"ContractID", "SearchText", "AgreementLabel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Added) != 2 {
		t.Fatalf("expected both hidden SQL helpers to be discovered: %+v", result)
	}
	shown := readmodel.VisibleColumns(result.Columns)
	if len(shown) != 1 || shown[0].Field != "AgreementLabel" {
		t.Fatalf("helper fields should be hidden and user columns preserved: %+v", shown)
	}
}
