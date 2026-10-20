package core

import (
    "database/sql"
    "fmt"
    "strconv"
    "strings"
)

// BuildListSearchQuery applies exactly the same word-based search to both
// lists. The configured SQL must be a SELECT suitable for a derived table,
// project ContractID and SearchText, and must not apply TOP before searching.
// Each search word is a named SQL parameter: user input is never interpolated
// into SQL source. The limit is applied only AFTER the word predicates.
func BuildListSearchQuery(baseSQL, search string, limit int) (string, []any, error) {
    base := strings.TrimSpace(baseSQL)
    for strings.HasSuffix(base, ";") {
        base = strings.TrimSpace(strings.TrimSuffix(base, ";"))
    }
    if base == "" {
        return "", nil, fmt.Errorf("не задан исходный SQL-запрос списка")
    }
    if limit < 1 {
        return "", nil, fmt.Errorf("некорректный лимит списка: %d", limit)
    }
    // TOP within the source query would truncate the candidates BEFORE the
    // outer word filter. Fail instead of silently missing agreements.
    if startsWithSelectTop(base) {
        return "", nil, fmt.Errorf("исходный SQL содержит TOP: ограничение должно применяться после поиска. Удалите TOP из list_sql / without_parameters_sql")
    }
    words := strings.Fields(search)
    // SQL Server permits at most 2100 query parameters per statement.
    if len(words) > 2000 {
        return "", nil, fmt.Errorf("слишком много слов поиска: %d (максимум 2000)", len(words))
    }

    var query strings.Builder
    query.WriteString("SELECT TOP (@limit) [Source].*\nFROM (\n")
    query.WriteString(base)
    query.WriteString("\n) AS [Source]")
    args := make([]any, 0, len(words)+2)
    args = append(args, sql.Named("limit", limit))
    // Optional backwards compatibility for custom SQL containing @search.
    // The source query must never filter by these words itself.
    args = append(args, sql.Named("search", ""))
    for i, word := range words {
        if i == 0 { query.WriteString("\nWHERE ") } else { query.WriteString("\n  AND ") }
        name := "SearchText" + strconv.Itoa(i+1)
        query.WriteString("(N'   ' + COALESCE(CONVERT(nvarchar(max), [Source].[SearchText]), N'') LIKE N'% ' + @")
        query.WriteString(name)
        query.WriteString(" + N'%' ESCAPE N'~')")
        args = append(args, sql.Named(name, escapeSearchLikeWord(word)))
    }
    query.WriteString("\nORDER BY [Source].[ContractID] DESC")
    return query.String(), args, nil
}

func startsWithSelectTop(s string) bool {
    // Only the unambiguously identifiable legacy TOP prefix is rejected.
    // Complex user SQL remains unchanged, and SQL Server validates it.
    fields := strings.Fields(strings.ToUpper(s))
    if len(fields) < 2 || fields[0] != "SELECT" { return false }
    if fields[1] == "DISTINCT" && len(fields) > 2 {
        return fields[2] == "TOP" || strings.HasPrefix(fields[2], "TOP(")
    }
    return fields[1] == "TOP" || strings.HasPrefix(fields[1], "TOP(")
}

func escapeSearchLikeWord(word string) string {
    return strings.NewReplacer("~", "~~", "%", "~%", "_", "~_", "[", "~[").Replace(word)
}
