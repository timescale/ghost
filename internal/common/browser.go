package common

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenBrowser opens the given URL in the user's default browser. It blocks
// until the browser command completes (which is usually as soon as the browser
// page is successfully opened, but could technically be when the browser page
// is closed on certain older Linux configurations). Prefer this function over
// OpenBrowserAsync if you can tolerate it potentially blocking while the
// browser remains open, or if handling async errors returned on a channel
// (see [OpenBrowserAsync]) is just too complex.
// Declared as a var so tests can replace it with a no-op.
var OpenBrowser = func(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		// Escape '&' so cmd.exe doesn't treat it as a command separator
		cmd = exec.Command("cmd", "/c", "start", strings.ReplaceAll(url, "&", "^&"))
	case "darwin":
		cmd = exec.Command("open", url)
	default: // "linux", "freebsd", "openbsd", "netbsd"
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Run()
}

// ErrNoAppModeBrowser is returned by OpenAppWindow when no Chromium-based
// browser capable of "app mode" (the --app=<url> flag) could be found.
var ErrNoAppModeBrowser = errors.New("no Chromium-based browser found for app-window mode")

// OpenAppWindow opens the given URL in a standalone application window, without
// the address bar / tab strip, using a Chromium-based browser's --app=<url>
// flag. It searches for Chrome, Chromium, Edge, or Brave (in that order) and
// returns ErrNoAppModeBrowser if none is found, so callers can fall back to
// OpenBrowser. Declared as a var so tests can replace it with a no-op.
var OpenAppWindow = func(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return openAppWindowDarwin(url)
	case "windows":
		return openAppWindowWindows(url)
	default: // "linux", "freebsd", "openbsd", "netbsd"
		return openAppWindowLinux(url)
	}
}

// openAppWindowDarwin launches a Chromium-based browser bundle in app mode.
// `open -na` returns as soon as the app is launched, so Run does not block on
// the window staying open.
func openAppWindowDarwin(url string) error {
	apps := []string{"Google Chrome", "Chromium", "Microsoft Edge", "Brave Browser"}
	searchDirs := []string{"/Applications", filepath.Join(os.Getenv("HOME"), "Applications")}
	for _, app := range apps {
		for _, dir := range searchDirs {
			if _, err := os.Stat(filepath.Join(dir, app+".app")); err == nil {
				return exec.Command("open", "-na", app, "--args", "--app="+url).Run()
			}
		}
	}
	return ErrNoAppModeBrowser
}

// openAppWindowLinux launches a Chromium-based browser in app mode. The browser
// is started detached (Start, not Run) because invoking the binary directly can
// block for the lifetime of the window when no instance is already running.
func openAppWindowLinux(url string) error {
	exes := []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "brave-browser"}
	for _, exe := range exes {
		if path, err := exec.LookPath(exe); err == nil {
			return exec.Command(path, "--app="+url).Start()
		}
	}
	return ErrNoAppModeBrowser
}

// openAppWindowWindows launches a Chromium-based browser in app mode. As with
// Linux, the browser is started detached so it does not block on the window.
func openAppWindowWindows(url string) error {
	var roots []string
	for _, env := range []string{"ProgramFiles", "ProgramFiles(x86)", "LocalAppData"} {
		if v := os.Getenv(env); v != "" {
			roots = append(roots, v)
		}
	}
	relPaths := []string{
		filepath.Join("Google", "Chrome", "Application", "chrome.exe"),
		filepath.Join("Chromium", "Application", "chrome.exe"),
		filepath.Join("Microsoft", "Edge", "Application", "msedge.exe"),
		filepath.Join("BraveSoftware", "Brave-Browser", "Application", "brave.exe"),
	}
	for _, rel := range relPaths {
		for _, root := range roots {
			path := filepath.Join(root, rel)
			if _, err := os.Stat(path); err == nil {
				return exec.Command(path, "--app="+url).Start()
			}
		}
	}
	return ErrNoAppModeBrowser
}

// OpenBrowserAsync opens the given URL in the user's default browser in a
// goroutine and returns a channel that receives an error if the browser command
// fails. This is used by the login flow to avoid blocking if the browser
// command hangs on certain Linux configurations. Prefer this function whenever
// you need to open the browser, then wait for something else to happen on a
// channel (because in that case, it's easy to also wait for errors on the
// returned error chan via a select statement), or if opening the browser is
// "best effort" and you don't care to handle errors at all.
func OpenBrowserAsync(url string) <-chan error {
	errCh := make(chan error, 1)
	// Capture the current function value before spawning the goroutine,
	// so that tests can safely restore the original via t.Cleanup without
	// racing with the goroutine.
	fn := OpenBrowser
	go func() {
		if err := fn(url); err != nil {
			errCh <- err
		}
	}()
	return errCh
}
