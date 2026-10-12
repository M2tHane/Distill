// Package gui is Distill's window: the agents, their sessions and the
// library, drawn with MyGo's native UI.
package gui

import (
	"context"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/sources"
	"github.com/M2tHane/Distill/app/builtin"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
)

type page string

const (
	pageAgents   page = "agents"
	pageSessions page = "sessions"
	pageLibrary  page = "library"
)

type app struct {
	win      *mygo.Window
	p        env.Paths
	cfg      *env.Config
	firstRun bool
	srcs     []agentlog.Source
	ix       *agentlog.Index
	trash    agentlog.Trash
	lib      library.Library
	page     page

	infos map[agentlog.Agent]*env.AgentInfo
	md    *mdView // Markdown parsed, for every view

	sessions sessionsState
	distill  distillState
	// distillFrame is where the user moved and sized the distill dialog.
	distillFrame panelFrame
	library      libraryState

	// toast is shown in the next frame, for work done off the main thread.
	toast *ui.Toast

	builtinPrompt bool
	builtinPick   map[agentlog.Agent]bool
}

// Run opens the window and returns when the app quits.
func Run(icon []byte) {
	p := env.NewPaths()
	_, err := os.Stat(p.Config)
	a := &app{
		p:        p,
		cfg:      env.LoadConfig(p),
		firstRun: os.IsNotExist(err),
		srcs:     sources.All(p.Home),
		lib:      library.Library{P: p},
		page:     pageSessions,
		infos:    map[agentlog.Agent]*env.AgentInfo{},
		md:       newMDView(),
	}
	a.ix = &agentlog.Index{Sources: a.srcs, CachePath: p.Index}
	a.trash = agentlog.Trash{Dir: p.Trash, Sources: a.srcs}
	for _, ag := range env.Agents {
		a.infos[ag] = &env.AgentInfo{Agent: ag, Detecting: true}
	}
	a.sessions.init()
	a.library.init()
	if err := a.writeBuiltin(); err != nil {
		log.Println("builtin:", err)
	}
	go a.trash.PurgeOlder(30 * 24 * time.Hour)

	mygo.App.WhenReady(func() {
		// A bare binary has no bundle icon; a bundle gets it from resources/icon.png.
		if mygo.App.Dock != nil {
			mygo.App.Dock.SetIcon(icon)
		}
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:         "Distill",
			Width:         1180,
			Height:        780,
			MinWidth:      860,
			MinHeight:     560,
			TitleBarStyle: mygo.TitleBarHiddenInset,
			Content:       ui.View(a.view),
		})
		go a.start()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

// start finds the agents and the sessions, away from the main thread.
func (a *app) start() {
	env.LoginPath()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.detectAgents()
	}()
	a.rescan()
	wg.Wait()
}

func (a *app) detectAgents() {
	var wg sync.WaitGroup
	for _, ag := range env.Agents {
		wg.Add(1)
		go func() {
			defer wg.Done()
			info := env.Detect(a.p, ag)
			a.win.Update(func() { *a.infos[ag] = info })
		}()
	}
	wg.Wait()
	a.win.Update(func() {
		if a.firstRun {
			// Turn on every agent found, the first time.
			for _, ag := range env.Agents {
				a.cfg.Agents[ag].Enabled = a.infos[ag].Installed()
			}
			a.firstRun = false
			a.saveConfig()
		}
		if !a.cfg.BuiltinAsked && a.anyInstalled() {
			a.builtinPick = map[agentlog.Agent]bool{}
			for _, ag := range env.Agents {
				a.builtinPick[ag] = a.infos[ag].Installed()
			}
			a.builtinPrompt = true
		}
	})
}

func (a *app) anyInstalled() bool {
	for _, ag := range env.Agents {
		if a.infos[ag].Installed() {
			return true
		}
	}
	return false
}

func (a *app) saveConfig() {
	if err := a.cfg.Save(a.p); err != nil {
		log.Println("config:", err)
	}
}

// rescan lists the sessions again, from the cache where logs did not change.
func (a *app) rescan() {
	a.win.Update(func() { a.sessions.scanning = true })
	res, err := a.ix.Scan(context.Background())
	var list []agentlog.Session
	for _, s := range res.Sessions {
		// Distill's own runs are not the user's sessions.
		if strings.HasPrefix(s.CWD+"/", a.p.Root+"/") || s.Messages == 0 {
			continue
		}
		list = append(list, s)
	}
	a.win.Update(func() {
		a.sessions.scanning = false
		a.sessions.all = list
		a.sessions.loaded = true
		if err != nil {
			a.sessions.err = err.Error()
		}
	})
}

// writeBuiltin puts the built-in skills and the library's README in
// ~/.distill, refreshing copies of them applied to agents.
func (a *app) writeBuiltin() error {
	if err := os.MkdirAll(a.p.Library, 0o755); err != nil {
		return err
	}
	readme, err := builtin.FS.ReadFile("LIBRARY.md")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(a.p.Library, "README.md"), readme, 0o644); err != nil {
		return err
	}
	for _, k := range library.Kinds {
		os.MkdirAll(filepath.Join(a.p.Library, k.Dir()), 0o755)
	}
	err = fs.WalkDir(builtin.FS, "skills", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dst := filepath.Join(a.p.Builtin, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, err := builtin.FS.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		return err
	}
	for _, it := range a.lib.Scan(a.p.Builtin, true) {
		for _, ag := range env.Agents {
			if st := library.Status(a.p, it, ag); st.Applied && !st.Linked {
				library.Apply(a.p, it, ag, true)
			}
		}
	}
	return nil
}

func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		a.sidebar(c)
		ui.Column(c).Grow(1).MinWidth(0).Background(t.Background).Children(func() {
			switch a.page {
			case pageAgents:
				a.agentsPage(c)
			case pageSessions:
				a.sessionsPage(c)
			case pageLibrary:
				a.libraryPage(c)
			}
		})
	})
	a.distillDialog(c)
	a.builtinDialog(c)

	if a.toast != nil {
		c.AddToast(*a.toast)
		a.toast = nil
	}
}

func (a *app) sidebar(c *ui.Context) {
	t := c.Theme()
	bar := c.TitleBar()
	side := ui.Hex("#F4F4F5")
	if t.Dark {
		side = ui.Hex("#1C1C1F")
	}
	ui.Column(c).Width(200).Background(side).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Height(titleHeight(bar)).Padding(0, 12, 0, bar.Left+8).DragWindow().Children(func() {
			ui.Text(c, "Distill").Bold()
		})
		ui.Column(c).Padding(8, 10).Gap(2).Children(func() {
			muted(c, "使用").FontSize(12).Padding(6, 8, 4)
			a.navItem(c, pageAgents, iconTerminal, "Agent", "")
			n := ""
			if len(a.sessions.all) > 0 {
				n = strconv.Itoa(len(a.sessions.all))
			}
			a.navItem(c, pageSessions, iconChat, "会话", n)
			a.navItem(c, pageLibrary, iconLibrary, "资源库", "")
		})
	})
}

func (a *app) navItem(c *ui.Context, p page, svg *ui.SVG, label, count string) {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(p)).Label(label).Padding(7, 10).Radius(8).Gap(10).Cursor(ui.CursorPointer)
	if a.page == p {
		b.Background(t.Background).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.08))
	} else if b.Hovered() {
		b.Background(ui.RGBA(0, 0, 0, 0.04))
	}
	b.Children(func() {
		ui.Icon(c, svg).FontSize(15)
		ui.Text(c, label).Grow(1)
		if count != "" {
			muted(c, count).FontSize(12)
		}
	})
	if b.Clicked() {
		a.page = p
	}
}

// header draws a page's title row, under the window controls.
// titleHeight is the height of the rows beside the window controls, which
// centers their text on the controls. On macOS the controls of a window
// with an inset title bar are centered 26 DIPs from the top, while the room
// the toolkit reports for them is higher.
func titleHeight(bar ui.TitleBar) float32 {
	if bar.Height == 0 {
		return 38
	}
	return min(bar.Height, 52)
}

func header(c *ui.Context, title string, right func()) {
	bar := c.TitleBar()
	ui.Row(c).Height(titleHeight(bar)).Padding(0, 16, 0, 24).Gap(8).DragWindow().Children(func() {
		ui.Text(c, title).FontSize(15).Bold()
		ui.Spacer(c)
		if right != nil {
			right()
		}
	})
}
