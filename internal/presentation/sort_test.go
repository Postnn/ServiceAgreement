package presentation

import (
	"testing"
	"time"

	"github.com/Postnn/ServiceAgreement/internal/domain"
	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

func TestServicePeriodFreeChoice(t *testing.T) {
	cases := map[string]int{"Один месяц (1)": 1, "Три месяца (3)": 3, "Двенадцать месяцев (12)": 12, "2": 2, "15": 15, "255": 255}
	for text, value := range cases {
		got, err := domain.ParseServicePeriod(text)
		if err != nil || got != value {
			t.Errorf("%s -> %d, %v", text, got, err)
		}
	}
	for _, text := range []string{"", "ежемесячно", "0", "-1", "256", "3.5"} {
		if _, err := domain.ParseServicePeriod(text); err == nil {
			t.Errorf("accepted %q", text)
		}
	}
	if domain.ServicePeriodLabel(13) != "13" {
		t.Fatal("arbitrary period label lost")
	}
}

func TestSortingBySQLField(t *testing.T) {
	values := []readmodel.ListItem{
		{ID: 20, Values: map[string]string{"Title": "10"}, Raw: map[string]any{"Ranking": int64(10), "Amount": []byte("2.5000")}},
		{ID: 1, Values: map[string]string{"Title": "5"}, Raw: map[string]any{"Ranking": int64(5), "Amount": []byte("10.2500")}},
		{ID: 3, Values: map[string]string{"Title": "20"}, Raw: map[string]any{"Ranking": int64(20), "Amount": []byte("1.1000")}},
	}
	SortListItems(values, "ranking", false)
	if values[0].ID != 1 || values[1].ID != 20 || values[2].ID != 3 {
		t.Fatalf("numeric sort: %+v", values)
	}
	SortListItems(values, "amount", true)
	if values[0].ID != 1 || values[1].ID != 20 || values[2].ID != 3 {
		t.Fatalf("money descending: %+v", values)
	}
	dates := []readmodel.ListItem{
		{ID: 1, Raw: map[string]any{"D": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}},
		{ID: 2, Raw: map[string]any{"D": time.Date(2024, 12, 1, 0, 0, 0, 0, time.UTC)}},
	}
	SortListItems(dates, "D", false)
	if dates[0].ID != 2 {
		t.Fatal("SQL date sort failed")
	}
}
