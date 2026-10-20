package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"testing"
)

func TestReconcileColumnsAddsVisibleAndPreservesSettings(t *testing.T) {
	original := []ListColumn{
		{Field: "ContractID", Title: "Договор", Width: 110, Order: 3, SortField: "SortID", Visible: true},
		{Field: "OldAlias", Title: "Удалённое", Width: 180, Order: 4, SortField: "OldAlias", Visible: false},
		{Field: "Title", Title: "Название договора", Width: 450, Order: 1, SortField: "Title", Visible: false},
	}
	names := []string{"ContractID", "TITLE", "AddedNumber", "AnotherColumn"}
	sync, err := ReconcileListColumns(original, names)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sync.Added, []string{"AddedNumber", "AnotherColumn"}) || !reflect.DeepEqual(sync.Removed, []string{"OldAlias"}) || !reflect.DeepEqual(sync.RepairedSort, []string{"ContractID"}) {
		t.Fatalf("unexpected synchronization report: %+v", sync)
	}
	if len(sync.Columns) != 4 || sync.Columns[0].Title != "Название договора" || sync.Columns[0].Visible || sync.Columns[0].Field != "TITLE" {
		t.Fatalf("lost user settings: %+v", sync.Columns)
	}
	if sync.Columns[1].SortField != "ContractID" || sync.Columns[1].Width != 110 {
		t.Fatalf("sort fallback lost settings: %+v", sync.Columns[1])
	}
	for _, c := range sync.Columns[2:] {
		if !c.Visible || c.SortField != c.Field || c.Width != 170 || c.Order < 5 {
			t.Fatalf("new column must be VISIBLE by default: %+v", c)
		}
	}
	again, err := ReconcileListColumns(sync.Columns, names)
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
		if _, err := ReconcileListColumns(nil, names); err == nil {
			t.Errorf("accepted bad projection %v", names)
		}
	}
}

type sqlSchemaDriver struct{ mismatch bool }
type schemaConn struct{ mismatch bool }

func (d sqlSchemaDriver) Open(_ string) (driver.Conn, error) {
	return &schemaConn{mismatch: d.mismatch}, nil
}
func (*schemaConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("unused") }
func (*schemaConn) Close() error                        { return nil }
func (*schemaConn) Begin() (driver.Tx, error)           { return nil, errors.New("unused") }
func (c *schemaConn) QueryContext(_ context.Context, sqlText string, args []driver.NamedValue) (driver.Rows, error) {
	if len(args) != 2 {
		return nil, errors.New("missing named arguments")
	}
	if sqlText == "list" {
		return &fakeSQLRows{names: []string{"ContractID", "Alpha", "SortID"}}, nil
	}
	if sqlText == "search" {
		if c.mismatch {
			return &fakeSQLRows{names: []string{"ContractID", "Alpha", "OtherID"}}, nil
		}
		return &fakeSQLRows{names: []string{"SortID", "Alpha", "ContractID"}}, nil
	}
	return nil, errors.New("unexpected SQL query")
}
func TestQueryListSchemaAndMismatch(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		name := "schemaTestOK"
		if mismatch {
			name = "schemaTestMismatch"
		}
		sql.Register(name, sqlSchemaDriver{mismatch: mismatch})
		db, err := sql.Open(name, "")
		if err != nil {
			t.Fatal(err)
		}
		c := DefaultConfig()
		c.ListSQL = "list"
		c.SearchSQL = "search"
		cols, err := QueryListSchema(context.Background(), db, c)
		db.Close()
		if mismatch && err == nil {
			t.Fatal("mismatched projections accepted")
		}
		if !mismatch && (err != nil || !reflect.DeepEqual(cols, []string{"ContractID", "Alpha", "SortID"})) {
			t.Fatalf("query failed: %v cols %v", err, cols)
		}
	}
}