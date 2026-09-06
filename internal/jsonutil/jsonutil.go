package jsonutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func MarshalIndent(data any) ([]byte, error) {
	return json.MarshalIndent(data, "", "  ")
}

func Marshal(data any) ([]byte, error) {
	return json.Marshal(data)
}

func Unmarshal(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return fmt.Errorf("json decode: %w", err)
	}
	return nil
}

func UnmarshalLenient(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

func PrettyPrint(data any) (string, error) {
	b, err := MarshalIndent(data)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func Minify(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Validate(data []byte) error {
	return json.Unmarshal(data, &struct{}{})
}

func Merge(base, override []byte) ([]byte, error) {
	var baseMap, overrideMap map[string]any
	if err := json.Unmarshal(base, &baseMap); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(override, &overrideMap); err != nil {
		return nil, err
	}
	for k, v := range overrideMap {
		baseMap[k] = v
	}
	return json.Marshal(baseMap)
}

func PrettyPrintString(data any) string {
	s, _ := PrettyPrint(data)
	return s
}

func ValidJSON(s string) bool {
	return json.Unmarshal([]byte(s), &struct{}{}) == nil
}

func Indent(data []byte, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(json.RawMessage(data), prefix, indent)
}

func Normalize(data []byte) ([]byte, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

func FromReader(r io.Reader, v any) error {
	return json.NewDecoder(r).Decode(v)
}

func ToWriter(w io.Writer, v any) error {
	return json.NewEncoder(w).Encode(v)
}

func ToJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func FromJSON(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

func Keys(data []byte) ([]string, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys, nil
}

func Get(data []byte, path string) (any, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	parts := strings.Split(path, ".")
	var current any = m
	for _, part := range parts {
		cm, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("path %s not found", path)
		}
		current, ok = cm[part]
		if !ok {
			return nil, fmt.Errorf("path %s not found", path)
		}
	}
	return current, nil
}
