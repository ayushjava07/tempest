package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type OutputFormat string

const (
	FormatText OutputFormat = "text"
	FormatJSON OutputFormat = "json"
	FormatCSV  OutputFormat = "csv"
)

func WriteTable(w io.Writer, headers []string, rows [][]string) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	for _, row := range rows {
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}

func WriteJSON(w io.Writer, data any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

func WriteCSV(w io.Writer, headers []string, rows [][]string) error {
	fmt.Fprintln(w, strings.Join(headers, ","))
	for _, row := range rows {
		escaped := make([]string, len(row))
		for i, cell := range row {
			if strings.ContainsAny(cell, ",\"") {
				cell = fmt.Sprintf("%q", cell)
			}
			escaped[i] = cell
		}
		fmt.Fprintln(w, strings.Join(escaped, ","))
	}
	return nil
}
