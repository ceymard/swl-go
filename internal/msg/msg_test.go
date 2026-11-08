package msg

import (
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	for d, want := range map[time.Duration]string{
		0:                                     "0ms",
		999 * time.Millisecond:                "999ms",
		time.Second:                           "1s0ms",
		12*time.Second + 345*time.Millisecond: "12s345ms",
		59*time.Second + 999*time.Millisecond: "59s999ms",
		time.Minute:                           "1m0s0ms",
		3*time.Minute + 12*time.Second + 5*time.Millisecond: "3m12s5ms",
	} {
		if got := FormatDuration(d); got != want {
			t.Errorf("%v: got %q want %q", d, got, want)
		}
	}
}
