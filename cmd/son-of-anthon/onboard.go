package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const onboardLogo = "🚀"

func onboardCmd() {
	args := os.Args[2:]
	fullSetup := contains(args, "--full") || contains(args, "-f")
	installDeps := contains(args, "--deps") || fullSetup
	download := contains(args, "--download") || fullSetup
	installService := contains(args, "--install") || fullSetup
	enableAutostart := contains(args, "--autostart") || fullSetup
	showStatus := contains(args, "--status") || fullSetup
	startService := contains(args, "--start") || fullSetup

	fmt.Printf("%s Son of Anthon - Onboard\n\n", onboardLogo)

	platform := detectPlatform()
	fmt.Printf("Detected platform: %s\n\n", platform)

	if installDeps {
		installDependencies(platform)
	}

	if download {
		downloadAndExtract(platform)
	}

	if installService {
		installServiceForPlatform(platform)
	}

	if enableAutostart {
		enableAutostartForPlatform(platform)
	}

	cfg, err := loadConfig()
	if err != nil || cfg == nil {
		fmt.Println("Running setup wizard...")
		setupCmd()
	} else {
		fmt.Println("Config already exists. Skipping setup.")
	}

	if startService {
		startServiceForPlatform(platform)
	}

	if showStatus {
		showStatusForPlatform(platform)
	}

	if !installDeps && !download && !installService && !enableAutostart && !startService && !showStatus {
		printOnboardHelp()
	}
}

func detectPlatform() string {
	if isProotDebian() {
		return "proot"
	}
	if isTermux() {
		return "termux"
	}
	if runtime.GOOS == "linux" {
		return "linux"
	}
	if runtime.GOOS == "darwin" {
		return "darwin"
	}
	if runtime.GOOS == "windows" {
		return "windows"
	}
	return "unknown"
}

func isTermux() bool {
	if _, err := os.Stat("/data/data/com.termux/files/usr/bin/termux-changeip"); err == nil {
		return true
	}
	if os.Getenv("TERMUX_APP__PACKAGE_NAME") != "" {
		return true
	}
	prefix := os.Getenv("PREFIX")
	if prefix != "" && prefix != "/usr" && prefix != "/usr/local" {
		if _, err := os.Stat(filepath.Join(prefix, "bin/termux-info")); err == nil {
			return true
		}
	}
	return false
}

func isProotDebian() bool {
	if exists("/usr/bin/apt") {
		if data, err := os.ReadFile("/etc/os-release"); err == nil {
			content := string(data)
			if strings.Contains(content, "Debian") || strings.Contains(content, "Ubuntu") {
				return true
			}
		}
	}
	return false
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func installDependencies(platform string) {
	fmt.Printf("[%s] Installing dependencies...\n", platform)

	switch platform {
	case "termux":
		runCmd("pkg", "update", "-y")
		runCmd("pkg", "install", "-y", "coreutils", "git", "curl", "wget", "termux-services", "sox", "ffmpeg")
	case "proot":
		fmt.Println("Make sure you have runit installed in proot-debian:")
		fmt.Println("  sudo apt install -y runit")
	case "linux":
		if exists("/usr/bin/apt") {
			runCmd("sudo", "apt", "update")
			runCmd("sudo", "apt", "install", "-y", "curl", "git", "sox", "ffmpeg")
		} else if exists("/usr/bin/dnf") {
			runCmd("sudo", "dnf", "install", "-y", "curl", "git", "sox", "ffmpeg")
		} else if exists("/usr/bin/pacman") {
			runCmd("sudo", "pacman", "-Sy", "--noconfirm", "curl", "git", "sox", "ffmpeg")
		}
	case "darwin":
		if exists("/usr/local/bin/brew") || exists("/opt/homebrew/bin/brew") {
			runCmd("brew", "install", "curl", "git", "sox", "ffmpeg")
		}
	case "windows":
		fmt.Println("Windows: Dependencies must be installed manually or via winget")
		fmt.Println("  winget install Git.Software.Git sox.ffmpeg")
	}
}

func downloadAndExtract(platform string) {
	fmt.Printf("[%s] Downloading latest release...\n", platform)

	version := getLatestVersion()
	if version == "" {
		fmt.Println("Failed to get latest version")
		return
	}

	fmt.Printf("Latest version: %s\n", version)

	dir := filepath.Join(os.Getenv("HOME"), "son-of-anthon")
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("son-of-anthon_%s_%s_%s.tar", version[1:], platform, runtime.GOARCH)
	url := fmt.Sprintf("https://github.com/JonyBepary/son-of-anthon/releases/download/%s/%s", version, filename)

	fmt.Printf("Downloading: %s\n", url)

	resp, err := http.Get(url)
	if err != nil {
		fmt.Printf("Download failed: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("Download failed with status: %d\n", resp.StatusCode)
		return
	}

	tarPath := filepath.Join(dir, "son-of-anthon.tar")
	out, err := os.Create(tarPath)
	if err != nil {
		fmt.Printf("Failed to create file: %v\n", err)
		return
	}
	defer out.Close()

	io.Copy(out, resp.Body)

	runCmd("tar", "-xf", tarPath, "-C", dir)
	os.Remove(tarPath)

	fmt.Printf("Extracted to: %s\n", dir)
}

func installServiceForPlatform(platform string) {
	fmt.Printf("[%s] Installing service...\n", platform)

	switch platform {
	case "termux":
		installTermuxService()
	case "proot":
		installProotService()
	case "linux":
		installLinuxService()
	case "darwin":
		installDarwinService()
	case "windows":
		installWindowsService()
	}
}

func installProotService() {
	home := os.Getenv("HOME")
	cwd, _ := os.Getwd()
	binaryPath := filepath.Join(cwd, "son-of-anthon")
	if !exists(binaryPath) {
		binaryPath = filepath.Join(home, "son-of-anthon", "son-of-anthon")
	}

	serviceDir := filepath.Join(home, ".config", "runit", "son-of-anthon")
	os.MkdirAll(serviceDir, 0755)
	os.MkdirAll(filepath.Join(serviceDir, "log"), 0755)

	runScript := filepath.Join(serviceDir, "run")
	content := fmt.Sprintf(`#!/bin/sh
exec 2>&1
export PATH="/usr/local/bin:/usr/bin:/bin"
export HOME="%s"
exec %s gateway
`, home, binaryPath)
	os.WriteFile(runScript, []byte(content), 0755)

	logDir := filepath.Join(home, ".picoclaw", "runit-logs")
	os.MkdirAll(logDir, 0755)

	logScript := filepath.Join(serviceDir, "log", "run")
	os.WriteFile(logScript, []byte("#!/bin/sh\nexec svlogd -tt "+logDir+"\n"), 0755)

	// Try to create symlink, but don't fail if it doesn't work
	os.MkdirAll("/etc/service", 0755)
	os.MkdirAll("/run/service", 0755)
	os.Symlink(serviceDir, "/etc/service/son-of-anthon")
	os.Symlink(serviceDir, "/run/service/son-of-anthon")

	fmt.Println("Proot service installed at:", serviceDir)
	fmt.Println("Binary path:", binaryPath)
	fmt.Println("")
	fmt.Println("To start service manually (if sv doesn't work):")
	fmt.Println("  ~/.config/runit/son-of-anthon/run &")
}

func installTermuxService() {
	prefix := os.Getenv("PREFIX")
	if prefix == "" {
		prefix = "/data/data/com.termux/files/usr"
	}
	home := os.Getenv("HOME")

	serviceDir := filepath.Join(prefix, "var", "service", "son-of-anthon")
	os.MkdirAll(serviceDir, 0755)
	os.MkdirAll(filepath.Join(serviceDir, "log"), 0755)

	runScript := filepath.Join(serviceDir, "run")
	content := fmt.Sprintf(`#!/data/data/com.termux/files/usr/bin/sh
exec 2>&1
export PATH="%s/bin:$PATH"
export GODEBUG=netdns=go
export HOME="/data/data/com.termux/files/home"
exec %s/bin/son-of-anthon gateway
`, prefix, prefix)
	os.WriteFile(runScript, []byte(content), 0755)

	logDir := filepath.Join(home, ".picoclaw", "termux-logs")
	os.MkdirAll(logDir, 0755)

	logScript := filepath.Join(serviceDir, "log", "run")
	os.WriteFile(logScript, []byte("#!/data/data/com.termux/files/usr/bin/sh\nexec svlogd -tt "+logDir+"\n"), 0755)

	fmt.Println("Termux service installed at:", serviceDir)
	fmt.Println("Use: sv up son-of-anthon")
}

func installLinuxService() {
	home := os.Getenv("HOME")
	serviceName := "son-of-anthon"

	serviceFile := fmt.Sprintf(`/etc/systemd/system/%s.service
[Unit]
Description=Son of Anthon - Multi-agent AI Assistant
After=network.target

[Service]
Type=simple
User=%s
WorkingDirectory=%s
ExecStart=%s/.local/bin/son-of-anthon gateway
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
`, serviceName, home, home, home)

	os.WriteFile("/tmp/"+serviceName+".service", []byte(serviceFile), 0644)
	runCmd("sudo", "mv", "/tmp/"+serviceName+".service", "/etc/systemd/system/")
	runCmd("sudo", "systemctl", "daemon-reload")
	runCmd("sudo", "systemctl", "enable", serviceName)

	fmt.Println("Linux systemd service installed")
	fmt.Println("Use: sudo systemctl start son-of-anthon")
}

func installDarwinService() {
	home := os.Getenv("HOME")
	launchdDir := filepath.Join(home, "Library", "LaunchAgents")
	os.MkdirAll(launchdDir, 0755)

	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.sonofanthon.gateway</string>
    <key>ProgramArguments</key>
    <array>
        <string>%s/son-of-anthon/son-of-anthon</string>
        <string>gateway</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
</dict>
</plist>
`, home)

	plistPath := filepath.Join(launchdDir, "com.sonofanthon.gateway.plist")
	os.WriteFile(plistPath, []byte(plist), 0644)

	runCmd("launchctl", "load", plistPath)

	fmt.Println("macOS launchd service installed")
	fmt.Println("Use: launchctl start com.sonofanthon.gateway")
}

func installWindowsService() {
	fmt.Println("Windows service installation requires NSSM")
	fmt.Println("Download from: https://nssm.cc/download")
	fmt.Println("Then run: nssm install SonOfAnthon C:\\Users\\...\\son-of-anthon.exe gateway")
	fmt.Println("Or simply run: son-of-anthon gateway")
}

func enableAutostartForPlatform(platform string) {
	fmt.Printf("[%s] Enabling autostart...\n", platform)

	switch platform {
	case "termux":
		home := os.Getenv("HOME")

		bootDir := filepath.Join(home, ".termux", "boot")
		os.MkdirAll(bootDir, 0755)

		bootScript := filepath.Join(bootDir, "son-of-anthon")
		os.WriteFile(bootScript, []byte("#!/data/data/com.termux/files/usr/bin/sh\nsv up son-of-anthon\n"), 0755)

		if !exists(filepath.Join(home, ".bashrc")) || !containsFile(".bashrc", "son-of-anthon") {
			f, _ := os.OpenFile(filepath.Join(home, ".bashrc"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			f.WriteString("\n# Auto-start son-of-anthon\nsv up son-of-anthon 2>/dev/null || true\n")
			f.Close()
		}

		fmt.Println("Termux autostart enabled (boot + bashrc)")

	case "proot":
		home := os.Getenv("HOME")

		// For proot, just add to bashrc to run directly (not via sv)
		if !exists(filepath.Join(home, ".bashrc")) || !containsFile(".bashrc", "son-of-anthon") {
			f, _ := os.OpenFile(filepath.Join(home, ".bashrc"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			f.WriteString("\n# Auto-start son-of-anthon\n")
			f.WriteString("if [ -f ~/son-of-anthon/son-of-anthon ]; then\n")
			f.WriteString("  ~/son-of-anthon/son-of-anthon gateway &\n")
			f.WriteString("fi\n")
			f.Close()
		}
		fmt.Println("Proot autostart enabled (bashrc - direct run)")

	case "linux", "darwin":
		fmt.Println("Autostart already enabled via systemd/launchd service")

	case "windows":
		fmt.Println("Windows autostart: Add to Task Scheduler or Startup folder")
	}
}

func startServiceForPlatform(platform string) {
	fmt.Printf("[%s] Starting service...\n", platform)

	home := os.Getenv("HOME")
	cwd, _ := os.Getwd()

	switch platform {
	case "termux":
		runCmd("sv", "up", "son-of-anthon")
	case "proot":
		// Try sv first
		err := runCmdErr("sv", "up", "son-of-anthon")
		if err != nil {
			// Fallback: run directly in background
			binaryPath := filepath.Join(cwd, "son-of-anthon")
			if !exists(binaryPath) {
				binaryPath = filepath.Join(home, "son-of-anthon", "son-of-anthon")
			}
			fmt.Println("sv not available or failed. Starting directly in background...")
			runCmd(binaryPath, "gateway", "&")
		}
	case "linux":
		runCmd("sudo", "systemctl", "start", "son-of-anthon")
	case "darwin":
		runCmd("launchctl", "start", "com.sonofanthon.gateway")
	case "windows":
		fmt.Println("Windows: Service starts automatically or run manually")
	}
}

func showStatusForPlatform(platform string) {
	fmt.Printf("[%s] Status:\n", platform)

	home := os.Getenv("HOME")
	cwd, _ := os.Getwd()

	fmt.Printf("  Binary: ")
	if exists(filepath.Join(cwd, "son-of-anthon")) || exists(filepath.Join(home, "son-of-anthon", "son-of-anthon")) || exists(filepath.Join(home, ".local", "bin", "son-of-anthon")) || exists("/usr/local/bin/son-of-anthon") {
		fmt.Println("✓ Installed")
	} else {
		fmt.Println("✗ Not found")
	}

	fmt.Printf("  Config: ")
	if exists(filepath.Join(home, ".picoclaw", "config.json")) {
		fmt.Println("✓ Configured")
	} else {
		fmt.Println("✗ Run setup")
	}

	fmt.Printf("  Service: ")
	switch platform {
	case "termux", "proot":
		runCmd("sv", "status", "son-of-anthon")
	case "linux":
		runCmd("sudo", "systemctl", "status", "son-of-anthon")
	case "darwin":
		runCmd("launchctl", "list", "|", "grep", "sonofanthon")
	}
}

func printOnboardHelp() {
	fmt.Println("Usage: son-of-anthon onboard [options]")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --full         Full setup (deps + download + install + autostart + start)")
	fmt.Println("  --deps         Install dependencies only")
	fmt.Println("  --download     Download and extract binary")
	fmt.Println("  --install      Install service")
	fmt.Println("  --autostart    Enable autostart")
	fmt.Println("  --start        Start the service")
	fmt.Println("  --status       Show status")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  son-of-anthon onboard --full    # Everything at once")
	fmt.Println("  son-of-anthon onboard --status  # Check status")
}

func getLatestVersion() string {
	resp, err := http.Get("https://api.github.com/repos/JonyBepary/son-of-anthon/releases/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	idx := strings.Index(string(body), `"tag_name": "v`)
	if idx == -1 {
		return ""
	}
	start := idx + len(`"tag_name": "v`)
	end := start
	for ; end < len(body) && body[end] != '"'; end++ {
	}
	return "v" + string(body[start:end])
}

func runCmd(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()
}

func runCmdErr(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func containsFile(filename, pattern string) bool {
	if !exists(filename) {
		return false
	}
	data, _ := os.ReadFile(filename)
	return strings.Contains(string(data), pattern)
}
