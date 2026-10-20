package core

import (
    "fmt"
    "strings"
)

// splitLeadingCTE separates a SQL Server CTE declaration from the final
// SELECT. T-SQL does not allow a WITH clause inside a derived table:
//
//     FROM ( ;WITH cte AS (...) SELECT ... ) AS Source  -- invalid
//
// Keep the CTE before the generated SELECT and wrap only its final SELECT:
//     ;WITH cte AS (...)
//     SELECT TOP (@limit) Source.* FROM (SELECT ... FROM cte) AS Source
//
// The tokenizer ignores keywords and parentheses inside strings, quoted
// identifiers and comments. The original SQL text is preserved byte-for-byte.
func splitLeadingCTE(source string) (declaration, finalSelect string, err error) {
    tokens := sqlTokens(source)
    if len(tokens) == 0 {
        return "", source, nil
    }

    // Record the source offsets without reconstructing or formatting SQL.
    offsets := make([]int, len(tokens))
    pos := 0
    for i, token := range tokens {
        offsets[i] = pos + len(token.gap)
        pos += len(token.gap) + len(token.value)
    }

    skipComments := func(i int) int {
        for i < len(tokens) && (tokens[i].kind == tokenCommentBlock ||
            tokens[i].kind == tokenCommentLine) {
            i++
        }
        return i
    }
    isWord := func(i int, word string) bool {
        return i < len(tokens) && tokens[i].kind == tokenWord &&
            strings.EqualFold(tokens[i].value, word)
    }
    // SQL Server commonly uses ;WITH, but WITH at the start of a batch is
    // valid too. Leading comments and statement terminators are retained.
    i := skipComments(0)
    for i < len(tokens) && tokens[i].value == ";" {
        i = skipComments(i + 1)
    }
    if !isWord(i, "WITH") {
        return "", source, nil
    }
    i++

    // Every CTE definition has the structure
    // cte_name [(column, ...)] AS (query), optionally followed by another CTE.
    for {
        i = skipComments(i)
        if i >= len(tokens) ||
            !(tokens[i].kind == tokenWord || tokens[i].kind == tokenQuoted) {
            return "", "", fmt.Errorf("после WITH ожидается имя CTE")
        }
        i++
        i = skipComments(i)

        if i < len(tokens) && tokens[i].value == "(" {
            i, err = consumeCTEParentheses(tokens, i)
            if err != nil {
                return "", "", fmt.Errorf("список колонок CTE: %w", err)
            }
            i = skipComments(i)
        }
        if !isWord(i, "AS") {
            return "", "", fmt.Errorf("после имени CTE ожидается AS")
        }
        i = skipComments(i + 1)
        if i >= len(tokens) || tokens[i].value != "(" {
            return "", "", fmt.Errorf("после AS ожидается запрос CTE в скобках")
        }
        i, err = consumeCTEParentheses(tokens, i)
        if err != nil {
            return "", "", fmt.Errorf("тело CTE: %w", err)
        }
        i = skipComments(i)
        if i < len(tokens) && tokens[i].value == "," {
            i++
            continue
        }
        if !isWord(i, "SELECT") {
            return "", "", fmt.Errorf("после объявления CTE ожидается итоговый SELECT")
        }
        return source[:offsets[i]], source[offsets[i]:], nil
    }
}

// consumeCTEParentheses skips a balanced group. Quoted strings/identifiers
// and comments are single tokens and cannot affect the parentheses depth.
func consumeCTEParentheses(tokens []sqlToken, start int) (int, error) {
    if start >= len(tokens) || tokens[start].value != "(" {
        return 0, fmt.Errorf("ожидается открывающая скобка")
    }
    depth := 0
    for i := start; i < len(tokens); i++ {
        if tokens[i].kind != tokenPunct {
            continue
        }
        switch tokens[i].value {
        case "(":
            depth++
        case ")":
            depth--
            if depth == 0 {
                return i + 1, nil
            }
        }
    }
    return 0, fmt.Errorf("не найдена закрывающая скобка")
}

// An ORDER BY at the end of the final SELECT cannot be moved into a derived
// table without changing its semantics. ORDER BY inside CTEs or subqueries
// is not rejected: the check runs only on the final SELECT at nesting level 0.
func hasTopLevelOrderBy(query string) bool {
    tokens := sqlTokens(query)
    depth := 0
    for i := 0; i < len(tokens); i++ {
        token := tokens[i]
        if token.kind == tokenPunct {
            if token.value == "(" {
                depth++
            } else if token.value == ")" && depth > 0 {
                depth--
            }
            continue
        }
        if depth == 0 && token.kind == tokenWord &&
            strings.EqualFold(token.value, "ORDER") {
            next := i + 1
            for next < len(tokens) &&
                (tokens[next].kind == tokenCommentBlock || tokens[next].kind == tokenCommentLine) {
                next++
            }
            if next < len(tokens) && tokens[next].kind == tokenWord &&
                strings.EqualFold(tokens[next].value, "BY") {
                return true
            }
        }
    }
    return false
}
