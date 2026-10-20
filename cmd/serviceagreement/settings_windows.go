//go:build windows

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/core"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func settingsError(owner walk.Form, title string, err error) {
	walk.MsgBox(owner, title, err.Error(), walk.MsgBoxIconError)
}

// errSQLUnavailable identifies connection failure, unlike invalid SELECT
// projections, and allows editing settings offline.
var errSQLUnavailable = errors.New("сервер SQL недоступен")

func synchronizeDraftColumns(owner walk.Form, cfg *core.Config) (bool, bool, error) {
	db, err := sql.Open("sqlserver", cfg.ConnectionString)
	if err != nil {
		return false, false, fmt.Errorf("%w: %v", errSQLUnavailable, err)
	}
	defer db.Close()
	ctx, cancel := cfg.QueryContext()
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		return false, false, fmt.Errorf("%w: %v", errSQLUnavailable, err)
	}
	return reconcileColumnSettings(owner, db, cfg)
}

// openSettings allows editing multi-line SQL without opening JSON manually.
// Changes are applied to a copy and persisted only on Save.
func (a *application) openSettings() {
	var dlg *walk.Dialog
	var connection, listLimit, lookupLimit, connectionTimeout *walk.LineEdit
	var sqlPicker *walk.ComboBox
	var sqlEdit *walk.TextEdit
	cfg := a.store.Config
	cfg.ListColumns = append([]core.ListColumn(nil), cfg.ListColumns...)
	selectedSQL := 0
	sqlNames := make([]string, len(core.SQLFields))
	for i, f := range core.SQLFields {
		sqlNames[i] = f.Caption
	}

	commitCurrentSQL := func() {
		if sqlEdit != nil && selectedSQL >= 0 && selectedSQL < len(core.SQLFields) {
			*cfg.SQLAt(selectedSQL) = sqlTextFromEditor(sqlEdit.Text())
		}
	}
	// Switching queries must not reformat or trim the user's SQL. Preserve
	// all indentation, spaces, tabs, blank lines, and comments as entered.
	onSQLChanged := func() {
		if sqlPicker == nil || sqlEdit == nil {
			return
		}
		next := sqlPicker.CurrentIndex()
		if next < 0 || next >= len(core.SQLFields) || next == selectedSQL {
			return
		}
		commitCurrentSQL()
		selectedSQL = next
		_ = sqlEdit.SetText(sqlTextForEditor(*cfg.SQLAt(selectedSQL)))
	}
	// Explicit button is the only place where SQL auto-formatting runs.
	formatCurrentSQL := func() {
		if sqlEdit != nil {
			_ = sqlEdit.SetText(sqlTextForEditor(core.FormatSQL(sqlTextFromEditor(sqlEdit.Text()))))
		}
	}
	collect := func() error {
		commitCurrentSQL()
		cfg.ConnectionString = strings.TrimSpace(connection.Text())
		var err error
		cfg.ListLimit, err = core.ParseIntField("Лимит списка", listLimit.Text())
		if err != nil {
			return err
		}
		cfg.LookupLimit, err = core.ParseIntField("Лимит поиска", lookupLimit.Text())
		if err != nil {
			return err
		}
        cfg.ConnectionTimeoutSeconds, err = core.ParseIntField("Таймаут SQL (сек.)", connectionTimeout.Text())
        if err != nil {
            return err
        }
		return cfg.Validate()
	}
	onTest := func() {
		dsn := strings.TrimSpace(connection.Text())
		if dsn == "" {
			settingsError(dlg, "Проверка подключения", fmt.Errorf("заполните строку подключения"))
			return
		}
		seconds, err := core.ParseIntField("Таймаут SQL (сек.)", connectionTimeout.Text())
        if err != nil || seconds < 1 || seconds > 3600 {
            settingsError(dlg, "Проверка подключения", fmt.Errorf("таймаут SQL должен быть целым числом от 1 до 3600 секунд"))
            return
        }
        db, err := sql.Open("sqlserver", dsn)
		if err == nil {
            draft := cfg
            draft.ConnectionTimeoutSeconds = seconds
			ctx, cancel := draft.QueryContext()
			err = db.PingContext(ctx)
			cancel()
			_ = db.Close()
		}
		if err != nil {
			settingsError(dlg, "Проверка подключения", err)
			return
		}
		walk.MsgBox(dlg, "Проверка подключения", "SQL Server доступен.", walk.MsgBoxIconInformation)
	}
	onSave := func() {
		if err := collect(); err != nil {
			settingsError(dlg, "Ошибка конфигурации", err)
			return
		}
		// Perform SQL schema synchronization BEFORE persisting the edited
		// queries. If SQL Server is offline, saving the draft is still allowed
		// after explicit confirmation; it will be synchronized when online.
		_, accepted, schemaErr := synchronizeDraftColumns(dlg, &cfg)
		if schemaErr != nil {
			// A valid connection but incompatible SQL queries should not be
			// silently saved. An unreachable server is handled separately.
			if !errors.Is(schemaErr, errSQLUnavailable) {
				settingsError(dlg, "Столбцы SQL-запросов", schemaErr)
				return
			}
			message := "Не удалось проверить структуру списка: " + schemaErr.Error() +
				"\n\nСохранить настройки без проверки колонок? Синхронизация выполнится при подключении."
			if walk.MsgBox(dlg, "SQL Server недоступен", message, walk.MsgBoxYesNo|walk.MsgBoxIconWarning) != walk.DlgCmdYes {
				return
			}
		} else if !accepted {
			return
		}
		if err := core.SaveConfig(a.cfgPath, cfg); err != nil {
			settingsError(dlg, "Не удалось сохранить config.json", err)
			return
		}
		if err := a.applyColumns(cfg.ListColumns); err != nil {
			settingsError(dlg, "Колонки", err)
			return
		}
		if err := a.connect(cfg); err != nil {
			if a.store.DB != nil {
				_ = a.store.DB.Close()
				a.store.DB = nil
			}
			a.store.Config = cfg
			a.status.SetText("Ошибка подключения — откройте настройки")
			walk.MsgBox(dlg, "Конфигурация сохранена", "config.json сохранён, но подключиться не удалось:\n"+err.Error(), walk.MsgBoxIconWarning)
		} else {
			a.refreshBoth()
		}
		dlg.Accept()
	}

	err := Dialog{
		AssignTo: &dlg, Title: "Настройки ServiceAgreement — SQL Server",
		MinSize: Size{Width: 790, Height: 600}, Size: Size{Width: 1020, Height: 760},
		Layout: VBox{Margins: Margins{Left: 14, Top: 12, Right: 14, Bottom: 12}, Spacing: 6},
		Children: []Widget{
			// Zero nested margins: Walk's default HBox padding otherwise creates
			// uneven vertical gaps of 18 px around every settings row.
			Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
				Label{Text: "Строка подключения к SQL Server:"},
				HSpacer{},
				PushButton{Text: "Проверить подключение", OnClicked: onTest},
			}},
			LineEdit{AssignTo: &connection, Text: cfg.ConnectionString},
			// Fixed widths keep number inputs compact, consistent and readable.
			// 76 logical pixels fit at least four decimal digits plus padding
			// (list_limit=1000, lookup_limit=500, timeout up to 3600).
			Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
				Label{Text: "Макс. строк списка:"},
				LineEdit{AssignTo: &listLimit, Text: strconv.Itoa(cfg.ListLimit), MaxLength: 5,
					MinSize: Size{Width: 76}, MaxSize: Size{Width: 76}},
				Label{Text: "Макс. совпадений справочника:"},
				LineEdit{AssignTo: &lookupLimit, Text: strconv.Itoa(cfg.LookupLimit), MaxLength: 5,
					MinSize: Size{Width: 76}, MaxSize: Size{Width: 76}},
				Label{Text: "Таймаут SQL (сек.):"},
				LineEdit{AssignTo: &connectionTimeout, Text: strconv.Itoa(cfg.ConnectionTimeoutSeconds), MaxLength: 4,
					MinSize: Size{Width: 76}, MaxSize: Size{Width: 76},
					ToolTipText: "Таймаут выполнения SQL-запросов: от 1 до 3600 секунд"},
				HSpacer{},
			}},
			Label{Text: "Редактирование SQL-запросов — выберите назначение, затем изменяйте текст:"},
			Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
				ComboBox{AssignTo: &sqlPicker, Model: sqlNames, CurrentIndex: 0, OnCurrentIndexChanged: onSQLChanged},
				PushButton{Text: "Форматировать SQL", OnClicked: formatCurrentSQL},
			}},
			TextEdit{AssignTo: &sqlEdit, Text: sqlTextForEditor(cfg.ListSQL), VScroll: true, HScroll: true,
				Font: Font{Family: "Consolas", PointSize: 10}},
			Label{Text: "SQL-параметры @search, @limit, @selected, @id и параметры записи описаны в README. Порядок SELECT для одной записи важен."},
			Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
				PushButton{Text: "Колонки списка…", OnClicked: func() {
					commitCurrentSQL()
					cfg.ConnectionString = strings.TrimSpace(connection.Text())
					a.editColumns(dlg, &cfg)
				}},
				HSpacer{},
				PushButton{Text: "Сохранить", OnClicked: onSave},
				PushButton{Text: "Отмена", OnClicked: func() { dlg.Cancel() }},
			}},
		},
	}.Create(a.win)
	if err != nil {
		a.warn("Настройки", err)
		return
	}
	dlg.Run()
}

type columnModel struct {
	walk.TableModelBase
	columns []core.ListColumn
}

func (m *columnModel) RowCount() int { return len(m.columns) }
func (m *columnModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.columns) {
		return ""
	}
	x := m.columns[row]
	switch col {
	case 0:
		return x.Order
	case 1:
		return x.Field
	case 2:
		return x.Title
	case 3:
		return x.Width
	case 4:
		return x.SortField
	case 5:
		if x.Visible {
			return "Да"
		}
		return "Нет"
	}
	return ""
}
func (m *columnModel) reset() { m.PublishRowsReset() }

func (a *application) editColumns(owner walk.Form, cfg *core.Config) {
	// Detect new SQL aliases on opening the editor, without requiring a
	// separate Add button. The SQL query may be edited but not yet saved.
	if _, accepted, err := synchronizeDraftColumns(owner, cfg); err != nil {
		// Editing display settings is still possible if the server is offline.
		if !errors.Is(err, errSQLUnavailable) {
			settingsError(owner, "Структура SQL-запросов", err)
			return
		}
	} else if !accepted {
		return
	}
	model := &columnModel{columns: core.OrderedColumns(cfg.ListColumns)}
	var dlg *walk.Dialog
	var table *walk.TableView
	var field, title, width, order, sortField *walk.LineEdit
	var visible *walk.CheckBox
	current := -1
	loadRow := func() {
		if table == nil || field == nil || title == nil || width == nil || order == nil || sortField == nil || visible == nil {
			return
		}
		current = table.CurrentIndex()
		if current >= 0 && current < len(model.columns) {
			x := model.columns[current]
			_ = field.SetText(x.Field)
			_ = title.SetText(x.Title)
			_ = width.SetText(strconv.Itoa(x.Width))
			_ = order.SetText(strconv.Itoa(x.Order))
			_ = sortField.SetText(x.SortField)
			visible.SetChecked(x.Visible && !core.IsServiceField(x.Field))
                visible.SetEnabled(!core.IsServiceField(x.Field))
		} else {
			_ = field.SetText("")
			_ = title.SetText("")
			_ = width.SetText("")
			_ = order.SetText("")
			_ = sortField.SetText("")
			visible.SetChecked(false)
                visible.SetEnabled(false)
		}
	}
	applyRow := func() bool {
		if current < 0 || current >= len(model.columns) {
			return true
		}
		w, err := core.ParseIntField("Ширина колонки", width.Text())
		if err != nil {
			settingsError(dlg, "Колонки", err)
			return false
		}
		o, err := core.ParseIntField("Порядок колонки", order.Text())
		if err != nil {
			settingsError(dlg, "Колонки", err)
			return false
		}
		x := core.ListColumn{
			Field: strings.TrimSpace(field.Text()), Title: strings.TrimSpace(title.Text()),
			Width: w, Order: o, SortField: strings.TrimSpace(sortField.Text()), Visible: visible.Checked() && !core.IsServiceField(field.Text()),
		}
		tmp := append([]core.ListColumn(nil), model.columns...)
		tmp[current] = x
		if err := core.ValidateColumns(tmp); err != nil {
			settingsError(dlg, "Колонки", err)
			return false
		}
		model.columns = core.OrderedColumns(tmp)
		model.reset()
		for i, v := range model.columns {
			if v.Order == x.Order {
				_ = table.SetCurrentIndex(i)
				break
			}
		}
		return true
	}
	onSync := func() {
		if !applyRow() {
			return
		}
		draft := *cfg
		draft.ListColumns = append([]core.ListColumn(nil), model.columns...)
		_, accepted, err := synchronizeDraftColumns(dlg, &draft)
		if err != nil {
			settingsError(dlg, "Не удалось обновить SQL-поля", err)
			return
		}
		if !accepted {
			return
		}
		model.columns = core.OrderedColumns(draft.ListColumns)
		model.reset()
		if len(model.columns) > 0 {
			_ = table.SetCurrentIndex(0)
			loadRow()
		}
	}
	onMove := func(delta int) {
		if !applyRow() {
			return
		}
		i := table.CurrentIndex()
		j := i + delta
		if i < 0 || j < 0 || j >= len(model.columns) {
			return
		}
		model.columns[i].Order, model.columns[j].Order = model.columns[j].Order, model.columns[i].Order
		model.columns = core.OrderedColumns(model.columns)
		model.reset()
		_ = table.SetCurrentIndex(j)
		loadRow()
	}
	onSave := func() {
		if !applyRow() {
			return
		}
		if err := core.ValidateColumns(model.columns); err != nil {
			settingsError(dlg, "Колонки", err)
			return
		}
		cfg.ListColumns = core.OrderedColumns(model.columns)
		dlg.Accept()
	}
	err := Dialog{
		AssignTo: &dlg, Title: "Колонки списка — порядок, сортировка и видимость",
		MinSize: Size{Width: 870, Height: 540}, Size: Size{Width: 1090, Height: 660},
		Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
		Children: []Widget{
			Label{Text: "Поля формируются автоматически по list_sql и search_sql. Новые поля видимы; остальные параметры настраиваются ниже."},
			TableView{AssignTo: &table, Model: model, NotSortableByHeaderClick: true, AlternatingRowBG: true,
				Columns: []TableViewColumn{
					{Title: "Порядок", Width: 80}, {Title: "Поле SQL", Width: 180},
					{Title: "Заголовок", Width: 310}, {Title: "Ширина", Width: 75},
					{Title: "sortField", Width: 170}, {Title: "Видна", Width: 75},
				},
				OnCurrentIndexChanged: loadRow,
			},
			Composite{Layout: Grid{Columns: 2, Spacing: 6}, Children: []Widget{
				Label{Text: "Имя поля SELECT (field):"}, LineEdit{AssignTo: &field, ReadOnly: true},
				Label{Text: "Заголовок (title):"}, LineEdit{AssignTo: &title},
				Label{Text: "Ширина (width, 45–1600):"}, LineEdit{AssignTo: &width},
				Label{Text: "Порядок (order, от 1):"}, LineEdit{AssignTo: &order},
				Label{Text: "Поле сортировки (sortField):"}, LineEdit{AssignTo: &sortField},
				Label{Text: "Видимость (visible):"}, CheckBox{AssignTo: &visible, Text: "Отображать колонку"},
			}},
			Composite{Layout: HBox{Spacing: 7}, Children: []Widget{
				PushButton{Text: "Применить к строке", OnClicked: func() { applyRow() }},
				PushButton{Text: "Обновить поля из SQL", OnClicked: onSync},
				PushButton{Text: "Вверх", OnClicked: func() { onMove(-1) }},
				PushButton{Text: "Вниз", OnClicked: func() { onMove(1) }},
			}},
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				HSpacer{}, PushButton{Text: "ОК", OnClicked: onSave},
				PushButton{Text: "Отмена", OnClicked: func() { dlg.Cancel() }},
			}},
		},
	}.Create(owner)
	if err != nil {
		settingsError(owner, "Колонки", err)
		return
	}
	if len(model.columns) > 0 {
		_ = table.SetCurrentIndex(0)
		loadRow()
	}
	dlg.Run()
}