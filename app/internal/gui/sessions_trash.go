package gui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/dade/agentlog"
)

func (a *app) deleteDialog(c *ui.Context) {
	s := &a.sessions
	if !s.confirmDelete {
		return
	}
	var size int64
	for _, x := range s.toDelete {
		size += x.Size
	}
	title := fmt.Sprintf("删除 %d 个会话？", len(s.toDelete))
	if len(s.toDelete) == 1 {
		title = fmt.Sprintf("删除「%s」？", s.toDelete[0].Title)
	}
	msg := fmt.Sprintf("共 %s。文件会移到 Distill 回收站，30 天内可以恢复；删除后在原 Agent 中无法 resume。", humanSize(size))
	if ui.AlertDialog(c, &s.confirmDelete, title, msg, "取消", "移到回收站") == 1 {
		a.deleteSessions(c, s.toDelete)
	}
}

func (a *app) deleteSessions(c *ui.Context, list []agentlog.Session) {
	s := &a.sessions
	var moved []agentlog.TrashItem
	gone := map[string]bool{}
	for _, x := range list {
		it, err := a.trash.Move(x)
		if err != nil {
			c.AddToast(ui.Toast{Title: "删除失败", Description: err.Error(), Type: "error"})
			continue
		}
		moved = append(moved, it)
		gone[x.Path] = true
		a.ix.Forget(x.Path)
		delete(s.selected, x.Path)
		if s.view.session != nil && s.view.session.Path == x.Path {
			s.view.close()
		}
	}
	kept := s.all[:0:0]
	for _, x := range s.all {
		if !gone[x.Path] {
			kept = append(kept, x)
		}
	}
	s.all = kept
	if len(moved) > 0 {
		c.ToastAction(fmt.Sprintf("已将 %d 个会话移到回收站", len(moved)), "撤销", func() {
			for _, it := range moved {
				a.trash.Restore(it.ID)
			}
			go a.rescan()
		})
	}
}

func (a *app) openTrash() {
	s := &a.sessions
	s.trashItems, _ = a.trash.List()
	s.trashOpen = true
}

func (a *app) trashDialog(c *ui.Context) {
	s := &a.sessions
	t := c.Theme()
	if !s.trashOpen {
		return
	}
	ui.DialogBase(c, &s.trashOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.3))
		panel.Width(680).MaxHeightPercent(80).Padding(20).Gap(12).Radius(14).Background(t.Background).Shadow(0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.25))
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, fmt.Sprintf("回收站（%d）", len(s.trashItems))).FontSize(17).Bold()
			ui.Spacer(c)
			if len(s.trashItems) > 0 && ui.Button(c, "全部恢复").Clicked() {
				for _, it := range s.trashItems {
					a.trash.Restore(it.ID)
				}
				s.trashItems, _ = a.trash.List()
				go a.rescan()
			}
			if len(s.trashItems) > 0 && ui.Button(c, "清空回收站").Clicked() {
				for _, it := range s.trashItems {
					a.trash.Purge(it.ID)
				}
				s.trashItems = nil
			}
			if iconButton(c, iconX, "关闭").Clicked() {
				s.trashOpen = false
			}
		})
		muted(c, "删除的会话在这里保留 30 天。").FontSize(12)
		ui.Scroll(c).MaxHeight(480).Gap(2).Children(func() {
			if len(s.trashItems) == 0 {
				muted(c, "回收站是空的").Padding(24).AlignSelf(ui.Center)
			}
			now := time.Now()
			for _, it := range s.trashItems {
				ui.Row(c.Key(it.ID)).Padding(8, 4).Gap(10).Children(func() {
					agentMark(c, it.Session.Agent, 18, true)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						ui.Text(c, it.Session.Title).SingleLine()
						muted(c, fmt.Sprintf("%s · %s · %s删除", filepath.Base(it.Session.CWD), humanSize(it.Session.Size), ago(it.DeletedAt, now))).FontSize(12).SingleLine()
					})
					if iconButton(c, iconRestore, "恢复").Clicked() {
						if err := a.trash.Restore(it.ID); err != nil {
							c.AddToast(ui.Toast{Title: "恢复失败", Description: err.Error(), Type: "error"})
						}
						s.trashItems, _ = a.trash.List()
						go a.rescan()
					}
					if iconButton(c, iconX, "彻底删除").Clicked() {
						a.trash.Purge(it.ID)
						s.trashItems, _ = a.trash.List()
					}
				})
			}
		})
	})
}
