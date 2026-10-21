//go:build windows

package main

import (
    "reflect"
    "testing"

    "github.com/Postnn/ServiceAgreement/internal/core"
)

func TestFilterValueModelChecksAndSearch(t *testing.T) {
    all:=[]core.ColumnValueCount{
        {Value:"",Count:2},
        {Value:"Договор",Count:3},
        {Value:"Услуги",Count:1},
        {Value:"2026",Count:5},
    }
    chosen:=map[string]bool{"":true,"Договор":true,"2026":true,"Услуги":true}
    m:=newFilterValueModel(all,chosen)
    if m.RowCount()!=4 || !m.Checked(0) { t.Fatal("initial filter checkbox state") }
    m.search("услуг")
    if m.RowCount()!=1 || m.Value(0,0)!="Услуги" {
        t.Fatalf("case insensitive value search: %v",m.indices)
    }
    if err:=m.SetChecked(0,false);err!=nil {t.Fatal(err)}
    if m.selectedCount()!=3 {t.Fatalf("checkbox update: %d",m.selectedCount())}
    m.search("")
    if m.RowCount()!=4 || m.Checked(2) { t.Fatal("search reset lost selection") }
    m.search("пустые")
    if m.RowCount()!=1 || m.Value(0,0)!="(Пустые)" {t.Fatal("empty SQL values should have visible label")}
    m.selectVisible(false)
    if m.checked[""] { t.Fatal("select visible did not uncheck empty values") }
    if m.selectedCount()!=2 {t.Fatalf("unexpected remaining selected values: %d",m.selectedCount())}
    m.search("")
    selected:=m.selectedValues()
    expected:=map[string]bool{"Договор":true,"2026":true}
    if !reflect.DeepEqual(selected,expected) {t.Fatalf("selected values differ: %v",selected)}
    m.selectVisible(true)
    if m.selectedCount()!=4 {t.Fatal("select-all failed")}
}
func TestFilterColumnHeadingVisualStates(t *testing.T) {
    col:=core.ListColumn{Title:"Сумма"}
    if inactive:=filterHeaderTitle(col,false); inactive!="Сумма  ▾" {
        t.Fatalf("missing dropdown glyph %q",inactive)
    }
    if active:=filterHeaderTitle(col,true);active!="Сумма  ●▾" {
        t.Fatalf("active filter marker missing: %q",active)
    }
}

func TestColumnSortChangesRowsButNotColumnSequence(t *testing.T) {
    columns := []core.ListColumn{
        {Field:"ContractLabel", Title:"Договор", Width:250, Order:1, SortField:"ContractLabel",Visible:true},
        {Field:"Amount", Title:"Сумма", Width:120, Order:2, SortField:"Amount",Visible:true},
        {Field:"PaymentText", Title:"Оплата", Width:300, Order:3, SortField:"PaymentCode",Visible:true},
    }
    model:=&tableModel{columns:append([]core.ListColumn(nil),columns...)}
    model.replace([]core.ListItem{
        {ID:1, Values:map[string]string{"ContractLabel":"Первый","Amount":"10,00"}, Raw:map[string]any{"Amount":int64(10)}},
        {ID:2, Values:map[string]string{"ContractLabel":"Второй","Amount":"20,00"}, Raw:map[string]any{"Amount":int64(20)}},
    })
    core.SortListItems(model.allItems,"Amount",true)
    model.applyFilters()
    if model.items[0].ID != 2 {
        t.Fatalf("sorting did not change row order: %#v",model.items)
    }
    if !reflect.DeepEqual(model.columns,columns) {
        t.Fatalf("column sequence was changed by sorting: %#v",model.columns)
    }
    model.filters.Set("ContractLabel",map[string]bool{"Первый":true})
    model.applyFilters()
    if len(model.items)!=1 || model.items[0].ID!=1 {
        t.Fatalf("sort + filter mismatch: %#v",model.items)
    }
    if !reflect.DeepEqual(model.columns,columns) {
        t.Fatal("column sequence was changed by filtering")
    }
}

func TestSearchWithinColumnValuesUsesLocalContains(t *testing.T) {
    values:=newFilterValueModel([]core.ColumnValueCount{
        {Value:"Заявление №138",Count:2},
        {Value:"Договор №138",Count:3},
        {Value:"Заявка на продление",Count:1},
    },map[string]bool{
        "Заявление №138":true,
        "Договор №138":true,
        "Заявка на продление":true,
    })
    // Before Enter or loss of focus, the list remains unfiltered.
    if values.RowCount()!=3 {t.Fatal("values were filtered prematurely")}
    // This is the local operation invoked by the editor's OnEditingFinished,
    // which Walk emits on both VK_RETURN and WM_KILLFOCUS.
    values.search("ЯВ")
    if values.RowCount()!=2 {t.Fatalf("case-insensitive substring failed, got %d",values.RowCount())}
    values.search("138")
    if values.RowCount()!=2 {t.Fatalf("middle-of-string contains failed: %d",values.RowCount())}
    values.search("000")
    if values.RowCount()!=0 {t.Fatal("non-matching substring should show empty list")}
    values.search("")
    if values.RowCount()!=3 {t.Fatal("clearing search must restore the checklist")}
}
