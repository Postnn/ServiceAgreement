//go:build windows

package main

import (
	"errors"
	"fmt"
	"github.com/Postnn/ServiceAgreement/internal/core"
	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
	"strings"
	"time"
)

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

// edit displays the record-editing dialog.
func (a *application) edit(original core.Record, existing bool) bool {
	return a.editWithContract(original, existing, false, "")
}

// The second list fixes ContractID to the selected agreement while all other
// service terms remain editable. No additional edit form is needed.
func (a *application) editWithContract(original core.Record, existing bool, lockContract bool, contractLabel string) bool {
	var dlg *walk.Dialog
	var amount, day *walk.LineEdit
	var docMonths, payMonths, offMonths *walk.LineEdit
	var docDirection, payDirection, offDirection *walk.ComboBox
	var docDD, payDD, offDD *walk.ComboBox
	var docHint, payHint, offHint *walk.Label
	var contractBox, typeBox, periodBox, monthBox *walk.ComboBox
	var autoBox, notifyBox *walk.CheckBox
	var saveButton *walk.PushButton
	lookup := func(sqlText, search string, selected int) ([]core.Option, error) {
		ctx, cancel := a.store.Config.QueryContext()
        defer cancel()
        restore := beginSQLWait(a.win)
        options, err := a.store.Lookup(ctx, sqlText, search, selected)
        restore()
        return options, err
	}
	contractOptions, err := lookup(a.store.Config.ContractsSQL, "", original.ContractID)
	if err != nil {
		a.warn("Ошибка выбора договора", err)
		return false
	}
	contractOptions = addMissing(contractOptions, original.ContractID)
	if lockContract && strings.TrimSpace(contractLabel) != "" {
		for i := range contractOptions {
			if contractOptions[i].ID == original.ContractID {
				contractOptions[i].Label = contractLabel
			}
		}
	}
	typeOptions, err := lookup(a.store.Config.DocumentTypesSQL, "", original.TypeID)
	if err != nil {
		a.warn("Ошибка выбора типа документа", err)
		return false
	}
	typeOptions = addMissing(typeOptions, original.TypeID)
	contractChoice := &lookupControl{owner: a.win, label: "Договор", options: contractOptions, selected: original.ContractID,
		query: func(q string, selected int) ([]core.Option, error) {
			return lookup(a.store.Config.ContractsSQL, q, selected)
		}}
	typeChoice := &lookupControl{owner: a.win, label: "Тип документа основания автооплаты", options: typeOptions, selected: original.TypeID,
		query: func(q string, selected int) ([]core.Option, error) {
			return lookup(a.store.Config.DocumentTypesSQL, q, selected)
		}}
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
		d, dayErr := core.ParseIntField("День начала периода", day.Text())
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
				target.SetText(fmt.Sprintf("Код БД: %d — %s; для примера даты задайте начало периода", code, c.Label()))
				return
			}
			date := core.ExampleDate(code, d, mo, year)
			target.SetText(fmt.Sprintf("Код БД: %d — %s; пример: %02d.%02d.%d → %s", code, c.Label(), d, mo, year, date))
		}
		preview("Документы", docDirection, docMonths, docDD, docHint)
		preview("Оплата", payDirection, payMonths, payDD, payHint)
		preview("Отключение", offDirection, offMonths, offDD, offHint)
	}
	displayedAmount, amountFormatOK := core.MoneyDisplayN2(original.Amount)
    if !amountFormatOK {
        displayedAmount = original.Amount
    }

	var saved bool
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
		period, err := core.ParseServicePeriod(periodBox.Text())
		if err != nil {
			a.warn("Некорректное значение", err)
			return
		}
		storedAmount, err := core.MoneyValueFromEditor(amount.Text(), displayedAmount, original.Amount)
        if err != nil {
            a.warn("Сумма абонентской платы", err)
            return
        }
        updated := core.Record{
			ContractID: contractChoice.value(),
			TypeID:     typeChoice.value(), Period: period,
			Amount: storedAmount, StartDay: d, StartMonth: monthBox.CurrentIndex() + 1,
			DocumentsCode: docCode, PaymentCode: payCode, DisconnectCode: offCode,
		}
		if lockContract {
			// A disabled ComboBox must not be able to change the selected contract.
			updated.ContractID = original.ContractID
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
        restore := beginSQLWait(dlg)
		ctx, cancel := a.store.Config.QueryContext()
		if existing {
			err = a.store.Update(ctx, original, updated)
		} else {
			err = a.store.Insert(ctx, updated)
		}
		cancel()
        restore()
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
		AssignTo: &dlg, Title: title, Icon: a.appIcon,
		MinSize: Size{Width: 800, Height: 700}, Size: Size{Width: 1040, Height: 835},
		Layout: VBox{Margins: Margins{Left: 16, Top: 12, Right: 16, Bottom: 12}, Spacing: 7},
		Children: []Widget{
			Label{Text: "Начните вводить код/название и нажмите Enter. Один найденный вариант выбирается автоматически; несколько — показываются в списке."},
			Composite{Layout: Grid{Columns: 2, Spacing: 6}, Children: []Widget{
				Label{Text: "Договор:"},
				ComboBox{AssignTo: &contractBox, Editable: true, Model: optionLabels(contractOptions),
					CurrentIndex: optionIndex(contractOptions, original.ContractID), Enabled: !existing && !lockContract,
					OnTextChanged:         func() { contractChoice.typed() },
					OnCurrentIndexChanged: func() { contractChoice.chose() },
					OnEditingFinished:     func() { contractChoice.search() },
					OnKeyDown: func(key walk.Key) {
						if key == walk.KeyReturn {
							contractChoice.search()
						}
					},
				},
				Label{Text: "Тип документа основания автооплаты:"},
				ComboBox{AssignTo: &typeBox, Editable: true, Model: optionLabels(typeOptions),
					CurrentIndex:          optionIndex(typeOptions, original.TypeID),
					OnTextChanged:         func() { typeChoice.typed() },
					OnCurrentIndexChanged: func() { typeChoice.chose() },
					OnEditingFinished:     func() { typeChoice.search() },
					OnKeyDown: func(key walk.Key) {
						if key == walk.KeyReturn {
							typeChoice.search()
						}
					},
				},
				Label{Text: "Период оказания услуги (1–255 месяцев):"},
				ComboBox{AssignTo: &periodBox, Editable: true, Model: core.ServicePeriodChoices, Value: core.ServicePeriodLabel(original.Period)},
				Label{Text: "День начала периода (1–31):"},
				LineEdit{AssignTo: &day, Text: intText(original.StartDay), OnTextChanged: func() { updateExamples() }},
				Label{Text: "Месяц начала периода:"},
				ComboBox{AssignTo: &monthBox, Model: monthNames, CurrentIndex: original.StartMonth - 1, OnCurrentIndexChanged: func() { updateExamples() }},
				Label{Text: "Автооплата:"},
				CheckBox{AssignTo: &autoBox, Text: "Создавать служебку автоматически", Checked: original.AutoPay == 1},
				Label{Text: "Сумма абонентской платы:"},
				LineEdit{AssignTo: &amount, Text: displayedAmount,
                    ToolTipText: "Сумма в формате N2 (например, 1 234,56). Исходная точность SQL money сохраняется, если сумму не менять."},
				Label{Text: "Оповещение куратора:"},
				CheckBox{AssignTo: &notifyBox, Text: "Оповещать при отсутствии документов", Checked: original.Notify == 1},
			}},
			Label{Text: "Сроки относительно начала периода. Укажите направление, число полных месяцев и день. DD=99 — последний день месяца."},

            GroupBox{
                Title: "Получение документов",
                Layout: VBox{Margins: Margins{Left: 10, Top: 10, Right: 10, Bottom: 6}, Spacing: 4},
                Children: []Widget{
                    Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
                        Label{Text: "Направление:"},
                        ComboBox{AssignTo: &docDirection, Model: directionNames,
                            MinSize: Size{Width: 180}, MaxSize: Size{Width: 205},
                            CurrentIndex: dirIndex(docInitial.Before), OnCurrentIndexChanged: func() { updateExamples() }},
                        Label{Text: "Месяцев:"},
                        LineEdit{AssignTo: &docMonths, Text: intText(docInitial.Months),
                            MinSize: Size{Width: 76}, MaxSize: Size{Width: 76},
                            OnTextChanged: func() { updateExamples() }},
                        Label{Text: "День:"},
                        ComboBox{AssignTo: &docDD, Model: dayLabels(),
                            MinSize: Size{Width: 160}, MaxSize: Size{Width: 190},
                            CurrentIndex: dayIndex(docInitial.Day), OnCurrentIndexChanged: func() { updateExamples() }},
                        HSpacer{},
                    }},
                    Label{AssignTo: &docHint, Text: ""},
                },
            },
            GroupBox{
                Title: "Оплата по договору",
                Layout: VBox{Margins: Margins{Left: 10, Top: 10, Right: 10, Bottom: 6}, Spacing: 4},
                Children: []Widget{
                    Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
                        Label{Text: "Направление:"},
                        ComboBox{AssignTo: &payDirection, Model: directionNames,
                            MinSize: Size{Width: 180}, MaxSize: Size{Width: 205},
                            CurrentIndex: dirIndex(payInitial.Before), OnCurrentIndexChanged: func() { updateExamples() }},
                        Label{Text: "Месяцев:"},
                        LineEdit{AssignTo: &payMonths, Text: intText(payInitial.Months),
                            MinSize: Size{Width: 76}, MaxSize: Size{Width: 76},
                            OnTextChanged: func() { updateExamples() }},
                        Label{Text: "День:"},
                        ComboBox{AssignTo: &payDD, Model: dayLabels(),
                            MinSize: Size{Width: 160}, MaxSize: Size{Width: 190},
                            CurrentIndex: dayIndex(payInitial.Day), OnCurrentIndexChanged: func() { updateExamples() }},
                        HSpacer{},
                    }},
                    Label{AssignTo: &payHint, Text: ""},
                },
            },
            GroupBox{
                Title: "Отключение услуги",
                Layout: VBox{Margins: Margins{Left: 10, Top: 10, Right: 10, Bottom: 6}, Spacing: 4},
                Children: []Widget{
                    Composite{Layout: HBox{MarginsZero: true, Spacing: 7}, Children: []Widget{
                        Label{Text: "Направление:"},
                        ComboBox{AssignTo: &offDirection, Model: directionNames,
                            MinSize: Size{Width: 180}, MaxSize: Size{Width: 205},
                            CurrentIndex: dirIndex(offInitial.Before), OnCurrentIndexChanged: func() { updateExamples() }},
                        Label{Text: "Месяцев:"},
                        LineEdit{AssignTo: &offMonths, Text: intText(offInitial.Months),
                            MinSize: Size{Width: 76}, MaxSize: Size{Width: 76},
                            OnTextChanged: func() { updateExamples() }},
                        Label{Text: "День:"},
                        ComboBox{AssignTo: &offDD, Model: dayLabels(),
                            MinSize: Size{Width: 160}, MaxSize: Size{Width: 190},
                            CurrentIndex: dayIndex(offInitial.Day), OnCurrentIndexChanged: func() { updateExamples() }},
                        HSpacer{},
                    }},
                    Label{AssignTo: &offHint, Text: ""},
                },
            },
			Label{Text: "Даты приведены для примера с началом периода в текущем году; в БД записывается только код XDD."},
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
	contractChoice.box = contractBox
	typeChoice.box = typeBox
	updateExamples()

	dlg.Run()
	return saved
}