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
