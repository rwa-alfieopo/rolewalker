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

// app holds the tray application state.
type app struct {
	cm  *aws.ConfigManager
	sm  *aws.SSOManager
	ps  *aws.ProfileSwitcher
	km  *aws.KubeManager
	db  *db.DB
	mu  sync.Mutex
	quit chan struct{}
}

// Run starts the system tray application.
func Run() {
	systray.Run(onReady, onExit)
}

func onReady() {
	a := &app{quit: make(chan struct{})}

	// Write our own PID so 'rw tray status/stop' can find us
	WritePIDFile(os.Getpid())

	// Initialise core managers
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
	a.km = aws.NewKubeManager()

	database, err := db.NewDB()
	if err == nil {
		a.db = database
	}

	systray.SetIcon(iconData)
	systray.SetTooltip("rolewalkers")

	a.buildInitialMenu()

	// Refresh tray title every 30 seconds to pick up profile/SSO changes
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				a.buildMenu()
			case <-a.quit:
				return
			}
		}
	}()
}

func onExit() {
	RemovePIDFile()
}
