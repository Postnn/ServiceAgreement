package core

import (
    "fmt"
    "strings"
    "time"
)

// ColumnPresentation expresses the column's user-facing semantic type.
// SQL Server metadata is authoritative for native types. Some configured
// SELECTs, however, deliberately CONVERT(money AS nvarchar) before assigning
// a user-friendly alias (e.g. "Абон.плата"). In such cases the provider
// reports NVARCHAR, so use the source values AND the alias / decimal scale
// rather than treating an obvious monetary amount as arbitrary text.
type ColumnPresentation struct {
    Kind FilterKind
    Money bool
}

func isMoneySQLType(sqlType string) bool {
    switch strings.ToUpper(strings.TrimSpace(sqlType)) {
    case "MONEY", "SMALLMONEY": return true
    }
    return false
}

func isNumericSQLType(sqlType string) bool {
    switch strings.ToUpper(strings.TrimSpace(sqlType)) {
    case "MONEY","SMALLMONEY","DECIMAL","NUMERIC","FLOAT","REAL",
        "TINYINT","SMALLINT","INT","BIGINT","BIT":
        return true
    }
    return false
}
func isDateSQLType(sqlType string) bool {
    switch strings.ToUpper(strings.TrimSpace(sqlType)) {
    case "DATE","DATETIME","DATETIME2","SMALLDATETIME","DATETIMEOFFSET":
        return true
    }
    return false
}
func isTextSQLType(sqlType string) bool {
    switch strings.ToUpper(strings.TrimSpace(sqlType)) {
    case "NVARCHAR","VARCHAR","NCHAR","CHAR","TEXT","NTEXT","XML","UNIQUEIDENTIFIER":
        return true
    }
    return false
}

// SQL aliases are configuration-controlled, and Russian abbreviations are
// frequent. This is a *hint*, not sufficient by itself to turn text into
// money: values must also be numeric (or SQL metadata must identify money).
func isMonetaryAlias(field string) bool {
    name:=strings.ToLower(strings.TrimSpace(field))
    for _,fragment:=range []string{
        "сумм","плат","абон","стоим","руб","денеж",
        "amount","money","price","fee","cost","charge","balance",
    } {
        if strings.Contains(name,fragment) {return true}
    }
    return false
}

func rawResultField(row ListItem, field string) (any,bool) {
    for name,value:=range row.Raw {
        if strings.EqualFold(name,field) {return value,true}
    }
    return nil,false
}
func sqlResultFieldType(rows []ListItem, field string) string {
    for _,row:=range rows {
        for name,sqlType:=range row.ColumnTypes {
            if strings.EqualFold(name,field) && sqlType!="" {
                return sqlType
            }
        }
    }
    return ""
}
func looksLikeMoneyDecimal(raw any) bool {
    var source string
    switch v:=raw.(type){
    case string: source=v
    case []byte: source=string(v)
    default:return false
    }
    source=strings.TrimSpace(source)
    if source=="" {return false}
    if strings.HasPrefix(source,"-") || strings.HasPrefix(source,"+") {
        source=source[1:]
    }
    point:=strings.LastIndexByte(source,'.')
    if point<=0 || len(source)-point-1!=4 {return false}
    for i,c:=range source {
        if i==point {continue}
        if c<'0' || c>'9' {return false}
    }
    return true
}

// InferColumnPresentation never bases a numeric inference on a column title
// alone. All nonempty source values must parse numerically for casted text.
// DECIMAL/INT/etc remain numeric even when their values are NULL; MONEY
// retains monetary status. A SQL text column with all values of the form
// decimal(19,4) is recognized as an amount for the legacy/customized queries.
func InferColumnPresentation(rows []ListItem,field string) ColumnPresentation {
    sqlType:=sqlResultFieldType(rows,field)
    if isMoneySQLType(sqlType) {
        return ColumnPresentation{Kind:FilterNumber,Money:true}
    }
    if isDateSQLType(sqlType) {
        return ColumnPresentation{Kind:FilterDate}
    }
    if isNumericSQLType(sqlType) {
        return ColumnPresentation{Kind:FilterNumber,Money:isMonetaryAlias(field) &&
            (strings.EqualFold(sqlType,"DECIMAL")||strings.EqualFold(sqlType,"NUMERIC"))}
    }

    sawNumber:=false
    sawNonblank:=false
    allNumeric:=true
    fourDecimal:=false
    sawRawDate:=false
    sawRawNumber:=false
    for _,row:=range rows {
        raw,ok:=rawResultField(row,field)
        if !ok || raw==nil {continue}
        if t,ok:=raw.(time.Time);ok && !t.IsZero() {
            sawRawDate=true
            continue
        }
        source:=strings.TrimSpace(sqlText(raw))
        if source=="" {continue}
        sawNonblank=true
        if _,err:=parseNumber(raw);err!=nil {
            allNumeric=false
            continue
        }
        sawNumber=true
        if looksLikeMoneyDecimal(raw) {fourDecimal=true}
        switch raw.(type){
        case int,int8,int16,int32,int64,uint,uint8,uint16,uint32,uint64,
            float32,float64,bool:
            sawRawNumber=true
        }
    }
    if sawRawDate {
        return ColumnPresentation{Kind:FilterDate}
    }
    if sawNumber && allNumeric && !isTextSQLType(sqlType) {
        return ColumnPresentation{Kind:FilterNumber,
            Money:isMonetaryAlias(field) || fourDecimal}
    }
    if sawNumber && allNumeric && isTextSQLType(sqlType) &&
        (fourDecimal || isMonetaryAlias(field)) {
        return ColumnPresentation{Kind:FilterNumber,Money:true}
    }
    if sawNumber && allNumeric && sawRawNumber {
        return ColumnPresentation{Kind:FilterNumber}
    }
    // No rows or all NULLs: a known financial alias (including the
    // historical Amount name and Russian Абон.плата) must offer numeric
    // operators even when a custom SELECT casts it to NVARCHAR.
    // But never force money on a nonempty column containing actual words.
    if !sawNonblank && isMonetaryAlias(field) {
        return ColumnPresentation{Kind:FilterNumber,Money:true}
    }
    return ColumnPresentation{Kind:FilterText}
}

// FormatListMoneyN2 changes display text only. Numeric comparisons and
// sorting continue to use the original unrounded ListItem.Raw values.
func FormatListMoneyN2(rows []ListItem) {
    if len(rows)==0 {return}
    for field:=range rows[0].Raw {
        if !InferColumnPresentation(rows,field).Money {continue}
        for i:=range rows {
            raw,ok:=rawResultField(rows[i],field)
            if !ok {continue}
            text,ok:=MoneyDisplayN2(raw)
            if !ok {continue}
            for k:=range rows[i].Values {
                if strings.EqualFold(k,field) {
                    rows[i].Values[k]=text
                    break
                }
            }
        }
    }
}

// Defensive type assertion used in tests and diagnostics; no parsing should
// silently convert an unrelated text column to numeric based on its name.
func ValidateColumnPresentation(rows []ListItem,field string) error {
    p:=InferColumnPresentation(rows,field)
    if p.Money && p.Kind!=FilterNumber {
        return fmt.Errorf("денежное поле %q должно иметь числовой тип",field)
    }
    return nil
}
