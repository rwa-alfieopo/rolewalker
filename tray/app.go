package tray

import (
	"fmt"
	"os"
	"sync"
	"time"

	"rolewalkers/aws"
	"rolewalkers/internal/db"

	"github.com/getlantern/systray"
)

// envItem pairs a systray menu item with its environment for dynamic updates.
type envItem struct {
	item *systray.MenuItem
	env  db.Environment
}

// app holds the tray application state.
type app struct {
	cm     *aws.ConfigManager
	sm     *aws.SSOManager
	ps     *aws.ProfileSwitcher
	km     *aws.KubeManager
	database *db.DB
	dbRepo *db.ConfigRepository
	mu     sync.Mutex
	quit   chan struct{}

	// Dynamic menu items that get refreshed
	mStatus  *systray.MenuItem
	mKube    *systray.MenuItem
	envItems []envItem
	nsItems  []*systray.MenuItem
}

// Run starts the system tray application.
func Run() {
	systray.Run(onReady, onExit)
}

func onReady() {
	a := &app{quit: make(chan struct{})}

	WritePIDFile(os.Getpid())

	cm, err := aws.NewConfigManager()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init config manager: %v\n", err)
		systray.Quit()
		return
	}
	a.cm = cm

	sm, err := aws.NewSSOManager(cm)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init SSO manager: %v\n", err)
	} else {
		a.sm = sm
	}

	a.ps = aws.NewProfileSwitcher(cm)

	database, err := db.NewDB()
	if err == nil {
		a.database = database
		a.dbRepo = db.NewConfigRepository(database)
		a.km = aws.NewKubeManagerWithRepo(a.dbRepo)
	} else {
		a.km = aws.NewKubeManager()
	}

	systray.SetIcon(iconData)
	systray.SetTooltip("rolewalkers")

	a.buildInitialMenu()

	// Refresh every 15 seconds
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.refreshMenu()
			case <-a.quit:
				return
			}
		}
	}()
}

func onExit() {
	RemovePIDFile()
}
