package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestWelcomeSkills(t *testing.T) {
	home := t.TempDir()
	makeSkill := func(relative string) string {
		t.Helper()
		dir := filepath.Join(home, relative)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "SKILL.md")
		if err := os.WriteFile(path, []byte("# Skill"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	agentSkill := makeSkill(".agents/skills/agent-skill")
	makeSkill(".codex/skills/.system/codex-skill")
	if err := os.Mkdir(filepath.Join(home, ".agents/skills/no-manifest"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude/skills"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	var opened string
	a := &readerWindow{
		welcomeSkills: discoverSkillDirectories(home),
		openSkill:     func(path string) { opened = path },
	}
	tt := ui.NewTester(a.view, 480, 360)
	const agents, claude, codex = "~/.agents/skills", "~/.claude/skills", "~/.codex/skills"
	open, _ := tt.Access("打开文件…")
	tab, ok := tt.Access(agents)
	if !ok || !tab.Selected || tab.Bounds.Y < open.Bounds.Y+open.Bounds.H || !tt.HasText("agent-skill") {
		t.Fatalf("default skills tab should appear below the open button: %v", tt.AccessAll())
	}
	if tt.HasText(claude) || tt.HasText("no-manifest") || tt.HasText(".system/codex-skill") {
		t.Fatalf("unexpected entries in the default tab: %v", tt.Texts())
	}
	if err := tt.Click(codex); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(".system/codex-skill") || tt.HasText("agent-skill") {
		t.Fatalf("unexpected Codex tab contents: %v", tt.Texts())
	}
	tt.Key(0, ui.KeyLeft)
	if err := tt.Click("agent-skill"); err != nil {
		t.Fatal(err)
	}
	if opened != agentSkill {
		t.Fatalf("opened %q, want %q", opened, agentSkill)
	}
	a.path, a.source = agentSkill, "# Loaded skill"
	prepareTestReader(t, a)
	tt.Frame()
	if tt.HasText(agents) || !tt.HasText("Loaded skill") {
		t.Fatalf("unexpected document contents: %v", tt.Texts())
	}
}

func TestWelcomeSkillsShowMore(t *testing.T) {
	directory := skillDirectory{label: "~/.agents/skills"}
	for i := range 12 {
		name := fmt.Sprintf("skill-%02d", i)
		directory.skills = append(directory.skills, welcomeSkill{name: name, path: filepath.Join(name, "SKILL.md")})
	}
	a := &readerWindow{welcomeSkills: []skillDirectory{directory, {label: "~/.claude/skills"}}}
	tt := ui.NewTester(a.view, 900, 720)
	tt.SetTitleBar(ui.TitleBar{Height: readerTitleBarHeight})
	openBefore, _ := tt.Access("打开文件…")
	tabBefore, _ := tt.Access(directory.label)
	rowBefore, _ := tt.Access("skill-00")
	more, ok := tt.Access("展示更多")
	if !ok || more.Bounds.H != welcomeMoreHeight || !tt.HasText("skill-00") || tt.HasText("skill-11") {
		t.Fatalf("expected a short preview ending in show more: %v", tt.AccessAll())
	}
	if err := tt.Click("展示更多"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("展示更多") {
		t.Fatal("show more is still visible after expanding")
	}
	row, ok := tt.Access("skill-00")
	if !ok {
		t.Fatal(tt.Texts())
	}
	openAfter, _ := tt.Access("打开文件…")
	tabAfter, _ := tt.Access(directory.label)
	if openBefore.Bounds != openAfter.Bounds || tabBefore.Bounds != tabAfter.Bounds || rowBefore.Bounds.Y != row.Bounds.Y {
		t.Fatal("expanding shifted the welcome controls or the start of the skill list")
	}
	tt.Scroll(row.Bounds.X+20, row.Bounds.Y+10, 0, 500)
	if !tt.HasText("skill-11") {
		t.Fatalf("expected the last skill after scrolling the expanded list: %v", tt.Texts())
	}
}

func TestWelcomeSkillsFallback(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &readerWindow{welcomeSkills: discoverSkillDirectories(home)}
	tt := ui.NewTester(a.view, 900, 720)
	if tab, ok := tt.Access("~/.claude/skills"); !ok || !tab.Selected || !tt.HasText("此目录没有 Skill") {
		t.Fatalf("expected the only existing directory to be selected: %v", tt.Texts())
	}
}
