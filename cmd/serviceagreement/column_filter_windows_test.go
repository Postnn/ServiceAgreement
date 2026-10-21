//go:build windows

package main

import (
    "reflect"
    "strings"
    "testing"

    "github.com/Postnn/ServiceAgreement/internal/core"
    "github.com/lxn/walk"
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


func TestRepeatedValueSearchDoesNotResetOnFocusLoss(t *testing.T) {
    all:=[]core.ColumnValueCount{
        {Value:"Договор оказания услуг",Count:2},
        {Value:"Заявка на оплату",Count:1},
        {Value:"Договор аренды",Count:3},
    }
    model:=newFilterValueModel(all,map[string]bool{
        "Договор оказания услуг":true,
        "Заявка на оплату":true,
        "Договор аренды":true,
    })
    resets:=0
    model.RowsReset().Attach(func(){resets++})
    model.search("") // initial blank search; this is the focus-loss before sort
    if resets!=0 {t.Fatalf("unchanged blank search caused %d unnecessary resets",resets)}
    model.search("дОгОвОр")
    if model.RowCount()!=2 || resets!=1 {
        t.Fatalf("substring query failed; rows=%d resets=%d",model.RowCount(),resets)
    }
    model.search("ДОГОВОР") // Enter + WM_KILLFOCUS must not trigger another reset
    if resets!=1 {t.Fatalf("duplicate EditingFinished reset rows during button click: %d",resets)}
    model.search("оплат")
    if model.RowCount()!=1 || resets!=2 || model.Value(0,0)!="Заявка на оплату" {
        t.Fatalf("new contains query not applied: rows=%d resets=%d",model.RowCount(),resets)
    }
}

func TestBulkCheckboxSelectionRedrawsVisibleRows(t *testing.T) {
    all:=[]core.ColumnValueCount{
        {Value:"A",Count:1},{Value:"B",Count:2},
        {Value:"C",Count:3},{Value:"D",Count:4},
    }
    selected:=map[string]bool{"A":true,"B":true,"C":true,"D":true}
    model:=newFilterValueModel(all,selected)
    resets:=0
    changes:=0
    from,to:=-1,-1
    notifications:=0
    model.RowsReset().Attach(func(){resets++})
    model.RowsChanged().Attach(func(first,last int){
        changes++
        from,to=first,last
    })
    model.changed=func(){notifications++}

    model.selectVisible(false)
    if model.selectedCount()!=0 || changes!=1 || from!=0 || to!=3 || resets!=0 || notifications!=1 {
        t.Fatalf("uncheck-all did not redraw rows correctly: checked=%d changes=%d range=[%d,%d] resets=%d notices=%d",
            model.selectedCount(),changes,from,to,resets,notifications)
    }
    for i:=0;i<model.RowCount();i++ {
        if model.Checked(i) { t.Fatalf("checkbox %d was not cleared",i) }
    }

    model.selectVisible(true)
    if model.selectedCount()!=4 || changes!=2 || resets!=0 || notifications!=2 {
        t.Fatalf("select-all did not repaint the native state images: checked=%d changes=%d resets=%d",
            model.selectedCount(),changes,resets)
    }
    for i:=0;i<model.RowCount();i++ {
        if !model.Checked(i) {t.Fatalf("checkbox %d was not set",i)}
    }

    // No real state change: prevent spurious repaint bursts.
    model.selectVisible(true)
    if changes!=2 {t.Fatalf("redundant selection redrew rows: %d",changes)}

    // With a local search, toggle only the visible values, preserve others.
    model.search("B")
    before:=changes
    model.selectVisible(false)
    if changes!=before+1 || model.RowCount()!=1 || model.Checked(0) {
        t.Fatal("searched visible checkbox did not refresh")
    }
    model.search("")
    if model.selectedCount()!=3 || model.Checked(1) {
        t.Fatal("search-scoped toggle affected unrelated checkboxes")
    }
}

func TestExcelStyleTypedMenuOperators(t *testing.T) {
    want:=map[core.FilterKind][]core.FilterOperator{
        core.FilterText:{
            core.FilterEqual,core.FilterNotEqual,
            core.FilterContains,core.FilterNotContains,
            core.FilterStarts,core.FilterEnds,
        },
        core.FilterNumber:{
            core.FilterEqual,core.FilterNotEqual,
            core.FilterGreater,core.FilterGreaterEqual,
            core.FilterLess,core.FilterLessEqual,
            core.FilterBetween,core.FilterNotBetween,
            core.FilterTop,core.FilterAboveAverage,core.FilterBelowAverage,
        },
        core.FilterDate:{
            core.FilterEqual,core.FilterNotEqual,
            core.FilterGreater,core.FilterGreaterEqual,
            core.FilterLess,core.FilterLessEqual,
            core.FilterBetween,core.FilterNotBetween,
        },
    }
    for kind,operators:=range want {
        menu:=conditionOperators(kind)
        if len(menu)!=len(operators) {
            t.Fatalf("%s expected %d operators, got %d",kind,len(operators),len(menu))
        }
        for i,op:=range operators {
            if menu[i].Operator!=op || menu[i].Caption=="" {
                t.Errorf("%s menu item %d: got %+v, expected %s",kind,i,menu[i],op)
            }
            if filterOperatorIndex(menu,op)!=i {
                t.Errorf("%s wrong index for %s",kind,op)
            }
        }
        if filterKindCaption(kind)=="" || filterKindInputExample(kind)=="" {
            t.Errorf("%s missing Russian UI labels",kind)
        }
    }
}


func TestComposeORFilterDialogInputs(t *testing.T) {
    c,err:=composeORFilter(
        core.FilterText,core.FilterContains,"Булычев","",
        true,core.FilterStarts,"Документ","",
    )
    if err!=nil {t.Fatal(err)}
    if c.Additional==nil || c.Additional.Operator!=core.FilterStarts ||
        c.Value!="Булычев" || c.Additional.Value!="Документ" {
        t.Fatalf("second input is not its own OR condition: %+v",c)
    }
    if text:=c.Description();text!="(содержит «Булычев» ИЛИ начинается с «Документ»)" {
        t.Fatalf("human readable OR expression: %s",text)
    }
    number,err:=composeORFilter(
        core.FilterNumber,core.FilterBetween,"10","20",
        true,core.FilterGreater,"100","",
    )
    if err!=nil {t.Fatal(err)}
    if number.Value2!="20" || number.Additional==nil ||
        number.Additional.Value!="100" || number.Additional.Value2!="" {
        t.Fatalf("BETWEEN boundary was confused with a second OR condition: %+v",number)
    }
    single,err:=composeORFilter(
        core.FilterText,core.FilterContains,"договор","",
        false,core.FilterGreater,"нельзя применять","",
    )
    if err!=nil || single.Additional!=nil {
        t.Fatalf("unused second condition should remain absent: %+v err=%v",single,err)
    }
    if _,err:=composeORFilter(
        core.FilterNumber,core.FilterGreater,"10","",
        true,core.FilterLess,"некорректно","",
    );err==nil {
        t.Fatal("invalid optional second condition was accepted")
    }
}


func TestUnusedConditionInputVisibility(t *testing.T) {
    cases:=[]struct {
        name string
        op core.FilterOperator
        active bool
        value, boundary bool
    }{
        {"inactive second OR",core.FilterContains,false,false,false},
        {"no operator selected","",false,false,false},
        {"text contains",core.FilterContains,true,true,false},
        {"numeric more than",core.FilterGreater,true,true,false},
        {"numeric between",core.FilterBetween,true,true,true},
        {"numeric outside range",core.FilterNotBetween,true,true,true},
        {"top N",core.FilterTop,true,true,false},
        {"above average",core.FilterAboveAverage,true,false,false},
        {"below average",core.FilterBelowAverage,true,false,false},
        {"date between",core.FilterBetween,true,true,true},
    }
    for _,tc:=range cases{
        actualValue,actualBoundary:=editorInputVisibility(tc.op,tc.active)
        if actualValue!=tc.value || actualBoundary!=tc.boundary {
            t.Errorf("%s: shown=(%v,%v), want (%v,%v)",
                tc.name,actualValue,actualBoundary,tc.value,tc.boundary)
        }
    }
}

func TestFilterSummaryUnderBothTables(t *testing.T) {
    cond:=core.FilterCondition{
        Kind:core.FilterText,Operator:core.FilterContains,Value:"Булычев",
        Additional:&core.FilterCondition{
            Kind:core.FilterText,Operator:core.FilterContains,Value:"Ник",
        },
    }
    m:=&tableModel{columns:[]core.ListColumn{
        {Field:"AgreementLabel",Title:"Договор"},
        {Field:"Amount",Title:"Сумма"},
        {Field:"Department",Title:"Отдел"},
    }}
    m.replace([]core.ListItem{
        {ID:1,Values:map[string]string{"AgreementLabel":"Булычев","Amount":"1000","Department":"A"}},
        {ID:2,Values:map[string]string{"AgreementLabel":"Ник","Amount":"500","Department":"B"}},
    })
    if filterSummaryText(m)!=""{
        t.Fatal("multiline filter summary should not take up space without conditions")
    }
    if err:=m.conditions.Set("AgreementLabel",cond);err!=nil {t.Fatal(err)}
    if err:=m.conditions.Set("Amount",core.FilterCondition{
        Kind:core.FilterNumber,Operator:core.FilterGreater,Value:"100",
    });err!=nil {t.Fatal(err)}
    m.filters.Set("Department",map[string]bool{"A":true})
    m.applyFilters()
    got:=filterSummaryText(m)
    for _,part:=range []string{
        "Действующие фильтры", "Договор:", "содержит «Булычев»",
        "ИЛИ содержит «Ник»", "\nИ Сумма: больше «100»",
        "\nИ Отдел: выбрано 1 уникальных значений",
    } {
        if !strings.Contains(got,part) {
            t.Errorf("summary omits %q: %s",part,got)
        }
    }
    m.conditions.ClearAll()
    if got:=filterSummaryText(m);got!="" {
        t.Fatalf("only checkbox filters should not force typed summary panel: %s",got)
    }

    // Same helper drives the independent second list.
    other:=&tableModel{columns:[]core.ListColumn{
        {Field:"Договор",Title:"Договор"},
    }}
    other.replace([]core.ListItem{{ID:8,Values:map[string]string{"Договор":"Тест"}}})
    if err:=other.conditions.Set("Договор",core.FilterCondition{
        Kind:core.FilterText,Operator:core.FilterEqual,Value:"Тест",
    });err!=nil {t.Fatal(err)}
    if got:=filterSummaryText(other);!strings.Contains(got,"Договор: равно «Тест»") {
        t.Fatalf("second tab does not show typed conditions: %q",got)
    }
}

func TestClearActiveFilterWhileDialogRemainsOpen(t *testing.T) {
    m:=&tableModel{columns:[]core.ListColumn{
        {Field:"Department",Title:"Отдел"},{Field:"Amount",Title:"Сумма"},
    }}
    m.replace([]core.ListItem{
        {ID:1,Values:map[string]string{"Department":"A","Amount":"100"},
            Raw:map[string]any{"Amount":int64(100)}},
        {ID:2,Values:map[string]string{"Department":"A","Amount":"500"},
            Raw:map[string]any{"Amount":int64(500)}},
        {ID:3,Values:map[string]string{"Department":"B","Amount":"900"},
            Raw:map[string]any{"Amount":int64(900)}},
    })
    m.filters.Set("Department",map[string]bool{"A":true})
    if err:=m.conditions.Set("Amount",core.FilterCondition{
        Kind:core.FilterNumber,Operator:core.FilterGreater,Value:"200",
    });err!=nil {t.Fatal(err)}
    m.applyFilters()
    if len(m.items)!=1 || m.items[0].ID!=2 {
        t.Fatalf("test setup incorrect: %+v",m.items)
    }
    clearOneColumnFilter(m,"Amount")
    if m.conditions.Active("Amount") || m.filters.Active("Amount") {
        t.Fatal("active typed condition not removed")
    }
    if !m.filters.Active("Department") {
        t.Fatal("clearing one condition changed a different column")
    }
    if len(m.items)!=2 || m.items[0].ID!=1 || m.items[1].ID!=2 {
        t.Fatalf("filter was not reapplied without reopening dialog: %+v",m.items)
    }
    if got:=filterSummaryText(m);got!="" {
        t.Fatalf("typed filter summary remains after clearing: %s",got)
    }
}

func TestFilterDialogFitsCurrentContentMode(t *testing.T) {
    condition:=filterPopupSize(true)
    checklist:=filterPopupSize(false)
    if condition.Width!=checklist.Width {
        t.Fatalf("dialog should keep convenient consistent width: condition=%v unique=%v",condition,checklist)
    }
    if condition.Width<650 || condition.Width>730 {
        t.Fatalf("dialog should be wide enough for range controls but not oversized: %v",condition)
    }
    if condition.Height<390 || condition.Height>450 {
        t.Fatalf("condition-only dialog is not compact: %v",condition)
    }
    if checklist.Height<530 || checklist.Height>610 {
        t.Fatalf("value checklist must remain a comfortable height: %v",checklist)
    }
    if checklist.Height-condition.Height<100 {
        t.Fatalf("hidden checkbox list has not reclaimed enough vertical space: %v %v",condition,checklist)
    }
    initial:=walk.Rectangle{X:220,Y:140,Width:checklist.Width,Height:checklist.Height}
    smaller:=filterPopupCenteredBounds(initial,true)
    if smaller.Width!=condition.Width || smaller.Height!=condition.Height {
        t.Fatalf("modal shrink dimensions incorrect: %v",smaller)
    }
    if smaller.Y <= initial.Y || smaller.X!=initial.X {
        t.Fatalf("shrinking dialog should preserve center, not pin upper-left: %v->%v",initial,smaller)
    }
    restored:=filterPopupCenteredBounds(smaller,false)
    if restored.Width!=initial.Width || restored.Height!=initial.Height ||
        restored.X!=initial.X || restored.Y!=initial.Y {
        t.Fatalf("modal switched back without preserving position: initial=%v, restored=%v",initial,restored)
    }
}
