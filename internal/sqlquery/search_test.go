package sqlquery

import (
	"database/sql"
	"reflect"
	"strings"
	"testing"
)

func TestBuildListSearchQueryWordParametersAndOrder(t *testing.T) {
	base := "SELECT ContractID, SearchText, Name FROM dbo.Contracts"
	query, args, err := BuildListSearchQuery(base, "   Номер  2026\tООО  ", 125)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(query, "SELECT TOP (@limit) [Source].*") ||
		!strings.Contains(query, "FROM (\n"+base+"\n) AS [Source]") ||
		!strings.Contains(query, "ORDER BY [Source].[ContractID] DESC") {
		t.Fatalf("invalid wrapped search SQL: %s", query)
	}
	for i := 1; i <= 3; i++ {
		if !strings.Contains(query, "LIKE N'% ' + @SearchText"+string(rune('0'+i))+" + N'%'") {
			t.Fatalf("search word %d not parameterized: %s", i, query)
		}
	}
	if strings.Count(query, "LIKE ") != 3 || strings.Count(query, " AND ") != 2 {
		t.Fatalf("search words must be ANDed, not ORed: %s", query)
	}
	want := []string{"Номер", "2026", "ООО"}
	got := make([]string, 0)
	for _, arg := range args {
		v, ok := arg.(sql.NamedArg)
		if !ok {
			t.Fatalf("non-named parameter: %#v", arg)
		}
		if strings.HasPrefix(v.Name, "SearchText") {
			got = append(got, v.Value.(string))
		}
	}
	if !reflect.DeepEqual(want, got) {
		t.Fatalf("wrong search tokens: %v", got)
	}
	if strings.Contains(query, "Номер") || strings.Contains(query, "ООО") {
		t.Fatal("untrusted search input injected into SQL text")
	}
	if len(args) != 5 {
		t.Fatalf("unexpected args: %v", args)
	}
}

func TestBuildListSearchQueryEmptyAndLiteralLikeWords(t *testing.T) {
	base := "SELECT ContractID, SearchText FROM t"
	q, args, err := BuildListSearchQuery(base+";", "  ", 200)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q, "WHERE ") || strings.Contains(q, " LIKE ") || len(args) != 2 ||
		!strings.Contains(q, "FROM (\n"+base+"\n)") {
		t.Fatalf("empty search shouldn't filter: %s args %v", q, args)
	}
	q, args, err = BuildListSearchQuery(base, "'; DROP 100% _ [ ~", 200)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(q, "DROP TABLE") || strings.Count(q, " LIKE ") != 6 {
		t.Fatalf("unsafe query: %s", q)
	}
	got := map[string]string{}
	for _, arg := range args {
		p := arg.(sql.NamedArg)
		if strings.HasPrefix(p.Name, "SearchText") {
			got[p.Name] = p.Value.(string)
		}
	}
	if got["SearchText3"] != "100~%" || got["SearchText4"] != "~_" ||
		got["SearchText5"] != "~[" || got["SearchText6"] != "~~" {
		t.Fatalf("LIKE metacharacters must be literal, got: %v", got)
	}
}
func TestBuildListSearchQueryRejectsEarlyTop(t *testing.T) {
	for _, src := range []string{
		"", "SELECT TOP (@limit) ContractID, SearchText FROM T ORDER BY ID",
		"SELECT DISTINCT TOP(20) ContractID, SearchText FROM T",
	} {
		if _, _, err := BuildListSearchQuery(src, "one", 200); err == nil {
			t.Errorf("unsafe or empty base SQL was accepted: %q", src)
		}
	}
}
