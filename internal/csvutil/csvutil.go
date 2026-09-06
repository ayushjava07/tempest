package csvutil

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"reflect"
	"strings"
)

func Marshal(records [][]string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	for _, record := range records {
		if err := w.Write(record); err != nil {
			return nil, fmt.Errorf("write record: %w", err)
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func Unmarshal(data []byte) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read csv: %w", err)
	}
	return records, nil
}

func MarshalStructs[T any](items []T) ([][]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	var records [][]string
	var headers []string
	t := reflect.TypeOf(items[0])
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		name := field.Name
		if tag, ok := field.Tag.Lookup("csv"); ok {
			name = tag
		}
		headers = append(headers, strings.ToLower(name))
	}
	records = append(records, headers)
	for _, item := range items {
		v := reflect.ValueOf(item)
		if v.Kind() == reflect.Ptr {
			v = v.Elem()
		}
		var row []string
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			row = append(row, fmt.Sprintf("%v", v.Field(i).Interface()))
		}
		records = append(records, row)
	}
	return records, nil
}

func ReadAll(r io.Reader) ([][]string, error) {
	cr := csv.NewReader(r)
	return cr.ReadAll()
}

func WriteAll(w io.Writer, records [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.WriteAll(records); err != nil {
		return fmt.Errorf("write csv: %w", err)
	}
	return nil
}
