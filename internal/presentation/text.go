package presentation

import (
	"fmt"
	"time"
)

// SQLText formats the value returned by SQL Server; it never derives extra fields.
func SQLText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(x)
	case time.Time:
		if x.Hour() == 0 && x.Minute() == 0 && x.Second() == 0 {
			return x.Format("02.01.2006")
		}
		return x.Format("02.01.2006 15:04:05")
	default:
		return fmt.Sprint(x)
	}
}
