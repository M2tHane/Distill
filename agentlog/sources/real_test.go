package sources_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/sources"
)

// TestRealLogs reads every session of this computer both ways: listing,
// without tool results, must count the messages a full read gives. It logs
// how the calls ended, to look at the filtering.
//
//	AGENTLOG_REAL=1 go test ./sources -run RealLogs -v
func TestRealLogs(t *testing.T) {
	if os.Getenv("AGENTLOG_REAL") == "" {
		t.Skip("AGENTLOG_REAL not set")
	}
	home, _ := os.UserHomeDir()
	srcs := sources.All(home)
	res, err := (&agentlog.Index{Sources: srcs}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	counts := map[agentlog.ToolStatus]int{}
	kept, total := 0, 0
	for _, s := range res.Sessions {
		tr, err := agentlog.Load(srcs, s)
		if err != nil {
			t.Fatal(err)
		}
		if len(tr.Messages) != s.Messages {
			t.Errorf("%s: %d messages listed, %d read", s.Path, s.Messages, len(tr.Messages))
		}
		for _, m := range tr.Messages {
			for _, tc := range m.Tools {
				total++
				counts[tc.Status]++
				kept += len(tc.Output)
				if tc.Status == agentlog.ToolFailed && counts[tc.Status]%40 == 1 {
					t.Logf("failed %s %q:\n%s", tc.Name, tc.Summary, indent(tc.Output))
				}
			}
		}
	}
	t.Logf("%d sessions, %d calls: %v; %d KB of output kept", len(res.Sessions), total, counts, kept>>10)
}

func indent(s string) string { return "    " + strings.ReplaceAll(s, "\n", "\n    ") }
