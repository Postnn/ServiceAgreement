package presentation

import (
	"reflect"
	"testing"

	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

func presentationRows(sqlType string) []readmodel.ListItem {
	vals := []string{"0.0000", "2222.0000", "4520.4600", "99.0000", "500.0000", "0.0000", "1150.2400"}
	rows := make([]readmodel.ListItem, 0, len(vals))
	for i, raw := range vals {
		rows = append(rows, readmodel.ListItem{
			ID:          i + 1,
			Values:      map[string]string{"Абон.плата": raw, "DocumentCode": "0001"},
			Raw:         map[string]any{"Абон.плата": []byte(raw), "DocumentCode": []byte("0001")},
			ColumnTypes: map[string]string{"Абон.плата": sqlType, "DocumentCode": "NVARCHAR"},
		})
	}
	return rows
}

func TestMoneyFormattingRequiresSQLMoneyType(t *testing.T) {
	for _, field := range []string{"Абон.плата", "Amount", "Random"} {
		for _, dbType := range []string{"NVARCHAR", "VARCHAR", "DECIMAL", "NUMERIC", "FLOAT", "SMALLMONEY"} {
			rows := []readmodel.ListItem{{
				Values:      map[string]string{field: "2222.0000"},
				Raw:         map[string]any{field: []byte("2222.0000")},
				ColumnTypes: map[string]string{field: dbType},
			}}
			p := InferColumnPresentation(rows, field)
			if p.Money {
				t.Fatalf("%s with %s must not get MONEY N2: %+v", field, dbType, p)
			}
			if dbType == "NVARCHAR" || dbType == "VARCHAR" {
				if p.Kind != FilterText {
					t.Errorf("%s/%s must be a textual filter, got %q", field, dbType, p.Kind)
				}
			} else if p.Kind != FilterNumber {
				t.Errorf("%s/%s is SQL numeric but filter kind %q", field, dbType, p.Kind)
			}
			FormatListMoneyN2(rows)
			if got := rows[0].Field(field); got != "2222.0000" {
				t.Fatalf("%s / %s was wrongly reformatted to %q", field, dbType, got)
			}
		}
	}
}

func TestSQLMoneyFormattingIsIndependentOfAlias(t *testing.T) {
	for _, alias := range []string{"Абон.плата", "Amount", "Random"} {
		rows := []readmodel.ListItem{{
			Values:      map[string]string{alias: "2222.0000"},
			Raw:         map[string]any{alias: []byte("2222.0000")},
			ColumnTypes: map[string]string{alias: "MONEY"},
		}}
		p := InferColumnPresentation(rows, alias)
		if p.Kind != FilterNumber || !p.Money {
			t.Fatalf("MONEY alias %s detected as %+v", alias, p)
		}
		FormatListMoneyN2(rows)
		if got := rows[0].Field(alias); got != "2\u00a0222,00" {
			t.Fatalf("MONEY alias %s didn't get N2: %q", alias, got)
		}
		if raw := SQLText(rows[0].SQLValue(alias)); raw != "2222.0000" {
			t.Fatalf("display formatting modified raw SQL value: %q", raw)
		}
	}
}

func TestMoneyInDistinctValuesOnlyWhenSQLTypeIsMoney(t *testing.T) {
	castRows := presentationRows("NVARCHAR")
	moneyRows := presentationRows("MONEY")
	if kind := DetectFilterKind(castRows, "Абон.плата"); kind != FilterText {
		t.Fatalf("casted money must be textual when SQL returns NVARCHAR: %s", kind)
	}
	if kind := DetectFilterKind(moneyRows, "Абон.плата"); kind != FilterNumber {
		t.Fatalf("MONEY must be numeric: %s", kind)
	}
	FormatListMoneyN2(castRows)
	FormatListMoneyN2(moneyRows)
	castGot := []string{}
	moneyGot := []string{}
	for i := range castRows {
		castGot = append(castGot, castRows[i].Field("Абон.плата"))
		moneyGot = append(moneyGot, moneyRows[i].Field("Абон.плата"))
		if castRows[i].Field("Абон.плата") != SQLText(castRows[i].SQLValue("Абон.плата")) {
			t.Fatal("text column was modified")
		}
	}
	moneyWant := []string{"0,00", "2\u00a0222,00", "4\u00a0520,46", "99,00", "500,00", "0,00", "1\u00a0150,24"}
	if !reflect.DeepEqual(moneyGot, moneyWant) {
		t.Fatalf("MONEY N2: %v expected %v", moneyGot, moneyWant)
	}
	if !reflect.DeepEqual(castGot, []string{"0.0000", "2222.0000", "4520.4600", "99.0000", "500.0000", "0.0000", "1150.2400"}) {
		t.Fatalf("text values were modified: %v", castGot)
	}
	unique := ColumnValues(moneyRows, "Абон.плата")
	counts := map[string]int{}
	for _, v := range unique {
		counts[v.Value] = v.Count
	}
	if counts["0,00"] != 2 || counts["2\u00a0222,00"] != 1 {
		t.Fatalf("N2 distinct values: %v", unique)
	}
	uniqueText := ColumnValues(castRows, "Абон.плата")
	counts = map[string]int{}
	for _, v := range uniqueText {
		counts[v.Value] = v.Count
	}
	if counts["0.0000"] != 2 || counts["2222.0000"] != 1 {
		t.Fatalf("text distinct values modified: %v", uniqueText)
	}
}

func TestEmptySQLMoneyAndTextStayDistinct(t *testing.T) {
	for _, field := range []string{"Amount", "Абон.плата", "СуммаАбонентскойПлаты"} {
		for _, dbType := range []string{"NVARCHAR", "MONEY"} {
			rows := []readmodel.ListItem{{ColumnTypes: map[string]string{field: dbType},
				Values: map[string]string{field: ""}, Raw: map[string]any{field: nil}}}
			p := InferColumnPresentation(rows, field)
			expectedMoney := dbType == "MONEY"
			if p.Money != expectedMoney {
				t.Fatalf("empty %s/%s: %+v", field, dbType, p)
			}
			expectedKind := FilterText
			if expectedMoney {
				expectedKind = FilterNumber
			}
			if p.Kind != expectedKind {
				t.Fatalf("empty %s/%s wrong kind: %+v", field, dbType, p)
			}
		}
	}
}

func TestTypedSQLDateNumericAndTextClassification(t *testing.T) {
	cases := []struct {
		typ   string
		kind  FilterKind
		money bool
	}{
		{"MONEY", FilterNumber, true},
		{"SMALLMONEY", FilterNumber, false},
		{"DECIMAL", FilterNumber, false},
		{"NUMERIC", FilterNumber, false},
		{"INT", FilterNumber, false},
		{"FLOAT", FilterNumber, false},
		{"DATE", FilterDate, false},
		{"DATETIME2", FilterDate, false},
		{"NVARCHAR", FilterText, false},
		{"VARCHAR", FilterText, false},
	}
	for _, tc := range cases {
		r := []readmodel.ListItem{{ColumnTypes: map[string]string{"X": tc.typ},
			Values: map[string]string{"X": "1234.0000"},
			Raw:    map[string]any{"X": []byte("1234.0000")}}}
		p := InferColumnPresentation(r, "X")
		if p.Kind != tc.kind || p.Money != tc.money {
			t.Errorf("%s: %+v expected %+v", tc.typ, p, tc)
		}
	}
}
