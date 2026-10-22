package presentation

import "testing"

func TestMoneyDisplayN2RussianFormat(t *testing.T) {
	cases := []struct {
		value any
		want  string
	}{
		{"0.0000", "0,00"},
		{"1234.5000", "1\u00a0234,50"},
		{"-1234567890.1260", "-1\u00a0234\u00a0567\u00a0890,13"},
		{"1.0050", "1,00"},
		{"1.0150", "1,02"},
		{"999.9950", "1\u00a0000,00"},
		{[]byte("30000.8"), "30\u00a0000,80"},
		{int64(42), "42,00"},
		{nil, ""},
	}
	for _, c := range cases {
		got, ok := MoneyDisplayN2(c.value)
		if !ok || got != c.want {
			t.Errorf("%#v => %q, %v; want %q", c.value, got, ok, c.want)
		}
	}
}
func TestSQLDisplayValueFormatsOnlySQLMoney(t *testing.T) {
	for _, typ := range []string{"MONEY", "money", " Money "} {
		if got := SQLDisplayValue("4567.2500", typ, "ServiceFee"); got != "4\u00a0567,25" {
			t.Errorf("%s: %q", typ, got)
		}
	}
	// Even names implying monetary data must not cause N2 unless SQL
	// actually returns type MONEY. SMALLMONEY is right aligned but not N2.
	for _, typ := range []string{"NVARCHAR", "VARCHAR", "DECIMAL", "NUMERIC", "SMALLMONEY", "INT", ""} {
		for _, alias := range []string{"Amount", "Абон.плата", "ContractName"} {
			got := SQLDisplayValue("1000.2500", typ, alias)
			if got != "1000.2500" {
				t.Errorf("untyped N2 conversion: %s %s => %q", typ, alias, got)
			}
		}
	}
}

func TestMoneyEditorN2PreservesUnchangedSQLMoneyPrecision(t *testing.T) {
	exactOriginal := "1234.5678"
	initial, ok := MoneyDisplayN2(exactOriginal)
	if !ok || initial != "1\u00a0234,57" {
		t.Fatalf("N2 initial %q", initial)
	}
	stillExact, err := MoneyValueFromEditor(initial, initial, exactOriginal)
	if err != nil || stillExact != exactOriginal {
		t.Fatalf("untouched edit rounded SQL money: %q %v", stillExact, err)
	}
	changed, err := MoneyValueFromEditor("1 235,40", initial, exactOriginal)
	if err != nil || changed != "1235.4000" {
		t.Fatalf("edited Russian N2 value: %q %v", changed, err)
	}
	changed, err = ParseMoneyEditorText("1\u00a0235,40")
	if err != nil || changed != "1235.4000" {
		t.Fatalf("NBSP thousands separator: %q %v", changed, err)
	}
	for _, bad := range []string{"1,2,3", "wrong", "1 234,12345"} {
		if _, err := ParseMoneyEditorText(bad); err == nil {
			t.Errorf("invalid N2 accepted: %q", bad)
		}
	}
}
