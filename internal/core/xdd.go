package core

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// XDD is a signed number of complete service-month boundaries followed by a
// calendar day. The sign expresses whether a deadline is before or after the
// cycle start, including X=0: +006 is stored as 6, -006 as -6.
// Day=99 means the last calendar day (February 28/29 is calculated by year).
// The database stores an int, not the leading zeros shown in the UI.
type XDD struct {
	Before bool
	Months int
	Day    int
}

func (c XDD) Code() (int, error) {
	if c.Months < 0 || c.Months > math.MaxInt32/100 {
		return 0, errors.New("смещение X выходит за диапазон SQL int")
	}
	if c.Day != 99 && (c.Day < 1 || c.Day > 31) {
		return 0, errors.New("день DD должен быть 01..31 либо 99 (последний день месяца)")
	}
	a := int64(c.Months)*100 + int64(c.Day)
	if a > math.MaxInt32 {
		return 0, errors.New("код XDD выходит за диапазон SQL int")
	}
	if c.Before {
		a = -a
	}
	return int(a), nil
}

func ParseXDD(value int) (XDD, error) {
	if value == 0 || value < math.MinInt32 || value > math.MaxInt32 {
		return XDD{}, errors.New("код XDD должен помещаться в SQL int и не быть нулём")
	}
	a := int64(value)
	if a < 0 {
		a = -a
	}
	c := XDD{Before: value < 0, Months: int(a / 100), Day: int(a % 100)}
	if _, err := c.Code(); err != nil {
		return XDD{}, err
	}
	return c, nil
}

func ValidateXDD(value int, kind string) error {
	if kind != "документы" && kind != "оплата" && kind != "отключение" {
		return errors.New("неизвестный тип XDD: " + kind)
	}
	if _, err := ParseXDD(value); err != nil {
		return fmt.Errorf("%s: %w", kind, err)
	}
	return nil
}

func (c XDD) DayLabel() string {
	if c.Day == 99 {
		return "последний день месяца"
	}
	return fmt.Sprintf("%d-е число", c.Day)
}

func (c XDD) Label() string {
	if c.Before {
		if c.Months == 0 {
			return "до начала, без полных месяцев · " + c.DayLabel()
		}
		return fmt.Sprintf("до начала, за %d мес. · %s", c.Months, c.DayLabel())
	}
	if c.Months == 0 {
		return "от начала, без полных месяцев · " + c.DayLabel()
	}
	return fmt.Sprintf("от начала, через %d мес. · %s", c.Months, c.DayLabel())
}

func ExplainXDD(value int) string {
	c, err := ParseXDD(value)
	if err != nil {
		return fmt.Sprintf("Некорректный код %d", value)
	}
	return c.Label()
}

// AddMonthsClamped moves by calendar months, preserving the original desired
// day where possible and clamping to the final day of shorter months.
func AddMonthsClamped(t time.Time, offset int) time.Time {
	y, m, d := t.Date()
	first := time.Date(y, m, 1, 0, 0, 0, 0, t.Location()).AddDate(0, offset, 0)
	last := daysInMonth(first.Year(), first.Month())
	if d > last {
		d = last
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, t.Location())
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

func resolveMonthDay(year int, month time.Month, day int) (time.Time, bool) {
	if day == 99 {
		day = daysInMonth(year, month)
	}
	if day > daysInMonth(year, month) || day < 1 {
		return time.Time{}, false
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), true
}

// ResolveXDD provides a *calendar illustration* for a concrete cycle start.
// For nonnegative codes X counts complete months after cycle start, then takes
// the first occurrence of DD not earlier than that boundary. This correctly
// maps 306 for a 07 Jan start to 06 May, and 320 to 20 April.
// For negative -NDD, N>=1 selects the Nth service-month window before start.
// For negative -0DD the last matching day before the start is used.
// Codes remain relative rules; an application must not treat the example year
// as the actual start date of the customer's contract.
func ResolveXDD(start time.Time, code int) (time.Time, error) {
	c, err := ParseXDD(code)
	if err != nil {
		return time.Time{}, err
	}
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, time.UTC)
	if !c.Before {
		boundary := AddMonthsClamped(start, c.Months)
		for monthOffset := 0; monthOffset < 14; monthOffset++ {
			first := AddMonthsClamped(time.Date(boundary.Year(), boundary.Month(), 1, 0, 0, 0, 0, time.UTC), monthOffset)
			if due, ok := resolveMonthDay(first.Year(), first.Month(), c.Day); ok && !due.Before(boundary) {
				return due, nil
			}
		}
		return time.Time{}, errors.New("не удалось определить следующую календарную дату")
	}
	if c.Months == 0 {
		// The minus sign of -006 is significant: 06 Jan is before 07 Jan.
		for back := 0; back < 14; back++ {
			first := AddMonthsClamped(time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC), -back)
			if due, ok := resolveMonthDay(first.Year(), first.Month(), c.Day); ok && due.Before(start) {
				return due, nil
			}
		}
		return time.Time{}, errors.New("не удалось определить предыдущую календарную дату")
	}
	boundary := AddMonthsClamped(start, -c.Months)
	upper := AddMonthsClamped(start, -c.Months+1)
	for step := 0; step < 3; step++ {
		first := AddMonthsClamped(time.Date(boundary.Year(), boundary.Month(), 1, 0, 0, 0, 0, time.UTC), step)
		if due, ok := resolveMonthDay(first.Year(), first.Month(), c.Day); ok && !due.Before(boundary) && due.Before(upper) {
			return due, nil
		}
	}
	return time.Time{}, errors.New("в этом месячном интервале указанного числа нет; используйте 99 для конца месяца")
}

// ExampleDate uses a representative year. It must be identified as an example,
// never as the real contractual payment or shut-off date.
func ExampleDate(code, startDay, startMonth, year int) string {
	if startDay < 1 || startDay > 31 || startMonth < 1 || startMonth > 12 {
		return ""
	}
	first, valid := resolveMonthDay(year, time.Month(startMonth), startDay)
	if !valid {
		return "нет даты в примере (проверьте 29–31 число)"
	}
	due, err := ResolveXDD(first, code)
	if err != nil {
		return "нет даты в примере: " + err.Error()
	}
	return due.Format("02.01.2006")
}