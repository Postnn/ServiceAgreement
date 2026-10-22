//go:build windows

package main

import (
"fmt"
"strings"

"github.com/Postnn/ServiceAgreement/internal/domain"
"github.com/lxn/walk"
"github.com/lxn/win"
)

func optionLabels(options []domain.Option) []string {
	labels := make([]string, 0, len(options))
	for _, o := range options {
		labels = append(labels, fmt.Sprintf("%d — %s", o.ID, o.Label))
	}
	return labels
}
func optionIndex(options []domain.Option, id int) int {
	for i, o := range options {
		if o.ID == id {
			return i
		}
	}
	return -1
}
func addMissing(options []domain.Option, id int) []domain.Option {
	if id > 0 && optionIndex(options, id) < 0 {
		return append(options, domain.Option{ID: id, Label: "Текущее значение (не возвращено запросом)"})
	}
	return options
}

type lookupControl struct {
	owner           *walk.MainWindow
	box             *walk.ComboBox
	options         []domain.Option
	selected        int
	busy            bool
	label           string
	lastSearch      string
	lastResultCount int
	query           func(search string, selected int) ([]domain.Option, error)
}

func (l *lookupControl) setOptions(options []domain.Option, selectID int) error {
	l.busy = true
	defer func() { l.busy = false }()
	l.options = options
	l.selected = selectID
	if l.box == nil {
		return nil
	}
	if err := l.box.SetModel(optionLabels(options)); err != nil {
		return err
	}
	return l.box.SetCurrentIndex(optionIndex(options, selectID))
}

func (l *lookupControl) typed() {
	if l.busy {
		return
	}
	// An edit invalidates the old selection. Free text must never be persisted
	// as an old, unrelated contract/type ID.
	l.selected = 0
	l.lastSearch = ""
}

func (l *lookupControl) chose() {
	if l.busy || l.box == nil {
		return
	}
	i := l.box.CurrentIndex()
	if i >= 0 && i < len(l.options) {
		l.selected = l.options[i].ID
	}
}

func (l *lookupControl) value() int {
	if l.box == nil {
		return 0
	}
	text := strings.TrimSpace(l.box.Text())
	for i, label := range optionLabels(l.options) {
		if text == label {
			return l.options[i].ID
		}
	}
	return 0
}

// search runs on Enter (or when the user finishes editing). The combo box is
// itself the search input: no redundant LineEdit or search button is needed.
func (l *lookupControl) search() {
	if l.busy || l.box == nil || !l.box.Enabled() {
		return
	}
	q := strings.TrimSpace(l.box.Text())
	if l.value() > 0 {
		return
	} // already picked a concrete option
	if l.lastResultCount > 1 && l.lastSearch == q {
		return // one Enter can fire both KeyDown and EditingFinished
	}
	options, err := l.query(q, 0)
	if err != nil {
		walk.MsgBox(l.owner, "Поиск — "+l.label, err.Error(), walk.MsgBoxIconError)
		return
	}
	if len(options) == 0 {
		walk.MsgBox(l.owner, "Поиск — "+l.label, "Совпадений не найдено. Измените строку поиска.", walk.MsgBoxIconInformation)
		return
	}
	l.lastSearch = q
	l.lastResultCount = len(options)
	if len(options) == 1 {
		if err := l.setOptions(options, options[0].ID); err != nil {
			walk.MsgBox(l.owner, "Выбор — "+l.label, err.Error(), walk.MsgBoxIconError)
		}
		return
	}
	if err := l.setOptions(options, 0); err != nil {
		walk.MsgBox(l.owner, "Выбор — "+l.label, err.Error(), walk.MsgBoxIconError)
		return
	}
	l.busy = true
	_ = l.box.SetText(q)
	l.busy = false
	// Open the same dropdown to let the user pick one of the multiple matches.
	l.box.SendMessage(win.CB_SHOWDROPDOWN, 1, 0)
}
