//go:build windows

package main

import (
    "fmt"
    "math"
    "strconv"
    "strings"
    "time"

    "github.com/Postnn/ServiceAgreement/internal/core"
)

// Captions are written for people rather than as mathematical shorthand.
// Every visible caption ends with a colon, including range boundaries.
func filterOperandCaptions(kind core.FilterKind, operator core.FilterOperator) (first, second string) {
    switch operator {
    case core.FilterBetween,core.FilterNotBetween:
        if kind==core.FilterDate {
            return "Дата начала:", "Дата окончания:"
        }
        return "От:", "До:"
    case core.FilterAboveAverage,core.FilterBelowAverage:
        return "", ""
    case core.FilterTop:
        return "Количество:", ""
    }
    switch kind {
    case core.FilterDate:return "Дата:", ""
    case core.FilterNumber:return "Число:", ""
    default:return "Текст:", ""
    }
}

// Native NumberEdit works in float64, while SQL money is decimal(4).
// Keep original text verbatim unless the native control is actually edited.
// For numeric input originating from NumberEdit, validate that a conversion
// back to text at four-decimal money precision does not overflow the float
// precision threshold silently. The dialog continues to use the exact input
// after the value passes validation.
type filterEditorValue struct {
    Text string
    Number float64
    Date time.Time
    Original string
    NumberEdited bool
}

func filterNumberForEditor(raw string) float64 {
    cleaned:=strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "", ",", ".").Replace(strings.TrimSpace(raw))
    n,err:=strconv.ParseFloat(cleaned,64)
    if err!=nil || math.IsNaN(n) || math.IsInf(n,0) {return 0}
    return n
}

func filterDateForEditor(raw string) time.Time {
    raw=strings.TrimSpace(raw)
    for _,format:=range []string{"02.01.2006","2.1.2006","2006-01-02","2006-01-02T15:04:05",time.RFC3339} {
        if d,err:=time.Parse(format,raw);err==nil {
            return time.Date(d.Year(),d.Month(),d.Day(),0,0,0,0,time.Local)
        }
    }
    return time.Time{}
}

func filterEditorOperand(kind core.FilterKind, operator core.FilterOperator, val filterEditorValue) (string,error) {
    show,_:=editorInputVisibility(operator,true)
    if !show {
        return "",nil
    }
    switch kind {
    case core.FilterDate:
        if val.Date.IsZero() {
            return "",fmt.Errorf("выберите дату в календаре")
        }
        return val.Date.Format("02.01.2006"),nil
    case core.FilterNumber:
        if math.IsNaN(val.Number) || math.IsInf(val.Number,0) {
            return "",fmt.Errorf("введите конечное числовое значение")
        }
        if operator==core.FilterTop {
            if val.Number!=math.Trunc(val.Number) {
                return "",fmt.Errorf("количество первых значений должно быть целым")
            }
            return strconv.FormatFloat(val.Number,'f',0,64),nil
        }
        if !val.NumberEdited && strings.TrimSpace(val.Original)!="" {
            return val.Original,nil
        }
        // The native NumberEdit uses float64. A filter input larger than the
        // safe 4-decimal money range may silently lose user-entered digits.
        // Reject that edit rather than submit an inaccurate condition.
        const maxSafeMoneyValue = float64(9007199254740991) / 10000
        if math.Abs(val.Number)>maxSafeMoneyValue {
            return "",fmt.Errorf("число слишком велико для точного ввода с 4 знаками; задайте меньшее значение")
        }
        return strconv.FormatFloat(val.Number,'f',4,64),nil
    default:
        return val.Text,nil
    }
}

// Use two captions per row only when a range operator needs both operands.
func filterNeedsBoundary(op core.FilterOperator) bool {
    return op==core.FilterBetween || op==core.FilterNotBetween
}


// Human-readable range semantics. Conditions within the same field still
// combine with OR, and distinct fields combine with AND.
func filterRangeExplanation(kind core.FilterKind, first, second core.FilterOperator) string {
    if kind==core.FilterDate {
        if first==core.FilterNotBetween || second==core.FilterNotBetween {
            return "Вне периода: Меньше От ИЛИ Больше По."
        }
        if first==core.FilterBetween || second==core.FilterBetween {
            return "Между датами: Больше или равно От, Меньше или равно По."
        }
    }
    if first==core.FilterBetween || second==core.FilterBetween {
        return "Между числами: ≥ От, ≤ По (включительно)."
    }
    if first==core.FilterNotBetween || second==core.FilterNotBetween {
        return "Вне диапазона: < От ИЛИ > По."
    }
    return "Внутри колонки — ИЛИ; между колонками — И."
}
