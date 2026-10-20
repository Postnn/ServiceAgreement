package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"testing"
)

type fakeSQLDriver struct{ state *fakeSQLState }
type fakeSQLState struct {
	Query    string
	Args     map[string]driver.Value
	Affected int64
}
type fakeSQLConn struct{ state *fakeSQLState }
type fakeSQLRows struct {
	names []string
	rows  [][]driver.Value
	index int
}

func (d fakeSQLDriver) Open(string) (driver.Conn, error) { return &fakeSQLConn{state: d.state}, nil }
func (*fakeSQLConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not implemented") }
func (*fakeSQLConn) Close() error                        { return nil }
func (*fakeSQLConn) Begin() (driver.Tx, error)           { return nil, errors.New("not implemented") }
func (c *fakeSQLConn) record(query string, args []driver.NamedValue) {
	c.state.Query = query
	c.state.Args = make(map[string]driver.Value)
	for _, x := range args {
		c.state.Args[x.Name] = x.Value
	}
}
func (c *fakeSQLConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.record(query, args)
	switch query {
	case "query_list", "query_search":
		return &fakeSQLRows{
			names: []string{"ContractID", "CustomColumn", "ServicePeriod", "AutoPay", "PaymentCode", "PaymentText", "SortByHidden"},
			rows:  [][]driver.Value{{int64(13068), []byte("Номер контракта"), int64(3), int64(1), int64(306), []byte("Оплата: до 6 мая"), int64(12)}},
		}, nil
	case "get_record":
		return &fakeSQLRows{names: []string{"id", "type", "period", "auto", "amount", "day", "month", "docs", "pay", "off", "notify"},
			rows: [][]driver.Value{{int64(13068), int64(23), int64(3), int64(1), "300.0000", int64(7), int64(1), int64(-6), int64(306), int64(320), int64(0)}}}, nil
	case "lookup_values":
		return &fakeSQLRows{names: []string{"id", "label"}, rows: [][]driver.Value{{int64(13068), "Контракт 13068"}}}, nil
	}
	return nil, errors.New("unexpected query: " + query)
}
func (c *fakeSQLConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.record(query, args)
	return driver.RowsAffected(c.state.Affected), nil
}
func (r *fakeSQLRows) Columns() []string { return r.names }
func (r *fakeSQLRows) Close() error      { return nil }
func (r *fakeSQLRows) Next(dst []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dst, r.rows[r.index])
	r.index++
	return nil
}

func TestStoreUsesConfiguredQueriesAndDynamicColumns(t *testing.T) {
	state := &fakeSQLState{Affected: 1}
	sql.Register("serviceAgreementTest", fakeSQLDriver{state: state})
	db, err := sql.Open("serviceAgreementTest", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := DefaultConfig()
	cfg.ConnectionString = "test"
	cfg.ListSQL = "query_list"
	cfg.SearchSQL = "query_search"
	cfg.ContractsSQL = "lookup_values"
	cfg.GetSQL = "get_record"
	cfg.InsertSQL = "insert_record"
	cfg.UpdateSQL = "update_record"
	cfg.DeleteSQL = "delete_record"
	cfg.ListColumns = []ListColumn{
		{Field: "ContractID", Title: "ID", Width: 80, Order: 1, SortField: "ContractID", Visible: true},
		{Field: "CustomColumn", Title: "Контракт", Width: 250, Order: 2, SortField: "SortByHidden", Visible: true},
		{Field: "PaymentText", Title: "Оплата", Width: 300, Order: 3, SortField: "PaymentCode", Visible: true},
	}
	store := Store{DB: db, Config: cfg}
	ctx := context.Background()
	rows, err := store.List(ctx, "  ")
	if err != nil {
		t.Fatal(err)
	}
	if state.Query != "query_list" || state.Args["search"] != "" || state.Args["limit"] != int64(cfg.ListLimit) && state.Args["limit"] != cfg.ListLimit {
		t.Fatalf("list %s: %v", state.Query, state.Args)
	}
	if len(rows) != 1 || rows[0].ID != 13068 || rows[0].Field("customcolumn") != "Номер контракта" {
		t.Fatalf("dynamic rows: %+v", rows)
	}
	if rows[0].Field("PaymentText") != "Оплата: до 6 мая" {
		t.Fatalf("SQL-provided description was altered or lost: %q", rows[0].Field("PaymentText"))
	}
	if rows[0].Field("DocumentsText") != "" {
		t.Fatal("unexpected derived/virtual column added", rows[0].Values)
	}
	_, err = store.List(ctx, "кот")
	if err != nil || state.Query != "query_search" || state.Args["search"] != "кот" {
		t.Fatalf("search %s args %v err %v", state.Query, state.Args, err)
	}
	opts, err := store.Lookup(ctx, cfg.ContractsSQL, "договор", 123)
	if err != nil || len(opts) != 1 || opts[0].ID != 13068 || state.Args["selected"] != int64(123) && state.Args["selected"] != 123 {
		t.Fatalf("lookup: %+v %v %v", opts, state.Args, err)
	}
	r, err := store.Get(ctx, 13068)
	if err != nil || r.TypeID != 23 || state.Query != "get_record" {
		t.Fatalf("get: %+v %s %v", r, state.Query, err)
	}
	if err := store.Insert(ctx, r); err != nil || state.Query != "insert_record" {
		t.Fatalf("insert %v %v", state.Query, err)
	}
	if !reflect.DeepEqual(state.Args["amount"], "300.0000") {
		t.Fatal("money parameters", state.Args)
	}
	updated := r
	updated.Notify = 1
	if err := store.Update(ctx, r, updated); err != nil || state.Query != "update_record" || state.Args["old_notify"] != int64(0) && state.Args["old_notify"] != 0 {
		t.Fatalf("update %s args %v err %v", state.Query, state.Args, err)
	}
	if err := store.Delete(ctx, 13068); err != nil || state.Query != "delete_record" {
		t.Fatalf("delete %s %v", state.Query, err)
	}
	state.Affected = 0
	if err := store.Update(ctx, r, updated); !errors.Is(err, ErrConflict) {
		t.Fatal("expected optimistic conflict", err)
	}
	if err := store.Delete(ctx, 13068); !errors.Is(err, ErrConflict) {
		t.Fatal("expected missing row", err)
	}
}

func TestListColumnsMustExist(t *testing.T) {
	// Config validation allows arbitrary aliases; execution must check that
	// the SELECT actually returns them, including when zero rows match.
	cfg := DefaultConfig()
	cfg.ConnectionString = "test"
	cfg.ListColumns = []ListColumn{{Field: "ContractID", Title: "ID", Width: 80, Order: 1, SortField: "ContractID", Visible: true}, {Field: "CustomNotInQuery", Title: "Missing", Width: 100, Order: 2, SortField: "CustomNotInQuery", Visible: true}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateResultColumns([]string{"ContractID", "CustomColumn"}, cfg.ListColumns); err == nil {
		t.Fatal("accepted a field not returned by SQL")
	}
	cfg.ListColumns[1].Field = "CustomColumn"
	if _, err := ValidateResultColumns([]string{"ContractID", "CustomColumn"}, cfg.ListColumns); err == nil {
		t.Fatal("accepted a missing sortField")
	}
	cfg.ListColumns[1].SortField = "ContractID"
	if _, err := ValidateResultColumns([]string{"ContractID", "CustomColumn"}, cfg.ListColumns); err != nil {
		t.Fatal(err)
	}
}