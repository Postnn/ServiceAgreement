// Package contracts_test runs language-neutral examples that can also be used
// by a future C# implementation with ordinary ClayGrid and fixed SQL.
package contracts_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Postnn/ServiceAgreement/internal/domain"
	"github.com/Postnn/ServiceAgreement/internal/presentation"
	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

func load(t *testing.T, name string, target any) {
	t.Helper()
	b, err := os.ReadFile("../../testdata/contracts/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, target); err != nil {
		t.Fatal(err)
	}
}
func TestPortableXDD(t *testing.T) {
	var cases struct {
		Version int
		Dates   []struct {
			Name, Start, Date string
			Code              int
			Error             bool
		}
		Codes []struct {
			Code        int
			Before      bool
			Months, Day int
			Error       bool
		}
	}
	load(t, "xdd", &cases)
	if cases.Version != 1 || len(cases.Dates) == 0 || len(cases.Codes) == 0 {
		t.Fatal("unsupported or empty fixture")
	}
	for _, c := range cases.Dates {
		t.Run(c.Name, func(t *testing.T) {
			start, err := time.Parse("2006-01-02", c.Start)
			if err != nil {
				t.Fatal(err)
			}
			got, err := domain.ResolveXDD(start, c.Code)
			if (err != nil) != c.Error {
				t.Fatalf("date=%v err=%v", got, err)
			}
			if !c.Error && got.Format("2006-01-02") != c.Date {
				t.Fatalf("got %s want %s", got.Format("2006-01-02"), c.Date)
			}
		})
	}
	for _, c := range cases.Codes {
		got, err := domain.ParseXDD(c.Code)
		if (err != nil) != c.Error {
			t.Fatalf("code %d: %v", c.Code, err)
		}
		if !c.Error {
			if got.Before != c.Before || got.Months != c.Months || got.Day != c.Day {
				t.Fatalf("code %d: %+v", c.Code, got)
			}
			back, err := got.Code()
			if err != nil || back != c.Code {
				t.Fatalf("round trip %d: %d %v", c.Code, back, err)
			}
		}
	}
}
func TestPortableMoney(t *testing.T) {
	var cases struct {
		Version   int
		Normalize []struct {
			Input, Value string
			Error        bool
		}
		Display []struct {
			Type       string
			Input      *string
			Text, Kind string
			Money      bool
		}
		Editor []struct {
			Input, Displayed, Original, Value string
			Error                             bool
		}
	}
	load(t, "money", &cases)
	if cases.Version != 1 || len(cases.Normalize) == 0 || len(cases.Display) == 0 || len(cases.Editor) == 0 {
		t.Fatal("unsupported or empty fixture")
	}
	for _, c := range cases.Normalize {
		got, err := domain.NormalizeMoney(c.Input)
		if (err != nil) != c.Error || !c.Error && got != c.Value {
			t.Errorf("normalize %q: %q %v", c.Input, got, err)
		}
	}
	for _, c := range cases.Display {
		var value any
		if c.Input != nil {
			value = *c.Input
		}
		if got := presentation.SQLDisplayValue(value, c.Type, "arbitrary-alias"); got != c.Text {
			t.Errorf("display %s %v: %q want %q", c.Type, value, got, c.Text)
		}
		// Classification must also work without data rows.
		result := readmodel.ListResult{Columns: []readmodel.ColumnMetadata{{Name: "x", DatabaseType: c.Type}}}
		got := presentation.ColumnPresentationFor(result, "x")
		if string(got.Kind) != c.Kind || got.Money != c.Money {
			t.Errorf("type %s: %+v", c.Type, got)
		}
	}
	for _, c := range cases.Editor {
		got, err := presentation.MoneyValueFromEditor(c.Input, c.Displayed, c.Original)
		if (err != nil) != c.Error || !c.Error && got != c.Value {
			t.Errorf("editor %q: %q %v", c.Input, got, err)
		}
	}
}
