//go:build windows

package main

import (
    "strings"
    "testing"
    "time"

    "github.com/Postnn/ServiceAgreement/internal/core"
)

func TestNativeFilterInputCaptionsAndRangeSemantics(t *testing.T) {
    cases:=[]struct {
        kind core.FilterKind
        op core.FilterOperator
        first,second string
    }{
        {core.FilterDate,core.FilterBetween,"≥ От","≤ По"},
        {core.FilterDate,core.FilterNotBetween,"< От","> По"},
        {core.FilterNumber,core.FilterBetween,"≥ От","≤ По"},
        {core.FilterNumber,core.FilterNotBetween,"< От","> По"},
        {core.FilterDate,core.FilterEqual,"Дата",""},
        {core.FilterText,core.FilterContains,"Текст",""},
        {core.FilterNumber,core.FilterGreater,"Число",""},
        {core.FilterNumber,core.FilterAboveAverage,"",""},
        {core.FilterNumber,core.FilterTop,"Кол-во",""},
    }
    for _,tc:=range cases {
        first,second:=filterOperandCaptions(tc.kind,tc.op)
        if first!=tc.first || second!=tc.second {
            t.Errorf("%s/%s: got %q,%q want %q,%q",tc.kind,tc.op,first,second,tc.first,tc.second)
        }
    }
    hints:=[]struct{op core.FilterOperator;expect []string}{
        {core.FilterBetween,[]string{"Больше или равно От","Меньше или равно По"}},
        {core.FilterNotBetween,[]string{"Меньше От","Больше По"}},
    }
    for _,tc:=range hints {
        text:=filterRangeExplanation(core.FilterDate,tc.op,"")
        for _,fragment:=range tc.expect {
            if !strings.Contains(text,fragment) {
                t.Errorf("missing %q in %q",fragment,text)
            }
        }
    }
    if msg:=filterRangeExplanation(core.FilterText,core.FilterContains,core.FilterStarts);
        !strings.Contains(msg,"ИЛИ")||!strings.Contains(msg,"И") {
        t.Fatal(msg)
    }
}

func TestNativeDateFilterEditorCalendarRoundTrip(t *testing.T) {
    original:="01.02.2026"
    date:=filterDateForEditor(original)
    if date.IsZero() {t.Fatal("DD.MM.YYYY was not parsed for native calendar")}
    if date.Year()!=2026 || date.Month()!=time.February || date.Day()!=1 {
        t.Fatalf("wrong initial DateEdit date: %v",date)
    }
    selected,err:=filterEditorOperand(core.FilterDate,core.FilterBetween,filterEditorValue{Date:date})
    if err!=nil || selected!=original {
        t.Fatalf("native calendar value lost: %q %v",selected,err)
    }
    notBetween:=core.FilterCondition{
        Kind:core.FilterDate,Operator:core.FilterNotBetween,
        Value:selected,Value2:"03.02.2026",
    }
    for _,tc:=range []struct{raw string;outside bool}{
        {"31.01.2026",true},
        {"01.02.2026",false}, // endpoints are NOT outside
        {"02.02.2026",false},
        {"03.02.2026",false},
        {"04.02.2026",true},
    }{
        row:=core.ListItem{Raw:map[string]any{"Date":filterDateForEditor(tc.raw)}}
        if got:=notBetween.Match(row,"Date");got!=tc.outside {
            t.Errorf("%s: outside=%v want %v",tc.raw,got,tc.outside)
        }
    }
    if _,err:=filterEditorOperand(core.FilterDate,core.FilterEqual,
        filterEditorValue{Date:time.Time{}});err==nil {
        t.Fatal("unselected date in native DateEdit accepted as a real date")
    }
    if value:=filterDateForEditor("01022026");!value.IsZero() {
        t.Fatalf("invalid hand-typed date should require calendar selection: %v",value)
    }
}

func TestNativeNumberEditValueExtraction(t *testing.T) {
    n:=filterNumberForEditor("1\u00a0234,5678")
    if n<1234.567799 || n>1234.567801 {
        t.Fatalf("N2 value initialization: %f",n)
    }
    for _,tc:=range []struct{
        op core.FilterOperator
        state filterEditorValue
        want string
    }{
        {core.FilterGreater,filterEditorValue{Number:2.5,NumberEdited:true},"2.5000"},
        {core.FilterBetween,filterEditorValue{Number:30,NumberEdited:true},"30.0000"},
        {core.FilterTop,filterEditorValue{Number:10,NumberEdited:true},"10"},
        {core.FilterGreater,filterEditorValue{Number:10,Original:"10.0001",NumberEdited:false},"10.0001"},
    } {
        v,err:=filterEditorOperand(core.FilterNumber,tc.op,tc.state)
        if err!=nil || v!=tc.want {
            t.Errorf("%s numeric value got %q, %v want %q",tc.op,v,err,tc.want)
        }
    }
    if _,err:=filterEditorOperand(core.FilterNumber,core.FilterGreater,
        filterEditorValue{Number:1e13,NumberEdited:true});err==nil {
        t.Fatal("unsafe float64 numeric precision was silently accepted")
    }
    if v,err:=filterEditorOperand(core.FilterNumber,core.FilterAboveAverage,
        filterEditorValue{});err!=nil||v!="" {
        t.Fatalf("average should not read any input: %q %v",v,err)
    }
    if filterNeedsBoundary(core.FilterGreater) ||
        !filterNeedsBoundary(core.FilterBetween) ||
        !filterNeedsBoundary(core.FilterNotBetween) {
        t.Fatal("number of native operand controls mismatched operator")
    }
}
