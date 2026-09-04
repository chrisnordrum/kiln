package eval

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Coerce converts a value that arrived from outside the program — a URL
// segment, a JSON body, a form field — into the type an action declared.
//
// The browser sends every form field as text, so this is the one place where
// "3" becomes 3 and "on" becomes true. Doing it here, against the declared
// type, is what keeps the rest of the runtime free of guessing.
func Coerce(raw any, kind string) (Value, error) {
	if raw == nil {
		return nil, nil
	}
	switch kind {
	case "text":
		return toText(raw), nil

	case "int", "ref":
		switch x := raw.(type) {
		case float64: // every JSON number decodes as float64
			return int64(x), nil
		case int64:
			return x, nil
		case string:
			n, err := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("%q is not a whole number", x)
			}
			return n, nil
		}

	case "num":
		switch x := raw.(type) {
		case float64:
			return x, nil
		case int64:
			return float64(x), nil
		case string:
			f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
			if err != nil {
				return nil, fmt.Errorf("%q is not a number", x)
			}
			return f, nil
		}

	case "bool":
		switch x := raw.(type) {
		case bool:
			return x, nil
		case string:
			// "on" is what an unchecked-then-checked HTML checkbox submits.
			switch strings.ToLower(strings.TrimSpace(x)) {
			case "true", "on", "yes", "1":
				return true, nil
			case "false", "off", "no", "0", "":
				return false, nil
			}
			return nil, fmt.Errorf("%q is not true or false", x)
		case float64:
			return x != 0, nil
		}

	case "at":
		switch x := raw.(type) {
		case time.Time:
			return x, nil
		case string:
			for _, layout := range []string{time.RFC3339, "2006-01-02T15:04", "2006-01-02"} {
				if t, err := time.Parse(layout, x); err == nil {
					return t.UTC(), nil
				}
			}
			return nil, fmt.Errorf("%q is not a timestamp", x)
		}
	}
	return nil, fmt.Errorf("cannot read %v as %s", raw, kind)
}

func toText(raw any) string {
	switch x := raw.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return fmt.Sprint(raw)
}
