package main

import (
	"os"
	"path/filepath"

	"github.com/egoist/mygo/ui"
)

type skillDirectory struct {
	label    string
	skills   []welcomeSkill
	failure  bool
	expanded bool
}

type welcomeSkill struct {
	name string
	path string
}

func (a *readerWindow) loadWelcomeSkills() {
	win := a.win
	go func() {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		directories := discoverSkillDirectories(home)
		win.Update(func() {
			if !a.closed && a.path == "" && a.sidebar.root == "" {
				a.welcomeSkills = directories
			}
		})
	}()
}

func discoverSkillDirectories(home string) []skillDirectory {
	var directories []skillDirectory
	for _, source := range []string{".agents", ".claude", ".codex"} {
		root := filepath.Join(home, source, "skills")
		if !isDirectory(root) {
			continue
		}
		directory := skillDirectory{label: "~/" + source + "/skills"}
		directory.skills, directory.failure = readWelcomeSkills(root, "")
		directories = append(directories, directory)
	}
	return directories
}

func readWelcomeSkills(root, relative string) ([]welcomeSkill, bool) {
	entries, err := os.ReadDir(filepath.Join(root, relative))
	if err != nil {
		return nil, true
	}
	var skills []welcomeSkill
	for _, entry := range entries {
		if !entry.IsDir() && entry.Type()&os.ModeSymlink == 0 {
			continue
		}
		name := filepath.Join(relative, entry.Name())
		path := filepath.Join(root, name, "SKILL.md")
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			skills = append(skills, welcomeSkill{name: filepath.ToSlash(name), path: path})
		} else if entry.IsDir() {
			// Scan groups such as .system, stopping at packages and symlinks.
			nested, _ := readWelcomeSkills(root, name)
			skills = append(skills, nested...)
		}
	}
	return skills, false
}

func (a *readerWindow) welcomeView(c *ui.Context) {
	t := c.Theme()
	welcome := ui.Column(c).Grow(1).MinHeight(0).Gap(16).AlignItems(ui.Center)
	if len(a.welcomeSkills) == 0 {
		// The extra 100 DIPs below move the centered welcome content up 50.
		welcome.Justify(ui.Center).Padding(32, 32, 132, 32)
	} else {
		welcome.Justify(ui.Center).Padding(24, 20)
	}
	welcome.Children(func() {
		ui.Text(c, "轻松阅读 Markdown 和 Skills").FontSize(20)
		ui.Text(c, "在 Finder 中选中文件，按空格即可快速预览。").TextColor(t.TextMuted)
		// Match a regular button's 1-DIP border without changing the accent face.
		ui.PrimaryButton(c, "打开文件…").Border(1, ui.Transparent).OnClick(a.open)
		if len(a.welcomeSkills) > 0 {
			a.welcomeSkillsView(c)
		}
	})
}

const (
	welcomePanelHeight = 360
	welcomeTabHeight   = 32
	welcomePanelGap    = 8
	welcomeListPadding = 8
	welcomeRowHeight   = 30
	welcomeMoreHeight  = 24
)

func (a *readerWindow) welcomeSkillsView(c *ui.Context) {
	t := c.Theme()
	panel := ui.Column(c).Key("welcome-skills").FillWidth().MaxWidth(520).Grow(1).MinHeight(0).
		MaxHeight(welcomePanelHeight).Gap(welcomePanelGap)
	panel.Children(func() {
		tabs := ui.TabsBase(c.Key("skill-directories"), &a.welcomeSkillTab, len(a.welcomeSkills))
		tabs.List.Gap(8).Children(func() {
			for i, directory := range a.welcomeSkills {
				c.Key(directory.label)
				tab := tabs.Tab(i).Height(welcomeTabHeight).Center().Padding(0, 14).Radius(welcomeTabHeight / 2)
				color := t.TextMuted
				if i == a.welcomeSkillTab {
					color = t.Text
					tab.Background(t.Text.Alpha(.08))
				} else if tab.Hovered() {
					tab.Background(t.Text.Alpha(.04))
				}
				tab.Children(func() {
					ui.Text(c, directory.label).FontSize(13).TextColor(color).Unselectable()
				})
			}
		})
		directory := &a.welcomeSkills[a.welcomeSkillTab]
		if len(directory.skills) == 0 {
			message := "此目录没有 Skill"
			if directory.failure {
				message = "无法读取目录"
			}
			ui.Text(c, message).Padding(8, 12).TextColor(t.TextMuted)
			return
		}
		listBackground := t.Text.Alpha(.025)
		if directory.expanded {
			ui.List(c.Key(directory.label), nil, len(directory.skills)).Grow(1).MinHeight(0).
				ScrollbarAxes(false, true).KeepScrollbars(true).
				Background(listBackground).Radius(6).Clip().Padding(welcomeListPadding).
				ItemKey(func(i int) any { return directory.skills[i].path }).Rows(func(row ui.ListRow) {
				a.welcomeSkillRow(row.Context, directory.skills[row.Index])
			})
			return
		}
		ui.Column(c).Key(directory.label).Background(listBackground).Radius(6).Clip().Padding(welcomeListPadding).Children(func() {
			_, height := c.Size()
			if c.TitleBar().Height != 0 {
				height -= readerTitleBarHeight
			}
			// Leave room for the welcome text, open button, and outer padding.
			rowsHeight := min(welcomePanelHeight, height-168) - welcomeTabHeight - welcomePanelGap - 2*welcomeListPadding
			count := len(directory.skills)
			if float32(count*welcomeRowHeight) > rowsHeight {
				count = max(0, int((rowsHeight-welcomeMoreHeight)/welcomeRowHeight))
			}
			for _, skill := range directory.skills[:count] {
				a.welcomeSkillRow(c, skill)
			}
			if count < len(directory.skills) {
				more := ui.ButtonBase(c.Key("show-more-skills")).Height(welcomeMoreHeight).AlignSelf(ui.Start).
					Padding(0, 8).Center().Label("展示更多").OnClick(func() { directory.expanded = true })
				color := t.TextMuted
				if more.Hovered() {
					color = t.Text
				}
				more.Children(func() {
					ui.Text(c, "展示更多").FontSize(12).TextColor(color).Unselectable()
				})
			}
		})
	})
}

func (a *readerWindow) welcomeSkillRow(c *ui.Context, skill welcomeSkill) {
	t := c.Theme()
	button := ui.ButtonBase(c.Key(skill.path)).Height(welcomeRowHeight).Radius(4).Padding(0, 8).Gap(8).
		Justify(ui.Start).AlignItems(ui.Center).Label(skill.name)
	if button.Hovered() {
		button.Background(t.TextMuted.Alpha(.08))
	}
	button.Children(func() {
		ui.Icon(c, readerFolderIcon).Size(16, 16).Shrink(0).TextColor(t.TextMuted)
		ui.Text(c, skill.name).MinWidth(0).FontSize(13).MaxLines(1).Ellipsis("…").Unselectable()
	})
	button.OnClick(func() {
		if a.openSkill != nil {
			a.openSkill(skill.path)
		}
	})
}
