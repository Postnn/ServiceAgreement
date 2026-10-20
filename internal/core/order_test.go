package core

import (
    "strings"
    "testing"
)

func TestDefaultSortExpressionCompiledSafely(t *testing.T) {
    cases:=[]struct{source,want string}{
        {"","[Source].[ContractID] DESC"},
        {"ContractID DESC","[Source].[ContractID] DESC"},
        {"Field1 ASC, Field2 DESC","[Source].[Field1] ASC, [Source].[Field2] DESC"},
        {"[Договор] ASC, [Сумма выплаты] DESC","[Source].[Договор] ASC, [Source].[Сумма выплаты] DESC"},
        {"[Иван]]ов] DESC","[Source].[Иван]]ов] DESC"},
    }
    for _,c:=range cases {
        got,err:=CompileDefaultOrderBy(c.source)
        if err!=nil || got!=c.want {
            t.Errorf("%q => %q, %v; want %q",c.source,got,err,c.want)
        }
    }
    for _,bad:=range []string{
        "Amount; DROP TABLE t","Amount DESC; --",
        "ABS(Amount) DESC","Amount ASC,","Amount DESC, [E] = 1",
        "a.b ASC", "Amount DESC UNION SELECT 1",
    } {
        if x,err:=CompileDefaultOrderBy(bad);err==nil {
            t.Errorf("unsafe ORDER BY %q accepted as %q",bad,x)
        }
    }
}
func TestDefaultSortAppliedToGeneratedOuterSelect(t *testing.T) {
    query,_,err:=BuildListSearchQuery(
        "SELECT ContractID, SearchText, [Договор] FROM dbo.[Documents]",
        "тест 2026",100,"[Договор] ASC, ContractID DESC")
    if err!=nil {t.Fatal(err)}
    if !strings.HasSuffix(query,"ORDER BY [Source].[Договор] ASC, [Source].[ContractID] DESC") {
        t.Fatalf("default sort not added to outer query:\n%s",query)
    }
    if strings.Count(query,"WHERE ")!=1 {
        t.Fatal("sort broke word search wrapper")
    }
}
func TestSortSettingsStoredAndValidated(t *testing.T) {
    c:=DefaultConfig()
    c.ConnectionString="sqlserver://test"
    c.ListOrderBy="ContractID ASC"
    c.WithoutParametersOrderBy="[Договор] DESC, ContractID DESC"
    if err:=c.Validate();err!=nil {t.Fatal(err)}
    c.WithoutParametersOrderBy="ContractID ASC; DELETE FROM t"
    if c.Validate()==nil {t.Fatal("SQL injection in sort config accepted")}
}
