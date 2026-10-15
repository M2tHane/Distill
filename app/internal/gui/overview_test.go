package gui

import (
	"testing"
	"time"

	"github.com/M2tHane/Distill/agentlog"
)

func TestSumStats(t *testing.T) {
	now := time.Date(2026, 10, 14, 15, 0, 0, 0, time.Local) // a Wednesday
	at := func(days int) time.Time { return now.AddDate(0, 0, -days) }
	all := []agentlog.Session{
		{Agent: agentlog.Claude, CWD: "/a", Updated: at(0), Messages: 3, Size: 10},
		{Agent: agentlog.Claude, CWD: "/a", Updated: at(0), Messages: 2, Size: 5},
		{Agent: agentlog.Codex, CWD: "/b", Updated: at(6)},
		{Agent: agentlog.Pi, CWD: "/c", Updated: at(7)},
		{Agent: agentlog.Pi, CWD: "/c", Updated: at(400)}, // outside the map
	}
	s := sumStats(all, now)
	if s.total != 5 || s.messages != 5 || s.size != 15 {
		t.Errorf("total %d, messages %d, size %d", s.total, s.messages, s.size)
	}
	if s.weekAll != 3 || s.week[agentlog.Claude] != 2 || s.week[agentlog.Codex] != 1 || s.week[agentlog.Pi] != 0 {
		t.Errorf("week %d %v", s.weekAll, s.week)
	}
	if s.daily[dayOf(now)] != 2 || s.maxDaily != 2 {
		t.Errorf("today %d, max %d", s.daily[dayOf(now)], s.maxDaily)
	}

	if n, top, k := projects(all, dayOf(now)); n != 1 || top != "/a" || k != 2 {
		t.Errorf("today's projects: %d %q %d", n, top, k)
	}
	if n, top, k := projects(all, time.Time{}); n != 3 || top != "/a" || k != 2 {
		t.Errorf("all projects: %d %q %d", n, top, k)
	}
}

func TestHeatLevel(t *testing.T) {
	for _, c := range []struct{ n, most, want int }{
		{0, 5, 0}, {1, 0, 0}, {1, 8, 1}, {2, 8, 1}, {3, 8, 2}, {6, 8, 3}, {7, 8, 4}, {8, 8, 4},
	} {
		if got := heatLevel(c.n, c.most); got != c.want {
			t.Errorf("heatLevel(%d, %d) = %d, want %d", c.n, c.most, got, c.want)
		}
	}
}

func TestHeatLayout(t *testing.T) {
	today := time.Date(2026, 10, 14, 0, 0, 0, 0, time.Local) // a Wednesday
	h := newHeatLayout(900)
	if h.weeks != heatWeeks {
		t.Errorf("weeks %d in a wide box", h.weeks)
	}
	if d := h.day(today, h.weeks-1, 3); !d.Equal(today) {
		t.Errorf("last column's Wednesday is %v", d)
	}
	if d := h.day(today, h.weeks-2, 0); !d.Equal(time.Date(2026, 10, 4, 0, 0, 0, 0, time.Local)) {
		t.Errorf("previous column's Sunday is %v", d)
	}
	if narrow := newHeatLayout(300); narrow.weeks >= heatWeeks || narrow.left < heatLabelW {
		t.Errorf("narrow: %d weeks from %v", narrow.weeks, narrow.left)
	}
}

func TestCompact(t *testing.T) {
	for n, want := range map[int]string{7: "7", 1234: "1,234", 9092: "9,092", 99999: "99,999", 123456: "12.3 万"} {
		if got := compact(n); got != want {
			t.Errorf("compact(%d) = %q, want %q", n, got, want)
		}
	}
}
