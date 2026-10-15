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
	"time"

	"github.com/Postnn/ServiceAgreement/internal/core"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	_ "github.com/microsoft/go-mssqldb"
)

type tableModel struct {
	walk.TableModelBase
	items []core.ListItem
}

func (m *tableModel) RowCount() int { return len(m.items) }
func (m *tableModel) Value(row, col int) interface{} {
	if row < 0 || row >= len(m.items) {
		return ""
	}
	r := m.items[row]
	switch col {
	case 0:
		return r.ID
	case 1:
		return r.Agreement
	case 2:
		return periodName(r.Period)
	case 3:
		return r.Amount
	case 4:
		return core.ExplainXDD(r.DocumentsCode) + "  →  " + core.ExampleDate(r.DocumentsCode, r.StartDay, r.StartMonth, time.Now().Year())
	case 5:
		return core.ExplainXDD(r.PaymentCode) + "  →  " + core.ExampleDate(r.PaymentCode, r.StartDay, r.StartMonth, time.Now().Year())
	case 6:
		return core.ExplainXDD(r.DisconnectCode) + "  →  " + core.ExampleDate(r.DisconnectCode, r.StartDay, r.StartMonth, time.Now().Year())
	case 7:
		if r.AutoPay == 1 {
			return "Да"
		}
		return "Нет"
	}
	return ""
}
func (m *tableModel) replace(items []core.ListItem) { m.items = items; m.PublishRowsReset() }
func periodName(p int) string {
	switch p {
	case 1:
		return "Ежемесячно"
	case 3:
		return "Ежеквартально"
	case 12:
		return "Ежегодно"
	}
	return strconv.Itoa(p)
}

type application struct {
	store  core.Store
	win    *walk.MainWindow
	table  *walk.TableView
	search *walk.LineEdit
	status *walk.Label
	model  *tableModel
}

func main() {
	if err := run(); err != nil {
		walk.MsgBox(nil, "ServiceAgreement — ошибка", err.Error(), walk.MsgBoxIconError)
	}
}
func run() error {
	path, err := os.Executable()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(filepath.Dir(path), "config.json")
	cfg, err := core.LoadConfig(cfgPath)
	if err != nil {
		return fmt.Errorf("%w\n\nПоместите config.json рядом с ServiceAgreement.exe (образец: config.example.json).", err)
	}
	db, err := sql.Open("sqlserver", cfg.ConnectionString)
	if err != nil {
		return fmt.Errorf("SQL Server: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(1)
	ctx, cancel := core.QueryContext()
	err = db.PingContext(ctx)
	cancel()
	if err != nil {
		return fmt.Errorf("соединение с SQL Server: %w", err)
	}
	app := &application{store: core.Store{DB: db, Config: cfg}, model: &tableModel{}}
	err = MainWindow{
		AssignTo: &app.win,
		Title:    "Договоры оказания услуг",
		MinSize:  Size{Width: 820, Height: 480}, Size: Size{Width: 1300, Height: 630},
		Layout: VBox{Margins: Margins{Left: 12, Top: 12, Right: 12, Bottom: 12}, Spacing: 8},
		Children: []Widget{
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				Label{Text: "Поиск по договору:"},
				LineEdit{AssignTo: &app.search, CueBanner: "Код или название договора"},
				PushButton{Text: "Найти", OnClicked: app.refresh},
				PushButton{Text: "Обновить", OnClicked: app.refresh},
			}},
			TableView{
				AssignTo: &app.table, Model: app.model,
				AlternatingRowBG: true, MultiSelection: false,
				NotSortableByHeaderClick: true,
				Columns: []TableViewColumn{
					{Title: "Код договора", Width: 115},
					{Title: "Договор", Width: 210},
					{Title: "Период", Width: 115},
					{Title: "Абонентская плата", Width: 130},
					{Title: "Получение документов", Width: 300},
					{Title: "Оплата по договору", Width: 300},
					{Title: "Отключение", Width: 300},
					{Title: "Автооплата", Width: 85},
				},
				OnItemActivated: app.editSelected,
			},
			Label{Text: "Даты справа рассчитаны как пример для начала цикла в текущем году (не фактические сроки конкретного договора). 99 = последний день месяца."},
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				PushButton{Text: "Добавить", OnClicked: app.add},
				PushButton{Text: "Изменить", OnClicked: app.editSelected},
				PushButton{Text: "Удалить", OnClicked: app.deleteSelected},
				HSpacer{},
				Label{AssignTo: &app.status, Text: ""},
			}},
		},
	}.Create()
	if err != nil {
		return fmt.Errorf("создание окна: %w", err)
	}
	app.refresh()
	app.win.Run()
	return nil
}
func (a *application) warn(title string, err error) {
	walk.MsgBox(a.win, title, err.Error(), walk.MsgBoxIconError)
}
func (a *application) refresh() {
	if a.search == nil {
		return
	}
	ctx, cancel := core.QueryContext()
	defer cancel()
	items, err := a.store.List(ctx, a.search.Text())
	if err != nil {
		a.warn("Ошибка чтения", err)
		return
	}
	a.model.replace(items)
	if a.status != nil {
		a.status.SetText(fmt.Sprintf("Записей: %d", len(items)))
	}
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
	r := core.Record{Period: 1, StartDay: 1, StartMonth: 1, DocumentsCode: 99, PaymentCode: 99, DisconnectCode: 99, Amount: "0.0000"}
	if a.edit(r, false) {
		a.refresh()
	}
}
func (a *application) editSelected() {
	id := a.selectedID()
	if id == 0 {
		walk.MsgBox(a.win, "Выбор записи", "Выберите договор в таблице.", walk.MsgBoxIconInformation)
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
		walk.MsgBox(a.win, "Выбор записи", "Выберите договор в таблице.", walk.MsgBoxIconInformation)
		return
	}
	result := walk.MsgBox(a.win, "Подтверждение удаления", fmt.Sprintf("Удалить параметры автооплаты договора №%d?\nИсходный документ договора удалён НЕ будет.", id), walk.MsgBoxYesNo|walk.MsgBoxIconWarning)
	if result != walk.DlgCmdYes {
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

func optionLabels(options []core.Option) []string {
	result := make([]string, 0, len(options))
	for _, o := range options {
		result = append(result, fmt.Sprintf("%d — %s", o.ID, o.Label))
	}
	return result
}
func optionIndex(options []core.Option, id int) int {
	for i, o := range options {
		if o.ID == id {
			return i
		}
	}
	return -1
}
func addMissing(options []core.Option, id int) []core.Option {
	if id > 0 && optionIndex(options, id) < 0 {
		return append(options, core.Option{ID: id, Label: "Текущее значение (вне выборки)"})
	}
	return options
}
func pick(options []core.Option, idx int) int {
	if idx >= 0 && idx < len(options) {
		return options[idx].ID
	}
	return 0
}
func intText(n int) string { return strconv.Itoa(n) }

func dayLabels() []string {
	result := make([]string, 0, 32)
	for n := 1; n <= 31; n++ {
		result = append(result, fmt.Sprintf("%02d", n))
	}
	return append(result, "Последний день (99)")
}

func dayIndex(dd int) int {
	if dd == 99 {
		return 31
	}
	if dd >= 1 && dd <= 31 {
		return dd - 1
	}
	return -1
}

func chosenDay(idx int) (int, error) {
	if idx == 31 {
		return 99, nil
	}
	if idx >= 0 && idx < 31 {
		return idx + 1, nil
	}
	return 0, errors.New("выберите число месяца или его последний день")
}

// edit is the application's second and only other window.
func (a *application) edit(original core.Record, existing bool) bool {
	var dlg *walk.Dialog
	var contractSearch, typeSearch, amount, day *walk.LineEdit
	var docMonths, payMonths, offMonths *walk.LineEdit
	var docDirection, payDirection, offDirection *walk.ComboBox
	var docDD, payDD, offDD *walk.ComboBox
	var docHint, payHint, offHint *walk.Label
	var contractBox, typeBox, periodBox, monthBox *walk.ComboBox
	var autoBox, notifyBox *walk.CheckBox
	var saveButton *walk.PushButton
	lookup := func(sqlText, search string, selected int) ([]core.Option, error) {
		ctx, cancel := core.QueryContext()
		defer cancel()
		return a.store.Lookup(ctx, sqlText, search, selected)
	}
	contractOptions, err := lookup(a.store.Config.ContractsSQL, "", original.ContractID)
	if err != nil {
		a.warn("Ошибка выбора договора", err)
		return false
	}
	contractOptions = addMissing(contractOptions, original.ContractID)
	typeOptions, err := lookup(a.store.Config.DocumentTypesSQL, "", original.TypeID)
	if err != nil {
		a.warn("Ошибка выбора типа документа", err)
		return false
	}
	typeOptions = addMissing(typeOptions, original.TypeID)
	periodIndex := 0
	if original.Period == 3 {
		periodIndex = 1
	} else if original.Period == 12 {
		periodIndex = 2
	}
	monthNames := []string{"01 — январь", "02 — февраль", "03 — март", "04 — апрель", "05 — май", "06 — июнь", "07 — июль", "08 — август", "09 — сентябрь", "10 — октябрь", "11 — ноябрь", "12 — декабрь"}
	directionNames := []string{"От начала (включительно)", "До начала"}
	docInitial, docError := core.ParseXDD(original.DocumentsCode)
	payInitial, payError := core.ParseXDD(original.PaymentCode)
	offInitial, offError := core.ParseXDD(original.DisconnectCode)
	if docError != nil || payError != nil || offError != nil {
		a.warn("Некорректные коды XDD", fmt.Errorf("в исходной записи есть неверный код: документы=%d, оплата=%d, отключение=%d", original.DocumentsCode, original.PaymentCode, original.DisconnectCode))
		return false
	}
	dirIndex := func(b bool) int {
		if b {
			return 1
		}
		return 0
	}
	readCode := func(title string, direction *walk.ComboBox, months *walk.LineEdit, dd *walk.ComboBox) (int, error) {
		if direction == nil || months == nil || dd == nil {
			return 0, errors.New("форма не инициализирована")
		}
		m, err := core.ParseIntField(title+": количество месяцев", months.Text())
		if err != nil {
			return 0, err
		}
		d, err := chosenDay(dd.CurrentIndex())
		if err != nil {
			return 0, fmt.Errorf("%s: %w", title, err)
		}
		if direction.CurrentIndex() < 0 || direction.CurrentIndex() > 1 {
			return 0, fmt.Errorf("%s: выберите направление", title)
		}
		code, err := (core.XDD{Before: direction.CurrentIndex() == 1, Months: m, Day: d}).Code()
		if err != nil {
			return 0, fmt.Errorf("%s: %w", title, err)
		}
		return code, nil
	}
	var updateExamples func()
	updateExamples = func() {
		if docHint == nil || payHint == nil || offHint == nil || day == nil || monthBox == nil ||
			docDirection == nil || payDirection == nil || offDirection == nil ||
			docMonths == nil || payMonths == nil || offMonths == nil || docDD == nil || payDD == nil || offDD == nil {
			return
		}
		d, dayErr := core.ParseIntField("День начала цикла", day.Text())
		mo := monthBox.CurrentIndex() + 1
		year := time.Now().Year()
		preview := func(label string, direction *walk.ComboBox, months *walk.LineEdit, dd *walk.ComboBox, target *walk.Label) {
			code, err := readCode(label, direction, months, dd)
			if err != nil {
				target.SetText("Введите месяц/день: " + err.Error())
				return
			}
			c, _ := core.ParseXDD(code)
			if dayErr != nil || d < 1 || d > 31 || mo < 1 || mo > 12 {
				target.SetText(fmt.Sprintf("Код БД: %d — %s; для примера даты задайте начало цикла", code, c.Label()))
				return
			}
			date := core.ExampleDate(code, d, mo, year)
			target.SetText(fmt.Sprintf("Код БД: %d — %s; пример: %02d.%02d.%d → %s", code, c.Label(), d, mo, year, date))
		}
		preview("Документы", docDirection, docMonths, docDD, docHint)
		preview("Оплата", payDirection, payMonths, payDD, payHint)
		preview("Отключение", offDirection, offMonths, offDD, offHint)
	}
	var saved bool
	reloadOptions := func(forContracts bool) {
		if forContracts {
			selected := pick(contractOptions, contractBox.CurrentIndex())
			opts, err := lookup(a.store.Config.ContractsSQL, contractSearch.Text(), selected)
			if err != nil {
				a.warn("Поиск договора", err)
				return
			}
			contractOptions = addMissing(opts, selected)
			if err := contractBox.SetModel(optionLabels(contractOptions)); err != nil {
				a.warn("Список договоров", err)
				return
			}
			contractBox.SetCurrentIndex(optionIndex(contractOptions, selected))
		} else {
			selected := pick(typeOptions, typeBox.CurrentIndex())
			opts, err := lookup(a.store.Config.DocumentTypesSQL, typeSearch.Text(), selected)
			if err != nil {
				a.warn("Поиск типа документа", err)
				return
			}
			typeOptions = addMissing(opts, selected)
			if err := typeBox.SetModel(optionLabels(typeOptions)); err != nil {
				a.warn("Список типов документов", err)
				return
			}
			typeBox.SetCurrentIndex(optionIndex(typeOptions, selected))
		}
	}
	onSave := func() {
		readInt := func(label string, e *walk.LineEdit) (int, error) { return core.ParseIntField(label, e.Text()) }
		d, err := readInt("День начала периода", day)
		if err != nil {
			a.warn("Некорректное значение", err)
			return
		}
		docCode, err := readCode("Получение документов", docDirection, docMonths, docDD)
		if err != nil {
			a.warn("Некорректное значение", err)
			return
		}
		payCode, err := readCode("Дата оплаты", payDirection, payMonths, payDD)
		if err != nil {
			a.warn("Некорректное значение", err)
			return
		}
		offCode, err := readCode("Дата отключения", offDirection, offMonths, offDD)
		if err != nil {
			a.warn("Некорректное значение", err)
			return
		}
		periodIndex := periodBox.CurrentIndex()
		if periodIndex < 0 || periodIndex > 2 {
			a.warn("Некорректное значение", errors.New("выберите период"))
			return
		}
		periods := []int{1, 3, 12}
		updated := core.Record{
			ContractID: pick(contractOptions, contractBox.CurrentIndex()),
			TypeID:     pick(typeOptions, typeBox.CurrentIndex()), Period: periods[periodIndex],
			Amount: strings.TrimSpace(amount.Text()), StartDay: d, StartMonth: monthBox.CurrentIndex() + 1,
			DocumentsCode: docCode, PaymentCode: payCode, DisconnectCode: offCode,
		}
		if autoBox.Checked() {
			updated.AutoPay = 1
		}
		if notifyBox.Checked() {
			updated.Notify = 1
		}
		if err := updated.Validate(); err != nil {
			a.warn("Проверка полей", err)
			return
		}
		if existing && updated.ContractID != original.ContractID {
			a.warn("Изменение договора", errors.New("ключ договора нельзя менять при редактировании"))
			return
		}
		saveButton.SetEnabled(false)
		ctx, cancel := core.QueryContext()
		if existing {
			err = a.store.Update(ctx, original, updated)
		} else {
			err = a.store.Insert(ctx, updated)
		}
		cancel()
		saveButton.SetEnabled(true)
		if err != nil {
			a.warn("Ошибка сохранения", err)
			return
		}
		saved = true
		dlg.Accept()
	}
	title := "Добавление параметров договора"
	if existing {
		title = fmt.Sprintf("Параметры договора №%d", original.ContractID)
	}
	err = Dialog{
		AssignTo: &dlg, Title: title,
		MinSize: Size{Width: 800, Height: 680}, Size: Size{Width: 1040, Height: 790},
		Layout: VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 7},
		Children: []Widget{
			Composite{Layout: HBox{Spacing: 7}, Children: []Widget{
				Label{Text: "Поиск договора:"},
				LineEdit{AssignTo: &contractSearch, CueBanner: "Введите код (или название, если SQL возвращает его)"},
				PushButton{Text: "Найти", OnClicked: func() { reloadOptions(true) }},
			}},
			Composite{Layout: Grid{Columns: 2, Spacing: 6}, Children: []Widget{
				Label{Text: "Договор:"},
				ComboBox{AssignTo: &contractBox, Model: optionLabels(contractOptions), CurrentIndex: optionIndex(contractOptions, original.ContractID), Enabled: !existing},
			}},
			Composite{Layout: HBox{Spacing: 7}, Children: []Widget{
				Label{Text: "Поиск типа документа:"},
				LineEdit{AssignTo: &typeSearch, CueBanner: "Код или название"},
				PushButton{Text: "Найти", OnClicked: func() { reloadOptions(false) }},
			}},
			Composite{Layout: Grid{Columns: 2, Spacing: 6}, Children: []Widget{
				Label{Text: "Тип документа автооплаты:"},
				ComboBox{AssignTo: &typeBox, Model: optionLabels(typeOptions), CurrentIndex: optionIndex(typeOptions, original.TypeID)},
				Label{Text: "Период оказания услуги:"},
				ComboBox{AssignTo: &periodBox, Model: []string{"Ежемесячно (1)", "Ежеквартально (3)", "Ежегодно (12)"}, CurrentIndex: periodIndex},
				Label{Text: "Автооплата:"},
				CheckBox{AssignTo: &autoBox, Text: "Создавать служебку автоматически", Checked: original.AutoPay == 1},
				Label{Text: "Сумма абонентской платы:"},
				LineEdit{AssignTo: &amount, Text: original.Amount},
				Label{Text: "День начала периода (1–31):"},
				LineEdit{AssignTo: &day, Text: intText(original.StartDay), OnTextChanged: func() { updateExamples() }},
				Label{Text: "Месяц начала цикла:"},
				ComboBox{AssignTo: &monthBox, Model: monthNames, CurrentIndex: original.StartMonth - 1, OnCurrentIndexChanged: func() { updateExamples() }},
				Label{Text: "Оповещение куратора:"},
				CheckBox{AssignTo: &notifyBox, Text: "Оповещать при отсутствии документов", Checked: original.Notify == 1},
			}},
			Label{Text: "Сроки относительно начала цикла. Укажите направление, число полных месяцев и день. DD=99 — последний день месяца."},
			Label{Text: "Получение документов:"},
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				Label{Text: "Направление:"}, ComboBox{AssignTo: &docDirection, Model: directionNames, CurrentIndex: dirIndex(docInitial.Before), OnCurrentIndexChanged: func() { updateExamples() }},
				Label{Text: "Месяцев:"}, LineEdit{AssignTo: &docMonths, Text: intText(docInitial.Months), OnTextChanged: func() { updateExamples() }},
				Label{Text: "День:"}, ComboBox{AssignTo: &docDD, Model: dayLabels(), CurrentIndex: dayIndex(docInitial.Day), OnCurrentIndexChanged: func() { updateExamples() }},
			}},
			Label{AssignTo: &docHint, Text: ""},
			Label{Text: "Оплата по договору:"},
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				Label{Text: "Направление:"}, ComboBox{AssignTo: &payDirection, Model: directionNames, CurrentIndex: dirIndex(payInitial.Before), OnCurrentIndexChanged: func() { updateExamples() }},
				Label{Text: "Месяцев:"}, LineEdit{AssignTo: &payMonths, Text: intText(payInitial.Months), OnTextChanged: func() { updateExamples() }},
				Label{Text: "День:"}, ComboBox{AssignTo: &payDD, Model: dayLabels(), CurrentIndex: dayIndex(payInitial.Day), OnCurrentIndexChanged: func() { updateExamples() }},
			}},
			Label{AssignTo: &payHint, Text: ""},
			Label{Text: "Отключение услуги:"},
			Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
				Label{Text: "Направление:"}, ComboBox{AssignTo: &offDirection, Model: directionNames, CurrentIndex: dirIndex(offInitial.Before), OnCurrentIndexChanged: func() { updateExamples() }},
				Label{Text: "Месяцев:"}, LineEdit{AssignTo: &offMonths, Text: intText(offInitial.Months), OnTextChanged: func() { updateExamples() }},
				Label{Text: "День:"}, ComboBox{AssignTo: &offDD, Model: dayLabels(), CurrentIndex: dayIndex(offInitial.Day), OnCurrentIndexChanged: func() { updateExamples() }},
			}},
			Label{AssignTo: &offHint, Text: ""},
			Label{Text: "Даты приведены для примера с началом цикла в текущем году; в БД записывается только код XDD."},
			Composite{Layout: HBox{Spacing: 10}, Children: []Widget{
				HSpacer{},
				PushButton{AssignTo: &saveButton, Text: "Сохранить", OnClicked: onSave},
				PushButton{Text: "Отмена", OnClicked: func() { dlg.Cancel() }},
			}},
		},
	}.Create(a.win)
	if err != nil {
		a.warn("Ошибка открытия формы", err)
		return false
	}
	updateExamples()
	dlg.Run()
	return saved
}