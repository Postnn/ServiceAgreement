package core

import (
    "reflect"
    "testing"
)

func sampleFilterRows() []ListItem {
    return []ListItem{
        {ID:1,Values:map[string]string{"Department":"A","State":"Open","Amount":"10,00"}},
        {ID:2,Values:map[string]string{"Department":"B","State":"Open","Amount":"20,00"}},
        {ID:3,Values:map[string]string{"Department":"A","State":"Closed","Amount":"10,00"}},
        {ID:4,Values:map[string]string{"Department":"","State":"Closed","Amount":"0,00"}},
    }
}
func ids(rows []ListItem) []int {
    result:=[]int{}
    for _,r:=range rows {result=append(result,r.ID)}
    return result
}
func TestColumnFiltersCombineAndAndRestoreOriginalData(t *testing.T) {
    rows:=sampleFilterRows()
    filters:=ColumnFilterState{}
    if got:=filters.Apply(rows);len(got)!=4 {t.Fatal("empty filters altered rows")}
    filters.Set("Department",map[string]bool{"A":true})
    if got:=ids(filters.Apply(rows));!reflect.DeepEqual(got,[]int{1,3}) {t.Fatalf("first filter: %v",got)}
    filters.Set("sTaTe",map[string]bool{"Closed":true})
    if got:=ids(filters.Apply(rows));!reflect.DeepEqual(got,[]int{3}) {t.Fatalf("AND filter: %v",got)}
    if filters.Count()!=2 || !filters.Active("STATE") {t.Fatalf("case-insensitive filter key failed: %v",filters)}
    filters.Clear("state")
    if got:=ids(filters.Apply(rows));!reflect.DeepEqual(got,[]int{1,3}) {t.Fatalf("clear single filter: %v",got)}
    filters.Set("Department",map[string]bool{})
    if got:=filters.Apply(rows);len(got)!=0 {t.Fatalf("empty selection must match none: %v",got)}
    filters.ClearAll()
    if got:=ids(filters.Apply(rows));!reflect.DeepEqual(got,[]int{1,2,3,4}) {t.Fatalf("clear all: %v",got)}
    if len(rows)!=4 || rows[0].Field("Department")!="A" {t.Fatal("input SQL records were mutated")}
}
func TestDistinctValuesAndFieldRemoval(t *testing.T) {
    rows:=sampleFilterRows()
    values:=ColumnValues(rows,"department")
    want:=[]ColumnValueCount{{Value:"",Count:1},{Value:"A",Count:2},{Value:"B",Count:1}}
    if !reflect.DeepEqual(values,want) {t.Fatalf("unexpected distinct values: %v",values)}
    filter:=ColumnFilterState{}
    filter.Set("State",map[string]bool{"Open":true})
    filter.Set("Department",map[string]bool{"B":true})
    filter.KeepFields([]ListColumn{{Field:"Department"}})
    if filter.Active("State") || !filter.Active("Department") {t.Fatalf("missing SQL alias filter not discarded: %v",filter)}
}
