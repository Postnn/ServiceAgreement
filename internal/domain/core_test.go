package domain_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Postnn/ServiceAgreement/internal/config"
	"github.com/Postnn/ServiceAgreement/internal/domain"
	"github.com/Postnn/ServiceAgreement/internal/presentation"
)

func TestNormalizeMoney(t *testing.T) {
	cases := []struct {
		in, want string
		valid    bool
	}{
		{"0", "0.0000", true}, {"12,3", "12.3000", true},
		{" 0012.003 ", "12.0030", true},
		{"922337203685477.5807", "922337203685477.5807", true},
		{"922337203685477.5808", "", false},
		{"-922337203685477.5808", "-922337203685477.5808", true},
		{"-922337203685477.5809", "", false},
		{"0.00001", "", false}, {"-1", "-1.0000", true}, {"1 000", "", false},
		{"1,234.55", "", false}, {"", "", false},
	}
	for _, tt := range cases {
		t.Run(tt.in, func(t *testing.T) {
			got, err := domain.NormalizeMoney(tt.in)
			if (err == nil) != tt.valid || got != tt.want {
				t.Fatalf("got %q err %v", got, err)
			}
		})
	}
}

func TestXDD(t *testing.T) {
	// X can be negative, zero or positive for all three scheduling rules.
	valid := []int{9, -6, 99, -99, 120, -120, 306, 320, 420,
		199, -199, 399, -399, 1299, -1299, 2147483599, -2147483599}
	for _, kind := range []string{"документы", "оплата", "отключение"} {
		for _, code := range valid {
			if err := domain.ValidateXDD(code, kind); err != nil {
				t.Errorf("%s %d unexpectedly invalid: %v", kind, code, err)
			}
		}
		for _, code := range []int{0, 100, 132, 198, -100, 2147483647, -2147483648} {
			if err := domain.ValidateXDD(code, kind); err == nil {
				t.Errorf("%s %d unexpectedly accepted", kind, code)
			}
		}
	}
	if err := domain.ValidateXDD(199, "другой"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

func TestXDDPartsAndMonthEnd(t *testing.T) {
	cases := []struct {
		c    domain.XDD
		code int
	}{
		{domain.XDD{Day: 9}, 9},
		{domain.XDD{Before: true, Day: 6}, -6},
		{domain.XDD{Months: 1, Day: 20}, 120},
		{domain.XDD{Before: true, Months: 1, Day: 20}, -120},
		{domain.XDD{Months: 3, Day: 6}, 306},
		{domain.XDD{Months: 3, Day: 20}, 320},
		{domain.XDD{Months: 4, Day: 20}, 420},
		{domain.XDD{Day: 99}, 99},
		{domain.XDD{Before: true, Day: 99}, -99},
		{domain.XDD{Months: 3, Day: 99}, 399},
		{domain.XDD{Before: true, Months: 3, Day: 99}, -399},
	}
	for _, tc := range cases {
		got, err := tc.c.Code()
		if err != nil || got != tc.code {
			t.Errorf("%+v: code=%d err=%v", tc.c, got, err)
		}
		back, err := domain.ParseXDD(got)
		if err != nil || back != tc.c {
			t.Errorf("round-trip %d got=%+v err=%v", got, back, err)
		}
	}
	for _, bad := range []domain.XDD{{Months: -1, Day: 9}, {Months: 21474837, Day: 9}, {Months: 2, Day: 32}, {Day: 0}, {Day: 98}} {
		if code, err := bad.Code(); err == nil {
			t.Errorf("bad code %+v -> %d", bad, code)
		}
	}
}

func TestResolvedExamples(t *testing.T) {
	// A service starting on 07.01, lasting 3 months, ends on 06.04.
	// 306 means the first 6th after crossing the 07.04 threshold: 06.05.
	start := time.Date(2026, time.January, 7, 0, 0, 0, 0, time.UTC)
	cases := map[int]string{
		9: "2026-01-09", 120: "2026-02-20", -120: "2025-12-20",
		-6: "2026-01-06", 306: "2026-05-06", 320: "2026-04-20", 420: "2026-05-20",
		99: "2026-01-31", -99: "2025-12-31", 199: "2026-02-28",
		399: "2026-04-30", -199: "2025-12-31", -106: "2026-01-06",
	}
	for code, want := range cases {
		date, err := domain.ResolveXDD(start, code)
		if err != nil || date.Format("2006-01-02") != want {
			t.Errorf("code %d got %v err=%v want %s", code, date, err, want)
		}
	}
	leapStart := time.Date(2024, time.February, 1, 0, 0, 0, 0, time.UTC)
	for code, want := range map[int]string{99: "2024-02-29", 199: "2024-03-31", -99: "2024-01-31"} {
		date, err := domain.ResolveXDD(leapStart, code)
		if err != nil || date.Format("2006-01-02") != want {
			t.Errorf("leap %d: %v %v", code, date, err)
		}
	}
	if !strings.Contains(domain.ExplainXDD(399), "последний день") {
		t.Fatal("no last-day explanation")
	}
	if got := domain.ExampleDate(306, 7, 1, 2026); got != "06.05.2026" {
		t.Fatalf("date example %s", got)
	}
}

func TestRecordValidation(t *testing.T) {
	r := domain.Record{ContractID: 1, TypeID: 2, Period: 3, AutoPay: 1, Amount: "123,45", StartDay: 31, StartMonth: 12, DocumentsCode: -99, PaymentCode: -6, DisconnectCode: 399, Notify: 0}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Amount != "123.4500" {
		t.Fatalf("unexpected money %q", r.Amount)
	}
	for _, period := range []int{2, 4, 11, 13, 255} {
		r.Period = period
		if err := r.Validate(); err != nil {
			t.Fatalf("valid period %d: %v", period, err)
		}
	}
	for _, period := range []int{0, -1, 256} {
		r.Period = period
		if err := r.Validate(); err == nil {
			t.Fatalf("invalid period %d accepted", period)
		}
	}
}

func TestConfig(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	valid := `{"connection_string":"sqlserver://host","list_sql":"SELECT 1","contracts_sql":"SELECT 1","document_types_sql":"SELECT 1"}`
	if err := os.WriteFile(p, []byte(valid), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := config.LoadConfig(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.ListLimit != 200 || c.LookupLimit != 100 {
		t.Fatalf("wrong defaults %+v", c)
	}
	for _, bad := range []string{`{}`, valid + `{}`, strings.Replace(valid, "list_sql", "unknown_sql", 1)} {
		os.WriteFile(p, []byte(bad), 0600)
		if _, err := config.LoadConfig(p); err == nil {
			t.Errorf("accepted invalid config %q", bad)
		}
	}
}

func TestParseInt(t *testing.T) {
	n, err := presentation.ParseIntField("день", " 12 ")
	if err != nil || n != 12 {
		t.Fatal(n, err)
	}
	_, err = presentation.ParseIntField("день", "domain.XDD")
	if err == nil || errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}
