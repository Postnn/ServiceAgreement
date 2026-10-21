package core

import (
    "math/big"
    "reflect"
    "strings"
    "testing"
    "time"
)

func conditionalRows() []ListItem {
    nums:=[]string{"5.0000","8.0000","15.5000","30.0000","10.0000"}
    labels:=[]string{"Договор аренды","Услуги связи","Договор оказания услуг","Подписан","Услуги связи"}
    groups:=[]string{"A","A","A","B","B"}
    dates:=[]string{"2026-01-03","2026-01-04","2026-02-10","2026-03-09","2026-05-01"}
    out:=make([]ListItem,len(nums))
    for i:=range nums {
        out[i]=ListItem{
            ID:i+1,
            Values:map[string]string{
                "Amount":nums[i],"Name":labels[i],"Department":groups[i],"SignedAt":dates[i],
            },
            Raw:map[string]any{
                "Amount":[]byte(nums[i]),"Name":labels[i],"Department":groups[i],
                "SignedAt":dates[i],
            },
            ColumnTypes:map[string]string{
                "Amount":"MONEY","Name":"NVARCHAR","Department":"NVARCHAR","SignedAt":"DATE",
            },
        }
    }
    return out
}

func TestConditionalKindDetectionUsesSQLMetadataAndRaw(t *testing.T) {
    rows:=conditionalRows()
    for _,tc:=range []struct{field string; want FilterKind}{
        {"Amount",FilterNumber},
        {"Name",FilterText},
        {"Department",FilterText},
        {"SignedAt",FilterDate},
    } {
        if got:=DetectFilterKind(rows,tc.field);got!=tc.want {
            t.Errorf("%s = %q, want %q",tc.field,got,tc.want)
        }
    }
    if got:=DetectFilterKind([]ListItem{{
        Raw:map[string]any{"X":time.Date(2026,1,1,0,0,0,0,time.UTC)},
    }},"X");got!=FilterDate {
        t.Fatalf("raw date fallback: %v",got)
    }
    if got:=DetectFilterKind([]ListItem{{
        Raw:map[string]any{"X":big.NewRat(123,10)},
    }},"X");got!=FilterNumber {t.Fatalf("raw numeric fallback: %v",got)}
    // SQL type always wins: aliases and numeric-looking text never turn
    // NVARCHAR into a numeric filter.
    if got:=DetectFilterKind([]ListItem{{ColumnTypes:map[string]string{"Amount":"NVARCHAR"}}},"Amount");
        got!=FilterText {t.Fatalf("NVARCHAR should be text even for Amount alias: %v",got)}
    if got:=DetectFilterKind([]ListItem{{ColumnTypes:map[string]string{"Amount":"MONEY"}}},"Amount");
        got!=FilterNumber {t.Fatalf("SQL MONEY must be numeric: %v",got)}
    for _,sqlType:=range []string{"SMALLMONEY","INT","BIGINT","REAL","FLOAT","DECIMAL","NUMERIC"} {
        if got:=DetectFilterKind([]ListItem{{ColumnTypes:map[string]string{"Column":sqlType}}},"Column");
            got!=FilterNumber {
            t.Errorf("%s should yield numeric filter, got %s",sqlType,got)
        }
    }
}

func TestTextConditionsWithRussianAndMissingValues(t *testing.T) {
    rows:=conditionalRows()
    cases:=[]struct{
        op FilterOperator; input string; ids []int; description string
    }{
        {FilterContains,"УСЛУГ",[]int{2,3,5},"содержит «УСЛУГ»"},
        {FilterNotContains,"услуг",[]int{1,4},"не содержит «услуг»"},
        {FilterStarts,"ДОГОВОР",[]int{1,3},"начинается с «ДОГОВОР»"},
        {FilterEnds,"связи",[]int{2,5},"заканчивается на «связи»"},
        {FilterEqual,"Услуги связи",[]int{2,5},"равно «Услуги связи»"},
        {FilterNotEqual,"Услуги связи",[]int{1,3,4},"не равно «Услуги связи»"},
    }
    for _,tc:=range cases {
        cond:=FilterCondition{Kind:FilterText,Operator:tc.op,Value:tc.input}
        if err:=cond.Validate();err!=nil {t.Error(err);continue}
        filtered:=FilterConditionState{"name":cond}.Apply(rows)
        if got:=ids(filtered);!reflect.DeepEqual(got,tc.ids) {
            t.Errorf("operator %s got %v, want %v",tc.op,got,tc.ids)
        }
        if desc:=cond.Description();desc!=tc.description {
            t.Errorf("description %s = %q",tc.op,desc)
        }
    }
    blank:=FilterCondition{Kind:FilterText,Operator:FilterEqual,Value:""}
    if err:=blank.Validate();err!=nil {t.Fatal(err)}
    if blank.Match(rows[0],"Name") {t.Fatal("nonempty row matched blank")}
}

func TestNumericConditionsPreserveSQLMoneyPrecision(t *testing.T) {
    rows:=conditionalRows()
    cases:=[]struct{op FilterOperator; val,val2 string; want []int}{
        {FilterGreater,"10","",[]int{3,4}},
        {FilterGreaterEqual,"10","",[]int{3,4,5}},
        {FilterLessEqual,"8","",[]int{1,2}},
        {FilterEqual,"15,5000","",[]int{3}},
        {FilterNotEqual,"15.5000","",[]int{1,2,4,5}},
        {FilterBetween,"8","15,50",[]int{2,3,5}},
        {FilterNotBetween,"8","15,50",[]int{1,4}},
        {FilterTop,"2","",[]int{3,4}},
        {FilterAboveAverage,"","",[]int{3,4}},
        {FilterBelowAverage,"","",[]int{1,2,5}},
    }
    for _,tc:=range cases {
        cond:=FilterCondition{Kind:FilterNumber,Operator:tc.op,Value:tc.val,Value2:tc.val2}
        if err:=cond.Validate();err!=nil {t.Errorf("%s: %v",tc.op,err);continue}
        got:=ids(FilterConditionState{"amount":cond}.Apply(rows))
        if !reflect.DeepEqual(got,tc.want) {t.Errorf("%s %q got %v, want %v",tc.op,tc.val,got,tc.want)}
    }
    if _,err:=parseNumber("1 234,5678");err!=nil {t.Fatalf("localized SQL money parse: %v",err)}
    if _,err:=parseNumber("1"+string('\u00a0')+"234,56");err!=nil {t.Fatalf("N2 parse: %v",err)}
    for _,input:=range []string{"oops","","1,2,3"} {
        if err:=(FilterCondition{Kind:FilterNumber,Operator:FilterGreater,Value:input}).Validate();err==nil {
            t.Errorf("invalid number %q accepted",input)
        }
    }
    if err:=(FilterCondition{Kind:FilterNumber,Operator:FilterTop,Value:"0"}).Validate();err==nil {
        t.Fatal("Top N must require positive count")
    }
}

func TestDatesCompareCalendarDays(t *testing.T) {
    rows:=conditionalRows()
    rows[1].Raw["SignedAt"]=time.Date(2026,1,4,22,30,0,0,time.FixedZone("+05",5*3600))
    for _,tc:=range []struct{op FilterOperator; from,to string; want []int}{
        {FilterEqual,"04.01.2026","",[]int{2}},
        {FilterGreater,"01.02.2026","",[]int{3,4,5}},
        {FilterLessEqual,"10.02.2026","",[]int{1,2,3}},
        {FilterBetween,"04.01.2026","09.03.2026",[]int{2,3,4}},
        {FilterNotBetween,"04.01.2026","09.03.2026",[]int{1,5}},
    }{
        cond:=FilterCondition{Kind:FilterDate,Operator:tc.op,Value:tc.from,Value2:tc.to}
        if err:=cond.Validate();err!=nil {t.Fatal(err)}
        if got:=ids(FilterConditionState{"signedat":cond}.Apply(rows));!reflect.DeepEqual(got,tc.want){
            t.Errorf("%s got %v, want %v",tc.op,got,tc.want)
        }
    }
    if desc:= (FilterCondition{Kind:FilterDate,Operator:FilterLess,Value:"01.01.2026"}).Description();
        !strings.Contains(desc,"раньше") {t.Fatal(desc)}
    if _,err:=parseDateValue([]byte("2026-12-31"));err!=nil {t.Fatal(err)}
}

func TestMixingConditionsAndCheckboxFiltersAndFacets(t *testing.T) {
    rows:=conditionalRows()
    checklist:=ColumnFilterState{}
    conditions:=FilterConditionState{}
    checklist.Set("Department",map[string]bool{"A":true})
    if err:=conditions.Set("Amount",FilterCondition{Kind:FilterNumber,Operator:FilterGreater,Value:"7"});err!=nil{
        t.Fatal(err)
    }
    got:=ids(conditions.Apply(checklist.Apply(rows)))
    if !reflect.DeepEqual(got,[]int{2,3}) {t.Fatalf("checkbox + numeric condition AND: %v",got)}
    if count:=conditions.Count();count!=1 {t.Fatalf("condition count %d",count)}

    // The choice set for a different column is conditioned on both modes.
    otherCandidates:=checklist.ApplyExcept(rows,"Name")
    otherCandidates=conditions.ApplyExcept(otherCandidates,"Name")
    wantNames:=[]ColumnValueCount{{Value:"Договор оказания услуг",Count:1},{Value:"Услуги связи",Count:1}}
    if v:=ColumnValues(otherCandidates,"Name");!reflect.DeepEqual(v,wantNames) {
        t.Fatalf("faceted values from numeric condition and checkbox: %v",v)
    }
    // When editing Amount itself, its own predicate is excluded from facets
    // but the Department selection is retained.
    ownCandidates:=conditions.ApplyExcept(checklist.ApplyExcept(rows,"Amount"),"Amount")
    if len(ownCandidates)!=3 {t.Fatalf("own predicate should be excluded: %v",ids(ownCandidates))}

    conditions.Clear("Amount")
    if conditions.Active("Amount") {t.Fatal("condition did not clear")}
    if ids:=ids(checklist.Apply(rows));!reflect.DeepEqual(ids,[]int{1,2,3}) {
        t.Fatalf("clearing a single condition affected other columns: %v",ids)
    }
    conditions.Set("Name",FilterCondition{Kind:FilterText,Operator:FilterContains,Value:"услуг"})
    conditions.KeepFields([]ListColumn{{Field:"Amount"}})
    if conditions.Count()!=0 {t.Fatal("removed SQL alias retained condition")}
}


func TestTwoConditionsSameColumnUseORDifferentColumnsUseAND(t *testing.T) {
    rows:=conditionalRows()
    cases:=[]struct {
        name,field string
        first,second FilterCondition
        want []int
    }{
        {
            name:"Russian text contains OR exact match",field:"Name",
            first:FilterCondition{Kind:FilterText,Operator:FilterContains,Value:"ДОГОВОР"},
            second:FilterCondition{Kind:FilterText,Operator:FilterEqual,Value:"Подписан"},
            want:[]int{1,3,4},
        },
        {
            name:"numeric less OR greater",field:"Amount",
            first:FilterCondition{Kind:FilterNumber,Operator:FilterLess,Value:"8"},
            second:FilterCondition{Kind:FilterNumber,Operator:FilterGreater,Value:"20"},
            want:[]int{1,4},
        },
        {
            name:"numeric range OR equal",field:"Amount",
            first:FilterCondition{Kind:FilterNumber,Operator:FilterBetween,Value:"8",Value2:"10"},
            second:FilterCondition{Kind:FilterNumber,Operator:FilterEqual,Value:"30"},
            want:[]int{2,4,5},
        },
        {
            name:"date earlier OR later",field:"SignedAt",
            first:FilterCondition{Kind:FilterDate,Operator:FilterLess,Value:"04.01.2026"},
            second:FilterCondition{Kind:FilterDate,Operator:FilterGreater,Value:"09.03.2026"},
            want:[]int{1,5},
        },
        {
            name:"top N OR lower than threshold",field:"Amount",
            first:FilterCondition{Kind:FilterNumber,Operator:FilterTop,Value:"1"},
            second:FilterCondition{Kind:FilterNumber,Operator:FilterLess,Value:"6"},
            want:[]int{1,4},
        },
        {
            name:"above average OR low number",field:"Amount",
            first:FilterCondition{Kind:FilterNumber,Operator:FilterAboveAverage},
            second:FilterCondition{Kind:FilterNumber,Operator:FilterLess,Value:"8"},
            want:[]int{1,3,4},
        },
        {
            name:"average second branch",field:"Amount",
            first:FilterCondition{Kind:FilterNumber,Operator:FilterEqual,Value:"8"},
            second:FilterCondition{Kind:FilterNumber,Operator:FilterAboveAverage},
            want:[]int{2,3,4},
        },
    }
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T){
            first:=tc.first
            second:=tc.second
            first.Additional=&second
            filters:=FilterConditionState{}
            if err:=filters.Set(tc.field,first);err!=nil {t.Fatal(err)}
            if got:=ids(filters.Apply(rows));!reflect.DeepEqual(got,tc.want) {
                t.Fatalf("within-column OR: got %v, expected %v",got,tc.want)
            }
            stored,exists:=filters.Get(tc.field)
            if !exists || stored.Additional==nil {t.Fatal("second OR condition not persisted")}
            if !strings.Contains(stored.Description()," ИЛИ ") ||
                !strings.HasPrefix(stored.Description(),"(") {
                t.Fatalf("OR not represented in filter description: %q",stored.Description())
            }
            if err:=stored.Validate();err!=nil {t.Fatal(err)}
        })
    }

    conditions:=FilterConditionState{}
    first:=FilterCondition{Kind:FilterText,Operator:FilterContains,Value:"Договор"}
    second:=FilterCondition{Kind:FilterText,Operator:FilterEqual,Value:"Подписан"}
    first.Additional=&second
    if err:=conditions.Set("Name",first);err!=nil {t.Fatal(err)}
    // (Договор OR Подписан) AND Department=A => only the two contracts.
    checkboxes:=ColumnFilterState{}
    checkboxes.Set("Department",map[string]bool{"A":true})
    if got:=ids(conditions.Apply(checkboxes.Apply(rows)));!reflect.DeepEqual(got,[]int{1,3}) {
        t.Fatalf("(OR inside Name) AND (Department=A): %v",got)
    }

    // Another typed condition is also connected by AND, not by OR.
    amount:=FilterCondition{Kind:FilterNumber,Operator:FilterGreater,Value:"10"}
    if err:=conditions.Set("Amount",amount);err!=nil {t.Fatal(err)}
    if got:=ids(conditions.Apply(checkboxes.Apply(rows)));!reflect.DeepEqual(got,[]int{3}) {
        t.Fatalf("(Name condition 1 OR 2) AND Amount > 10 AND Department=A: %v",got)
    }
    if conditions.Count()!=2 {t.Fatalf("two columns expected, got %d",conditions.Count())}
    if conditions.ApplyExcept(rows,"Name")==nil {t.Fatal("apply-except unexpectedly nil")}
    // Removing only the Name condition must leave the amount requirement.
    conditions.Clear("Name")
    if conditions.Active("Name") || !conditions.Active("Amount") {
        t.Fatalf("wrong column filter cleared: %v",conditions)
    }
}

func TestOptionalSecondConditionValidationAndCloning(t *testing.T) {
    first:=FilterCondition{Kind:FilterText,Operator:FilterStarts,Value:"Договор"}
    if err:=first.Validate();err!=nil {t.Fatal("single branch should work",err)}
    if desc:=first.Description();strings.Contains(desc,"ИЛИ") {
        t.Fatalf("single branch shown as pair: %s",desc)
    }
    badNumber:=FilterCondition{Kind:FilterNumber,Operator:FilterGreater,Value:"не число"}
    first.Additional=&badNumber
    if err:=first.Validate();err==nil {
        t.Fatal("a second condition of a different type must fail")
    }
    first.Kind=FilterNumber
    first.Operator=FilterGreater
    first.Value="10"
    if err:=first.Validate();err==nil {
        t.Fatal("invalid numeric input for OR branch was accepted")
    }
    second:=FilterCondition{Kind:FilterNumber,Operator:FilterLess,Value:"20"}
    first.Additional=&second
    if err:=first.Validate();err!=nil {t.Fatal(err)}
    filters:=FilterConditionState{}
    if err:=filters.Set("Amount",first);err!=nil {t.Fatal(err)}
    // The stored second condition must not change when the calling code
    // subsequently edits its temporary UI values.
    second.Value="1000"
    stored,_:=filters.Get("Amount")
    if stored.Additional==nil || stored.Additional.Value!="20" {
        t.Fatalf("stored condition was mutated through shared pointer: %+v",stored)
    }
    third:=FilterCondition{Kind:FilterNumber,Operator:FilterEqual,Value:"15"}
    second.Additional=&third
    first.Additional=&second
    if err:=first.Validate();err==nil {t.Fatal("three levels of OR must be rejected")}
    first.Additional=nil
    if err:=first.Validate();err!=nil {t.Fatal("one branch should still validate",err)}
}

func TestTwoConditionsWithMixedTextOperators(t *testing.T) {
    condition:=FilterCondition{
        Kind:FilterText,Operator:FilterContains,Value:"булычев",
        Additional:&FilterCondition{
            Kind:FilterText,Operator:FilterEnds,Value:"Петров",
        },
    }
    description:=condition.Description()
    want:="(содержит «булычев» ИЛИ заканчивается на «Петров»)"
    if description!=want {
        t.Fatalf("user-facing filter expression = %q, want %q",description,want)
    }
    examples:=[]struct{value string;want bool}{
        {"Кир Булычев",true},
        {"Иван Петров",true},
        {"Алексей Сидоров",false},
        {"булычев-Петров",true},
    }
    for _,example:=range examples {
        row:=ListItem{Values:map[string]string{"Договор":example.value}}
        if got:=condition.Match(row,"Договор");got!=example.want {
            t.Errorf("OR text match %q = %v, want %v",example.value,got,example.want)
        }
    }
}
