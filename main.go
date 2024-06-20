package main

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path"
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
	appNamespace      string = "com.github.mikeharris.DeskClean"
	sweptMenuLabel    string = "Swept at %s"
	sweepMenuLabel    string = "Sweep now"
	settingsMenuLabel string = "Settings"
	logFileExt        string = ".log"
	appNameDefault    string = "DeskClean"
)

var (
	allowedRunIntervals = []string{"every minute", "every 5 minutes", "every 15 minutes", "every 30 minutes", "every hour", "every 4 hours", "every 24 hours", "on demand"}
	allowedDateFormats  = []string{"2006-01-02", "2006-Jan-02", "01-02-2006", "Jan-02-2006", "2006-01", "2006-Jan", "01-2006", "Jan-2006"}
	version             string
	build               string
	buildDate           string
	commit              string
)

func main() {
	doneChan := make(chan bool)
	resetChan := make(chan int)

	a := app.NewWithID(appNamespace)
	prefs := a.Preferences()
	appName := prefs.StringWithFallback("AppName", appNameDefault)

	logger := slog.New(slog.NewJSONHandler(getLogFile(runtime.GOOS, appName, appName+logFileExt), nil))
	slog.SetDefault(logger)
	slog.SetLogLoggerLevel(slog.LevelDebug)

	var menu *fyne.Menu
	lastSweepMenu := fyne.NewMenuItem(fmt.Sprintf(sweptMenuLabel, prefs.String("LastSweep")), func() {})

	if prefs.BoolWithFallback("FirstRun", true) {
		initAppDefaults(a.Preferences())
	}

	exe, err := getAppExecutable(runtime.GOOS, appName)
	if err != nil {
		panic("Unable to determine app executable.")
	}
	startApp := &autostart.App{
		Name:        appNamespace,
		DisplayName: appName,
		Exec:        []string{exe},
	}
	autoLaunch := prefs.Bool("AutoLaunchApp")

	w := a.NewWindow(appName + " Settings")

	if desk, ok := a.(desktop.App); ok {
		menu = fyne.NewMenu(appName,
			fyne.NewMenuItem(sweepMenuLabel, func() {
				err := sweepFiles(os.DirFS(prefs.String("SourcePath")), prefs.String("SourcePath"), getTargetPath(prefs))
				if err != nil {
					slog.Warn("Failed to move source files. ", slog.Any("error", err))
				}
				prefs.SetString("LastSweep", time.Now().Format(time.Kitchen))
				lastSweepMenu.Label = fmt.Sprintf(sweptMenuLabel, prefs.String("LastSweep"))
				menu.Refresh()
			}),
			fyne.NewMenuItem(settingsMenuLabel, func() {
				w.Show()
			}),
			fyne.NewMenuItemSeparator(),
			lastSweepMenu)

		desk.SetSystemTrayIcon(resourceDeskcleanicondarkSvg)
		desk.SetSystemTrayMenu(menu)
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
					slog.Warn("Unable to unset auto launch.", slog.Any("error", err))
				}
			} else if !startApp.IsEnabled() && autoLaunch {
				slog.Info("Creating LaunchAgent plist.")
				if err := startApp.Enable(); err != nil {
					slog.Warn("Unable to set auto launch.", slog.Any("error", err))
				}
			}
		}
		w.Hide()
	})

	// Invoke background ticker with reasonable tick rate
	sweepTicker := time.NewTicker(60 * time.Minute)

	// Reset ticker if preference is set to anything other than on demand
	if prefs.Int("RunIntervalMinutes") > -1 {
		sweepTicker.Reset(time.Duration(prefs.Int("RunIntervalMinutes")) * time.Minute)
	}

	// Create a data binding with the pref RunIntervalMinutes
	runInterval := binding.BindPreferenceInt("RunIntervalMinutes", prefs)

	// Add change listener to the RunIntervalMinutes property/preference
	callback := binding.NewDataListener(func() {
		ri, _ := runInterval.Get()
		resetChan <- ri
	})
	runInterval.AddListener(callback)

	// Start background sweeper thread
	go func() {
		for {
			select {
			case <-doneChan:
				return
			case i := <-resetChan:
				if i > -1 {
					sweepTicker.Reset(time.Duration(i) * time.Minute)
					slog.Info("Set sweep timer event.", slog.Int("RunIntervalMinutes", prefs.Int("RunIntervalMinutes")))
				} else {
					slog.Info("Sweeper set to run on demand.")
				}
			case <-sweepTicker.C:
				if prefs.Int("RunIntervalMinutes") > 0 {
					err := sweepFiles(os.DirFS(prefs.String("SourcePath")), prefs.String("SourcePath"), getTargetPath(prefs))
					if err != nil {
						slog.Error("Failed to sweep source files.", slog.Any("error", err))
					}
					prefs.SetString("LastSweep", time.Now().Format(time.Kitchen))
					lastSweepMenu.Label = fmt.Sprintf(sweptMenuLabel, prefs.String("LastSweep"))
					menu.Refresh()
				}
			}
		}
	}()

	a.Run()
	doneChan <- true
}

func getLogFile(osName, appName, logFilename string) *os.File {
	var log string

	switch osName {
	case "darwin":
		log = path.Join(xdg.Home, "Library", "Logs", appName, logFilename)
	case "default":
		log = path.Join(xdg.DataHome, appName, "logs", logFilename)

	}

	if _, err := os.Stat(log); os.IsNotExist(err) {
		os.MkdirAll(path.Dir(log), 0700)
	}

	logFile, err := os.OpenFile(log, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("Unable to create log file.", "file", log)
		logFile = os.Stdout
	}

	return logFile
}

func getAppExecutable(osName, appName string) (string, error) {
	var e string
	var err error

	switch osName {
	case "darwin":
		e = path.Join(xdg.ApplicationDirs[0], appName) + ".app"
	case "default":
		e, err = os.Executable()
		if err != nil {
			return "", err
		}

	}

	return e, nil
}

func makeSettingsUI(pref fyne.Preferences) fyne.CanvasObject {
	al := widget.NewLabel(pref.String("HomeDir"))
	ri := widget.NewSelect(allowedRunIntervals, func(value string) { pref.SetString("RunInterval", value) })
	ri.SetSelected(pref.String("RunInterval"))

	df := widget.NewSelect(allowedDateFormats, func(value string) { pref.SetString("TargetFolderDateScheme", value) })
	df.SetSelected(pref.String("TargetFolderDateScheme"))

	af := widget.NewEntryWithData(binding.BindPreferenceString("AppFolder", pref))
	af.OnChanged = func(s string) {
		al.SetText(path.Join(pref.String("HomeDir"), s))
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
	pref.SetString("AppName", appNameDefault)
	pref.SetString("AppFolder", appNameDefault)
	pref.SetString("HomeDir", xdg.Home)
	pref.SetString("TargetFolderLabel", "Archive")
	pref.SetString("TargetFolderSeperator", "-")
	pref.SetString("TargetFolderDateScheme", "2006-01-02")
	pref.SetString("RunInterval", "every hour")
	pref.SetInt("RunIntervalMinutes", 60)
	pref.SetString("SourcePath", xdg.UserDirs.Desktop)
	pref.SetBool("FirstRun", false)
	pref.SetBool("AutoLaunchApp", false)
	slog.Info("Configuration initialized to defaults.")
}

func getTargetPath(pref fyne.Preferences) string {
	now := time.Now()
	dateStr := now.Format(pref.String("TargetFolderDateScheme"))
	folderDateLabel := fmt.Sprintf("%s%s%s", dateStr, pref.String("TargetFolderSeperator"), pref.String("TargetFolderLabel"))
	return path.Join(pref.String("HomeDir"), pref.String("AppFolder"), folderDateLabel)
}

func sweepFiles(fsys fs.FS, sourcePath, targetPath string) error {
	moveCount := 0
	skippedCount := 0
	errorCount := 0
	targeExists := false

	fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() || d.Type().IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				skippedCount++
			} else {
				if !targeExists {
					// Determine if parent path needs created and only create if there is a file/folder to write
					err = createTargetDirectory(targetPath)
					if err != nil {
						slog.Error("Unable to create target directory. ", slog.Any("error", err))
						// If we cannot create the containing folder then fail fast
						return err
					}
					targeExists = true
				}

				err = os.Rename(path.Join(sourcePath, p), path.Join(targetPath, p))
				if err != nil {
					errorCount++
					slog.Warn("Failed to move file.", slog.Any("error", err), slog.String("file", p))
				} else {
					moveCount++
				}
			}
		}
		return nil
	})
	slog.Info("Sweep completed.", slog.Int("sweptFileCount", moveCount), slog.Int("skippedFileCount", skippedCount), slog.Int("fileErrorCount", errorCount))
	return nil
}

func createTargetDirectory(targetPath string) error {
	if _, err := os.Stat(targetPath); errors.Is(err, os.ErrNotExist) {
		err := os.MkdirAll(targetPath, os.ModePerm)
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

	default:
		return -1
	}
}
