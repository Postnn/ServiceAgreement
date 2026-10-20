package core

import (
    "fmt"
    "strings"
    "unicode"
    "unicode/utf8"
)

// CompileDefaultOrderBy validates the user-configured SQL-style sort list,
// e.g. "AgreementLabel ASC, Amount DESC" or "[Договор] ASC, [Сумма] DESC".
// It only accepts SELECT aliases followed by optional ASC or DESC. It cannot
// introduce SQL statements, functions or expressions into the generated SQL.
func CompileDefaultOrderBy(spec string) (string, error) {
    spec = strings.TrimSpace(spec)
    if spec == "" {
        return "[Source].[ContractID] DESC", nil
    }
    tokens := sqlTokens(spec)
    var output []string
    for i := 0; i < len(tokens); {
        t := tokens[i]
        alias, err := sortAlias(t)
        if err != nil {
            return "", fmt.Errorf("неверное поле сортировки %q: %w", t.value, err)
        }
        i++
        direction := "ASC"
        if i < len(tokens) && tokens[i].kind == tokenWord {
            switch strings.ToUpper(tokens[i].value) {
            case "ASC", "DESC":
                direction = strings.ToUpper(tokens[i].value)
                i++
            }
        }
        output = append(output, "[Source].["+strings.ReplaceAll(alias, "]", "]]")+"] "+direction)
        if i == len(tokens) {
            break
        }
        if tokens[i].kind != tokenPunct || tokens[i].value != "," {
            return "", fmt.Errorf("в ORDER BY после %q ожидается запятая и следующее поле", alias)
        }
        i++
        if i == len(tokens) {
            return "", fmt.Errorf("список ORDER BY не должен заканчиваться запятой")
        }
    }
    return strings.Join(output, ", "), nil
}

func sortAlias(t sqlToken) (string, error) {
    if t.kind == tokenQuoted && strings.HasPrefix(t.value, "[") &&
        strings.HasSuffix(t.value, "]") {
        // Names with spaces and punctuation must use SQL Server brackets.
        inner := t.value[1:len(t.value)-1]
        inner = strings.ReplaceAll(inner, "]]", "]")
        if strings.TrimSpace(inner) == "" {
            return "", fmt.Errorf("пустое имя SQL-алиаса")
        }
        return inner, nil
    }
    if t.kind != tokenWord || t.value == "" {
        return "", fmt.Errorf("разрешены имена SQL-алиасов, ASC/DESC и запятые")
    }
    for i, r := range t.value {
        if i == 0 {
            if !(unicode.IsLetter(r) || r == '_') {
                return "", fmt.Errorf("некавыченный SQL-алиас должен начинаться с буквы или '_'")
            }
        } else if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '$' || r == '#') {
            return "", fmt.Errorf("недопустимый символ в имени SQL-алиаса")
        }
    }
    if !utf8.ValidString(t.value) {
        return "", fmt.Errorf("неверная кодировка имени")
    }
    return t.value, nil
}
