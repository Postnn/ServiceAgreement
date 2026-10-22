package presentation

import (
	"strings"

	"github.com/Postnn/ServiceAgreement/internal/readmodel"
)

// UnconfiguredColumns displays all returned columns except ContractID and
// SearchText. Those service aliases remain in the SQL result for identity
// and searching, but are never added to the table UI.
func UnconfiguredColumns(names []string) []readmodel.ListColumn {
	columns := make([]readmodel.ListColumn, 0, len(names))
	for _, name := range names {
		if readmodel.IsServiceField(name) {
			continue
		}
		width := 170
		if strings.EqualFold(name, "ContractName") || strings.EqualFold(name, "Договор") {
			width = 350
		}
		columns = append(columns, readmodel.ListColumn{
			Field: name, Title: name, Width: width, Order: len(columns) + 1,
			SortField: name, Visible: true,
		})
	}
	return columns
}
