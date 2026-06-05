package dbtypes

import (
	"fmt"
	"time"
)

type Date string

func (d *Date) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*d = Date(val)
	case time.Time:
		*d = Date(val.Format(time.DateOnly))
	default:
		return fmt.Errorf("unexpected date type: %T", val)
	}
	return nil
}

type ClockTime string

func (c *ClockTime) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*c = ClockTime(val)
	case time.Time:
		*c = ClockTime(val.Format("15:04:05.999999"))
	default:
		return fmt.Errorf("unexpected clock time type: %T", val)
	}
	return nil
}

type ClockTimeTZ string

func (c *ClockTimeTZ) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*c = ClockTimeTZ(val)
	case time.Time:
		*c = ClockTimeTZ(val.Format("15:04:05.999999-" + getTimeZoneOffsetLayout(val)))
	default:
		return fmt.Errorf("unexpected clock time with time zone type: %T", val)
	}
	return nil
}

type DateTime string

func (d *DateTime) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*d = DateTime(val)
	case time.Time:
		*d = DateTime(val.Format("2006-01-02 15:04:05.999999"))
	default:
		return fmt.Errorf("unexpected date time type: %T", val)
	}
	return nil
}

type Timestamp string

func (t *Timestamp) Scan(src any) error {
	switch val := src.(type) {
	case string:
		*t = Timestamp(val)
	case time.Time:
		*t = Timestamp(val.Format("2006-01-02 15:04:05.999999-" + getTimeZoneOffsetLayout(val)))
	default:
		return fmt.Errorf("unexpected date time type: %T", val)
	}
	return nil
}

// getTimeZoneOffsetLayout returns the hour(:minute) portion of Go's numeric
// timezone-offset layout token. If the offset has a non-zero minutes portion,
// both hours and minutes are included (matching Postgres default behavior);
// otherwise just the hours portion is included.
//
// The callers prepend "-" to form the full token (e.g. "-07" or "-07:00").
// That leading "-" is NOT a literal dash: in Go's reference-time layout it is
// the sign placeholder, which time.Format substitutes with the actual sign of
// the offset. So "-07" renders as "+02" or "-05" as appropriate -- the values
// are correct, not double-signed.
func getTimeZoneOffsetLayout(t time.Time) string {
	_, offset := t.Zone()
	minutes := (offset % 3600) / 60
	if minutes == 0 {
		return "07"
	}
	return "07:00"
}
