package core

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"strings"
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
    types []string
    rows [][]driver.Value
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
	switch {
    case strings.Contains(query,"FROM (\nquery_textmoney_list\n)"):
        return &fakeSQLRows{
            names:[]string{"ContractID","Абон.плата","SearchText"},
            types:[]string{"INT","NVARCHAR","NVARCHAR"},
            rows:[][]driver.Value{
                {int64(701),[]byte("2222.0000"),[]byte("701 Контракт")},
            },
        },nil
    case strings.Contains(query,"FROM (\nquery_unconfigured_textmoney\n)"):
        return &fakeSQLRows{
            names:[]string{"ContractID","Абон.плата","SearchText"},
            types:[]string{"INT","NVARCHAR","NVARCHAR"},
            rows:[][]driver.Value{
                {int64(801),[]byte("4520.4600"),[]byte("801 Договор")},
            },
        },nil
    case strings.Contains(query,"FROM (\nquery_money_list\n)"):
        return &fakeSQLRows{
            names:[]string{"ContractID","Абон.плата","SearchText"},
            types:[]string{"INT","MONEY","NVARCHAR"},
            rows:[][]driver.Value{
                {int64(501),[]byte("2222.0000"),[]byte("501 Контракт")},
                {int64(502),[]byte("1150.2400"),[]byte("502 Контракт")},
            },
        },nil
    case strings.Contains(query,"FROM (\nquery_unconfigured_money\n)"):
        return &fakeSQLRows{
            names:[]string{"ContractID","Абон.плата","SearchText"},
            types:[]string{"INT","MONEY","NVARCHAR"},
            rows:[][]driver.Value{
                {int64(601),[]byte("0.0000"),[]byte("601 Договор")},
                {int64(602),[]byte("4520.4600"),[]byte("602 Договор")},
            },
        },nil
	case strings.Contains(query, "query_list"):
		return &fakeSQLRows{
			names: []string{"ContractID", "CustomColumn", "ServicePeriod", "AutoPay", "PaymentCode", "PaymentText", "SortByHidden", "SearchText"},
			rows:  [][]driver.Value{{int64(13068), []byte("Номер контракта"), int64(3), int64(1), int64(306), []byte("Оплата: до 6 мая"), int64(12), []byte("13068 Номер контракта")}},
		}, nil
	case strings.Contains(query, "FROM (\nquery_unconfigured\n)"):
		return &fakeSQLRows{
			names: []string{"ContractID", "ContractName", "DateSigned", "CustomExtra", "SearchText"},
			rows: [][]driver.Value{
				{int64(1206), "Договор №1206", "2026-03-07", []byte("Поставщик"), "1206 Договор №1206"},
				{int64(1307), "Договор №1307", "2026-04-11", []byte("Клиент"), "1307 Договор №1307"},
			},
		}, nil
	case strings.Contains(query, "query_unconfigured_empty"):
		return &fakeSQLRows{names: []string{"ContractID", "ContractName", "SomethingElse", "SearchText"}}, nil
	case strings.Contains(query, "query_unconfigured_bad"):
		return &fakeSQLRows{names: []string{"SomeOtherID", "ContractName"}}, nil
	case query == "get_record":
		return &fakeSQLRows{names: []string{"id", "type", "period", "auto", "amount", "day", "month", "docs", "pay", "off", "notify"},
			rows: [][]driver.Value{{int64(13068), int64(23), int64(3), int64(1), "300.0000", int64(7), int64(1), int64(-6), int64(306), int64(320), int64(0)}}}, nil
	case query == "lookup_values":
		return &fakeSQLRows{names: []string{"id", "label"}, rows: [][]driver.Value{{int64(13068), "Контракт 13068"}}}, nil
	}
	return nil, errors.New("unexpected query: " + query)
}
func (c *fakeSQLConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.record(query, args)
	return driver.RowsAffected(c.state.Affected), nil
}
func (r *fakeSQLRows) Columns() []string { return r.names }
func (r *fakeSQLRows) ColumnTypeDatabaseTypeName(index int) string {
    if index>=0 && index<len(r.types) {return r.types[index]}
    return ""
}
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
	if !strings.Contains(state.Query, "FROM (\nquery_list\n)") || state.Args["limit"] != int64(cfg.ListLimit) && state.Args["limit"] != cfg.ListLimit {
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
	if err != nil || !strings.Contains(state.Query, "FROM (\nquery_list\n)") || state.Args["SearchText1"] != "кот" ||
        !strings.Contains(state.Query, "@SearchText1") {
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
	if _, err := ValidateResultColumns([]string{"ContractID", "CustomColumn", "SearchText"}, cfg.ListColumns); err == nil {
		t.Fatal("accepted a field not returned by SQL")
	}
	cfg.ListColumns[1].Field = "CustomColumn"
	if _, err := ValidateResultColumns([]string{"ContractID", "CustomColumn", "SearchText"}, cfg.ListColumns); err == nil {
		t.Fatal("accepted a missing sortField")
	}
	cfg.ListColumns[1].SortField = "ContractID"
	if _, err := ValidateResultColumns([]string{"ContractID", "CustomColumn", "SearchText"}, cfg.ListColumns); err != nil {
		t.Fatal(err)
	}
}

func TestUnconfiguredContractsUseConfiguredSQLAndDisplayEveryColumn(t *testing.T) {
	state := &fakeSQLState{Affected: 1}
	const driverName = "serviceAgreementUnconfiguredTest"
	sql.Register(driverName, fakeSQLDriver{state: state})
	db, err := sql.Open(driverName, "")
	if err != nil { t.Fatal(err) }
	defer db.Close()
	cfg := DefaultConfig()
	cfg.ConnectionString = "test"
	cfg.WithoutParametersSQL = "query_unconfigured"
	store := Store{DB: db, Config: cfg}
	result, err := store.ListUnconfigured(context.Background(), "  1206 ")
	if err != nil { t.Fatal(err) }
	if !strings.Contains(state.Query, "FROM (\nquery_unconfigured\n)") || state.Args["SearchText1"] != "1206" ||
		state.Args["limit"] != int64(cfg.ListLimit) && state.Args["limit"] != cfg.ListLimit {
		t.Fatalf("wrong configured SQL or named search parameters: %s, %v", state.Query, state.Args)
	}
	if len(result.Columns) != 5 || len(result.Items) != 2 || result.Items[0].ID != 1206 ||
		result.Items[0].Field("CustomExtra") != "Поставщик" {
		t.Fatalf("SQL columns not exposed: %+v", result)
	}
	cols := UnconfiguredColumns(result.Columns)
	if len(cols) != len(result.Columns)-2 {
        t.Fatalf("service fields must be hidden from the second list: %v", cols)
    }
    want := []string{"ContractName", "DateSigned", "CustomExtra"}
    for i, c := range cols {
        if !c.Visible || c.Field != want[i] || c.Order != i+1 || c.Title != c.Field {
            t.Errorf("second list column %d: %+v",i,c)
        }
    }
    if result.Items[0].ID != 1206 || result.Items[0].Field("SearchText") == "" ||
       result.Items[0].Field("ContractID") != "1206" {
        t.Fatal("hidden service fields must still be available internally")
    }
	cfg.WithoutParametersSQL = "query_unconfigured_empty"
	store.Config = cfg
	empty, err := store.ListUnconfigured(context.Background(), "")
	if err != nil || len(empty.Items) != 0 || len(empty.Columns) != 4 {
		t.Fatalf("empty SQL result must still define columns: %+v %v", empty, err)
	}
	cfg.WithoutParametersSQL = "query_unconfigured_bad"
	store.Config = cfg
	if _, err := store.ListUnconfigured(context.Background(), ""); err == nil {
		t.Fatal("missing ContractID must be rejected before displaying records")
	}
}


func TestBothListsFormatSQLMoneyByType(t *testing.T) {
    state:=&fakeSQLState{}
    dbName:="serviceAgreementAliasedMoneyListTest"
    sql.Register(dbName,fakeSQLDriver{state:state})
    db,err:=sql.Open(dbName,"")
    if err!=nil {t.Fatal(err)}
    defer db.Close()
    cfg:=DefaultConfig()
    cfg.ListSQL="query_money_list"
    cfg.WithoutParametersSQL="query_unconfigured_money"
    cfg.ListColumns=[]ListColumn{
        {Field:"ContractID",Title:"Код",Width:100,Order:1,SortField:"ContractID",Visible:false},
        {Field:"Абон.плата",Title:"Абон.плата",Width:150,Order:2,SortField:"Абон.плата",Visible:true},
        {Field:"SearchText",Title:"SearchText",Width:100,Order:3,SortField:"SearchText",Visible:false},
    }
    store:=Store{DB:db,Config:cfg}
    a,err:=store.List(context.Background(),"")
    if err!=nil {t.Fatal(err)}
    if len(a)!=2 || a[0].Field("Абон.плата")!="2\u00a0222,00" ||
        a[1].Field("Абон.плата")!="1\u00a0150,24" {
        t.Fatalf("money in list_sql not formatted N2: %+v",a)
    }
    if kind:=DetectFilterKind(a,"Абон.плата");kind!=FilterNumber {
        t.Errorf("SQL MONEY in list_sql must be numeric: %q",kind)
    }
    if raw:=sqlText(a[0].SQLValue("Абон.плата"));raw!="2222.0000" {
        t.Fatalf("original money precision lost: %q",raw)
    }
    without,err:=store.ListUnconfigured(context.Background(),"")
    if err!=nil {t.Fatal(err)}
    if len(without.Items)!=2 || without.Items[0].Field("Абон.плата")!="0,00" ||
        without.Items[1].Field("Абон.плата")!="4\u00a0520,46" {
        t.Fatalf("money in without_parameters_sql not formatted N2: %+v",without)
    }
    if kind:=DetectFilterKind(without.Items,"Абон.плата");kind!=FilterNumber {
        t.Errorf("SQL MONEY in second list must be numeric: %q",kind)
    }
    unique:=ColumnValues(without.Items,"Абон.плата")
    if len(unique)!=2 || unique[0].Value!="0,00" || unique[1].Value!="4\u00a0520,46" {
        t.Fatalf("unique values popup does not use N2: %+v",unique)
    }
}


// Text returned by SQL must remain text in both lists, no matter how the
// alias is spelled or how numeric its content looks.
func TestBothListsKeepCastMoneyNVARCHARAsText(t *testing.T) {
    state:=&fakeSQLState{}
    sql.Register("serviceAgreementCastedMoneyAsText",fakeSQLDriver{state:state})
    db,err:=sql.Open("serviceAgreementCastedMoneyAsText","")
    if err!=nil {t.Fatal(err)}
    defer db.Close()
    cfg:=DefaultConfig()
    cfg.ListSQL="query_textmoney_list"
    cfg.WithoutParametersSQL="query_unconfigured_textmoney"
    cfg.ListColumns=[]ListColumn{
        {Field:"ContractID",Title:"Код",Width:80,Order:1,SortField:"ContractID",Visible:false},
        {Field:"Абон.плата",Title:"Абон.плата",Width:120,Order:2,SortField:"Абон.плата",Visible:true},
        {Field:"SearchText",Title:"Поиск",Width:180,Order:3,SortField:"SearchText",Visible:false},
    }
    store:=Store{DB:db,Config:cfg}
    rows,err:=store.List(context.Background(),"")
    if err!=nil {t.Fatal(err)}
    if len(rows)!=1 || rows[0].Field("Абон.плата")!="2222.0000" {
        t.Fatalf("SQL NVARCHAR is NOT MONEY, unchanged text expected: %+v",rows)
    }
    p:=InferColumnPresentation(rows,"Абон.плата")
    if p.Kind!=FilterText || p.Money {
        t.Fatalf("SQL NVARCHAR must be left-aligned textual data: %+v",p)
    }
    without,err:=store.ListUnconfigured(context.Background(),"")
    if err!=nil {t.Fatal(err)}
    if len(without.Items)!=1 || without.Items[0].Field("Абон.плата")!="4520.4600" {
        t.Fatalf("NVARCHAR in second list should remain unformatted: %+v",without)
    }
    if k:=DetectFilterKind(without.Items,"Абон.плата");k!=FilterText {
        t.Fatalf("NVARCHAR second list should have a text filter, got %q",k)
    }
    values:=ColumnValues(without.Items,"Абон.плата")
    if len(values)!=1 || values[0].Value!="4520.4600" {
        t.Fatalf("NVARCHAR values in checkbox list must not be formatted: %+v",values)
    }
}
