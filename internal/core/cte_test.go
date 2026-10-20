package core

import (
    "database/sql"
    "strings"
    "testing"
)

func TestSearchFromLeadingCTESQLServer(t *testing.T) {
    src := `;WITH Periods AS (
    SELECT TOP (15) [КодДокумента] AS ContractID,
           [Договор] AS SearchText
    FROM dbo.[ДокументыДанные]
    WHERE [Договор] <> N'строка SELECT ) -- пример'
    ORDER BY [КодДокумента] DESC
)
SELECT ContractID, SearchText
FROM Periods`
    got, args, err := BuildListSearchQuery(src, "договор 2026", 100)
    if err != nil { t.Fatal(err) }

    marker := "\nSELECT ContractID, SearchText\nFROM Periods"
    sourcePrefix := src[:strings.Index(src, marker)]
    outerStart := strings.Index(got, "\nSELECT TOP (@limit) [Source].*")
    if outerStart < 0 { t.Fatalf("CTE prefix was placed inside FROM (...):\n%s",got) }
    if got[:outerStart] != sourcePrefix {
        t.Fatalf("CTE content was reformatted:\n%s\nversus\n%s",got[:outerStart],sourcePrefix)
    }
    if !strings.Contains(got, "FROM (\nSELECT ContractID, SearchText\nFROM Periods\n) AS [Source]") {
        t.Fatalf("only the final CTE SELECT should be wrapped:\n%s",got)
    }
    if strings.Count(got,"LIKE N'% ' + @SearchText")!=2 ||
        !strings.Contains(got,"@SearchText1") || !strings.Contains(got,"@SearchText2") {
        t.Fatalf("CTE search word conditions missing:\n%s",got)
    }
    if len(args)!=4 || args[2].(sql.NamedArg).Value!="договор" ||
        args[3].(sql.NamedArg).Value!="2026" {
        t.Fatalf("CTE search arguments: %#v",args)
    }
}

func TestCTESplittingIgnoresNestedSelectQuotedWordsAndComments(t *testing.T) {
    samples := []struct{
        name, sql, expectBody string
    }{
        {
            name:"multiple named CTEs with comments",
            sql:`/* ;WITH fake AS (SELECT 2) */
;WITH [Первый]([ContractID], [SearchText]) AS (
    SELECT d.ID, d.Name AS SearchText FROM dbo.D AS d
    WHERE d.Name LIKE N'% SELECT ) --%'
), -- SELECT TOP(1) not the final query
[Второй] AS (
    SELECT ContractID, SearchText FROM [Первый]
    WHERE SearchText LIKE N'%AS (%%)%'
)
-- final select after comments
SELECT ContractID, SearchText FROM [Второй]`,
            expectBody:"SELECT ContractID, SearchText FROM [Второй]",
        },
        {
            name:"WITH without semicolon",
            sql:`WITH C AS(SELECT 1 AS ContractID, 'SELECT' AS SearchText)
SELECT ContractID, SearchText FROM C`,
            expectBody:"SELECT ContractID, SearchText FROM C",
        },
        {
            name:"multiple CTEs on one line",
            sql:`;WITH A AS(SELECT 1 AS ContractID, N'a' AS SearchText), B AS(SELECT * FROM A) SELECT ContractID, SearchText FROM B`,
            expectBody:"SELECT ContractID, SearchText FROM B",
        },
        {
            name:"CRLF preserved",
            sql:";WITH A AS (\r\nSELECT 1 AS ContractID, N'(' AS SearchText\r\n)\r\nSELECT ContractID, SearchText FROM A",
            expectBody:"SELECT ContractID, SearchText FROM A",
        },
    }
    for _, c := range samples {
        t.Run(c.name,func(t *testing.T){
            prefix, body, err := splitLeadingCTE(c.sql)
            if err != nil {t.Fatal(err)}
            if body != c.expectBody {t.Fatalf("wrong final SELECT: %q",body)}
            if prefix+body != c.sql {t.Fatalf("CTE split changed the original SQL")}
            wrapped, _, err := BuildListSearchQuery(c.sql, "abc", 20)
            if err != nil { t.Fatal(err) }
            if !strings.HasPrefix(wrapped,prefix) ||
               !strings.Contains(wrapped,"FROM (\n"+body+"\n) AS [Source]") {
                t.Fatalf("bad wrapper for %q:\n%s",c.name,wrapped)
            }
            if strings.Contains(prefix,"SELECT TOP (@limit) [Source].*") {
                t.Fatal("injected SELECT into the declaration")
            }
        })
    }
}

func TestLeadingCTEErrorsAndTopLevelOrdering(t *testing.T) {
    invalid := []string {
        ";WITH",
        ";WITH CTE AS (SELECT 1)",
        ";WITH CTE AS SELECT 1 SELECT 2",
        ";WITH CTE AS (SELECT (1) SELECT ContractID, SearchText FROM CTE",
        ";WITH CTE AS (SELECT 1), Other AS (SELECT 2)",
        ";WITH CTE AS (SELECT 1) SELECT TOP (1) ContractID, SearchText FROM CTE",
        ";WITH CTE AS (SELECT 1) SELECT /*comment*/ DISTINCT TOP (1) ContractID, SearchText FROM CTE",
        ";WITH CTE AS (SELECT 1) SELECT ContractID, SearchText FROM CTE ORDER BY ContractID",
        "SELECT ContractID, SearchText FROM dbo.T ORDER BY ContractID",
    }
    for _, sql := range invalid {
        if _, _, err := BuildListSearchQuery(sql, "abc", 100); err == nil {
            t.Errorf("invalid or prelimited query accepted: %q",sql)
        }
    }
    // TOP within a CTE is part of that CTE's semantics: it may restrict
    // candidates intentionally. Only TOP in the final SELECT is prohibited.
    valid := `;WITH CTE AS (
        SELECT TOP (5) ContractID, SearchText FROM T ORDER BY ContractID
    ) SELECT ContractID, SearchText FROM CTE`
    if _, _, err := BuildListSearchQuery(valid,"",100); err != nil {
        t.Errorf("valid CTE-internal TOP/ORDER BY rejected: %v",err)
    }
    // Quoted strings and comments containing WITH are not CTE declarations.
    plain := `SELECT ';WITH fake AS (SELECT 1)' AS SearchText, 1 AS ContractID /* ;WITH */`
    prefix, final, err := splitLeadingCTE(plain)
    if err != nil || prefix!="" || final!=plain {
        t.Fatalf("plain SELECT mistaken for CTE: %q %q %v",prefix,final,err)
    }
}
