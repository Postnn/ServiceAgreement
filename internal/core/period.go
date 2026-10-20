package core

import (
	"errors"
	"strconv"
	"strings"
)

// ServicePeriodChoices are labels shown in the editable period combo box.
// These are shortcuts, not a restriction on the value stored in SQL tinyint.
var ServicePeriodChoices = []string{
	"Один месяц (1)",
	"Три месяца (3)",
	"Двенадцать месяцев (12)",
}

func ServicePeriodLabel(period int) string {
	switch period {
	case 1:
		return ServicePeriodChoices[0]
	case 3:
		return ServicePeriodChoices[1]
	case 12:
		return ServicePeriodChoices[2]
	default:
		return strconv.Itoa(period)
	}
}

func ParseServicePeriod(text string) (int, error) {
	text = strings.TrimSpace(text)
	for i, option := range ServicePeriodChoices {
		if strings.EqualFold(option, text) {
			return []int{1, 3, 12}[i], nil
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > 255 {
		return 0, errors.New("период оказания услуги: введите целое число месяцев от 1 до 255")
	}
	return n, nil
}