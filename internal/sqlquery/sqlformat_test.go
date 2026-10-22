package sqlquery

import (
	"reflect"
	"strings"
	"testing"
)

func TestFormatSQLPreservesSQLTokensAndLiterals(t *testing.T) {
	cases := []string{
		`SELECT TOP (@limit) d.[ContractID],N'строка FROM,WHERE -- внутри' AS [Сумма],d.[Name] FROM dbo.[ДокументыДанные] AS d LEFT JOIN dbo.[ТипыДокументов] AS t ON t.[Id] = d.[Id] WHERE d.[Id]=@id ORDER BY d.[Id] DESC`,
		"SELECT N'a''b', [from]]where], \"order\", 'line\nsecond' FROM x -- last comment\nWHERE [Key]=1",
		"-- standalone comment\nSELECT a,b FROM table1 /* nested /* FROM fake */ KEEP */ WHERE x='/* comment */'",
		`SELECT CASE WHEN [A]=1 THEN N'Три месяца (3)' ELSE N'Один месяц (1)' END AS [PeriodText], CONVERT(nvarchar(40), CONVERT(decimal(19,4), [Money])) AS [Amount] FROM dbo.[T];`,
		`UPDATE dbo.T SET a=@a,b=(SELECT MAX(x) FROM dbo.S WHERE [Name] LIKE N'%' + @search + N'%') WHERE ID=@id;`,
		"SELECT a - -b, c + +d, e -- comment\nFROM T",
		`WITH X AS (SELECT a,b FROM t) SELECT a FROM X UNION SELECT a FROM y`,
	}
	for _, src := range cases {
		out := FormatSQL(src)
		var want, have []string
		for _, tkn := range sqlTokens(src) {
			want = append(want, tkn.value)
		}
		for _, tkn := range sqlTokens(out) {
			have = append(have, tkn.value)
		}
		if !reflect.DeepEqual(want, have) {
			t.Errorf("tokens changed: %q\n%s\nwant %q\nhave %q", src, out, want, have)
		}
		if !strings.Contains(out, "\n") || !strings.Contains(out, "\t") {
			t.Errorf("unformatted SQL: %q", out)
		}
		again := FormatSQL(out)
		if again != out {
			t.Errorf("formatter not idempotent\n%s\n----\n%s", out, again)
		}
	}
}

func TestFormatSQLEmptyAndUnicode(t *testing.T) {
	if got := FormatSQL(" \n\t "); got != "" {
		t.Fatalf("empty string: %q", got)
	}
	got := FormatSQL("SELECT [ДеньМесяцаОплаты],N'Привет, O''Connor --FROM' FROM [Таблица]")
	if !strings.Contains(got, "N'Привет, O''Connor --FROM'") {
		t.Fatalf("corrupted literal: %q", got)
	}
}
