package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	p "path"
	"runtime"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/adrg/xdg"
	"github.com/jannson/go-autostart"
)

const (
	appNamespace string = "com.github.mikeharris.DeskClean"
	macAppExt    string = ".app"
)

var (
	allowedRunIntervals = []string{"every minute", "every 5 minutes", "every 15 minutes", "every 30 minutes", "every hour", "every 4 hours", "every 24 hours", "on demand"}
	allowedDateFormats  = []string{"2006-01-02", "2006-Jan-02", "01-02-2006", "Jan-02-2006", "2006-01", "2006-Jan", "01-2006", "Jan-2006"}
	version             string
	build               string
)

func main() {
	doneChan := make(chan bool)
	resetChan := make(chan int)

	a := app.NewWithID(appNamespace)
	prefs := a.Preferences()

	if prefs.BoolWithFallback("FirstRun", true) {
		initAppDefaults(a.Preferences())
	}

	var e string
	if runtime.GOOS == "darwin" {
		e = p.Join(xdg.ApplicationDirs[0], prefs.String("AppName")) + macAppExt
	} else {
		var err error
		e, err = os.Executable()
		if err != nil {
			log.Fatal("Unable to determine executable. ", err)
		}
	}

	startApp := &autostart.App{
		Name:        appNamespace,
		DisplayName: prefs.String("AppName"),
		Exec:        []string{e},
	}
	autoLaunch := prefs.Bool("AutoLaunchApp")

	w := a.NewWindow(prefs.String("AppName") + " Settings")

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu(prefs.String("AppName"),
			fyne.NewMenuItem("Run Now", func() {
				err := moveFiles(prefs.String("SourcePath"), getTargetPath(prefs))
				if err != nil {
					log.Println("Failed to move source files. ", err)
				}
			}),
			fyne.NewMenuItem("Settings", func() {
				w.Show()
			}))

		desk.SetSystemTrayIcon(resourceDeskcleanicondarkSvg)
		desk.SetSystemTrayMenu(m)
	}

	w.SetContent(makeSettingsUI(prefs))
	w.SetCloseIntercept(func() {
		prefs.SetInt("RunIntervalMinutes", runIntervalToInt(prefs.String("RunInterval")))
		// Determine if AutoLaunchApp is dirty
		if autoLaunch != prefs.Bool("AutoLaunchApp") {
			// autoLaunch and the pref aren't the same so inverse it to make it the same
			autoLaunch = !autoLaunch
			if startApp.IsEnabled() && !autoLaunch {
				if err := startApp.Disable(); err != nil {
					log.Fatal(err)
				}
			} else if !startApp.IsEnabled() && autoLaunch {
				log.Println("Creating LaunchAgent plist.")
				if err := startApp.Enable(); err != nil {
					log.Fatal(err)
				}
			}
		}
		w.Hide()
	})

	sweepTicker := time.NewTicker(60 * time.Minute)
	if prefs.Int("RunIntervalMinutes") > -1 {
		sweepTicker.Reset(time.Duration(prefs.Int("RunIntervalMinutes")) * time.Minute)
	}

	runInterval := binding.BindPreferenceInt("RunIntervalMinutes", prefs)
	callback := binding.NewDataListener(func() {
		ri, _ := runInterval.Get()
		resetChan <- ri
	})
	runInterval.AddListener(callback)

	go func() {
		for {
			select {
			case <-doneChan:
				return
			case i := <-resetChan:
				if i > -1 {
					sweepTicker.Reset(time.Duration(i) * time.Minute)
					log.Printf("Reset timer to %d minutes.\n", prefs.Int("RunIntervalMinutes"))
				} else {
					log.Println("Sweeper set to run on demand.")
				}
			case <-sweepTicker.C:
				if prefs.Int("RunIntervalMinutes") > 0 {
					err := moveFiles(prefs.String("SourcePath"), getTargetPath(prefs))
					if err != nil {
						log.Println("Failed to move source files. ", err)
					}
				}
			}
		}
	}()

	a.Run()
	doneChan <- true
}

func makeSettingsUI(pref fyne.Preferences) fyne.CanvasObject {
	al := widget.NewLabel(pref.String("HomeDir"))
	ri := widget.NewSelect(allowedRunIntervals, func(value string) { pref.SetString("RunInterval", value) })
	ri.SetSelected(pref.String("RunInterval"))

	df := widget.NewSelect(allowedDateFormats, func(value string) { pref.SetString("TargetFolderDateScheme", value) })
	df.SetSelected(pref.String("TargetFolderDateScheme"))

	af := widget.NewEntryWithData(binding.BindPreferenceString("AppFolder", pref))
	af.OnChanged = func(s string) {
		al.SetText(p.Join(p.Join(pref.String("HomeDir"), s)))
	}

	wc := container.NewPadded(container.NewPadded(container.New(layout.NewFormLayout(),
		widget.NewLabel("App Folder:"), af,
		widget.NewLabel("Sweep Folder Name:"), widget.NewEntryWithData(binding.BindPreferenceString("TargetFolderLabel", pref)),
		widget.NewLabel("Sweep Folder Seperator:"), widget.NewEntryWithData(binding.BindPreferenceString("TargetFolderSeperator", pref)),
		widget.NewLabel("Sweep Folder Date Format:"), df,
		widget.NewLabel("Run Inteval:"), ri,
		widget.NewLabel("Launch app at login:"), widget.NewCheckWithData("Enabled", binding.BindPreferenceBool("AutoLaunchApp", pref)),
		widget.NewLabel("Sweep Location:"), widget.NewLabelWithData(binding.BindPreferenceString("SourcePath", pref)),
		widget.NewLabel("Archive Location:"), al)))
	return wc
}

func initAppDefaults(pref fyne.Preferences) {
	pref.SetString("AppName", "DeskClean")
	pref.SetString("AppFolder", "DeskClean")
	pref.SetString("HomeDir", xdg.Home)
	pref.SetString("TargetFolderLabel", "Archive")
	pref.SetString("TargetFolderSeperator", "-")
	pref.SetString("TargetFolderDateScheme", "2006-01-02")
	pref.SetString("RunInterval", "every hour")
	pref.SetInt("RunIntervalMinutes", 60)
	pref.SetString("SourcePath", xdg.UserDirs.Desktop)
	pref.SetBool("FirstRun", false)
	pref.SetBool("AutoLaunchApp", false)
	log.Println("Configuration initialized.")
}

func getTargetPath(pref fyne.Preferences) string {
	now := time.Now()
	dateStr := now.Format(pref.String("TargetFolderDateScheme"))
	folderDateLabel := fmt.Sprintf("%s%s%s", dateStr, pref.String("TargetFolderSeperator"), pref.String("TargetFolderLabel"))
	return p.Join(pref.String("HomeDir"), pref.String("AppFolder"), folderDateLabel)
}

func moveFiles(sourcePath, targetPath string) error {
	moveCount := 0
	skippedCount := 0
	sourceFs := os.DirFS(sourcePath)

	fs.WalkDir(sourceFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() || d.Type().IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				skippedCount++
			} else {
				// Determine if parent path needs created and only create if there is a file/folder to write
				err = createTargetDirectory(targetPath)
				if err != nil {
					log.Println("Unable to create target directory. ", err)
					// If we cannot create the containing folder then fail fast
					return err
				}

				err = os.Rename(p.Join(sourcePath, path), p.Join(targetPath, path))
				if err != nil {
					log.Println("Failed to move file - ", err)
				} else {
					moveCount++
				}
			}
		}
		return nil
	})
	log.Printf("Skipped %d hidden files.\n", skippedCount)
	log.Printf("Cleaned %d files.\n", moveCount)
	return nil
}

func createTargetDirectory(targetPath string) error {
	appPath := p.Dir(targetPath)
	if _, err := os.Stat(appPath); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(appPath, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
		err := os.Mkdir(targetPath, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

func runIntervalToInt(text string) int {
	switch text {
	case "every minute":
		return 1
	case "every 5 minutes":
		return 5
	case "every 15 minutes":
		return 15
	case "every 30 minutes":
		return 30
	case "every 60 minutes":
		return 60
	case "every 4 hours":
		return 240
	case "every 12 hours":
		return 720
	case "every 24 hours":
		return 1440
	default:
		return -1
	}
}
