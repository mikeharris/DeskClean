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
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
	"github.com/adrg/xdg"
)

type AppConfig struct {
	AppName                string
	HomeDir                string
	AppFolder              string
	SourcePath             string
	TargetFolderLabel      string
	TargetFolderDateScheme string
	TargetFolderSeperator  string
	RunIntervalMinutes     int
	FirstRun               bool
}

const appNamespace string = "com.github.mikeharris.DeskClean"

func main() {
	a := app.NewWithID(appNamespace)
	conf := loadAppConfig(a)
	w := a.NewWindow(conf.AppName)

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu(conf.AppName,
			fyne.NewMenuItem("Run Now", func() {
				runSweep(conf)
			}))
		desk.SetSystemTrayMenu(m)
	}

	w.SetContent(widget.NewLabel("Fyne System Tray"))
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	if conf.FirstRun {
		saveAppConfig(a, conf)
	}
	a.Run()
}

func loadAppConfig(a fyne.App) AppConfig {
	ac := AppConfig{}
	ac.AppName = a.Preferences().StringWithFallback("AppName", "DeskClean")
	ac.AppFolder = a.Preferences().StringWithFallback("AppFolder", "DeskClean")
	ac.HomeDir = xdg.Home
	ac.TargetFolderLabel = a.Preferences().StringWithFallback("TargetFolderLabel", "Archive")
	ac.TargetFolderSeperator = a.Preferences().StringWithFallback("TargetFolderSeperator", "-")
	ac.TargetFolderDateScheme = a.Preferences().StringWithFallback("TargetFolderDateScheme", "2006-01-02")
	ac.RunIntervalMinutes = a.Preferences().IntWithFallback("RunIntervalMinutes", 60)
	ac.SourcePath = a.Preferences().StringWithFallback("SourcePath", xdg.UserDirs.Desktop)
	ac.FirstRun = a.Preferences().BoolWithFallback("FirstRun", true)
	log.Println("Configuration loaded.")
	return ac
}

func saveAppConfig(a fyne.App, ac AppConfig) {
	a.Preferences().SetString("AppName", ac.AppName)
	a.Preferences().SetString("AppFolder", ac.AppFolder)
	// a.Preferences().SetString("HomeDir", ac.HomeDir)
	a.Preferences().SetString("TargetFolderLabel", ac.TargetFolderLabel)
	a.Preferences().SetString("TargetFolderSeperator", ac.TargetFolderSeperator)
	a.Preferences().SetString("TargetFolderDateScheme", ac.TargetFolderDateScheme)
	a.Preferences().SetInt("RunIntervalMinutes", ac.RunIntervalMinutes)
	a.Preferences().SetString("SourcePath", ac.SourcePath)
	a.Preferences().SetBool("FirstRun", false)
	log.Println("Configuration saved.")
}

func getTargetPath(ac AppConfig) string {
	now := time.Now()
	dateStr := now.Format(ac.TargetFolderDateScheme)
	folderDateLabel := fmt.Sprintf("%s%s%s", dateStr, ac.TargetFolderSeperator, ac.TargetFolderLabel)
	return p.Join(ac.HomeDir, ac.AppFolder, folderDateLabel)
}

func runSweep(ac AppConfig) {
	targetPath := getTargetPath(ac)
	err := createTargetDirectory(targetPath)
	if err != nil {
		log.Fatal("Unable to create target directory. ", err)
	}
	err = moveFiles(ac.SourcePath, targetPath)
	if err != nil {
		log.Fatal("Failed to move source files. ", err)
	}
}

func moveFiles(sourceRoot, targetRoot string) error {
	moveCount := 0
	skippedCount := 0
	sourceFs := os.DirFS(sourceRoot)
	fs.WalkDir(sourceFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() || d.Type().IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				// log.Println("Skipping hidden file", path)
				skippedCount++
			} else {
				err = os.Rename(p.Join(sourceRoot, path), p.Join(targetRoot, path))
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
