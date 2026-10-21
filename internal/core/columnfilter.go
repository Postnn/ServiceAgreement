package core

import (
    "sort"
    "strings"
)

// ColumnFilterState stores value selections independently for each SQL alias.
// An absent alias means no restriction. An empty map intentionally matches no rows.
type ColumnFilterState map[string]map[string]bool

func normalizeColumnAlias(field string) string {
    return strings.ToLower(strings.TrimSpace(field))
}

func (s ColumnFilterState) Active(field string) bool {
    _, ok := s[normalizeColumnAlias(field)]
    return ok
}

func (s ColumnFilterState) Set(field string, allowed map[string]bool) {
    if s == nil {return}
    field = normalizeColumnAlias(field)
    if allowed == nil {
        delete(s,field)
        return
    }
    copyAllowed := make(map[string]bool,len(allowed))
    for v,ok:=range allowed {
        if ok {copyAllowed[v]=true}
    }
    s[field]=copyAllowed
}

func (s ColumnFilterState) Clear(field string) {
    delete(s,normalizeColumnAlias(field))
}

func (s ColumnFilterState) ClearAll() {
    for key:=range s {delete(s,key)}
}

func (s ColumnFilterState) Count() int {return len(s)}

func (s ColumnFilterState) KeepFields(columns []ListColumn) {
    available:=make(map[string]bool,len(columns))
    for _,col:=range columns {available[normalizeColumnAlias(col.Field)] = true}
    for name:=range s {
        if !available[name] {delete(s,name)}
    }
}

// Apply evaluates all active column restrictions using AND logic.
func (s ColumnFilterState) Apply(rows []ListItem) []ListItem {
    return s.ApplyExcept(rows, "")
}

// ApplyExcept evaluates every active filter EXCEPT the named column.
// This is essential for Excel-style cascading filter checklists: the
// available values in column B depend on filters for A and C, but not B's
// own current selection, so users can change or broaden B's selection.
func (s ColumnFilterState) ApplyExcept(rows []ListItem, excludedField string) []ListItem {
    if len(s)==0 { return rows }
    excluded := normalizeColumnAlias(excludedField)
    hasOther := false
    for alias := range s {
        if alias != excluded { hasOther = true; break }
    }
    if !hasOther { return rows }
    result := make([]ListItem, 0, len(rows))
    for _, row := range rows {
        accepted := true
        for alias, allowed := range s {
            if alias == excluded { continue }
            if !allowed[row.Field(alias)] {
                accepted = false
                break
            }
        }
        if accepted { result = append(result,row) }
    }
    return result
}

// FacetedValues returns distinct values and counts for a particular column,
// restricted by the OTHER columns' active filters. No SQL round-trip occurs;
// the source is the loaded list (up to list_limit).
func (s ColumnFilterState) FacetedValues(rows []ListItem, field string) []ColumnValueCount {
    return ColumnValues(s.ApplyExcept(rows, field), field)
}

// ColumnValueCount describes a distinct value in loaded SQL data.
type ColumnValueCount struct {
    Value string
    Count int
}

// ColumnValues enumerates each unique display value in the currently loaded
// result, independently of existing filters. This mirrors Excel filters over
// the current worksheet, but does not issue additional DB queries.
func ColumnValues(rows []ListItem, field string) []ColumnValueCount {
    count:=make(map[string]int)
    for _,row:=range rows {count[row.Field(field)]++}
    result:=make([]ColumnValueCount,0,len(count))
    for v,n:=range count {result=append(result,ColumnValueCount{Value:v,Count:n})}
    sort.SliceStable(result,func(i,j int)bool{
        a,b:=strings.ToLower(result[i].Value),strings.ToLower(result[j].Value)
        if a==b {return result[i].Value<result[j].Value}
        if result[i].Value=="" {return true}
        if result[j].Value=="" {return false}
        return a<b
    })
    return result
}
