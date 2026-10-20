package core

import (
    "database/sql"
    "fmt"
    "strconv"
    "strings"
)

// BuildListSearchQuery applies exactly the same word-based search to both
// lists. The configured SQL may start with SELECT or ;WITH / WITH (CTE),
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
    // SQL Server CTEs must be declared before the generated outer SELECT.
    // Never place a leading ;WITH inside FROM (...).
    ctePrefix, finalSelect, err := splitLeadingCTE(base)
    if err != nil {
        return "", nil, fmt.Errorf("ошибка в CTE исходного списка: %w", err)
    }
    // TOP within the final SELECT truncates candidates BEFORE word filtering.
    // TOP/ORDER BY inside a CTE body is legal and does not pre-limit the
    // final SELECT, so only the last SELECT is checked here.
    if startsWithSelectTop(finalSelect) {
        return "", nil, fmt.Errorf("итоговый SELECT содержит TOP: ограничение должно применяться после поиска. Удалите TOP из итогового SELECT списка")
    }
    if hasTopLevelOrderBy(finalSelect) {
        return "", nil, fmt.Errorf("итоговый SELECT содержит ORDER BY: сортировка должна выполняться внешним запросом по ContractID. Удалите завершающий ORDER BY")
    }
    words := strings.Fields(search)
    // SQL Server permits at most 2100 query parameters per statement.
    if len(words) > 2000 {
        return "", nil, fmt.Errorf("слишком много слов поиска: %d (максимум 2000)", len(words))
    }

    var query strings.Builder
    if ctePrefix != "" {
        query.WriteString(ctePrefix)
        if !strings.HasSuffix(ctePrefix, "\n") && !strings.HasSuffix(ctePrefix, "\r") {
            query.WriteByte('\n')
        }
    }
    query.WriteString("SELECT TOP (@limit) [Source].*\nFROM (\n")
    query.WriteString(finalSelect)
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
    // Handle comments between SELECT / DISTINCT / TOP, as well as TOP(10).
    tokens := sqlTokens(s)
    next := func(i int) int {
        for i < len(tokens) &&
            (tokens[i].kind == tokenCommentLine || tokens[i].kind == tokenCommentBlock) {
            i++
        }
        return i
    }
    i := next(0)
    if i >= len(tokens) || tokens[i].kind != tokenWord ||
        !strings.EqualFold(tokens[i].value, "SELECT") {
        return false
    }
    i = next(i + 1)
    if i < len(tokens) && tokens[i].kind == tokenWord &&
        (strings.EqualFold(tokens[i].value, "DISTINCT") ||
            strings.EqualFold(tokens[i].value, "ALL")) {
        i = next(i + 1)
    }
    return i < len(tokens) && tokens[i].kind == tokenWord &&
        strings.EqualFold(tokens[i].value, "TOP")
}

func escapeSearchLikeWord(word string) string {
    return strings.NewReplacer("~", "~~", "%", "~%", "_", "~_", "[", "~[").Replace(word)
}
