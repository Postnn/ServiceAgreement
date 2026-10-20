//go:build windows

package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

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
	for _, col := range c {
		result = append(result, TableViewColumn{Title: col.Title, Width: col.Width})
	}
	return result
}

type application struct {
	cfgPath string
	store   core.Store
	win     *walk.MainWindow
	table   *walk.TableView
	search  *walk.LineEdit
	status  *walk.Label
	model   *tableModel
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
	app := &application{cfgPath: path, store: core.Store{Config: cfg}, model: &tableModel{columns: cfg.ListColumns}}
	err = MainWindow{
		AssignTo: &app.win, Title: "Договоры оказания услуг — ServiceAgreement",
		MinSize: Size{Width: 850, Height: 490}, Size: Size{Width: 1370, Height: 680},
		Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
		Children: []Widget{
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				Label{Text: "Поиск по договору:"},
				LineEdit{AssignTo: &app.search, CueBanner: "Введите код или название; Enter — искать", OnKeyDown: func(key walk.Key) {
					if key == walk.KeyReturn {
						app.refresh()
					}
				}},
				PushButton{Text: "Найти", OnClicked: app.refresh},
				PushButton{Text: "Обновить", OnClicked: app.refresh},
			}},
			TableView{AssignTo: &app.table, Model: app.model, AlternatingRowBG: true,
				MultiSelection: false, NotSortableByHeaderClick: true,
				Columns: tableColumns(cfg.ListColumns), OnItemActivated: app.editSelected,
			},
			Label{Text: "Даты XDD в списке — примеры для цикла текущего года, а не фактические даты оплаты или отключения договора."},
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				PushButton{Text: "Добавить", OnClicked: app.add},
				PushButton{Text: "Изменить", OnClicked: app.editSelected},
				PushButton{Text: "Удалить", OnClicked: app.deleteSelected},
				PushButton{Text: "Настройки SQL и колонок…", OnClicked: app.openSettings},
				HSpacer{}, Label{AssignTo: &app.status, Text: ""},
			}},
		},
	}.Create()
	if err != nil {
		return fmt.Errorf("создание главного окна: %w", err)
	}
	if loadErr != nil {
		app.status.SetText("Требуется настройка config.json — нажмите «Настройки SQL и колонок»")
	} else if err := app.connect(cfg); err != nil {
		app.status.SetText("Нет подключения к SQL Server — проверьте настройки")
	} else {
		app.refresh()
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
	a.model.columns = cols
	for _, col := range cols {
		c := walk.NewTableViewColumn()
		_ = c.SetTitle(col.Title)
		_ = c.SetWidth(col.Width)
		if err := a.table.Columns().Add(c); err != nil {
			return err
		}
	}
	a.model.replace(nil)
	return nil
}

func (a *application) warn(title string, err error) {
	walk.MsgBox(a.win, title, err.Error(), walk.MsgBoxIconError)
}
func (a *application) refresh() {
	if a.store.DB == nil {
		a.status.SetText("Нет подключения. Откройте настройки.")
		return
	}
	ctx, cancel := core.QueryContext()
	defer cancel()
	rows, err := a.store.List(ctx, a.search.Text())
	if err != nil {
		a.warn("Ошибка чтения списка", err)
		return
	}
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
		a.refresh()
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
		a.refresh()
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
	a.refresh()
}

func intText(n int) string { return strconv.Itoa(n) }
