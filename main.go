package main

import (
	"github.com/egoist/mygo"
	"log"
)

func main() {
	manager := &application{windows: make(map[string]*readerWindow)}
	mygo.App.SetName("MDFlick")
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "文件", Submenu: []*mygo.MenuItem{
			{Label: "打开…", Accelerator: "CmdOrCtrl+O", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				var target *readerWindow
				for _, a := range manager.windows {
					if a.win == w {
						target = a
						break
					}
				}
				manager.openDialog(target)
			}},
			{Role: mygo.RoleOpenRecent},
			{Label: "重新载入", Accelerator: "CmdOrCtrl+R", Click: func(_ *mygo.MenuItem, w *mygo.Window) {
				for _, a := range manager.windows {
					if a.win == w && a.path != "" {
						a.load()
					}
				}
			}},
			mygo.Separator(), {Role: mygo.RoleClose},
		}},
		{Label: "编辑", Submenu: []*mygo.MenuItem{{Role: mygo.RoleCopy}, {Role: mygo.RoleSelectAll}}},
		{Role: mygo.RoleWindowMenu},
	}))
	mygo.App.OnOpenFile(func(path string) { mygo.App.WhenReady(func() { manager.openPath(path, nil) }) })
	// MyGo quits when the last window closes unless a listener overrides it.
	mygo.App.WhenReady(func() {
		if len(manager.windows) == 0 {
			manager.newWindow("")
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
