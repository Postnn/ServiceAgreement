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

// The two filter modes have different natural heights. A single large
// fixed-size dialog leaves empty vertical bands when the checkbox list is
// hidden, so keep a generous width but shrink only the condition mode.
const (
    filterPopupWidth = 680
    filterConditionHeight = 405
    filterValueListHeight = 570
)

func filterPopupSize(conditions bool) walk.Size {
    height:=filterValueListHeight
    if conditions {height=filterConditionHeight}
    return walk.Size{Width:filterPopupWidth,Height:height}
}

// Preserve the center of the dialog while switching between two modes
// instead of expanding or shrinking exclusively toward the lower-right.
func filterPopupCenteredBounds(current walk.Rectangle, conditions bool) walk.Rectangle {
    target:=filterPopupSize(conditions)
    return walk.Rectangle{
        X:current.X+(current.Width-target.Width)/2,
        Y:current.Y+(current.Height-target.Height)/2,
        Width:target.Width,Height:target.Height,
    }
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

// filterSummaryText renders a full multiline explanation below the table.
// Show it if at least one typed condition is active, and include checkbox
// filters in other columns so the whole AND/OR expression is understandable.
func filterSummaryText(model *tableModel) string {
    if model == nil || model.conditions.Count()==0 {return ""}
    parts:=make([]string,0,model.conditions.Count()+model.filters.Count())
    for _,col:=range model.columns {
        if cond,ok:=model.conditions.Get(col.Field);ok {
            parts=append(parts,col.Title+": "+cond.Description())
        }else if model.filters.Active(col.Field) {
            selected:=model.filters[strings.ToLower(strings.TrimSpace(col.Field))]
            parts=append(parts,fmt.Sprintf("%s: выбрано %d уникальных значений",col.Title,len(selected)))
        }
    }
    if len(parts)==0 {return ""}
    return "Действующие фильтры (ИЛИ внутри колонки, И между колонками):\n"+
        strings.Join(parts,"\nИ ")
}

func (a *application) updateFilteredStatus(unconfigured bool) {
    model,status,details:=a.model,a.status,a.filterDetails
    prefix:="Договоров с параметрами"
    if unconfigured {
        model,status,details=a.withoutModel,a.withoutStatus,a.withoutFilterDetails
        prefix="Договоров без параметров"
    }
    if model==nil || status==nil {return}
    msg:=fmt.Sprintf("%s: %d из %d",prefix,len(model.items),len(model.allItems))
    if count:=model.filters.Count()+model.conditions.Count();count>0 {
        msg+=fmt.Sprintf(" · фильтров: %d",count)
    }
    status.SetText(msg)
    if details!=nil {
        summary:=filterSummaryText(model)
        if summary=="" {
            details.SetVisible(false)
            _=details.SetText("")
        }else{
            // Native Win32 EDIT controls expect CRLF to display new lines.
            _=details.SetText(strings.ReplaceAll(summary,"\n","\r\n"))
            details.SetVisible(true)
        }
    }
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
// editorInputVisibility is independent of Windows controls and therefore
// unit-testable. Disabled, grey input boxes should never remain visible.
func editorInputVisibility(op core.FilterOperator,active bool) (showValue,showBoundary bool) {
    if !active {return false,false}
    showValue=op!=core.FilterAboveAverage && op!=core.FilterBelowAverage
    showBoundary=op==core.FilterBetween || op==core.FilterNotBetween
    return
}

func filterOperatorIndex(ops []operatorChoice, op core.FilterOperator) int {
    for i,option:=range ops {
        if option.Operator==op {return i}
    }
    return 0
}

// Clearing the active condition must immediately update the loaded view,
// without closing the filter dialog or touching filters on other columns.
func clearOneColumnFilter(model *tableModel, field string) {
    if model==nil {return}
    model.conditions.Clear(field)
    model.filters.Clear(field)
    model.applyFilters()
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
    firstLabels:=[]string{"— не использовать —"}
    secondLabels:=[]string{"— не использовать —"}
    for i,op:=range ops {
        labels[i]=op.Caption
        firstLabels=append(firstLabels,op.Caption)
        secondLabels=append(secondLabels,op.Caption)
    }
    modeIndex:=0
    if activeCondition {modeIndex=1}
    initialOp:=1 // convenient default when enabling condition mode
    if kind==core.FilterText {initialOp=filterOperatorIndex(ops,core.FilterContains)+1}
    initialSecondOp:=0 // second OR branch absent by default
    if activeCondition && previous.Kind==kind {
        initialOp=filterOperatorIndex(ops,previous.Operator)+1
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
    var conditionHint,currentFilterLabel,valueHeader,boundaryHeader *walk.Label

    textStatus:="Текущий фильтр: не установлен"
    if activeCondition {
        textStatus="Текущий фильтр: "+previous.Description()
    }else if activeValues {
        textStatus=fmt.Sprintf("Текущий фильтр: выбрано %d уникальных значений",len(allowed))
    }

    currentFirstOperation:=func() (core.FilterOperator,bool) {
        if operationOne==nil {
            if initialOp<=0 {return "",false}
            return ops[initialOp-1].Operator,true
        }
        idx:=operationOne.CurrentIndex()
        if idx<=0 || idx>len(ops) {return "",false}
        return ops[idx-1].Operator,true
    }
    currentSecondOperation:=func() (core.FilterOperator,bool) {
        if operationTwo==nil {return "",false}
        idx:=operationTwo.CurrentIndex()
        if idx<=0 || idx>len(ops) {return "",false}
        return ops[idx-1].Operator,true
    }
    // Hide irrelevant inputs entirely. Disabled empty grey boxes previously
    // confused users: a range boundary is only visible for Between/NotBetween,
    // and the second value appears only with an active second OR operator.
    updateOperation:=func() {
        firstOp,firstActive:=currentFirstOperation()
        secondOp,secondActive:=currentSecondOperation()
        showFirstValue,showFirstBoundary:=editorInputVisibility(firstOp,firstActive)
        showSecondValue,showSecondBoundary:=editorInputVisibility(secondOp,secondActive)
        if valueOne!=nil {valueOne.SetVisible(showFirstValue)}
        if valueOneBoundary!=nil {valueOneBoundary.SetVisible(showFirstBoundary)}
        if valueTwo!=nil {valueTwo.SetVisible(showSecondValue)}
        if valueTwoBoundary!=nil {valueTwoBoundary.SetVisible(showSecondBoundary)}
        if valueHeader!=nil {
            valueHeader.SetVisible(showFirstValue || showSecondValue)
        }
        if boundaryHeader!=nil {
            boundaryHeader.SetVisible(showFirstBoundary || showSecondBoundary)
        }
        if firstActive && firstOp==core.FilterTop && valueOne!=nil &&
            strings.TrimSpace(valueOne.Text())=="" {
            _=valueOne.SetText("10")
        }
        if secondActive && secondOp==core.FilterTop && valueTwo!=nil &&
            strings.TrimSpace(valueTwo.Text())=="" {
            _=valueTwo.SetText("10")
        }
        if conditionHint!=nil {
            conditionHint.SetText("Внутри колонки — ИЛИ; между колонками — И.")
        }
    }
    // Entire unused mode is hidden rather than shown disabled.
    updateMode:=func() {
        if modeBox==nil || uniqueGroup==nil || conditionGroup==nil {return}
        useConditions:=modeBox.CurrentIndex()==1
        uniqueGroup.SetVisible(!useConditions)
        conditionGroup.SetVisible(useConditions)
        if useConditions {updateOperation()}
        // Layout is re-evaluated with only the visible group, then resize
        // the form to the mode's content height. No large blank area above
        // the tabs or below the action buttons.
        if dlg!=nil {
            _=dlg.SetBounds(filterPopupCenteredBounds(dlg.Bounds(),useConditions))
        }
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
            firstOp,firstActive:=currentFirstOperation()
            secondOp,secondActive:=currentSecondOperation()
            if !firstActive {
                if secondActive {
                    walk.MsgBox(dlg,"Некорректный фильтр",
                        "Сначала выберите первое условие, либо отключите второе.",
                        walk.MsgBoxIconError)
                    return
                }
                model.conditions.Clear(field)
                model.filters.Clear(field)
            }else{
                cond,err:=composeORFilter(
                    kind,firstOp,valueOne.Text(),valueOneBoundary.Text(),
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
                model.filters.Clear(field)
            }
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
    // Unlike "Сбросить колонку", this button deliberately keeps the
    // dialog open. The clearing is applied immediately, so the table and
    // multiline summary behind the dialog update before any new condition.
    clearConditionInDialog:=func() {
        clearOneColumnFilter(model,field)
        a.updateColumnFilterTitles(table,model)
        a.updateFilteredStatus(unconfigured)
        if currentFilterLabel!=nil {
            _=currentFilterLabel.SetText("Текущий фильтр: не установлен")
        }
        if operationOne!=nil {_=operationOne.SetCurrentIndex(0)}
        if operationTwo!=nil {_=operationTwo.SetCurrentIndex(0)}
        for _,edit:=range []*walk.LineEdit{
            valueOne,valueOneBoundary,valueTwo,valueTwoBoundary,
        } {
            if edit!=nil {_=edit.SetText("")}
        }
        updateOperation()
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
    initialSize:=filterPopupSize(modeIndex==1)
    err:=Dialog{
        AssignTo:&dlg,
        Title:"Фильтр: "+col.Title,
        Icon:a.appIcon,
        MinSize:Size{Width:660,Height:380},
        Size:Size{Width:initialSize.Width,Height:initialSize.Height},
        Layout:VBox{Margins:Margins{Left:12,Top:10,Right:12,Bottom:10},Spacing:6},
        Children:[]Widget{
            Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                PushButton{Text:"↑  По возрастанию",OnClicked:func(){sortAndClose(false)}},
                PushButton{Text:"↓  По убыванию",OnClicked:func(){sortAndClose(true)}},
                HSpacer{},
            }},
            Label{AssignTo:&currentFilterLabel,Text:textStatus,
                EllipsisMode:EllipsisEnd,
                ToolTipText:textStatus,MaxSize:Size{Width:630}},
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
                Visible:modeIndex==0,
                MinSize:Size{Height:335},
                Layout:VBox{Margins:Margins{Left:8,Top:10,Right:8,Bottom:8},Spacing:5},
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
                        MinSize:Size{Height:245},
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
                Visible:modeIndex==1,
                MinSize:Size{Height:155},MaxSize:Size{Height:192},
                Layout:VBox{Margins:Margins{Left:8,Top:10,Right:8,Bottom:8},Spacing:5},
                Children:[]Widget{
                    // Fixed column widths guarantee that every operator and
                    // its operands line up in two rows at all dialog sizes.
                    Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                        Label{Text:"",MinSize:Size{Width:112},MaxSize:Size{Width:112}},
                        Label{Text:"Оператор",MinSize:Size{Width:190},MaxSize:Size{Width:190}},
                        Label{AssignTo:&valueHeader,Text:"Значение",MinSize:Size{Width:130},MaxSize:Size{Width:130}},
                        Label{AssignTo:&boundaryHeader,Text:"До (для диапазона)",Visible:false,
                            ToolTipText:"Показывается только для «Между» / «Вне диапазона»",
                            MinSize:Size{Width:130},MaxSize:Size{Width:130}},
                        HSpacer{},
                    }},
                    Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                        Label{Text:"Условие 1:",MinSize:Size{Width:112},MaxSize:Size{Width:112}},
                        ComboBox{AssignTo:&operationOne,Model:firstLabels,
                            CurrentIndex:initialOp,
                            MinSize:Size{Width:190},MaxSize:Size{Width:190},
                            OnCurrentIndexChanged:updateOperation},
                        LineEdit{AssignTo:&valueOne,Text:initialValue,
                            CueBanner:filterKindInputExample(kind),
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:filterKindInputExample(kind)},
                        LineEdit{AssignTo:&valueOneBoundary,Text:initialValueBoundary,Visible:false,
                            CueBanner:"До",
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
                        LineEdit{AssignTo:&valueTwo,Text:initialSecondValue,Visible:false,
                            CueBanner:filterKindInputExample(kind),
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:"Необязательное второе условие (ИЛИ)"},
                        LineEdit{AssignTo:&valueTwoBoundary,Text:initialSecondBoundary,Visible:false,
                            CueBanner:"До",
                            MinSize:Size{Width:130},MaxSize:Size{Width:130},
                            ToolTipText:"Вторая граница для «Между» во втором условии"},
                        HSpacer{},
                    }},
                    Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                        Label{AssignTo:&conditionHint,
                            Text:"Внутри колонки — ИЛИ; между колонками — И."},
                        HSpacer{},
                        PushButton{
                            Text:"Очистить условие",
                            ToolTipText:"Сразу удалить фильтр этой колонки, не закрывая окно. Другие колонки не изменятся.",
                            OnClicked:clearConditionInDialog,
                        },
                    }},
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
