package pg

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ceymard/swl-go/internal/jsonx"
	"github.com/jackc/pgx/v5/pgtype"
)

// cellErr describes why a value cannot be loaded. Kind is a short stable
// label used to aggregate counts once the detailed-report cap is reached.
type cellErr struct {
	Kind string
	Msg  string
}

// textChecker validates the text Postgres would feed to the column's input
// function. It must never reject something Postgres accepts: when unsure it
// returns nil (a missed error is still caught by Postgres at flush).
type textChecker func(s string) *cellErr

// cellText returns the text json_populate_record hands to a scalar column's
// input function for v: strings verbatim, numbers and booleans as JSON text.
func cellText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'f', -1, 32)
	case time.Time:
		return x.Format(time.RFC3339Nano)
	}
	b, err := jsonx.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if jsonx.Unmarshal(b, &s) == nil {
			return s
		}
	}
	return string(b)
}

// checkerFor builds the checker for a column, or nil when nothing is worth
// checking (text, json, arrays, composites, extension types, ...).
func checkerFor(c pgColumn) textChecker {
	if len(c.EnumVals) > 0 {
		set := make(map[string]struct{}, len(c.EnumVals))
		for _, l := range c.EnumVals {
			set[l] = struct{}{}
		}
		name := c.TypeName
		return func(s string) *cellErr {
			if _, ok := set[s]; ok {
				return nil
			}
			return &cellErr{"enum", fmt.Sprintf("invalid input value for enum %s: %q", name, s)}
		}
	}
	switch c.TypeOID {
	case pgtype.Int2OID:
		return intChecker("smallint", 16)
	case pgtype.Int4OID:
		return intChecker("integer", 32)
	case pgtype.Int8OID:
		return intChecker("bigint", 64)
	case pgtype.Float4OID:
		return floatChecker("real", 32)
	case pgtype.Float8OID:
		return floatChecker("double precision", 64)
	case pgtype.NumericOID:
		return numericChecker(c.Typmod)
	case pgtype.BoolOID:
		return boolChecker
	case pgtype.UUIDOID:
		return uuidChecker
	case pgtype.DateOID:
		return datetimeChecker("date", true, false)
	case pgtype.TimestampOID, pgtype.TimestamptzOID:
		return datetimeChecker("timestamp", true, true)
	case pgtype.TimeOID, pgtype.TimetzOID:
		return datetimeChecker("time", false, true)
	case pgtype.VarcharOID, pgtype.BPCharOID:
		return charChecker(c.TypeOID, c.Typmod)
	case pgtype.TextOID:
		return textNulChecker
	}
	return nil
}

func syntaxErr(typ, s string) *cellErr {
	return &cellErr{"invalid " + typ, fmt.Sprintf("invalid input syntax for type %s: %q", typ, s)}
}

func rangeErr(typ, s string) *cellErr {
	return &cellErr{"out of range " + typ, fmt.Sprintf("value %q is out of range for type %s", s, typ)}
}

// hasNumericExtras reports forms recent Postgres versions accept in integer
// and float input (underscores, 0x/0o/0b prefixes) that strconv base 10
// does not; such values are given the benefit of the doubt.
func hasNumericExtras(t string) bool {
	if strings.IndexByte(t, '_') >= 0 {
		return true
	}
	u := strings.TrimLeft(t, "+-")
	return len(u) > 2 && u[0] == '0' && strings.IndexByte("xXoObB", u[1]) >= 0
}

func intChecker(typ string, bits int) textChecker {
	return func(s string) *cellErr {
		t := strings.TrimSpace(s)
		if t == "" {
			return syntaxErr(typ, s)
		}
		if hasNumericExtras(t) {
			return nil
		}
		_, err := strconv.ParseInt(strings.TrimPrefix(t, "+"), 10, bits)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, strconv.ErrRange):
			return rangeErr(typ, s)
		}
		return syntaxErr(typ, s)
	}
}

func floatChecker(typ string, bits int) textChecker {
	return func(s string) *cellErr {
		t := strings.TrimSpace(s)
		if t == "" {
			return syntaxErr(typ, s)
		}
		if strings.IndexByte(t, '_') >= 0 {
			return nil
		}
		_, err := strconv.ParseFloat(t, bits)
		switch {
		case err == nil:
			return nil
		case errors.Is(err, strconv.ErrRange):
			return rangeErr(typ, s)
		}
		return syntaxErr(typ, s)
	}
}

// parseDecimal splits a plain decimal into significant digits and the position
// of the decimal point relative to the start of those digits (value =
// 0.DIGITS * 10^point). ok is false on a syntax error.
func parseDecimal(t string) (digits string, point int, ok bool) {
	i := 0
	if i < len(t) && (t[i] == '+' || t[i] == '-') {
		i++
	}
	var b strings.Builder
	intLen, seen := 0, false
	for ; i < len(t) && t[i] >= '0' && t[i] <= '9'; i++ {
		b.WriteByte(t[i])
		intLen++
		seen = true
	}
	if i < len(t) && t[i] == '.' {
		i++
		for ; i < len(t) && t[i] >= '0' && t[i] <= '9'; i++ {
			b.WriteByte(t[i])
			seen = true
		}
	}
	if !seen {
		return "", 0, false
	}
	exp := 0
	if i < len(t) && (t[i] == 'e' || t[i] == 'E') {
		i++
		j := i
		if j < len(t) && (t[j] == '+' || t[j] == '-') {
			j++
		}
		k := j
		for k < len(t) && t[k] >= '0' && t[k] <= '9' {
			k++
		}
		if k == j {
			return "", 0, false
		}
		n, err := strconv.Atoi(t[i:k])
		if err != nil {
			return "", 0, false
		}
		exp = n
		i = k
	}
	if i != len(t) {
		return "", 0, false
	}
	ds := b.String()
	point = intLen + exp
	trimmed := strings.TrimLeft(ds, "0")
	point -= len(ds) - len(trimmed)
	return strings.TrimRight(trimmed, "0"), point, true
}

func numericChecker(typmod int32) textChecker {
	constrained := typmod >= 4
	var prec, scale int
	if constrained {
		m := int(typmod) - 4
		prec = (m >> 16) & 0xffff
		scale = ((m & 0x7ff) ^ 1024) - 1024
	}
	return func(s string) *cellErr {
		t := strings.TrimSpace(s)
		switch strings.ToLower(strings.TrimLeft(t, "+-")) {
		case "nan", "inf", "infinity":
			return nil
		}
		digits, point, ok := parseDecimal(t)
		if !ok {
			return syntaxErr("numeric", s)
		}
		if !constrained || digits == "" {
			return nil
		}
		keep := point + scale
		if keep < 0 {
			return nil // rounds to zero
		}
		if keep < len(digits) && digits[keep] >= '5' {
			if strings.Trim(digits[:keep], "9") == "" {
				point++
			}
		}
		if point > prec-scale {
			return &cellErr{"numeric overflow", fmt.Sprintf("numeric field overflow: precision %d, scale %d cannot hold %q", prec, scale, s)}
		}
		return nil
	}
}

func boolChecker(s string) *cellErr {
	t := strings.ToLower(strings.TrimSpace(s))
	ok := false
	if t != "" {
		switch t[0] {
		case 't':
			ok = strings.HasPrefix("true", t)
		case 'f':
			ok = strings.HasPrefix("false", t)
		case 'y':
			ok = strings.HasPrefix("yes", t)
		case 'n':
			ok = strings.HasPrefix("no", t)
		case 'o':
			ok = t == "on" || (len(t) >= 2 && strings.HasPrefix("off", t))
		case '1':
			ok = t == "1"
		case '0':
			ok = t == "0"
		}
	}
	if ok {
		return nil
	}
	return syntaxErr("boolean", s)
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func uuidChecker(s string) *cellErr {
	t := strings.TrimSpace(s)
	if len(t) >= 2 && t[0] == '{' && t[len(t)-1] == '}' {
		t = t[1 : len(t)-1]
	}
	n := 0
	for i := 0; i < len(t); i++ {
		switch {
		case isHex(t[i]):
			n++
		case t[i] == '-':
		default:
			return syntaxErr("uuid", s)
		}
	}
	if n != 32 {
		return syntaxErr("uuid", s)
	}
	return nil
}

func daysIn(year, month int) int {
	switch month {
	case 2:
		if year%4 == 0 && (year%100 != 0 || year%400 == 0) {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}

func digitsAt(s string, i, n int) (int, bool) {
	if i+n > len(s) {
		return 0, false
	}
	v := 0
	for _, c := range []byte(s[i : i+n]) {
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	return v, true
}

// checkClock validates an HH:MM[:SS[.fff]] prefix of s. It returns false only
// when the text is clock-shaped yet out of range.
func checkClock(s string) bool {
	h, ok := digitsAt(s, 0, 2)
	if !ok || len(s) < 5 || s[2] != ':' {
		return true
	}
	m, ok := digitsAt(s, 3, 2)
	if !ok {
		return true
	}
	sec := 0
	if len(s) >= 8 && s[5] == ':' {
		if v, ok := digitsAt(s, 6, 2); ok {
			sec = v
		}
	}
	return h <= 24 && m <= 59 && sec <= 60 && (h < 24 || (m == 0 && sec == 0))
}

// datetimeChecker only rejects text in unambiguous ISO shape whose fields are
// out of range; every other spelling Postgres understands ("today",
// "01/02/2020", "Jan 2 2020", 20200102, ...) is accepted.
func datetimeChecker(typ string, hasDate, hasTime bool) textChecker {
	return func(s string) *cellErr {
		t := strings.TrimSpace(s)
		if t == "" {
			return syntaxErr(typ, s)
		}
		bad := func() *cellErr {
			return &cellErr{"invalid " + typ, fmt.Sprintf("date/time field value out of range: %q", s)}
		}
		rest := t
		if hasDate {
			if len(t) < 10 || t[4] != '-' || t[7] != '-' {
				return nil
			}
			y, ok1 := digitsAt(t, 0, 4)
			mo, ok2 := digitsAt(t, 5, 2)
			d, ok3 := digitsAt(t, 8, 2)
			if !ok1 || !ok2 || !ok3 {
				return nil
			}
			if y == 0 || mo < 1 || mo > 12 || d < 1 || d > daysIn(y, mo) {
				return bad()
			}
			rest = t[10:]
			if hasTime && len(rest) > 1 && (rest[0] == ' ' || rest[0] == 'T') {
				rest = rest[1:]
			} else {
				return nil
			}
		}
		if hasTime && !checkClock(rest) {
			return bad()
		}
		return nil
	}
}

func textNulChecker(s string) *cellErr {
	if strings.IndexByte(s, 0) >= 0 {
		return &cellErr{"NUL character", "NUL (\\u0000) cannot be stored in a text value"}
	}
	return nil
}

func charChecker(oid uint32, typmod int32) textChecker {
	max := int(typmod) - 4
	label := "character varying"
	if oid == pgtype.BPCharOID {
		label = "character"
	}
	return func(s string) *cellErr {
		if e := textNulChecker(s); e != nil {
			return e
		}
		if typmod < 5 {
			return nil
		}
		// Postgres silently drops trailing spaces beyond the limit.
		if utf8.RuneCountInString(strings.TrimRight(s, " ")) > max {
			return &cellErr{"too long " + label, fmt.Sprintf("value too long for type %s(%d)", label, max)}
		}
		return nil
	}
}
