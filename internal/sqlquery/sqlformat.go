package sqlquery

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// FormatSQL improves indentation and line breaks of SQL Server queries for the
// settings editor. It NEVER parses the SQL into another dialect, changes
// tokens, alters literal/comment contents, or executes statements.
// It only changes whitespace between already existing lexical tokens.
func FormatSQL(input string) string {
	tokens := sqlTokens(strings.ReplaceAll(strings.ReplaceAll(input, "\r\n", "\n"), "\r", "\n"))
	if len(tokens) == 0 {
		return ""
	}
	var out strings.Builder
	depth := 0
	caseDepth := 0
	clauseIndent := 0
	contentIndent := 0
	needBreak := false
	breakIndent := 0
	for i, token := range tokens {
		upper := ""
		if token.kind == tokenWord {
			upper = strings.ToUpper(token.value)
		}
		clause := isSQLClause(tokens, i, upper)
		if token.kind == tokenCommentLine || token.kind == tokenCommentBlock {
			if strings.Contains(token.gap, "\n") {
				needBreak = true
				breakIndent = contentIndent
			}
		}
		if clause && i > 0 {
			needBreak = true
			clauseIndent = depth
			breakIndent = clauseIndent
		}
		if upper == "WHEN" && caseDepth > 0 || upper == "ELSE" && caseDepth > 0 {
			needBreak = true
			breakIndent = depth + 1
		}
		if upper == "END" && caseDepth > 0 {
			needBreak = true
			breakIndent = depth
			caseDepth--
		}
		if upper == "GO" && i > 0 {
			needBreak = true
			breakIndent = 0
		}
		if i == 0 {
			// Initial indentation in the saved query is replaced, never injected
			// before the first token.
		} else if needBreak {
			out.WriteByte('\n')
			for n := 0; n < breakIndent; n++ {
				out.WriteByte('\t')
			}
		} else if len(token.gap) > 0 {
			// A single space preserves separation (e.g. `- -` cannot become a
			// `--` comment). Quoted text is opaque to the formatter.
			out.WriteByte(' ')
		}
		out.WriteString(token.value)
		needBreak = false
		if upper == "CASE" {
			caseDepth++
		}
		if token.kind == tokenPunct {
			switch token.value {
			case "(":
				depth++
			case ")":
				if depth > 0 {
					depth--
				}
			case ",":
				if depth == 0 {
					needBreak = true
					breakIndent = contentIndent
				}
			case ";":
				needBreak = true
				breakIndent = 0
			}
		}
		if clause {
			// The clause header is on its own line; its contents and
			// comma-separated SELECT items use one additional tab level.
			contentIndent = depth + 1
			needBreak = true
			breakIndent = contentIndent
			// Keep compound SQL clause keywords together, then break before
			// their body: ORDER BY, GROUP BY, LEFT JOIN, DELETE FROM, etc.
			next := ""
			if i+1 < len(tokens) && tokens[i+1].kind == tokenWord {
				next = strings.ToUpper(tokens[i+1].value)
			}
			if (upper == "ORDER" || upper == "GROUP") && next == "BY" ||
				(upper == "LEFT" || upper == "RIGHT" || upper == "INNER" || upper == "FULL" || upper == "CROSS" || upper == "OUTER") && next == "JOIN" ||
				upper == "DELETE" && next == "FROM" || upper == "INSERT" && next == "INTO" {
				needBreak = false
			}
		}
		if i > 0 && tokens[i-1].kind == tokenWord {
			before := strings.ToUpper(tokens[i-1].value)
			if (upper == "BY" && (before == "ORDER" || before == "GROUP")) ||
				(upper == "JOIN" && (before == "LEFT" || before == "RIGHT" || before == "INNER" || before == "FULL" || before == "CROSS" || before == "OUTER")) ||
				(upper == "FROM" && before == "DELETE") || (upper == "INTO" && before == "INSERT") {
				contentIndent = depth + 1
				needBreak = true
				breakIndent = contentIndent
			}
		}
		if upper == "GO" {
			needBreak = true
			breakIndent = 0
		}
		if token.kind == tokenCommentLine {
			needBreak = true
			breakIndent = contentIndent
		}
		// Keep comments and N'...' attached to their neighboring tokens when
		// the original SQL had no separating whitespace.
		if (token.kind == tokenCommentBlock || token.kind == tokenCommentLine) && i+1 < len(tokens) && strings.Contains(tokens[i+1].gap, "\n") {
			needBreak = true
			breakIndent = contentIndent
		}
	}
	return strings.TrimSpace(out.String())
}

type sqlTokenKind uint8

const (
	tokenWord sqlTokenKind = iota
	tokenQuoted
	tokenPunct
	tokenCommentLine
	tokenCommentBlock
)

type sqlToken struct {
	value, gap string
	kind       sqlTokenKind
}

func sqlTokens(s string) []sqlToken {
	result := make([]sqlToken, 0, len(s)/5)
	gapStart := 0
	i := 0
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		start := i
		kind := tokenPunct
		switch {
		case strings.HasPrefix(s[i:], "--"):
			kind = tokenCommentLine
			i += 2
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case strings.HasPrefix(s[i:], "/*"):
			kind = tokenCommentBlock
			i += 2
			nesting := 1
			for i < len(s) && nesting > 0 {
				if strings.HasPrefix(s[i:], "/*") {
					nesting++
					i += 2
				} else if strings.HasPrefix(s[i:], "*/") {
					nesting--
					i += 2
				} else {
					_, width := utf8.DecodeRuneInString(s[i:])
					i += width
				}
			}
		case r == '\'' || r == '"' || r == '[':
			kind = tokenQuoted
			closing := r
			if r == '[' {
				closing = ']'
			}
			i += size
			for i < len(s) {
				next, w := utf8.DecodeRuneInString(s[i:])
				i += w
				if next == closing {
					peek, _ := utf8.DecodeRuneInString(s[i:])
					if peek == closing {
						_, width := utf8.DecodeRuneInString(s[i:])
						i += width
						continue
					}
					break
				}
			}
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '@' || r == '#' || r == '$':
			kind = tokenWord
			i += size
			for i < len(s) {
				next, w := utf8.DecodeRuneInString(s[i:])
				if !unicode.IsLetter(next) && !unicode.IsDigit(next) && next != '_' && next != '@' && next != '#' && next != '$' {
					break
				}
				i += w
			}
		default:
			i += size
		}
		result = append(result, sqlToken{value: s[start:i], gap: s[gapStart:start], kind: kind})
		gapStart = i
	}
	return result
}

func isSQLClause(tokens []sqlToken, i int, upper string) bool {
	switch upper {
	case "SELECT", "FROM", "WHERE", "GROUP", "ORDER", "HAVING", "JOIN", "LEFT", "RIGHT", "INNER", "FULL", "CROSS", "OUTER", "UNION", "EXCEPT", "INTERSECT", "INSERT", "UPDATE", "DELETE", "SET", "VALUES", "OUTPUT", "MERGE":
		// LEFT, INNER, etc. are kept on the same line as the following JOIN;
		// JOIN itself does not begin an additional line after the modifier.
		if upper == "FROM" && i > 0 && strings.EqualFold(tokens[i-1].value, "DELETE") {
			return false
		}
		if upper == "JOIN" && i > 0 {
			prev := strings.ToUpper(tokens[i-1].value)
			if prev == "LEFT" || prev == "RIGHT" || prev == "INNER" || prev == "FULL" || prev == "CROSS" || prev == "OUTER" {
				return false
			}
		}
		// ORDER/GROUP BY is treated as a single clause header.
		return true
	case "ON":
		// ON is a JOIN clause unless used as an identifier (which would need
		// quoting in SQL Server). Breaking here is formatting only.
		return true
	}
	return false
}
