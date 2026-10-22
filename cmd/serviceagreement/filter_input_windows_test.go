//go:build windows

package main

import (
"strings"
"testing"
"time"

"github.com/Postnn/ServiceAgreement/internal/presentation"
"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

func TestNativeFilterInputCaptionsAndRangeSemantics(t *testing.T) {
	cases := []struct {
		kind          presentation.FilterKind
		op            presentation.FilterOperator
		first, second string
	}{
		{presentation.FilterDate, presentation.FilterBetween, "Дата начала:", "Дата окончания:"},
		{presentation.FilterDate, presentation.FilterNotBetween, "Дата начала:", "Дата окончания:"},
		{presentation.FilterNumber, presentation.FilterBetween, "От:", "До:"},
		{presentation.FilterNumber, presentation.FilterNotBetween, "От:", "До:"},
		{presentation.FilterDate, presentation.FilterEqual, "Дата:", ""},
		{presentation.FilterText, presentation.FilterContains, "Текст:", ""},
		{presentation.FilterNumber, presentation.FilterGreater, "Число:", ""},
		{presentation.FilterNumber, presentation.FilterAboveAverage, "", ""},
		{presentation.FilterNumber, presentation.FilterTop, "Количество:", ""},
	}
	for _, tc := range cases {
		first, second := filterOperandCaptions(tc.kind, tc.op)
		if first != tc.first || second != tc.second {
			t.Errorf("%s/%s: got %q,%q want %q,%q", tc.kind, tc.op, first, second, tc.first, tc.second)
		}
	}
	hints := []struct {
		op     presentation.FilterOperator
		expect []string
	}{
		{presentation.FilterBetween, []string{"с начала по окончание периода", "Обе даты включены"}},
		{presentation.FilterNotBetween, []string{"до начала периода", "после его окончания", "не входят"}},
	}
	for _, tc := range hints {
		text := filterRangeExplanation(presentation.FilterDate, tc.op, "")
		for _, fragment := range tc.expect {
			if !strings.Contains(text, fragment) {
				t.Errorf("missing %q in %q", fragment, text)
			}
		}
	}
	if msg := filterRangeExplanation(presentation.FilterText, presentation.FilterContains, presentation.FilterStarts); !strings.Contains(msg, "ИЛИ") || !strings.Contains(msg, "И") {
		t.Fatal(msg)
	}
}

func TestNativeDateFilterEditorCalendarRoundTrip(t *testing.T) {
	original := "01.02.2026"
	date := filterDateForEditor(original)
	if date.IsZero() {
		t.Fatal("DD.MM.YYYY was not parsed for native calendar")
	}
	if date.Year() != 2026 || date.Month() != time.February || date.Day() != 1 {
		t.Fatalf("wrong initial DateEdit date: %v", date)
	}
	selected, err := filterEditorOperand(presentation.FilterDate, presentation.FilterBetween, filterEditorValue{Date: date})
	if err != nil || selected != original {
		t.Fatalf("native calendar value lost: %q %v", selected, err)
	}
	notBetween := presentation.FilterCondition{
		Kind: presentation.FilterDate, Operator: presentation.FilterNotBetween,
		Value: selected, Value2: "03.02.2026",
	}
	for _, tc := range []struct {
		raw     string
		outside bool
	}{
		{"31.01.2026", true},
		{"01.02.2026", false}, // endpoints are NOT outside
		{"02.02.2026", false},
		{"03.02.2026", false},
		{"04.02.2026", true},
	} {
		row := readmodel.ListItem{Raw: map[string]any{"Date": filterDateForEditor(tc.raw)}}
		if got := notBetween.Match(row, "Date"); got != tc.outside {
			t.Errorf("%s: outside=%v want %v", tc.raw, got, tc.outside)
		}
	}
	if _, err := filterEditorOperand(presentation.FilterDate, presentation.FilterEqual,
		filterEditorValue{Date: time.Time{}}); err == nil {
		t.Fatal("unselected date in native DateEdit accepted as a real date")
	}
	if value := filterDateForEditor("01022026"); !value.IsZero() {
		t.Fatalf("invalid hand-typed date should require calendar selection: %v", value)
	}
}

func TestNativeNumberEditValueExtraction(t *testing.T) {
	n := filterNumberForEditor("1\u00a0234,5678")
	if n < 1234.567799 || n > 1234.567801 {
		t.Fatalf("N2 value initialization: %f", n)
	}
	for _, tc := range []struct {
		op    presentation.FilterOperator
		state filterEditorValue
		want  string
	}{
		{presentation.FilterGreater, filterEditorValue{Number: 2.5, NumberEdited: true}, "2.5000"},
		{presentation.FilterBetween, filterEditorValue{Number: 30, NumberEdited: true}, "30.0000"},
		{presentation.FilterTop, filterEditorValue{Number: 10, NumberEdited: true}, "10"},
		{presentation.FilterGreater, filterEditorValue{Number: 10, Original: "10.0001", NumberEdited: false}, "10.0001"},
	} {
		v, err := filterEditorOperand(presentation.FilterNumber, tc.op, tc.state)
		if err != nil || v != tc.want {
			t.Errorf("%s numeric value got %q, %v want %q", tc.op, v, err, tc.want)
		}
	}
	if _, err := filterEditorOperand(presentation.FilterNumber, presentation.FilterGreater,
		filterEditorValue{Number: 1e13, NumberEdited: true}); err == nil {
		t.Fatal("unsafe float64 numeric precision was silently accepted")
	}
	if v, err := filterEditorOperand(presentation.FilterNumber, presentation.FilterAboveAverage,
		filterEditorValue{}); err != nil || v != "" {
		t.Fatalf("average should not read any input: %q %v", v, err)
	}
	if filterNeedsBoundary(presentation.FilterGreater) ||
		!filterNeedsBoundary(presentation.FilterBetween) ||
		!filterNeedsBoundary(presentation.FilterNotBetween) {
		t.Fatal("number of native operand controls mismatched operator")
	}
}

func TestEveryVisibleOperandCaptionEndsWithColon(t *testing.T) {
	cases := []struct {
		kind presentation.FilterKind
		ops  []presentation.FilterOperator
	}{
		{presentation.FilterDate, []presentation.FilterOperator{
			presentation.FilterEqual, presentation.FilterLess, presentation.FilterGreater,
			presentation.FilterBetween, presentation.FilterNotBetween,
		}},
		{presentation.FilterNumber, []presentation.FilterOperator{
			presentation.FilterEqual, presentation.FilterTop, presentation.FilterBetween, presentation.FilterNotBetween,
		}},
		{presentation.FilterText, []presentation.FilterOperator{
			presentation.FilterEqual, presentation.FilterContains, presentation.FilterStarts,
		}},
	}
	for _, tc := range cases {
		for _, op := range tc.ops {
			first, second := filterOperandCaptions(tc.kind, op)
			if first != "" && !strings.HasSuffix(first, ":") {
				t.Errorf("%v %v first caption lacks colon: %q", tc.kind, op, first)
			}
			if second != "" && !strings.HasSuffix(second, ":") {
				t.Errorf("%v %v second caption lacks colon: %q", tc.kind, op, second)
			}
		}
	}
}
