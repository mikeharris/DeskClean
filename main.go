package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	p "path"
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
)

const appNamespace string = "com.github.mikeharris.DeskClean"

var (
	allowedRunIntervals = []string{"every minute", "every 5 minutes", "every 15 minutes", "every 30 minutes", "every hour", "every 4 hours", "every 24 hours", "on demand"}
	allowedDateFormats  = []string{"2006-01-02", "2006-Jan-02", "01-02-2006", "Jan-02-2006", "2006-01", "2006-Jan", "01-2006", "Jan-2006"}
)

func main() {
	doneChan := make(chan bool)
	resetChan := make(chan int)

	a := app.NewWithID(appNamespace)
	prefs := a.Preferences()

	if prefs.BoolWithFallback("FirstRun", true) {
		initAppDefaults(a.Preferences())
	}

	runInterval := prefs.Int("RunIntervalMinutes")

	w := a.NewWindow(prefs.String("AppName") + " Settings")

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu(prefs.String("AppName"),
			fyne.NewMenuItem("Run Now", func() {
				runSweep(getTargetPath(prefs), prefs.String("SourcePath"))
			}),
			fyne.NewMenuItem("Settings", func() {
				w.Show()
			}))

		desk.SetSystemTrayIcon(resourceDeskcleanicondarkSvg)
		desk.SetSystemTrayMenu(m)
	}

	w.SetContent(makeSettingsUI(prefs))
	w.SetCloseIntercept(func() {
		switch prefs.String("RunInterval") {
		case "every minute":
			prefs.SetInt("RunIntervalMinutes", 1)
		case "every 5 minutes":
			prefs.SetInt("RunIntervalMinutes", 5)
		case "every 15 minutes":
			prefs.SetInt("RunIntervalMinutes", 15)
		case "every 30 minutes":
			prefs.SetInt("RunIntervalMinutes", 30)
		case "every 60 minutes":
			prefs.SetInt("RunIntervalMinutes", 60)
		case "every 4 hours":
			prefs.SetInt("RunIntervalMinutes", 240)
		case "every 12 hours":
			prefs.SetInt("RunIntervalMinutes", 720)
		case "every 24 hours":
			prefs.SetInt("RunIntervalMinutes", 1440)
		default:
			prefs.SetInt("RunIntervalMinutes", -1)
		}

		if runInterval != prefs.Int("RunIntervalMinutes") {
			// Run inteval changed so inform the sweeper job
			resetChan <- prefs.Int("RunIntervalMinutes")
		}

		runInterval = prefs.Int("RunIntervalMinutes")
		w.Hide()
	})

	sweepTicker := time.NewTicker(60 * time.Minute)
	if prefs.Int("RunIntervalMinutes") > -1 {
		sweepTicker.Reset(time.Duration(prefs.Int("RunIntervalMinutes")) * time.Minute)
	}

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
					runSweep(getTargetPath(prefs), prefs.String("SourcePath"))
				}
			}
		}
	}()

	a.Run()
	doneChan <- true
}

func makeSettingsUI(pref fyne.Preferences) fyne.CanvasObject {
	ri := widget.NewSelect(allowedRunIntervals, func(value string) { pref.SetString("RunInterval", value) })
	ri.SetSelected(pref.String("RunInterval"))

	df := widget.NewSelect(allowedDateFormats, func(value string) { pref.SetString("TargetFolderDateScheme", value) })
	df.SetSelected(pref.String("TargetFolderDateScheme"))

	wc := container.NewPadded(container.NewPadded(container.New(layout.NewFormLayout(),
		widget.NewLabel("App Folder:"), widget.NewEntryWithData(binding.BindPreferenceString("AppFolder", pref)),
		widget.NewLabel("Sweep Folder Name:"), widget.NewEntryWithData(binding.BindPreferenceString("TargetFolderLabel", pref)),
		widget.NewLabel("Sweep Folder Seperator:"), widget.NewEntryWithData(binding.BindPreferenceString("TargetFolderSeperator", pref)),
		widget.NewLabel("Sweep Folder Date Format:"), df,
		widget.NewLabel("Run Inteval:"), ri,
		widget.NewLabel("Sweep Location:"), widget.NewLabelWithData(binding.BindPreferenceString("SourcePath", pref)),
		widget.NewLabel("Archive Location:"), widget.NewLabelWithData(binding.BindPreferenceString("HomeDir", pref)))))

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
	log.Println("Configuration initialized.")
}

func getTargetPath(pref fyne.Preferences) string {
	now := time.Now()
	dateStr := now.Format(pref.String("TargetFolderDateScheme"))
	folderDateLabel := fmt.Sprintf("%s%s%s", dateStr, pref.String("TargetFolderSeperator"), pref.String("TargetFolderLabel"))
	return p.Join(pref.String("HomeDir"), pref.String("AppFolder"), folderDateLabel)
}

func runSweep(targetPath, sourcePath string) {
	err := createTargetDirectory(targetPath)
	if err != nil {
		log.Fatal("Unable to create target directory. ", err)
	}
	err = moveFiles(sourcePath, targetPath)
	if err != nil {
		log.Fatal("Failed to move source files. ", err)
	}
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
