package main

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestReaderPropertiesDisclosures(t *testing.T) {
	source := "---\nname: playwright\ndescription: Browser automation from the terminal\nmetadata:\n  version: '1.0'\n---\n\n# Playwright CLI Skill"
	a := &readerWindow{documentSession: documentSession{path: "/tmp/SKILL.md", source: source}}
	prepareTestReader(t, a)
	tt := ui.NewTester(a.view, 420, 680)
	value, ok := tt.Find("Browser automation from the terminal")
	heading, body := tt.Find("Playwright CLI Skill")
	if !ok || !body || value.Y >= heading.Y || value.W <= 0 || value.X+value.W > 420 || tt.HasText("version") || !tt.HasText("SKILL 信息") || tt.HasText("查看 YAML") {
		t.Fatalf("unexpected initial view: %v", tt.Texts())
	}
	tt.ClickAt(value.X+4, value.Y+value.H/2)
	tt.Command("selectAll")
	tt.Command("copy")
	if !strings.Contains(tt.Clipboard(), "Browser automation from the terminal") {
		t.Fatal("metadata could not be selected and copied")
	}
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		tt.Advance(250 * time.Millisecond)
		tt.Frame()
	}
	click("1 个字段")
	if !tt.HasText("version") {
		t.Fatal("nested metadata remained hidden")
	}
	click("SKILL 信息")
	if !tt.HasText("Browser automation") {
		t.Fatal("clicking the title hid the card")
	}
	tt.SetDark(true)
	if !tt.HasText("Browser automation") {
		t.Fatal("theme change hid the open card")
	}
	a.path = "/tmp/README.md"
	tt.Frame()
	if !tt.HasText("文档属性") || tt.HasText("SKILL 信息") {
		t.Fatalf("unexpected title for ordinary document: %v", tt.Texts())
	}
}
