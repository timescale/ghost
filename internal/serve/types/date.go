package types

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

// If the time zone offset includes a non-zero minutes portion, include it in
// the timestamp layout. Otherwise, just include the hours portion. This
// matches the default Postgres behavior.
func getTimeZoneOffsetLayout(t time.Time) string {
	_, offset := t.Zone()
	minutes := (offset % 3600) / 60

	if minutes == 0 {
		return "07"
	}
	return "07:00"
}
