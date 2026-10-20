//go:build windows

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/core"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	_ "github.com/microsoft/go-mssqldb"
)

type tableModel struct {
	walk.TableModelBase
	items   []core.ListItem
	columns []core.ListColumn
}

func (m *tableModel) RowCount() int { return len(m.items) }
func (m *tableModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.items) || col < 0 || col >= len(m.columns) {
		return ""
	}
	return m.items[row].Field(m.columns[col].Field)
}
func (m *tableModel) replace(rows []core.ListItem) { m.items = rows; m.PublishRowsReset() }

func tableColumns(c []core.ListColumn) []TableViewColumn {
	result := make([]TableViewColumn, 0, len(c))
	for _, col := range core.VisibleColumns(c) {
		result = append(result, TableViewColumn{Name: col.Field, Title: col.Title, Width: col.Width})
	}
	return result
}

type application struct {
	cfgPath        string
	store          core.Store
	win            *walk.MainWindow
	table          *walk.TableView
	search         *walk.LineEdit
	status         *walk.Label
	model          *tableModel
	withoutTable   *walk.TableView
	withoutSearch  *walk.LineEdit
	withoutStatus  *walk.Label
	withoutModel   *tableModel
	withoutSortField string
	withoutSortDescending bool
	sortField      string
	sortDescending bool
}

func main() {
	if err := run(); err != nil {
		walk.MsgBox(nil, "ServiceAgreement — ошибка", err.Error(), walk.MsgBoxIconError)
	}
}

func run() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	path := filepath.Join(filepath.Dir(exe), "config.json")
	cfg, loadErr := core.LoadConfig(path)
	if loadErr != nil {
		cfg = core.DefaultConfig()
	}
	app := &application{cfgPath: path, store: core.Store{Config: cfg}, model: &tableModel{columns: core.VisibleColumns(cfg.ListColumns)}, withoutModel: &tableModel{columns: core.UnconfiguredColumns([]string{"ContractID"})}}
	gearIcon, err := makeSettingsGearBitmap()
	if err != nil {
		return fmt.Errorf("значок настроек: %w", err)
	}
	defer gearIcon.Dispose()
	err = MainWindow{
		AssignTo: &app.win, Title: "Договоры оказания услуг — ServiceAgreement",
		MinSize: Size{Width: 850, Height: 490}, Size: Size{Width: 1370, Height: 680},
		Layout: VBox{Margins: Margins{Left: 12, Top: 4, Right: 12, Bottom: 12}, Spacing: 3},
		Children: []Widget{
			// Small borderless icon rather than a native PushButton. The
			// 20-pixel header leaves the tab strip directly below the icon.
			Composite{
				MinSize: Size{Height: 20}, MaxSize: Size{Height: 20},
				Layout: HBox{MarginsZero: true, SpacingZero: true},
				Children: []Widget{
					HSpacer{},
					ImageView{
						Name: "settingsGear", Image: gearIcon,
						Mode: ImageViewModeIdeal,
						MinSize: Size{Width: 20, Height: 20},
						MaxSize: Size{Width: 20, Height: 20},
						ToolTipText: "Настройки SQL и колонок",
						OnMouseUp: func(x, y int, button walk.MouseButton) {
							if button == walk.LeftButton {
								app.openSettings()
							}
						},
					},
				},
			},
			TabWidget{Pages: []TabPage{
				{
					Title: "Договоры с параметрами",
					Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 8},
					Children: []Widget{
						Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
							Label{Text: "Поиск по договору:"},
							LineEdit{AssignTo: &app.search, CueBanner: "Код или название; Enter — искать",
								OnKeyDown: func(key walk.Key) {
									if key == walk.KeyReturn { app.refresh() }
								}},
							PushButton{Text: "Найти", OnClicked: app.refresh},
							PushButton{Text: "Обновить", OnClicked: app.refresh},
						}},
						TableView{AssignTo: &app.table, Model: app.model, AlternatingRowBG: true,
							MultiSelection: false, Columns: tableColumns(cfg.ListColumns),
							OnItemActivated: app.editSelected},
						Label{Text: "Все значения возвращает SQL. Заголовок колонки сортирует по sortField."},
						Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
							PushButton{Text: "Добавить", OnClicked: app.add},
							PushButton{Text: "Изменить", OnClicked: app.editSelected},
							PushButton{Text: "Удалить", OnClicked: app.deleteSelected},
							HSpacer{}, Label{AssignTo: &app.status, Text: ""},
						}},
					},
				},
				{
					Title: "Договоры без параметров",
					Layout: VBox{Margins: Margins{Left: 8, Top: 8, Right: 8, Bottom: 8}, Spacing: 8},
					Children: []Widget{
						Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
							Label{Text: "Поиск по коду или названию:"},
							LineEdit{AssignTo: &app.withoutSearch, CueBanner: "Код или название; Enter — искать",
								OnKeyDown: func(key walk.Key) {
									if key == walk.KeyReturn { app.refreshWithout() }
								}},
							PushButton{Text: "Найти", OnClicked: app.refreshWithout},
							PushButton{Text: "Обновить", OnClicked: app.refreshWithout},
						}},
						TableView{AssignTo: &app.withoutTable, Model: app.withoutModel,
							AlternatingRowBG: true, MultiSelection: false,
							Columns: tableColumns(app.withoutModel.columns),
							OnItemActivated: app.addFromWithout},
						Label{Text: "Все поля из without_parameters_sql отображаются. Двойной щелчок открывает добавление параметров."},
						Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
							PushButton{Text: "Задать параметры", OnClicked: app.addFromWithout},
							HSpacer{}, Label{AssignTo: &app.withoutStatus, Text: ""},
						}},
					},
				},
			}},
		},
	}.Create()
	if err != nil {
		return fmt.Errorf("создание главного окна: %w", err)
	}
	app.table.ColumnClicked().Attach(app.sortByColumn)
	app.withoutTable.ColumnClicked().Attach(app.sortWithoutByColumn)
	if loadErr != nil {
		app.status.SetText("Требуется настройка config.json — нажмите значок ⚙ справа вверху")
	} else if err := app.connect(cfg); err != nil {
		app.status.SetText("Нет подключения к SQL Server — проверьте настройки")
	} else {
		app.refreshBoth()
	}
	app.win.Run()
	if app.store.DB != nil {
		_ = app.store.DB.Close()
	}
	return nil
}

func (a *application) connect(cfg core.Config) error {
	db, err := sql.Open("sqlserver", cfg.ConnectionString)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	ctx, cancel := core.QueryContext()
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		db.Close()
		return fmt.Errorf("SQL Server: %w", err)
	}
	if a.store.DB != nil {
		_ = a.store.DB.Close()
	}
	a.store.DB = db
	a.store.Config = cfg
	return nil
}

func (a *application) applyColumns(cols []core.ListColumn) error {
	if err := a.table.Columns().Clear(); err != nil {
		return err
	}
	a.model.columns = core.VisibleColumns(cols)
	a.sortField, a.sortDescending = "", false
	for _, col := range a.model.columns {
		c := walk.NewTableViewColumn()
		c.SetName(col.Field)
		_ = c.SetTitle(col.Title)
		_ = c.SetWidth(col.Width)
		if err := a.table.Columns().Add(c); err != nil {
			return err
		}
	}
	a.model.replace(nil)
	return nil
}

func (a *application) sortByColumn(index int) {
	if index < 0 || index >= len(a.model.columns) {
		return
	}
	field := a.model.columns[index].SortField
	if a.sortField == field {
		a.sortDescending = !a.sortDescending
	} else {
		a.sortField, a.sortDescending = field, false
	}
	core.SortListItems(a.model.items, a.sortField, a.sortDescending)
	a.model.PublishRowsReset()
	arrow := "↑"
	if a.sortDescending {
		arrow = "↓"
	}
	a.status.SetText(fmt.Sprintf("Записей: %d; сортировка: %s %s", len(a.model.items), field, arrow))
}

// mergeColumnsFromSQL reconciles the stored appearance settings with both SQL
// result sets. New SELECT aliases are visible by default. SQL fields which
// were removed are discarded only after the user confirms the proposal.
func reconcileColumnSettings(owner walk.Form, db *sql.DB, cfg *core.Config) (bool, bool, error) {
	ctx, cancel := core.QueryContext()
	names, err := core.QueryListSchema(ctx, db, *cfg)
	cancel()
	if err != nil {
		return false, false, err
	}
	result, err := core.ReconcileListColumns(cfg.ListColumns, names)
	if err != nil {
		return false, false, err
	}
	if len(result.Removed) > 0 {
		message := "В SQL-запросах списка больше нет следующих полей:\n\n" + strings.Join(result.Removed, ", ") +
			"\n\nУдалить их настройки из list_columns?\nОтмена сохранит прежнюю конфигурацию."
		if walk.MsgBox(owner, "Изменился состав колонок SQL", message, walk.MsgBoxYesNo|walk.MsgBoxIconQuestion) != walk.DlgCmdYes {
			return false, false, nil
		}
	}
	cfg.ListColumns = result.Columns
	return result.Changed(), true, nil
}

// Refreshing the SQL-based list also discovers new SELECT aliases when the
// query changes outside the application. Auto-saved changes keep .bak backup.
func (a *application) synchronizeListColumns() bool {
	cfg := a.store.Config
	changed, accepted, err := reconcileColumnSettings(a.win, a.store.DB, &cfg)
	if err != nil {
		a.warn("Структура колонок списка", err)
		a.status.SetText("SQL-запросы списка возвращают несовместимые поля")
		return false
	}
	if !accepted {
		a.status.SetText("Синхронизация отменена. Исправьте SQL или настройки колонок.")
		return false
	}
	if !changed {
		return true
	}
	if err := core.SaveConfig(a.cfgPath, cfg); err != nil {
		a.warn("Сохранение колонок", err)
		return false
	}
	if err := a.applyColumns(cfg.ListColumns); err != nil {
		a.warn("Отображение колонок", err)
		return false
	}
	a.store.Config = cfg
	return true
}

func (a *application) warn(title string, err error) {
	walk.MsgBox(a.win, title, err.Error(), walk.MsgBoxIconError)
}
func (a *application) refresh() {
	if a.store.DB == nil {
		a.status.SetText("Нет подключения. Откройте настройки.")
		return
	}
	if !a.synchronizeListColumns() {
		return
	}
	ctx, cancel := core.QueryContext()
	defer cancel()
	rows, err := a.store.List(ctx, a.search.Text())
	if err != nil {
		a.warn("Ошибка чтения списка", err)
		return
	}
	core.SortListItems(rows, a.sortField, a.sortDescending)
	a.model.replace(rows)
	a.status.SetText(fmt.Sprintf("Записей: %d", len(rows)))
}
func (a *application) selectedID() int {
	if a.table == nil {
		return 0
	}
	idx := a.table.CurrentIndex()
	if idx < 0 || idx >= len(a.model.items) {
		return 0
	}
	return a.model.items[idx].ID
}
func (a *application) add() {
	if a.store.DB == nil {
		a.warn("Подключение", errors.New("сначала настройте подключение к SQL Server"))
		return
	}
	r := core.Record{Period: 1, StartDay: 1, StartMonth: 1, DocumentsCode: 99, PaymentCode: 99, DisconnectCode: 99, Amount: "0.0000"}
	if a.edit(r, false) {
		a.refreshBoth()
	}
}
func (a *application) editSelected() {
	id := a.selectedID()
	if id == 0 {
		walk.MsgBox(a.win, "Выбор записи", "Выберите договор в списке.", walk.MsgBoxIconInformation)
		return
	}
	ctx, cancel := core.QueryContext()
	r, err := a.store.Get(ctx, id)
	cancel()
	if err != nil {
		a.warn("Ошибка чтения записи", err)
		return
	}
	if a.edit(r, true) {
		a.refreshBoth()
	}
}
func (a *application) deleteSelected() {
	id := a.selectedID()
	if id == 0 {
		walk.MsgBox(a.win, "Выбор записи", "Выберите договор в списке.", walk.MsgBoxIconInformation)
		return
	}
	answer := walk.MsgBox(a.win, "Удалить запись?", fmt.Sprintf("Удалить параметры договора №%d?\nИсходный документ договора останется в базе.", id), walk.MsgBoxYesNo|walk.MsgBoxIconWarning)
	if answer != walk.DlgCmdYes {
		return
	}
	ctx, cancel := core.QueryContext()
	err := a.store.Delete(ctx, id)
	cancel()
	if err != nil {
		a.warn("Ошибка удаления", err)
		return
	}
	a.refreshBoth()
}

// Both lists are refreshed after every successful insert, update or delete.
// Each retains its own search text, so filtered results remain predictable.
func (a *application) refreshBoth() {
	a.refresh()
	a.refreshWithout()
}

func (a *application) applyUnconfiguredColumns(names []string) error {
	unchanged := len(a.withoutModel.columns) == len(names)
	if unchanged {
		for i, name := range names {
			if !strings.EqualFold(a.withoutModel.columns[i].Field, name) {
				unchanged = false
				break
			}
		}
	}
	if unchanged { return nil }
	if err := a.withoutTable.Columns().Clear(); err != nil { return err }
	cols := core.UnconfiguredColumns(names)
	a.withoutModel.columns = cols
	a.withoutSortField, a.withoutSortDescending = "", false
	for _, col := range cols {
		c := walk.NewTableViewColumn()
		c.SetName(col.Field)
		_ = c.SetTitle(col.Title)
		_ = c.SetWidth(col.Width)
		if err := a.withoutTable.Columns().Add(c); err != nil { return err }
	}
	a.withoutModel.replace(nil)
	return nil
}

func (a *application) refreshWithout() {
	if a.store.DB == nil {
		a.withoutStatus.SetText("Нет подключения к SQL Server")
		return
	}
	ctx, cancel := core.QueryContext()
	defer cancel()
	result, err := a.store.ListUnconfigured(ctx, a.withoutSearch.Text())
	if err != nil {
		a.warn("Договоры без параметров", err)
		a.withoutStatus.SetText("Ошибка получения списка")
		return
	}
	if err := a.applyUnconfiguredColumns(result.Columns); err != nil {
		a.warn("Колонки договоров без параметров", err)
		return
	}
	core.SortListItems(result.Items, a.withoutSortField, a.withoutSortDescending)
	a.withoutModel.replace(result.Items)
	a.withoutStatus.SetText(fmt.Sprintf("Договоров без параметров: %d", len(result.Items)))
}

func (a *application) sortWithoutByColumn(index int) {
	if index < 0 || index >= len(a.withoutModel.columns) { return }
	field := a.withoutModel.columns[index].Field
	if a.withoutSortField == field { a.withoutSortDescending = !a.withoutSortDescending
	} else { a.withoutSortField, a.withoutSortDescending = field, false }
	core.SortListItems(a.withoutModel.items, a.withoutSortField, a.withoutSortDescending)
	a.withoutModel.PublishRowsReset()
}

func (a *application) addFromWithout() {
	if a.store.DB == nil {
		a.warn("Подключение", errors.New("сначала настройте подключение к SQL Server"))
		return
	}
	idx := a.withoutTable.CurrentIndex()
	if idx < 0 || idx >= len(a.withoutModel.items) {
		walk.MsgBox(a.win, "Выбор договора", "Выберите договор без параметров.", walk.MsgBoxIconInformation)
		return
	}
	id := a.withoutModel.items[idx].ID
	r := core.Record{
		ContractID: id, Period: 1, StartDay: 1, StartMonth: 1,
		DocumentsCode: 99, PaymentCode: 99, DisconnectCode: 99, Amount: "0.0000",
	}
	// Contract is preselected and locked for this specific agreement.
	label := a.withoutModel.items[idx].Field("Договор")
    if label == "" {
        // Older custom queries may still return the previous alias.
        label = a.withoutModel.items[idx].Field("ContractName")
    }
	if label == "" {
		for _, col := range a.withoutModel.columns {
			if !strings.EqualFold(col.Field, "ContractID") {
				label = a.withoutModel.items[idx].Field(col.Field)
				break
			}
		}
	}
	if a.editWithContract(r, false, true, label) { a.refreshBoth() }
}

func intText(n int) string { return strconv.Itoa(n) }