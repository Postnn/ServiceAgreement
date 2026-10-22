package presentation

import (
	"reflect"
	"testing"

	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

func sampleFilterRows() []readmodel.ListItem {
	return []readmodel.ListItem{
		{ID: 1, Values: map[string]string{"Department": "A", "State": "Open", "Amount": "10,00"}},
		{ID: 2, Values: map[string]string{"Department": "B", "State": "Open", "Amount": "20,00"}},
		{ID: 3, Values: map[string]string{"Department": "A", "State": "Closed", "Amount": "10,00"}},
		{ID: 4, Values: map[string]string{"Department": "", "State": "Closed", "Amount": "0,00"}},
	}
}
func ids(rows []readmodel.ListItem) []int {
	result := []int{}
	for _, r := range rows {
		result = append(result, r.ID)
	}
	return result
}
func TestColumnFiltersCombineAndAndRestoreOriginalData(t *testing.T) {
	rows := sampleFilterRows()
	filters := ColumnFilterState{}
	if got := filters.Apply(rows); len(got) != 4 {
		t.Fatal("empty filters altered rows")
	}
	filters.Set("Department", map[string]bool{"A": true})
	if got := ids(filters.Apply(rows)); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("first filter: %v", got)
	}
	filters.Set("sTaTe", map[string]bool{"Closed": true})
	if got := ids(filters.Apply(rows)); !reflect.DeepEqual(got, []int{3}) {
		t.Fatalf("AND filter: %v", got)
	}
	if filters.Count() != 2 || !filters.Active("STATE") {
		t.Fatalf("case-insensitive filter key failed: %v", filters)
	}
	filters.Clear("state")
	if got := ids(filters.Apply(rows)); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("clear single filter: %v", got)
	}
	filters.Set("Department", map[string]bool{})
	if got := filters.Apply(rows); len(got) != 0 {
		t.Fatalf("empty selection must match none: %v", got)
	}
	filters.ClearAll()
	if got := ids(filters.Apply(rows)); !reflect.DeepEqual(got, []int{1, 2, 3, 4}) {
		t.Fatalf("clear all: %v", got)
	}
	if len(rows) != 4 || rows[0].Field("Department") != "A" {
		t.Fatal("input SQL records were mutated")
	}
}
func TestDistinctValuesAndFieldRemoval(t *testing.T) {
	rows := sampleFilterRows()
	values := ColumnValues(rows, "department")
	want := []ColumnValueCount{{Value: "", Count: 1}, {Value: "A", Count: 2}, {Value: "B", Count: 1}}
	if !reflect.DeepEqual(values, want) {
		t.Fatalf("unexpected distinct values: %v", values)
	}
	filter := ColumnFilterState{}
	filter.Set("State", map[string]bool{"Open": true})
	filter.Set("Department", map[string]bool{"B": true})
	filter.KeepFields([]readmodel.ListColumn{{Field: "Department"}})
	if filter.Active("State") || !filter.Active("Department") {
		t.Fatalf("missing SQL alias filter not discarded: %v", filter)
	}
}

func TestFacetedFiltersDependOnOtherColumnsOnly(t *testing.T) {
	all := sampleFilterRows()
	filters := ColumnFilterState{}
	filters.Set("Department", map[string]bool{"A": true})

	// With Department=A, the State dropdown offers values only from A.
	states := filters.FacetedValues(all, "State")
	wantStates := []ColumnValueCount{
		{Value: "Closed", Count: 1},
		{Value: "Open", Count: 1},
	}
	if !reflect.DeepEqual(states, wantStates) {
		t.Fatalf("State should be scoped by Department A: %v", states)
	}
	// Editing Department must ignore Department's OWN active selection.
	departments := filters.FacetedValues(all, "Department")
	wantDepartments := []ColumnValueCount{
		{Value: "", Count: 1}, {Value: "A", Count: 2}, {Value: "B", Count: 1},
	}
	if !reflect.DeepEqual(departments, wantDepartments) {
		t.Fatalf("Department should offer other choices despite own filter: %v", departments)
	}

	// Second column filter compounds the first, not replacing it.
	filters.Set("State", map[string]bool{"Open": true})
	if got := ids(filters.Apply(all)); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("two simultaneous AND filters lost: %v", got)
	}
	if filters.Count() != 2 || !filters.Active("Department") || !filters.Active("State") {
		t.Fatalf("second column filter replaced the first: %v", filters)
	}

	// With State=Open, only A and B are available in the Department menu.
	departments = filters.FacetedValues(all, "Department")
	wantDepartments = []ColumnValueCount{{Value: "A", Count: 1}, {Value: "B", Count: 1}}
	if !reflect.DeepEqual(departments, wantDepartments) {
		t.Fatalf("other filter must constrain available values: %v", departments)
	}
	// Current State selection should not remove Closed from State's OWN menu,
	// provided it exists under Department=A.
	if states = filters.FacetedValues(all, "State"); !reflect.DeepEqual(states, wantStates) {
		t.Fatalf("State must ignore its own restriction: %v", states)
	}
	// A third facet sees the intersection of the first two filters.
	amounts := filters.FacetedValues(all, "Amount")
	if !reflect.DeepEqual(amounts, []ColumnValueCount{{Value: "10,00", Count: 1}}) {
		t.Fatalf("third facet ignored other columns: %v", amounts)
	}

	// Switching the first facet preserves the second facet.
	filters.Set("Department", map[string]bool{"B": true})
	if got := ids(filters.Apply(all)); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("switching Department cleared State: %v", got)
	}
	filters.Clear("State")
	if got := ids(filters.Apply(all)); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("clearing State also cleared Department: %v", got)
	}
}

func TestFiltersAreIndependentBetweenTwoAgreementTabs(t *testing.T) {
	withParameters := ColumnFilterState{}
	withoutParameters := ColumnFilterState{}
	withParameters.Set("Department", map[string]bool{"A": true})
	withoutParameters.Set("Department", map[string]bool{"B": true})
	all := sampleFilterRows()
	if got := ids(withParameters.Apply(all)); !reflect.DeepEqual(got, []int{1, 3}) {
		t.Fatalf("first tab filter: %v", got)
	}
	if got := ids(withoutParameters.Apply(all)); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("second tab filter: %v", got)
	}
	withParameters.ClearAll()
	if len(withoutParameters) != 1 || withoutParameters.Count() != 1 {
		t.Fatal("clearing first tab filter unexpectedly cleared second tab")
	}
}
