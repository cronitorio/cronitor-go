package cronitor

import (
	"encoding/json"
	"strconv"
)

// MonitorData is the parsed JSON document describing a monitor, as returned
// by the Monitor API. It is a plain map so every field is accessible; the
// methods provide typed access to the most common fields and to nested values.
type MonitorData map[string]any

// Key returns the monitor key.
func (d MonitorData) Key() string { return d.GetString("key") }

// Name returns the monitor name.
func (d MonitorData) Name() string { return d.GetString("name") }

// Type returns the monitor type (job, check or heartbeat).
func (d MonitorData) Type() string { return d.GetString("type") }

// Schedule returns the monitor schedule.
func (d MonitorData) Schedule() string { return d.GetString("schedule") }

// Get walks the document along path and returns the value found there. Map
// keys are matched by name and list elements by their decimal index, so
// d.Get("latest_events", "0", "event") reads the event of the first entry.
// ok is false when any segment of the path is missing.
func (d MonitorData) Get(path ...string) (value any, ok bool) {
	var cur any = map[string]any(d)
	for _, segment := range path {
		switch node := cur.(type) {
		case map[string]any:
			cur, ok = node[segment]
			if !ok {
				return nil, false
			}
		case []any:
			idx, err := strconv.Atoi(segment)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}
			cur = node[idx]
		default:
			return nil, false
		}
	}
	return cur, true
}

// GetString returns the string value at path, or "" if it is missing or not
// a string.
func (d MonitorData) GetString(path ...string) string {
	v, ok := d.Get(path...)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// GetFloat returns the numeric value at path, or 0 if it is missing or not a
// number.
func (d MonitorData) GetFloat(path ...string) float64 {
	v, ok := d.Get(path...)
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	}
	return 0
}

// GetBool returns the boolean value at path, or false if it is missing or not
// a boolean.
func (d MonitorData) GetBool(path ...string) bool {
	v, ok := d.Get(path...)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

// GetMap returns the object at path, or nil if it is missing or not an object.
func (d MonitorData) GetMap(path ...string) MonitorData {
	v, ok := d.Get(path...)
	if !ok {
		return nil
	}
	m, _ := v.(map[string]any)
	return MonitorData(m)
}

// GetList returns the list at path, or nil if it is missing or not a list.
func (d MonitorData) GetList(path ...string) []any {
	v, ok := d.Get(path...)
	if !ok {
		return nil
	}
	l, _ := v.([]any)
	return l
}

// String renders the document as indented JSON.
func (d MonitorData) String() string {
	out, err := json.MarshalIndent(map[string]any(d), "", "  ")
	if err != nil {
		return "{}"
	}
	return string(out)
}
