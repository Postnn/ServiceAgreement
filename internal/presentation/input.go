package presentation

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseIntField is shared by UI and unit tests.
func ParseIntField(label, input string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return 0, fmt.Errorf("%s: требуется целое число", label)
	}
	return n, nil
}
