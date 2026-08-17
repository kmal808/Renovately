package handlers

import (
	"sort"
	"strconv"

	"reno/internal/ui"
)

func strconvFormat(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func parseFloatOrZero(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// materialsSort sorts rows in place by the given column.
func materialsSort(rows []ui.MaterialRow, field string, desc bool) {
	less := func(a, b ui.MaterialRow) bool {
		switch field {
		case "category":
			return a.Category < b.Category
		case "quantity":
			return parseFloatOrZero(a.Quantity) < parseFloatOrZero(b.Quantity)
		case "unit_cost_cents":
			return a.UnitCost < b.UnitCost
		case "status":
			return a.Status < b.Status
		case "expected_delivery":
			return a.Expected < b.Expected
		default:
			return a.Item < b.Item
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if desc {
			return less(rows[j], rows[i])
		}
		return less(rows[i], rows[j])
	})
}
