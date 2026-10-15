package runner

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/testutil"
)

// TestRunners runs each installed agent twice in one session: the second
// turn must remember the first. It calls the real agents.
//
//	DISTILL_RUNNERS=1 go test -run Runners -v
func TestRunners(t *testing.T) {
	if os.Getenv("DISTILL_RUNNERS") == "" {
		t.Skip("DISTILL_RUNNERS not set")
	}
	env.LoginPath()
	p := env.NewPaths()
	models := map[agentlog.Agent]string{agentlog.Claude: "haiku", agentlog.Pi: os.Getenv("DISTILL_PI_MODEL")}
	for _, ag := range env.Agents {
		t.Run(string(ag), func(t *testing.T) {
			info := env.Detect(p, ag)
			if !info.Installed() {
				t.Skip("not installed")
			}
			dir := testutil.AgentDir(t)
			req := Request{Info: info, Model: models[ag], Effort: "low", Dir: dir, SessionID: NewUUID()}
			turn := func(prompt string) string {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				req.Prompt = prompt
				var text, errs strings.Builder
				id, err := Run(ctx, req, func(ev Event) {
					text.WriteString(ev.Text)
					errs.WriteString(ev.Err)
				})
				if err != nil {
					t.Fatalf("run: %v: %s", err, errs.String())
				}
				req.SessionID, req.Resume = id, true
				return text.String()
			}
			first := turn("记住暗号：蓝鲸42。只回复“好”。")
			second := turn("刚才的暗号是什么？只回复暗号本身。")
			t.Logf("first %q, second %q", first, second)
			if !strings.Contains(second, "蓝鲸42") {
				t.Errorf("the second turn did not remember the first: %q", second)
			}
		})
	}
}
