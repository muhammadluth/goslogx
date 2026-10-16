package pg

import "strings"

// safeOperation admits only fixed SQL operation names, including raw-query fallback.
func safeOperation(operation string) string {
	switch strings.ToUpper(operation) {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER", "TRUNCATE", "MERGE", "BEGIN", "COMMIT", "ROLLBACK", "SHOW", "EXPLAIN":
		return strings.ToUpper(operation)
	default:
		return "QUERY"
	}
}
