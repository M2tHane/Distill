package gui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/dade/agentlog"
)

// exportMarkdown writes sessions as Markdown: one to a file the user names,
// several to a folder the user picks.
func (a *app) exportMarkdown(list []agentlog.Session) {
	go func() {
		var dir, file string
		if len(list) == 1 {
			p, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      a.win,
				Title:       "导出 Markdown",
				DefaultPath: filepath.Join(a.p.Home, "Downloads", fileName(list[0].Title)+".md"),
			})
			if err != nil || p == "" {
				return
			}
			file = p
		} else {
			picked, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:            a.win,
				Title:             fmt.Sprintf("导出 %d 个会话到文件夹", len(list)),
				ButtonLabel:       "导出到这里",
				Directory:         true,
				CreateDirectories: true,
			})
			if err != nil || len(picked) == 0 {
				return
			}
			dir = picked[0]
		}
		var failed []string
		for _, x := range list {
			out := file
			if out == "" {
				out = filepath.Join(dir, exportName(x))
			}
			if err := a.writeMarkdown(x, out); err != nil {
				failed = append(failed, x.Title+"："+err.Error())
			}
		}
		a.win.Update(func() {
			if len(failed) > 0 {
				a.toast = &ui.Toast{Title: "部分会话导出失败", Description: strings.Join(failed, "\n"), Type: "error"}
				return
			}
			where := file
			if where == "" {
				where = dir
			}
			a.toast = &ui.Toast{Title: fmt.Sprintf("已导出 %d 个会话", len(list)), Description: where, Action: "在 Finder 中显示", OnAction: func() { mygo.Shell.ShowItemInFolder(where) }}
		})
	}()
}

func exportName(x agentlog.Session) string {
	return fmt.Sprintf("%s-%s-%s.md", x.Updated.Local().Format("20060102"), x.Agent, fileName(x.Title))
}

func (a *app) writeMarkdown(x agentlog.Session, out string) error {
	t, err := agentlog.Load(a.srcs, x)
	if err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := agentlog.WriteMarkdown(f, t, agentlog.MarkdownOptions{}); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
