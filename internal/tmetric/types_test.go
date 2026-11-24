package tmetric

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLocalTimeRoundTrip(t *testing.T) {
	var lt LocalTime
	if err := json.Unmarshal([]byte(`"2026-11-23T23:59:58.9876543"`), &lt); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(lt)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"2026-11-23T23:59:58"` {
		t.Errorf("marshal = %s", out)
	}
}

func TestLocalTimeKeepsWallClock(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skip("no tzdata:", err)
	}
	// 23:30 in Madrid is 22:30 UTC; the API must see 23:30, the same day.
	lt := NewLocalTime(time.Date(2026, 11, 23, 23, 30, 0, 0, madrid))
	if got := lt.String(); got != "2026-11-23T23:30:00" {
		t.Errorf("String = %s", got)
	}
	back := lt.In(madrid)
	if !back.Equal(time.Date(2026, 11, 23, 23, 30, 0, 0, madrid)) {
		t.Errorf("In = %v", back)
	}
}

func TestLocalTimeRejectsBadInput(t *testing.T) {
	var lt LocalTime
	for _, in := range []string{`42`, `"yesterday"`, `"2026-11-23T09:00:00Z"`} {
		if err := json.Unmarshal([]byte(in), &lt); err == nil {
			t.Errorf("Unmarshal(%s) should fail", in)
		}
	}
}
