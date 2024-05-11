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
)

func main() {
	currentTime := time.Now()
	userHomeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal("Unable to determine user's home directory: ", err)
	}
	targetFolder := fmt.Sprintf("%d-%02d-%02d-%s", currentTime.Year(), currentTime.Month(), currentTime.Day(), "Archive")
	appFolder := "DeskClean"
	cleanFolder := "Desktop"
	targetRoot := p.Join(userHomeDir, appFolder, targetFolder)
	sourceRoot := p.Join(userHomeDir, cleanFolder)
	err = createTargetDirectory(userHomeDir, appFolder, targetFolder)
	if err != nil {
		log.Fatal("Unable to create target directory. ", err)
	}
	err = moveSourceFiles(sourceRoot, targetRoot)
	if err != nil {
		log.Fatal("Failed to move source files. ", err)
	}
}

func moveSourceFiles(sourceRoot, targetRoot string) error {
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

func createTargetDirectory(userHomeDir, appFolder, targetFolder string) error {
	appPath := p.Join(userHomeDir, appFolder)
	targetPath := p.Join(appPath, targetFolder)
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
