package core

import (
    "reflect"
    "strings"
    "testing"
)

func monetarySampleRows() []ListItem {
    originals:=[]string{"0.0000","2222.0000","4520.4600","99.0000","500.0000","0.0000","1150.2400"}
    rows:=make([]ListItem,0,len(originals))
    for i,v:=range originals {
        rows=append(rows,ListItem{
            ID:i+1,
            Values:map[string]string{
                "Абон.плата":v,"DocumentCode":"0001",
            },
            Raw:map[string]any{
                "Абон.плата":[]byte(v),"DocumentCode":[]byte("0001"),
            },
            ColumnTypes:map[string]string{
                "Абон.плата":"NVARCHAR",
                "DocumentCode":"NVARCHAR",
            },
        })
    }
    return rows
}

func TestCastedMoneyTypeDetectedFromNumericValuesAndRussianAlias(t *testing.T) {
    rows:=monetarySampleRows()
    p:=InferColumnPresentation(rows,"абон.ПЛАТА")
    if p.Kind!=FilterNumber || !p.Money {
        t.Fatalf("Abon.plata was treated as text: %+v",p)
    }
    if got:=DetectFilterKind(rows,"Абон.плата");got!=FilterNumber {
        t.Fatalf("Filter must expose numeric operators: %s",got)
    }
    if other:=InferColumnPresentation(rows,"DocumentCode");other.Kind!=FilterText || other.Money {
        t.Fatalf("varchar document code must remain text (including leading zeros): %+v",other)
    }

    FormatListMoneyN2(rows)
    want:=[]string{
        "0,00","2\u00a0222,00","4\u00a0520,46","99,00",
        "500,00","0,00","1\u00a0150,24",
    }
    actual:=make([]string,len(rows))
    for i,r:=range rows {
        actual[i]=r.Field("АБОН.ПЛАТА")
        if r.Field("DocumentCode")!="0001" {
            t.Fatalf("text columns must not be mutated: %v",r.Values)
        }
        original:=strings.TrimSpace(sqlText(r.SQLValue("Абон.плата")))
        if original!=strings.TrimSpace(string(r.Raw["Абон.плата"].([]byte))) {
            t.Fatalf("Money values must remain unrounded for numeric sorting: %q",original)
        }
    }
    if !reflect.DeepEqual(actual,want) {
        t.Fatalf("N2 amounts as shown in screenshot: got %q, want %q",actual,want)
    }
    if err:=ValidateColumnPresentation(rows,"Абон.плата");err!=nil {t.Fatal(err)}

    // The distinct-values popup must use exactly the same displayed N2
    // strings as the main table (including grouping equal amounts).
    choices:=ColumnValues(rows,"Абон.плата")
    counts:=map[string]int{}
    for _,c:=range choices { counts[c.Value]=c.Count }
    if counts["0,00"]!=2 ||
        counts["2\u00a0222,00"]!=1 ||
        counts["1\u00a0150,24"]!=1 {
        t.Fatalf("N2 checklist values or counts are inconsistent: %+v",choices)
    }

    cond:=FilterCondition{Kind:FilterNumber,Operator:FilterGreater,Value:"999"}
    filtered:=FilterConditionState{"абон.плата":cond}.Apply(rows)
    if got:=ids(filtered);!reflect.DeepEqual(got,[]int{2,3,7}) {
        t.Fatalf("typed filter must compare unrounded raw values: %v",got)
    }
}

func TestMoneyInferenceUsesSQLTypeAndNumericEvidence(t *testing.T) {
    cases:=[]struct{
        name,alias,sqlType,stringValue string
        expected FilterKind
        money bool
    }{
        {"native-money","Cost","MONEY","4567.2500",FilterNumber,true},
        {"sql-smallmoney","Fee","SMALLMONEY","42.0000",FilterNumber,true},
        {"cast-as-text","Абон.плата","NVARCHAR","1150.2400",FilterNumber,true},
        {"different-alias-cast","PaidValue","NVARCHAR","0.0000",FilterNumber,true},
        {"native-int","ServicePeriod","INT","12",FilterNumber,false},
        {"native-date","SignedAt","DATE","2026-10-21",FilterDate,false},
        {"date-as-varchar","SignedAt","NVARCHAR","21.10.2026",FilterText,false},
        {"plain-numeric-string","DocumentCode","NVARCHAR","0012",FilterText,false},
        {"mixed-text","Абон.плата","NVARCHAR","Не определено",FilterText,false},
        {"non-financial-description","PaymentText","NVARCHAR","Оплата: 120",FilterText,false},
    }
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T){
            row:=ListItem{
                Values:map[string]string{tc.alias:tc.stringValue},
                Raw:map[string]any{tc.alias:[]byte(tc.stringValue)},
                ColumnTypes:map[string]string{tc.alias:tc.sqlType},
            }
            value:=InferColumnPresentation([]ListItem{row},tc.alias)
            if value.Kind!=tc.expected || value.Money!=tc.money {
                t.Fatalf("%s / %q => %+v, want %s money=%v",
                    tc.sqlType,tc.stringValue,value,tc.expected,tc.money)
            }
        })
    }
}

func TestN2FormattingOnlyWhenAllCastedValuesAreNumeric(t *testing.T) {
    rows:=[]ListItem{
        {Values:map[string]string{"Абон.плата":"100.0000"},
            Raw:map[string]any{"Абон.плата":[]byte("100.0000")},
            ColumnTypes:map[string]string{"Абон.плата":"NVARCHAR"}},
        {Values:map[string]string{"Абон.плата":"Ошибка"},
            Raw:map[string]any{"Абон.плата":[]byte("Ошибка")},
            ColumnTypes:map[string]string{"Абон.плата":"NVARCHAR"}},
    }
    if k:=InferColumnPresentation(rows,"Абон.плата");k.Kind!=FilterText || k.Money {
        t.Fatalf("mixed text column should remain text: %+v",k)
    }
    FormatListMoneyN2(rows)
    if rows[0].Field("Абон.плата")!="100.0000" ||
        rows[1].Field("Абон.плата")!="Ошибка" {
        t.Fatal("mixed text column was partially formatted")
    }
}


func TestEmptyCastedMoneyColumnKeepsNumericFilterType(t *testing.T) {
    for _,field:=range []string{"Amount","Абон.плата","СуммаАбонентскойПлаты"} {
        rows:=[]ListItem{{
            ColumnTypes:map[string]string{field:"NVARCHAR"},
            Values:map[string]string{field:""},
            Raw:map[string]any{field:nil},
        }}
        kind:=InferColumnPresentation(rows,field)
        if !kind.Money || kind.Kind!=FilterNumber {
            t.Errorf("empty monetary alias %q should still expose numeric filters: %+v",field,kind)
        }
    }
    blank:=[]ListItem{{
        ColumnTypes:map[string]string{"DocumentCode":"NVARCHAR"},
        Raw:map[string]any{"DocumentCode":nil},
    }}
    if k:=InferColumnPresentation(blank,"DocumentCode");k.Kind!=FilterText {
        t.Fatalf("empty non-money varchar should remain text: %+v",k)
    }
}
