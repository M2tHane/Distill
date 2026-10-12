package gui

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

const mdSample = "# 标题一\n\n## 标题二\n\n#### 标题四\n\n" +
	"一段**粗体**、*斜体*、`行内代码`、~~删除线~~ 和 [链接](https://example.com)。\n第二行保留换行。\n\n" +
	"- 列表项\n- 嵌套：\n  1. 第一\n  2. 第二\n- [x] 已完成\n- [ ] 未完成\n\n" +
	"> 引用的一段话\n\n" +
	"```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n\n" +
	"| 原型文件 | 截图 | 路由 |\n| --- | --- | --- |\n| Main | Main.png | `/chat` |\n| Trace | Trace.png | `/trace` |\n\n---\n\n结尾。"

func TestMarkdown(t *testing.T) {
	md := newMDView()
	view := func(c *ui.Context) {
		ui.Scroll(c).Fill().Padding(16).Selectable().Children(func() { markdown(c, mdSample, md) })
	}
	tt := ui.NewTester(view, 640, 900)
	tt.Frame()
	for _, s := range []string{"标题一", "粗体", "行内代码", "第一", "已完成", "引用的一段话", "Trace.png", "结尾。", "go"} {
		if !tt.HasText(s) {
			t.Errorf("no %q in %q", s, tt.Texts())
		}
	}
	for _, s := range []string{"**", "```", "| ---"} {
		for _, x := range tt.Texts() {
			if strings.Contains(x, s) {
				t.Errorf("raw markdown %q shows: %q", s, x)
			}
		}
	}
	if out := os.Getenv("DISTILL_SNAPSHOTS"); out != "" {
		f, _ := os.Create(filepath.Join(out, "6-markdown.png"))
		png.Encode(f, tt.Image())
		f.Close()
	}
}

func TestMDSize(t *testing.T) {
	for level, want := range map[int]float32{1: 16, 2: 15, 3: 14, 4: 13, 6: 11, 9: 11} {
		if got := mdSize(level); got != want {
			t.Errorf("level %d: %v, want %v", level, got, want)
		}
	}
}
