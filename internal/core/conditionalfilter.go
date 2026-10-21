package core

import (
    "fmt"
    "math/big"
    "sort"
    "strconv"
    "strings"
    "time"
)

// FilterKind is derived from SQL Server result-column metadata or raw values,
// not from a substring of the column heading.
type FilterKind string

const (
    FilterText FilterKind = "text"
    FilterNumber FilterKind = "number"
    FilterDate FilterKind = "date"
)

type FilterOperator string

const (
    FilterEqual FilterOperator = "eq"
    FilterNotEqual FilterOperator = "ne"
    FilterContains FilterOperator = "contains"
    FilterNotContains FilterOperator = "not_contains"
    FilterStarts FilterOperator = "starts"
    FilterEnds FilterOperator = "ends"
    FilterGreater FilterOperator = "gt"
    FilterGreaterEqual FilterOperator = "gte"
    FilterLess FilterOperator = "lt"
    FilterLessEqual FilterOperator = "lte"
    FilterBetween FilterOperator = "between"
    FilterNotBetween FilterOperator = "not_between"
    FilterTop FilterOperator = "top"
    FilterAboveAverage FilterOperator = "above_avg"
    FilterBelowAverage FilterOperator = "below_avg"
)

// FilterCondition is a local, typed comparison against one SQL list column.
// Only display/filter state uses this value; no additional SQL is executed.
type FilterCondition struct {
    Kind FilterKind
    Operator FilterOperator
    Value string
    Value2 string
}

// FilterConditionState stores mutually exclusive alternatives to checkbox
// selection for the same column. The caller clears the old checkboxes when a
// condition is applied and clears the condition when checkbox selection wins.
type FilterConditionState map[string]FilterCondition

func (s FilterConditionState) Active(field string) bool {
    _,found:=s[normalizeColumnAlias(field)]
    return found
}
func (s FilterConditionState) Get(field string) (FilterCondition,bool) {
    v,ok:=s[normalizeColumnAlias(field)]
    return v,ok
}
func (s FilterConditionState) Set(field string, c FilterCondition) error {
    if err:=c.Validate();err!=nil {return err}
    if s==nil {return fmt.Errorf("не инициализирован список условий фильтра")}
    s[normalizeColumnAlias(field)] = c
    return nil
}
func (s FilterConditionState) Clear(field string) { delete(s,normalizeColumnAlias(field)) }
func (s FilterConditionState) ClearAll() { for k:=range s {delete(s,k)} }
func (s FilterConditionState) Count() int {return len(s)}
func (s FilterConditionState) KeepFields(columns []ListColumn) {
    known:=map[string]bool{}
    for _,c:=range columns { known[normalizeColumnAlias(c.Field)]=true }
    for k:=range s { if !known[k] {delete(s,k)} }
}

func DetectFilterKind(rows []ListItem, field string) FilterKind {
    // The legacy Amount alias is textual SQL but semantically money.
    if strings.EqualFold(field,"Amount") {return FilterNumber}
    for _,row:=range rows {
        for name,dbType:=range row.ColumnTypes {
            if strings.EqualFold(name,field) {
                typ:=strings.ToUpper(strings.TrimSpace(dbType))
                switch typ {
                case "MONEY","SMALLMONEY","DECIMAL","NUMERIC","FLOAT","REAL",
                    "TINYINT","SMALLINT","INT","BIGINT","BIT":
                    return FilterNumber
                case "DATE","DATETIME","DATETIME2","SMALLDATETIME","DATETIMEOFFSET":
                    return FilterDate
                case "VARCHAR","NVARCHAR","CHAR","NCHAR","TEXT","NTEXT",
                    "UNIQUEIDENTIFIER","XML":
                    return FilterText
                }
            }
        }
    }
    for _,row:=range rows {
        raw:=row.SQLValue(field)
        if raw==nil {continue}
        switch raw.(type) {
        case time.Time: return FilterDate
        case int,int8,int16,int32,int64,uint,uint8,uint16,uint32,uint64,
            float32,float64,*big.Rat,*big.Float,bool:
            return FilterNumber
        default: return FilterText
        }
    }
    return FilterText
}

func parseNumber(raw any) (*big.Rat,error) {
    if raw==nil {return nil,fmt.Errorf("пустое значение")}
    switch x:=raw.(type) {
    case *big.Rat:
        if x==nil {return nil,fmt.Errorf("пустое значение")}
        return new(big.Rat).Set(x),nil
    case bool:
        if x {return big.NewRat(1,1),nil}
        return big.NewRat(0,1),nil
    }
    var t string
    switch x:=raw.(type) {
    case []byte: t=string(x)
    case float64: t=strconv.FormatFloat(x,'g',-1,64)
    case float32: t=strconv.FormatFloat(float64(x),'g',-1,32)
    default: t=fmt.Sprint(raw)
    }
    t=strings.TrimSpace(t)
    t=strings.NewReplacer(" ","","\u00a0","","\u202f","","\t","","\r","","\n","",",",".").Replace(t)
    if t=="" {return nil,fmt.Errorf("пустое число")}
    if v,ok:=new(big.Rat).SetString(t);ok {return v,nil}
    // Float literals in scientific notation are legitimate SQL float values.
    if f,err:=strconv.ParseFloat(t,64);err==nil {
        v:=new(big.Rat).SetFloat64(f)
        if v!=nil {return v,nil}
    }
    return nil,fmt.Errorf("неверное число %q",t)
}

func parseDateValue(v any) (time.Time,error) {
    if t,ok:=v.(time.Time);ok {
        y,m,d:=t.Date()
        return time.Date(y,m,d,0,0,0,0,time.UTC),nil
    }
    if v==nil {return time.Time{},fmt.Errorf("пустая дата")}
    var t string
    if b,ok:=v.([]byte);ok {t=strings.TrimSpace(string(b))
    }else{t=strings.TrimSpace(fmt.Sprint(v))}
    for _,layout:=range []string{
        "02.01.2006","2.1.2006","02.01.2006 15:04:05","02.01.2006 15:04",
        "2006-01-02","2006-01-02 15:04:05","2006-01-02T15:04:05",
        time.RFC3339Nano,time.RFC3339,
    } {
        if parsed,err:=time.Parse(layout,t);err==nil {
            y,m,d:=parsed.Date()
            return time.Date(y,m,d,0,0,0,0,time.UTC),nil
        }
    }
    return time.Time{},fmt.Errorf("неверная дата %q; укажите ДД.ММ.ГГГГ",t)
}

func (c FilterCondition) Validate() error {
    switch c.Kind {
    case FilterText:
        switch c.Operator {
        case FilterEqual,FilterNotEqual,FilterContains,FilterNotContains,FilterStarts,FilterEnds:
            // Empty string is meaningful: match blank text.
            return nil
        }
    case FilterNumber:
        switch c.Operator {
        case FilterTop:
            n,err:=strconv.Atoi(strings.TrimSpace(c.Value))
            if err!=nil || n<1 || n>1000 {
                return fmt.Errorf("количество наибольших значений должно быть от 1 до 1000")
            }
            return nil
        case FilterAboveAverage,FilterBelowAverage: return nil
        case FilterEqual,FilterNotEqual,FilterGreater,FilterGreaterEqual,
            FilterLess,FilterLessEqual,FilterBetween,FilterNotBetween:
            if _,err:=parseNumber(c.Value);err!=nil {return err}
            if c.Operator==FilterBetween || c.Operator==FilterNotBetween {
                if _,err:=parseNumber(c.Value2);err!=nil {return err}
            }
            return nil
        }
    case FilterDate:
        switch c.Operator {
        case FilterEqual,FilterNotEqual,FilterGreater,FilterGreaterEqual,
            FilterLess,FilterLessEqual,FilterBetween,FilterNotBetween:
            if _,err:=parseDateValue(c.Value);err!=nil {return err}
            if c.Operator==FilterBetween || c.Operator==FilterNotBetween {
                if _,err:=parseDateValue(c.Value2);err!=nil {return err}
            }
            return nil
        }
    }
    return fmt.Errorf("неподдерживаемое условие фильтра %q для типа %q",c.Operator,c.Kind)
}

func (c FilterCondition) Description() string {
    var name string
    switch c.Operator {
    case FilterEqual:name="равно"
    case FilterNotEqual:name="не равно"
    case FilterContains:name="содержит"
    case FilterNotContains:name="не содержит"
    case FilterStarts:name="начинается с"
    case FilterEnds:name="заканчивается на"
    case FilterGreater:
        if c.Kind==FilterDate {name="позже"}else{name="больше"}
    case FilterGreaterEqual:
        if c.Kind==FilterDate {name="не раньше"}else{name="больше или равно"}
    case FilterLess:
        if c.Kind==FilterDate {name="раньше"}else{name="меньше"}
    case FilterLessEqual:
        if c.Kind==FilterDate {name="не позже"}else{name="меньше или равно"}
    case FilterBetween:name="между"
    case FilterNotBetween:name="вне диапазона"
    case FilterTop:name="первые"
    case FilterAboveAverage:return "выше среднего"
    case FilterBelowAverage:return "ниже среднего"
    default:return "неизвестное условие"
    }
    if c.Operator==FilterBetween || c.Operator==FilterNotBetween {
        return name+" «"+strings.TrimSpace(c.Value)+"» и «"+strings.TrimSpace(c.Value2)+"»"
    }
    if c.Operator==FilterTop {return name+" "+strings.TrimSpace(c.Value)+" значений"}
    return name+" «"+strings.TrimSpace(c.Value)+"»"
}

func compareNumber(op FilterOperator, cmp int) bool {
    switch op {
    case FilterEqual:return cmp==0
    case FilterNotEqual:return cmp!=0
    case FilterGreater:return cmp>0
    case FilterGreaterEqual:return cmp>=0
    case FilterLess:return cmp<0
    case FilterLessEqual:return cmp<=0
    }
    return false
}
func (c FilterCondition) Match(row ListItem, field string) bool {
    if c.Kind==FilterText {
        input:=strings.ToLower(row.Field(field))
        value:=strings.ToLower(c.Value)
        switch c.Operator {
        case FilterEqual:return input==value
        case FilterNotEqual:return input!=value
        case FilterContains:return strings.Contains(input,value)
        case FilterNotContains:return !strings.Contains(input,value)
        case FilterStarts:return strings.HasPrefix(input,value)
        case FilterEnds:return strings.HasSuffix(input,value)
        }
        return false
    }
    if c.Kind==FilterNumber {
        actual,err:=parseNumber(row.SQLValue(field))
        if err!=nil {return false}
        expected,err:=parseNumber(c.Value)
        if err!=nil {return false}
        cmp:=actual.Cmp(expected)
        if c.Operator==FilterBetween || c.Operator==FilterNotBetween {
            max,err:=parseNumber(c.Value2)
            if err!=nil {return false}
            lower,upper:=expected,max
            if lower.Cmp(upper)>0 {lower,upper=upper,lower}
            inside:=actual.Cmp(lower)>=0 && actual.Cmp(upper)<=0
            if c.Operator==FilterNotBetween {return !inside}
            return inside
        }
        return compareNumber(c.Operator,cmp)
    }
    if c.Kind==FilterDate {
        actual,err:=parseDateValue(row.SQLValue(field))
        if err!=nil {return false}
        expected,err:=parseDateValue(c.Value)
        if err!=nil {return false}
        cmp:=actual.Compare(expected)
        if c.Operator==FilterBetween || c.Operator==FilterNotBetween {
            max,err:=parseDateValue(c.Value2)
            if err!=nil {return false}
            lower,upper:=expected,max
            if lower.After(upper) {lower,upper=upper,lower}
            inside:=!actual.Before(lower)&&!actual.After(upper)
            if c.Operator==FilterNotBetween {return !inside}
            return inside
        }
        return compareNumber(c.Operator,cmp)
    }
    return false
}

// ApplyExcept combines different column predicates using AND. Dynamic
// Top-N/average predicates compute their threshold from the already
// restricted loaded rows. Iteration order is deterministic by alias.
func (s FilterConditionState) ApplyExcept(rows []ListItem, excluded string) []ListItem {
    if len(s)==0 {return rows}
    keys:=make([]string,0,len(s))
    exclude:=normalizeColumnAlias(excluded)
    for key:=range s {if key!=exclude {keys=append(keys,key)}}
    sort.Strings(keys)
    out:=rows
    for _,key:=range keys {
        c:=s[key]
        if len(out)==0 {return out}
        var threshold *big.Rat
        var dynamic bool
        if c.Kind==FilterNumber &&
            (c.Operator==FilterTop||c.Operator==FilterAboveAverage||c.Operator==FilterBelowAverage) {
            dynamic=true
            nums:=make([]*big.Rat,0,len(out))
            for _,row:=range out {
                if n,err:=parseNumber(row.SQLValue(key));err==nil {nums=append(nums,n)}
            }
            if len(nums)==0 {return []ListItem{}}
            if c.Operator==FilterTop {
                sort.Slice(nums,func(i,j int)bool{return nums[i].Cmp(nums[j])>0})
                count,_:=strconv.Atoi(strings.TrimSpace(c.Value))
                if count<1 {return []ListItem{}}
                if count>len(nums) {count=len(nums)}
                threshold=nums[count-1]
            } else {
                total:=new(big.Rat)
                for _,v:=range nums {total.Add(total,v)}
                threshold=total.Quo(total,big.NewRat(int64(len(nums)),1))
            }
        }
        filtered:=make([]ListItem,0,len(out))
        for _,row:=range out {
            keep:=false
            if dynamic {
                n,err:=parseNumber(row.SQLValue(key))
                if err==nil {
                    cmp:=n.Cmp(threshold)
                    switch c.Operator {
                    case FilterTop:keep=cmp>=0
                    case FilterAboveAverage:keep=cmp>0
                    case FilterBelowAverage:keep=cmp<0
                    }
                }
            } else {
                keep=c.Match(row,key)
            }
            if keep {filtered=append(filtered,row)}
        }
        out=filtered
    }
    return out
}
func (s FilterConditionState) Apply(rows []ListItem) []ListItem {
    return s.ApplyExcept(rows,"")
}
