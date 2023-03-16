package timeutils

import "time"

// Now returns the current local time in UTC timezone
func Now() time.Time {
	return time.Now().UTC()
}

// Unix returns the local time corresponding to the Unix time of Now,
// which is the number of seconds elapsed since January 1, 1970 UTC
func UnixNow() int64 {
	return Now().Unix()
}

// UnixMilliNow returns the local time corresponding to the UnixMilli time of Now,
// which is the number of milliseconds elapsed since January 1, 1970 UTC
func UnixMilliNow() int64 {
	return Now().UnixMilli()
}

// UnixNanoNow returns the local time corresponding to the UnixNano time of Now,
// which is the number of nanoseconds elapsed since January 1, 1970 UTC
func UnixNanoNow() int64 {
	return Now().UnixMilli()
}

// Format returns a string representation of the time value formatted using the given layout string
func Format(t time.Time, layout string) string {
	return t.UTC().Format(layout)
}
