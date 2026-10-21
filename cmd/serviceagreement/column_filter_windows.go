//go:build windows

package main

import (
    "fmt"
    "strings"

    "github.com/Postnn/ServiceAgreement/internal/core"
    "github.com/lxn/walk"
    . "github.com/lxn/walk/declarative"
)

// Excel-like column chooser: values and their counts come solely from rows
// already returned by the configured SQL query (subject to list_limit).
// This state is local to the dialog until Apply is clicked.
type filterValueModel struct {
    walk.TableModelBase
    values []core.ColumnValueCount
    indices []int
    checked map[string]bool
    changed func()
    // WM_KILLFOCUS and Enter can both emit EditingFinished. Repeating an
    // identical filter would reset the checklist during the first button
    // click and interfere with the native Windows mouse activation.
    lastSearch string
    searchInitialized bool
}

func newFilterValueModel(all []core.ColumnValueCount, checked map[string]bool) *filterValueModel {
    m:=&filterValueModel{values:all,checked:checked}
    m.search("")
    return m
}
func (m *filterValueModel) RowCount() int {return len(m.indices)}
func (m *filterValueModel) Value(row,col int) interface{} {
    if row<0 || row>=len(m.indices) {return ""}
    v:=m.values[m.indices[row]]
    switch col {
    case 0:
        if v.Value=="" {return "(Пустые)"}
        return v.Value
    case 1:return v.Count
    }
    return ""
}
func (m *filterValueModel) Checked(index int) bool {
    if index<0 || index>=len(m.indices) {return false}
    return m.checked[m.values[m.indices[index]].Value]
}
func (m *filterValueModel) SetChecked(index int, state bool) error {
    if index>=0 && index<len(m.indices) {
        m.checked[m.values[m.indices[index]].Value]=state
        m.PublishRowChanged(index)
        if m.changed!=nil {m.changed()}
    }
    return nil
}
func (m *filterValueModel) search(query string) {
    q:=strings.ToLower(strings.TrimSpace(query))
    if m.searchInitialized && q==m.lastSearch {
        return
    }
    m.searchInitialized=true
    m.lastSearch=q
    m.indices=m.indices[:0]
    for i,val:=range m.values {
        if q=="" || strings.Contains(strings.ToLower(val.Value),q) ||
            val.Value=="" && strings.Contains(strings.ToLower("(Пустые)"),q) {
            m.indices=append(m.indices,i)
        }
    }
    m.PublishRowsReset()
    // Search can change row content without changing row count. Walk may
    // suppress list invalidation in that case, so explicitly repaint.
    if m.changed!=nil {m.changed()}
}
// Bulk checkbox changes must publish RowsChanged, not RowsReset. Walk's
// RowsReset calls LVM_SETITEMCOUNT with LVSICF_NOINVALIDATEALL, so the
// Win32 ListView caches the old checkbox state until mouse hover. In
// contrast RowsChanged invalidates every visible state image immediately.
func (m *filterValueModel) selectVisible(state bool) {
    changed := false
    for _, i := range m.indices {
        value := m.values[i].Value
        if m.checked[value] == state { continue }
        m.checked[value] = state
        changed = true
    }
    if changed && len(m.indices)>0 {
        m.PublishRowsChanged(0,len(m.indices)-1)
    }
    if m.changed!=nil {m.changed()}
}
func (m *filterValueModel) selectedCount() int {
    result:=0
    for _,v:=range m.values {if m.checked[v.Value] {result++}}
    return result
}
func (m *filterValueModel) selectedValues() map[string]bool {
    selected:=make(map[string]bool)
    for _,v:=range m.values {if m.checked[v.Value] {selected[v.Value]=true}}
    return selected
}

func filterHeaderTitle(col core.ListColumn, active bool) string {
    if active { return col.Title + "  ●▾" }
    return col.Title + "  ▾"
}

func (a *application) updateColumnFilterTitles(table *walk.TableView, model *tableModel) {
    if table==nil || model==nil {return}
    for index, col:=range model.columns {
        if index>=table.Columns().Len() {break}
        title:=filterHeaderTitle(col,model.filters.Active(col.Field)||model.conditions.Active(col.Field))
        _=table.Columns().At(index).SetTitle(title)
    }
}

func (a *application) updateFilteredStatus(unconfigured bool) {
    model,status:=a.model,a.status
    prefix:="Договоров с параметрами"
    if unconfigured {
        model,status=a.withoutModel,a.withoutStatus
        prefix="Договоров без параметров"
    }
    if model==nil || status==nil {return}
    msg:=fmt.Sprintf("%s: %d из %d",prefix,len(model.items),len(model.allItems))
    if count:=model.filters.Count()+model.conditions.Count();count>0 {
        msg+=fmt.Sprintf(" · фильтров: %d",count)
    }
    var descriptions []string
    for _,col:=range model.columns {
        if cond,ok:=model.conditions.Get(col.Field);ok {
            descriptions=append(descriptions,col.Title+": "+cond.Description())
        }else if model.filters.Active(col.Field) {
            selected:=model.filters[strings.ToLower(strings.TrimSpace(col.Field))]
            descriptions=append(descriptions,fmt.Sprintf("%s: выбрано значений %d",col.Title,len(selected)))
        }
    }
    if len(descriptions)>0 {
        msg+=" · "+strings.Join(descriptions," И ")
    }
    status.SetText(msg)
}

// queueColumnFilter opens the modal only after Walk finishes processing
// the table header's native WM_NOTIFY / column-click message. Opening a modal
// from inside that notification can make the first subsequent mouse click
// activate a button without executing its OnClicked handler.
func (a *application) queueColumnFilter(index int, unconfigured bool) {
    if a.win == nil || a.filterPopupQueued || a.filterPopupOpen {
        return
    }
    a.filterPopupQueued = true
    a.win.Synchronize(func() {
        a.filterPopupQueued = false
        if a.filterPopupOpen {
            return
        }
        a.filterPopupOpen = true
        defer func() { a.filterPopupOpen = false }()
        a.showColumnFilter(index, unconfigured)
    })
}

// Each title displays an Excel-style down arrow (▾). Clicking the header
// opens a filter dialog that combines value checkboxes with sort controls.
// Previously clicking the header sorted immediately; those actions now live
// at the top of this contextual menu, as in Excel.
// A SQL field can be shown with Excel-style value selection OR with an
// operator-based condition. The inactive mode is disabled (mutually exclusive).
// Conditions across different columns, and across text/numeric/date types,
// are combined using AND.
type operatorChoice struct {
    Caption string
    Operator core.FilterOperator
}
func conditionOperators(kind core.FilterKind) []operatorChoice {
    switch kind {
    case core.FilterNumber:
        return []operatorChoice{
            {"Равно",core.FilterEqual},{"Не равно",core.FilterNotEqual},
            {"Больше",core.FilterGreater},{"Больше или равно",core.FilterGreaterEqual},
            {"Меньше",core.FilterLess},{"Меньше или равно",core.FilterLessEqual},
            {"Между (включительно)",core.FilterBetween},
            {"Вне диапазона",core.FilterNotBetween},
            {"Первые N значений",core.FilterTop},
            {"Выше среднего",core.FilterAboveAverage},
            {"Ниже среднего",core.FilterBelowAverage},
        }
    case core.FilterDate:
        return []operatorChoice{
            {"Равно (в этот день)",core.FilterEqual},
            {"Не равно",core.FilterNotEqual},
            {"Позже",core.FilterGreater},{"Не раньше",core.FilterGreaterEqual},
            {"Раньше",core.FilterLess},{"Не позже",core.FilterLessEqual},
            {"Между датами",core.FilterBetween},
            {"Вне периода",core.FilterNotBetween},
        }
    default:
        return []operatorChoice{
            {"Равно",core.FilterEqual},{"Не равно",core.FilterNotEqual},
            {"Содержит",core.FilterContains},
            {"Не содержит",core.FilterNotContains},
            {"Начинается с",core.FilterStarts},
            {"Заканчивается на",core.FilterEnds},
        }
    }
}
func filterKindCaption(kind core.FilterKind) string {
    switch kind {
    case core.FilterNumber:return "Числовой фильтр"
    case core.FilterDate:return "Фильтр по дате"
    default:return "Текстовый фильтр"
    }
}
func filterKindInputExample(kind core.FilterKind) string {
    switch kind {
    case core.FilterNumber:return "Число, например 1 234,56"
    case core.FilterDate:return "Дата, например 21.10.2026"
    default:return "Введите текст или его часть"
    }
}
func filterOperatorIndex(ops []operatorChoice, op core.FilterOperator) int {
    for i,option:=range ops {
        if option.Operator==op {return i}
    }
    return 0
}

// composeORFilter keeps a Between operator's second boundary separate
// from the optional SECOND condition. The latter joins with OR; SQL
// columns are then joined with AND by the filter state.
func composeORFilter(kind core.FilterKind,
    firstOperator core.FilterOperator, firstValue, firstBoundary string,
    secondEnabled bool, secondOperator core.FilterOperator, secondValue, secondBoundary string,
) (core.FilterCondition,error) {
    c:=core.FilterCondition{
        Kind:kind,Operator:firstOperator,
        Value:firstValue,Value2:firstBoundary,
    }
    if secondEnabled {
        alternative:=core.FilterCondition{
            Kind:kind,Operator:secondOperator,
            Value:secondValue,Value2:secondBoundary,
        }
        c.Additional=&alternative
    }
    return c,c.Validate()
}

func (a *application) showColumnFilter(index int, unconfigured bool) {
    model,table:=a.model,a.table
    if unconfigured {model,table=a.withoutModel,a.withoutTable}
    if model==nil || table==nil || index<0 || index>=len(model.columns) {return}
    if model.filters==nil {model.filters=make(core.ColumnFilterState)}
    if model.conditions==nil {model.conditions=make(core.FilterConditionState)}
    col:=model.columns[index]
    field:=col.Field
    kind:=core.DetectFilterKind(model.allItems,field)

    // Facets are limited by ALL OTHER columns, including both checkbox and
    // typed conditions. Excluding this column lets users edit its own filter.
    eligible:=model.filters.ApplyExcept(model.allItems,field)
    eligible=model.conditions.ApplyExcept(eligible,field)
    source:=core.ColumnValues(eligible,field)
    selected:=make(map[string]bool,len(source))
    allowed,activeValues:=model.filters[strings.ToLower(strings.TrimSpace(field))]
    previous,activeCondition:=model.conditions.Get(field)
    for _,v:=range source {
        selected[v.Value]=!activeValues || allowed[v.Value]
    }
    values:=newFilterValueModel(source,selected)

    ops:=conditionOperators(kind)
    labels:=make([]string,len(ops))
    secondLabels:=make([]string,0,len(ops)+1)
    secondLabels=append(secondLabels,"— не использовать —")
    for i,op:=range ops {
        labels[i]=op.Caption
        secondLabels=append(secondLabels,op.Caption)
    }
    modeIndex:=0
    if activeCondition {modeIndex=1}
    initialOp:=0
    initialSecondOp:=0 // index 0 explicitly means the optional OR branch is absent
    if activeCondition && previous.Kind==kind {
        initialOp=filterOperatorIndex(ops,previous.Operator)
        if previous.Additional!=nil && previous.Additional.Kind==kind {
            initialSecondOp=filterOperatorIndex(ops,previous.Additional.Operator)+1
        }
    }

    var dlg *walk.Dialog
    var entries *walk.TableView
    var searchBox *walk.LineEdit
    var selectedLabel *walk.Label
    var modeBox,operationOne,operationTwo *walk.ComboBox
    var valueOne,valueOneBoundary,valueTwo,valueTwoBoundary *walk.LineEdit
    var uniqueGroup,conditionGroup *walk.GroupBox
    var conditionHint *walk.Label

    textStatus:="Текущий фильтр: не установлен"
    if activeCondition {
        textStatus="Текущий фильтр: "+previous.Description()
    }else if activeValues {
        textStatus=fmt.Sprintf("Текущий фильтр: выбрано %d уникальных значений",len(allowed))
    }

    currentFirstOperation:=func() core.FilterOperator {
        if operationOne==nil {return ops[initialOp].Operator}
        idx:=operationOne.CurrentIndex()
        if idx<0 || idx>=len(ops) {return ops[0].Operator}
        return ops[idx].Operator
    }
    currentSecondOperation:=func() (core.FilterOperator,bool) {
        if operationTwo==nil {return "",false}
        idx:=operationTwo.CurrentIndex()
        if idx<=0 || idx>len(ops) {return "",false}
        return ops[idx-1].Operator,true
    }
    // Two identical form rows: operator, value, and optional second bound.
    // The second row is an OPTIONAL alternative and is disabled unless the
    // user selects an operator other than "— не использовать —".
    updateOperation:=func() {
        updateRow:=func(op core.FilterOperator,active bool,first,boundary *walk.LineEdit) {
            if first==nil || boundary==nil {return}
            wantsValue:=active &&
                op!=core.FilterAboveAverage && op!=core.FilterBelowAverage
            wantsRange:=active && (op==core.FilterBetween || op==core.FilterNotBetween)
            first.SetEnabled(wantsValue)
            boundary.SetEnabled(wantsRange)
            if active && op==core.FilterTop && strings.TrimSpace(first.Text())=="" {
                _=first.SetText("10")
            }
        }
        updateRow(currentFirstOperation(),true,valueOne,valueOneBoundary)
        if secondOp,active:=currentSecondOperation();active {
            updateRow(secondOp,true,valueTwo,valueTwoBoundary)
        }else{
            updateRow("",false,valueTwo,valueTwoBoundary)
        }
        if conditionHint!=nil {
            conditionHint.SetText("Условия этой колонки — ИЛИ; фильтры разных колонок — И.")
        }
    }
    updateMode:=func() {
        if modeBox==nil || uniqueGroup==nil || conditionGroup==nil {return}
        useConditions:=modeBox.CurrentIndex()==1
        uniqueGroup.SetEnabled(!useConditions)
        conditionGroup.SetEnabled(useConditions)
        if useConditions {updateOperation()}
    }
    setStatus:=func() {
        if selectedLabel!=nil {
            _=selectedLabel.SetText(fmt.Sprintf(
                "Выбрано: %d из %d значений · найдено строк: %d",
                values.selectedCount(),len(source),len(eligible)))
        }
    }
    values.changed=func() {
        setStatus()
        if entries!=nil {_=entries.Invalidate()}
    }

    applyAndClose:=func(clearAll,clearColumn bool) {
        if clearAll {
            model.filters.ClearAll()
            model.conditions.ClearAll()
        }else if clearColumn {
            model.filters.Clear(field)
            model.conditions.Clear(field)
        }else if modeBox!=nil && modeBox.CurrentIndex()==1 {
            secondOp,secondActive:=currentSecondOperation()
            cond,err:=composeORFilter(
                kind,currentFirstOperation(),valueOne.Text(),valueOneBoundary.Text(),
                secondActive,secondOp,valueTwo.Text(),valueTwoBoundary.Text(),
            )
            if err!=nil {
                walk.MsgBox(dlg,"Некорректный фильтр",err.Error(),walk.MsgBoxIconError)
                return
            }
            if err:=model.conditions.Set(field,cond);err!=nil {
                walk.MsgBox(dlg,"Некорректный фильтр",err.Error(),walk.MsgBoxIconError)
                return
            }
            // The checkbox selection for the same column becomes inactive.
            model.filters.Clear(field)
        }else{
            model.conditions.Clear(field)
            if values.selectedCount()==len(source) {
                model.filters.Clear(field)
            }else{
                model.filters.Set(field,values.selectedValues())
            }
        }
        model.applyFilters()
        a.updateColumnFilterTitles(table,model)
        a.updateFilteredStatus(unconfigured)
        dlg.Accept()
    }
    sortAndClose:=func(desc bool) {
        if unconfigured{
            a.withoutSortField,a.withoutSortDescending=col.SortField,desc
        }else{
            a.sortField,a.sortDescending=col.SortField,desc
        }
        core.SortListItems(model.allItems,col.SortField,desc)
        model.applyFilters()
        a.updateFilteredStatus(unconfigured)
        dlg.Accept()
    }

    initialValue:=""
    initialValueBoundary:=""
    initialSecondValue:=""
    initialSecondBoundary:=""
    if activeCondition && previous.Kind==kind {
        initialValue=previous.Value
        initialValueBoundary=previous.Value2
        if previous.Additional!=nil && previous.Additional.Kind==kind {
            initialSecondValue=previous.Additional.Value
            initialSecondBoundary=previous.Additional.Value2
        }
    }
    err:=Dialog{
        AssignTo:&dlg,
        Title:"Фильтр: "+col.Title,
        Icon:a.appIcon,
        MinSize:Size{Width:680,Height:600},
        Size:Size{Width:740,Height:780},
        Layout:VBox{Margins:Margins{Left:12,Top:10,Right:12,Bottom:12},Spacing:7},
        Children:[]Widget{
            Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                PushButton{Text:"↑  По возрастанию",OnClicked:func(){sortAndClose(false)}},
                PushButton{Text:"↓  По убыванию",OnClicked:func(){sortAndClose(true)}},
                HSpacer{},
            }},
            Label{Text:textStatus,ToolTipText:textStatus},
            Composite{Layout:HBox{MarginsZero:true,Spacing:8},Children:[]Widget{
                Label{Text:"Режим фильтрации:"},
                ComboBox{AssignTo:&modeBox,
                    Model:[]string{"Выбор уникальных значений",filterKindCaption(kind)},
                    CurrentIndex:modeIndex,MinSize:Size{Width:260},
                    OnCurrentIndexChanged:updateMode},
                HSpacer{},
            }},
            GroupBox{
                AssignTo:&uniqueGroup,
                Title:"Выбор уникальных значений",
                Enabled:modeIndex==0,
                Layout:VBox{Margins:Margins{Left:8,Top:10,Right:8,Bottom:8},Spacing:6},
                Children:[]Widget{
                    LineEdit{AssignTo:&searchBox,CueBanner:"Поиск среди значений...",
                        ToolTipText:"Поиск локально по подстроке, запускается по Enter или потере фокуса.",
                        OnEditingFinished:func(){
                            if searchBox!=nil{values.search(searchBox.Text())}
                        }},
                    Composite{Layout:HBox{MarginsZero:true,Spacing:6},Children:[]Widget{
                        PushButton{Text:"Выбрать всё",OnClicked:func(){values.selectVisible(true)},
                            ToolTipText:"Отметить все видимые значения (с учётом локального поиска)"},
                        PushButton{Text:"Снять всё",OnClicked:func(){values.selectVisible(false)},
                            ToolTipText:"Снять отметки с видимых значений (с учётом локального поиска)"},
                        HSpacer{},
                    }},
                    TableView{
                        AssignTo:&entries,
                        Model:values,CheckBoxes:true,
                        AlternatingRowBG:true,MultiSelection:false,
                        NotSortableByHeaderClick:true,ColumnsOrderable:false,
                        Columns:[]TableViewColumn{
                            {Title:"Значение",Width:375},
                            {Title:"Строк",Width:75},
                        },
                    },
                    Label{AssignTo:&selectedLabel,Text:""},
                },
            },
            GroupBox{
                AssignTo:&conditionGroup,
                Title:filterKindCaption(kind)+" — по введённому значению",
                Enabled:modeIndex==1,
                Layout:VBox{Margins:Margins{Left:8,Top:10,Right:8,Bottom:8},Spacing:6},
                Children:[]Widget{
                    // Fixed column widths guarantee that every operator and
                    // its operands line up in two rows at all dialog sizes.
                    Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                        Label{Text:"",MinSize:Size{Width:112},MaxSize:Size{Width:112}},
                        Label{Text:"Оператор",MinSize:Size{Width:190},MaxSize:Size{Width:190}},
                        Label{Text:"Значение",MinSize:Size{Width:130},MaxSize:Size{Width:130}},
                        Label{Text:"Вторая граница",MinSize:Size{Width:130},MaxSize:Size{Width:130}},
                        HSpacer{},
                    }},
                    Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                        Label{Text:"Условие 1:",MinSize:Size{Width:112},MaxSize:Size{Width:112}},
                        ComboBox{AssignTo:&operationOne,Model:labels,
                            CurrentIndex:initialOp,
                            MinSize:Size{Width:190},MaxSize:Size{Width:190},
                            OnCurrentIndexChanged:updateOperation},
                        LineEdit{AssignTo:&valueOne,Text:initialValue,
                            CueBanner:filterKindInputExample(kind),
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:filterKindInputExample(kind)},
                        LineEdit{AssignTo:&valueOneBoundary,Text:initialValueBoundary,
                            CueBanner:"До / граница",
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:"Вторая граница для операторов «Между» / «Вне диапазона»"},
                        HSpacer{},
                    }},
                    Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                        Label{Text:"ИЛИ условие 2:",MinSize:Size{Width:112},MaxSize:Size{Width:112}},
                        ComboBox{AssignTo:&operationTwo,Model:secondLabels,
                            CurrentIndex:initialSecondOp,
                            MinSize:Size{Width:190},MaxSize:Size{Width:190},
                            OnCurrentIndexChanged:updateOperation},
                        LineEdit{AssignTo:&valueTwo,Text:initialSecondValue,
                            CueBanner:filterKindInputExample(kind),
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:"Необязательное второе условие (ИЛИ)"},
                        LineEdit{AssignTo:&valueTwoBoundary,Text:initialSecondBoundary,
                            CueBanner:"До / граница",
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:"Вторая граница для «Между» во втором условии"},
                        HSpacer{},
                    }},
                    Label{AssignTo:&conditionHint,
                        Text:"Условия этой колонки — ИЛИ; фильтры разных колонок — И."},
                },
            },
            Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                PushButton{Text:"Сбросить колонку",
                    OnClicked:func(){applyAndClose(false,true)}},
                PushButton{Text:"Сбросить все",
                    OnClicked:func(){applyAndClose(true,false)}},
                HSpacer{},
                PushButton{Text:"Применить",
                    OnClicked:func(){applyAndClose(false,false)}},
                PushButton{Text:"Отмена",
                    OnClicked:func(){dlg.Cancel()}},
            }},
        },
    }.Create(a.win)
    if err!=nil {
        a.warn("Фильтр по колонке",err)
        return
    }
    updateMode()
    setStatus()
    dlg.Run()
}
