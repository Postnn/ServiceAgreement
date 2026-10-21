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
}
func (m *filterValueModel) selectVisible(state bool) {
    for _,i:=range m.indices {m.checked[m.values[i].Value]=state}
    m.PublishRowsReset()
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
        title:=filterHeaderTitle(col,model.filters.Active(col.Field))
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
    if count:=model.filters.Count(); count>0 {
        msg+=fmt.Sprintf(" · активных фильтров: %d",count)
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
func (a *application) showColumnFilter(index int, unconfigured bool) {
    model,table:=a.model,a.table
    if unconfigured {model,table=a.withoutModel,a.withoutTable}
    if model==nil || table==nil || index<0 || index>=len(model.columns) {return}
    col:=model.columns[index]
    field:=col.Field
    source:=core.ColumnValues(model.allItems,field)
    selected:=make(map[string]bool,len(source))
    allowed,active:=model.filters[strings.ToLower(field)]
    for _,v:=range source {
        selected[v.Value]=!active || allowed[v.Value]
    }
    values:=newFilterValueModel(source,selected)
    var dlg *walk.Dialog
    var entries *walk.TableView
    var searchBox *walk.LineEdit
    var selectedLabel *walk.Label

    setStatus:=func(){
        if selectedLabel!=nil {
            _=selectedLabel.SetText(fmt.Sprintf("Выбрано: %d из %d значений. Всего записей: %d",
                values.selectedCount(),len(source),len(model.allItems)))
        }
    }
    values.changed=setStatus

    applyAndClose:=func(clearAll,clearColumn bool){
        if clearAll {
            model.filters.ClearAll()
        } else if clearColumn || values.selectedCount()==len(source) {
            model.filters.Clear(field)
        } else {
            model.filters.Set(field,values.selectedValues())
        }
        model.applyFilters()
        a.updateColumnFilterTitles(table,model)
        a.updateFilteredStatus(unconfigured)
        dlg.Accept()
    }
    sortAndClose:=func(desc bool) {
        if unconfigured {
            a.withoutSortField,a.withoutSortDescending=col.SortField,desc
        } else {
            a.sortField,a.sortDescending=col.SortField,desc
        }
        core.SortListItems(model.allItems,col.SortField,desc)
        model.applyFilters()
        a.updateFilteredStatus(unconfigured)
        dlg.Accept()
    }

    err:=Dialog{
        AssignTo:&dlg,
        Title:"Фильтр: "+col.Title,
        Icon:a.appIcon,
        MinSize:Size{Width:390,Height:425},
        Size:Size{Width:440,Height:500},
        Layout:VBox{Margins:Margins{Left:12,Top:10,Right:12,Bottom:12},Spacing:7},
        Children:[]Widget{
            Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                PushButton{Text:"↑  По возрастанию",OnClicked:func(){sortAndClose(false)}},
                PushButton{Text:"↓  По убыванию",OnClicked:func(){sortAndClose(true)}},
                HSpacer{},
            }},
            GroupBox{
                Title:"Значения колонки",
                Layout:VBox{Margins:Margins{Left:8,Top:10,Right:8,Bottom:8},Spacing:6},
                Children:[]Widget{
                    LineEdit{AssignTo:&searchBox,CueBanner:"Поиск среди значений...",
                        ToolTipText:"Введите фрагмент значения и нажмите Enter или перейдите к другому элементу.",
                        // Walk's EditingFinished event fires on Enter and WM_KILLFOCUS.
                        // Do not reset the checklist on every keystroke.
                        OnEditingFinished:func(){
                            if searchBox!=nil {values.search(searchBox.Text())}
                        }},
                    Composite{Layout:HBox{MarginsZero:true,Spacing:6},Children:[]Widget{
                        PushButton{Text:"Выбрать всё",OnClicked:func(){values.selectVisible(true)}},
                        PushButton{Text:"Снять всё",OnClicked:func(){values.selectVisible(false)}},
                        HSpacer{},
                    }},
                    TableView{
                        AssignTo:&entries,
                        Model:values,CheckBoxes:true,
                        AlternatingRowBG:true,MultiSelection:false,
                        NotSortableByHeaderClick:true,
                        Columns:[]TableViewColumn{
                            {Title:"Значение",Width:285},
                            {Title:"Строк",Width:75},
                        },
                    },
                    Label{AssignTo:&selectedLabel,Text:""},
                },
            },
            Composite{Layout:HBox{MarginsZero:true,Spacing:7},Children:[]Widget{
                PushButton{Text:"Сбросить колонку",OnClicked:func(){applyAndClose(false,true)}},
                PushButton{Text:"Сбросить все",OnClicked:func(){applyAndClose(true,false)}},
                HSpacer{},
                PushButton{Text:"Применить",OnClicked:func(){applyAndClose(false,false)}},
                PushButton{Text:"Отмена",OnClicked:func(){dlg.Cancel()}},
            }},
        },
    }.Create(a.win)
    if err!=nil {
        a.warn("Фильтр по колонке",err)
        return
    }
    setStatus()
    dlg.Run()
}
