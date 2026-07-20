package csvutil

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"time"
)

// WriteCSV sets the appropriate HTTP headers and writes the provided headers and rows as a CSV file.
// It also writes the UTF-8 BOM so Excel opens it correctly.
func WriteCSV(w http.ResponseWriter, filename string, headers []string, rows [][]string) error {
	// Set headers for download
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Write the UTF-8 BOM so Excel automatically recognizes the encoding
	_, err := w.Write([]byte{0xEF, 0xBB, 0xBF})
	if err != nil {
		return err
	}

	cw := csv.NewWriter(w)
	
	// Write headers
	if err := cw.Write(headers); err != nil {
		return err
	}

	// Write rows
	for _, row := range rows {
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	cw.Flush()
	return cw.Error()
}

// FormatDate converts a time.Time to the standard dd-MM-yyyy format.
// If the time is zero, it returns an empty string.
func FormatDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("02-01-2006")
}

// FormatDatePointer converts a *time.Time to the standard dd-MM-yyyy format.
// If the pointer is nil or the time is zero, it returns an empty string.
func FormatDatePointer(t *time.Time) string {
	if t == nil || t.IsZero() {
		return ""
	}
	return t.Format("02-01-2006")
}
