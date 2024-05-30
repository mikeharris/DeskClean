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
)

type AppConfig struct {
	AppName      string
	HomeDir      string
	AppFolder    string
	SourcePath   string
	TargetPath   string
	TargetFolder string
}

func main() {
	ac := AppConfig{}

	ac.AppName = "DeskClean"
	currentTime := time.Now()
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Unable to determine user's home directory: ", err)
	}
	ac.HomeDir = userHomeDir
	targetFolderLabel := "Archive"
	ac.TargetFolder = fmt.Sprintf("%d-%02d-%02d-%s", currentTime.Year(), currentTime.Month(), currentTime.Day(), targetFolderLabel)
	ac.AppFolder = ac.AppName
	cleanFolder := "Desktop"
	ac.TargetPath = p.Join(ac.HomeDir, ac.AppFolder, ac.TargetFolder)
	ac.SourcePath = p.Join(userHomeDir, cleanFolder)

	a := app.New()
	w := a.NewWindow(ac.AppName)

	if desk, ok := a.(desktop.App); ok {
		m := fyne.NewMenu(ac.AppName,
			fyne.NewMenuItem("Run Now", func() {
				runSweep(ac)
			}))
		desk.SetSystemTrayMenu(m)
	}

	w.SetContent(widget.NewLabel("Fyne System Tray"))
	w.SetCloseIntercept(func() {
		w.Hide()
	})
	a.Run()
}

func runSweep(ac AppConfig) {
	err := createTargetDirectory(ac.TargetPath)
	if err != nil {
		log.Fatal("Unable to create target directory. ", err)
	}
	err = moveFiles(ac.SourcePath, ac.TargetPath)
	if err != nil {
		log.Fatal("Failed to move source files. ", err)
	}
}

func moveFiles(sourceRoot, targetRoot string) error {
	sourceFs := os.DirFS(sourceRoot)
	fs.WalkDir(sourceFs, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() || d.Type().IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				log.Println("Skipping hidden file", path)
			} else {
				err = os.Rename(p.Join(sourceRoot, path), p.Join(targetRoot, path))
				if err != nil {
					log.Println("Failed to move file - ", err)
				}
			}
		}
		return nil
	})
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
