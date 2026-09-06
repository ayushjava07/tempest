package duration

import (
	"fmt"
	"strings"
	"time"
)

func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	return time.ParseDuration(s)
}

func FormatShort(d time.Duration) string {
	if d < time.Millisecond {
		return fmt.Sprintf("%dus", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

func FormatHuman(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d milliseconds", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	return fmt.Sprintf("%d hours", int(d.Hours()))
}

func ParseHuman(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	parts := strings.Fields(s)
	if len(parts) != 2 {
		return ParseDuration(s)
	}
	var num int
	var unit string
	n, err := fmt.Sscanf(parts[0], "%d", &num)
	if n != 1 || err != nil {
		return ParseDuration(s)
	}
	unit = parts[1]
	if strings.HasSuffix(unit, "s") {
		unit = unit[:len(unit)-1]
	}
	switch unit {
	case "ns":
		return time.Duration(num) * time.Nanosecond, nil
	case "us", "microsecond":
		return time.Duration(num) * time.Microsecond, nil
	case "ms", "millisecond":
		return time.Duration(num) * time.Millisecond, nil
	case "s", "second":
		return time.Duration(num) * time.Second, nil
	case "m", "minute":
		return time.Duration(num) * time.Minute, nil
	case "h", "hour":
		return time.Duration(num) * time.Hour, nil
	case "d", "day":
		return time.Duration(num) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("unknown unit: %s", unit)
	}
}

func Clamp(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

func Ceil(d, precision time.Duration) time.Duration {
	if precision <= 0 {
		return d
	}
	return ((d + precision - 1) / precision) * precision
}

func Floor(d, precision time.Duration) time.Duration {
	if precision <= 0 {
		return d
	}
	return (d / precision) * precision
}
