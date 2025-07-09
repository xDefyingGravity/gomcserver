package gomcserver

import (
	"errors"
	"fmt"
	"github.com/magiconair/properties"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/xDefyingGravity/gomcserver/backup"
	"github.com/xDefyingGravity/gomcserver/download"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Server represents a Minecraft server instance.
type Server struct {
	Name         string
	Version      string
	Directory    string
	Port         int
	MemoryMB     int
	Props        *properties.Properties
	EULAAccepted bool
	PlayerCount  int

	stdoutPipe io.Writer
	stderrPipe io.Writer
	stdinPipe  io.WriteCloser
	running    bool
	cmd        *exec.Cmd
	pid        int

	onStdout      func(string)
	onStderr      func(string)
	onPlayerJoin  func(string, int)
	onPlayerLeave func(string, int)

	signals chan os.Signal
}

// StartOptions configures how the server is started.
type StartOptions struct {
	StdoutPipe       io.Writer
	StderrPipe       io.Writer
	UseManifestCache *bool
	CacheDir         *string
}

// ServerStats holds runtime statistics for the server process.
type ServerStats struct {
	CPUPercent  float64
	MemoryMB    float64
	ThreadCount int32
	Uptime      time.Duration
}

// NewServer creates a new Server instance.
func NewServer(name, versionOrUrl string) *Server {
	absDir, err := filepath.Abs(name)
	if err != nil {
		absDir = name
	}

	return &Server{
		Version:   versionOrUrl,
		Port:      25565,
		MemoryMB:  2048,
		Props:     properties.NewProperties(),
		Directory: absDir,
		signals:   make(chan os.Signal, 1),
	}
}

// AcceptEULA marks the EULA as accepted.
func (s *Server) AcceptEULA() {
	s.EULAAccepted = true
}

// IsRunning returns true if the server is running.
func (s *Server) IsRunning() bool {
	return s.running
}

// GetPID returns the process ID of the running server, or -1 if not running.
func (s *Server) GetPID() int {
	if s.cmd != nil && s.cmd.Process != nil {
		return s.cmd.Process.Pid
	}
	return -1
}

// SetEventListener registers a callback for a specific event type.
func (s *Server) SetEventListener(listenerType string, fn interface{}) error {
	switch listenerType {
	case "stdout":
		if f, ok := fn.(func(string)); ok {
			s.onStdout = f
			return nil
		}
	case "stderr":
		if f, ok := fn.(func(string)); ok {
			s.onStderr = f
			return nil
		}
	case "playerJoin":
		if f, ok := fn.(func(string, int)); ok {
			s.onPlayerJoin = f
			return nil
		}
	case "playerLeave":
		if f, ok := fn.(func(string, int)); ok {
			s.onPlayerLeave = f
			return nil
		}
	}
	return fmt.Errorf("unknown or invalid listener type: %s", listenerType)
}

// SetProperty sets a server property.
func (s *Server) SetProperty(key, value string) {
	if s.Props == nil {
		s.Props = properties.NewProperties()
	}
	_, _, _ = s.Props.Set(key, value)
}

// GetProperty retrieves a server property.
func (s *Server) GetProperty(key string) (string, bool) {
	if s.Props == nil {
		return "", false
	}
	return s.Props.Get(key)
}

// GetProperties returns all server properties.
func (s *Server) GetProperties() *properties.Properties {
	if s.Props == nil {
		s.Props = properties.NewProperties()
	}
	return s.Props
}

// Start launches the Minecraft server process.
func (s *Server) Start(opts *StartOptions) error {
	if err := s.ensureDirectory(); err != nil {
		return err
	}
	opts = s.applyDefaultStartOptions(opts)
	if err := s.prepare(*opts.UseManifestCache, *opts.CacheDir); err != nil {
		return err
	}
	s.setupSignalHandlers(opts)
	return s.launchProcess(opts)
}

// Stop terminates the running server process.
func (s *Server) Stop() error {
	if !s.running {
		return errors.New("server is not running")
	}
	if s.cmd == nil || s.cmd.Process == nil {
		return errors.New("server process is not available")
	}

	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("failed to send SIGTERM: %w", err)
	}

	const maxWait = 30 * time.Second
	const interval = 1 * time.Second
	waited := time.Duration(0)

	for waited < maxWait {
		if err := s.cmd.Process.Signal(syscall.Signal(0)); err != nil {
			s.running = false
			s.pid = -1
			return nil
		}
		time.Sleep(interval)
		waited += interval
	}

	if err := s.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to force kill server after timeout: %w", err)
	}

	s.running = false
	s.pid = -1
	return nil
}

// SendCommand sends a command to the server's stdin.
func (s *Server) SendCommand(command string) error {
	if !s.running {
		return errors.New("server is not running")
	}
	if s.stdinPipe == nil {
		return errors.New("stdin pipe is not available")
	}
	_, err := io.WriteString(s.stdinPipe, command+"\n")
	return err
}

// GetStats returns runtime statistics for the server process.
func (s *Server) GetStats() (*ServerStats, error) {
	if !s.running {
		return nil, errors.New("server is not running")
	}
	if s.pid <= 0 {
		return nil, errors.New("server process PID is not available")
	}
	return getPIDStats(int32(s.pid))
}

// SetDifficulty sets the server difficulty.
func (s *Server) SetDifficulty(difficulty string) error {
	valid := map[string]bool{"peaceful": true, "easy": true, "normal": true, "hard": true}
	if !valid[difficulty] {
		return fmt.Errorf("invalid difficulty '%s'. Valid options: peaceful, easy, normal, hard", difficulty)
	}
	s.SetProperty("difficulty", difficulty)
	return nil
}

// SetWeather changes the in-game weather.
func (s *Server) SetWeather(weather string) error {
	valid := map[string]bool{"clear": true, "rain": true, "thunder": true}
	if !valid[weather] {
		return fmt.Errorf("invalid weather '%s'. Valid options: clear, rain, thunder", weather)
	}
	return s.SendCommand("weather " + weather)
}

// SetTime sets the in-game time.
func (s *Server) SetTime(timeValue string) error {
	valid := map[string]bool{"day": true, "night": true, "noon": true, "midnight": true}
	if !valid[timeValue] {
		return fmt.Errorf("invalid time '%s'. Valid options: day, night, noon, midnight", timeValue)
	}
	return s.SendCommand("time set " + timeValue)
}

// --- Internal helpers ---

func (s *Server) ensureDirectory() error {
	if _, err := os.Stat(s.Directory); os.IsNotExist(err) {
		return os.MkdirAll(s.Directory, 0755)
	}
	return nil
}

func (s *Server) applyDefaultStartOptions(opts *StartOptions) *StartOptions {
	if opts == nil {
		opts = &StartOptions{}
	}
	if opts.StdoutPipe == nil {
		opts.StdoutPipe = os.Stdout
	}
	if opts.StderrPipe == nil {
		opts.StderrPipe = os.Stderr
	}
	if opts.CacheDir == nil {
		dir, err := os.UserCacheDir()
		if err != nil {
			dir = os.TempDir()
		}
		cacheDir := filepath.Join(dir, "mcserverlib_cache")
		opts.CacheDir = &cacheDir
	}
	if opts.UseManifestCache == nil {
		defaultUseManifestCache := true
		opts.UseManifestCache = &defaultUseManifestCache
	}
	return opts
}

func (s *Server) setupSignalHandlers(opts *StartOptions) {
	signal.Notify(s.signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for sig := range s.signals {
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				_ = s.Stop()
			case syscall.SIGHUP:
				_ = s.Stop()
				_ = s.Start(opts)
			}
		}
	}()
}

func (s *Server) launchProcess(opts *StartOptions) error {
	s.cmd = exec.Command("java", "-Xmx"+strconv.Itoa(s.MemoryMB)+"M", "-jar", "server.jar", "nogui")
	s.cmd.Dir = s.Directory

	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}
	go s.listenToStdout(stdout)

	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to get stderr pipe: %w", err)
	}
	go s.listenToStderr(stderr)

	stdinPipe, err := s.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdin pipe: %w", err)
	}
	s.stdinPipe = stdinPipe

	if err := s.cmd.Start(); err != nil {
		return err
	}

	s.running = true
	s.pid = s.cmd.Process.Pid
	s.stdoutPipe = nil
	s.stderrPipe = nil
	return nil
}

func (s *Server) prepare(useManifestCache bool, cacheDir string) error {
	if err := s.validateConfig(); err != nil {
		return err
	}
	if err := s.writeEULA(); err != nil {
		return err
	}
	if err := s.writeProperties(); err != nil {
		return err
	}
	_, err := download.DownloadServerJar(s.Version, s.Directory, useManifestCache, cacheDir)
	return err
}

func (s *Server) validateConfig() error {
	if s.Directory == "" {
		return errors.New("server directory is not set")
	}
	if s.Name == "" {
		s.Name = s.generateServerName()
	}
	if s.Port < 1 || s.Port > 65535 {
		return fmt.Errorf("port %d is out of range (1–65535)", s.Port)
	}
	totalMB, err := getTotalMemoryMB()
	if err != nil {
		return fmt.Errorf("failed to get total memory: %w", err)
	}
	if s.MemoryMB < 512 || s.MemoryMB > totalMB-512 {
		return fmt.Errorf("memory %d MB is out of range (512–%d)", s.MemoryMB, totalMB-512)
	}
	if s.MemoryMB%512 != 0 {
		return fmt.Errorf("memory %d MB must be a multiple of 512", s.MemoryMB)
	}
	if !s.EULAAccepted {
		return errors.New("EULA not accepted")
	}
	if s.running {
		return errors.New("server is already running")
	}
	return nil
}

func (s *Server) generateServerName() string {
	serversInDirectory, err := os.ReadDir(s.Directory)
	if err != nil {
		return s.Version + "-server"
	}
	var matchedFiles []string
	target := s.Version + "-server"
	for _, entry := range serversInDirectory {
		if !entry.IsDir() && strings.Contains(entry.Name(), target) {
			matchedFiles = append(matchedFiles, entry.Name())
		}
	}
	name := s.Version + "-server"
	if len(matchedFiles) > 0 {
		name += "-" + strconv.Itoa(len(matchedFiles)+1)
	}
	return name
}

func (s *Server) writeEULA() error {
	return os.WriteFile(filepath.Join(s.Directory, "eula.txt"), []byte("eula=true\n"), 0644)
}

func (s *Server) writeProperties() error {
	if s.Props == nil {
		return nil
	}

	propsFilePath := filepath.Join(s.Directory, "server.properties")

	existingProps := properties.NewProperties()
	if data, err := os.ReadFile(propsFilePath); err == nil {
		err = existingProps.Load(data, properties.UTF8)
		if err != nil {
			return fmt.Errorf("failed to load existing properties: %w", err)
		}
	}

	for _, key := range s.Props.Keys() {
		val, _ := s.Props.Get(key)
		err := existingProps.SetValue(key, val)
		if err != nil {
			return err
		}
	}

	return os.WriteFile(propsFilePath, []byte(existingProps.String()), 0644)
}

func (s *Server) listenToStdout(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 && s.onStdout != nil {
			s.internalOnStdout(string(buf[:n]))
			s.onStdout(string(buf[:n]))
		}
		if err != nil {
			break
		}
	}
}

func (s *Server) listenToStderr(r io.Reader) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 && s.onStderr != nil {
			s.internalOnStderr(string(buf[:n]))
			s.onStderr(string(buf[:n]))
		}
		if err != nil {
			break
		}
	}
}

func (s *Server) internalOnStdout(message string) {
	if strings.Contains(message, "joined the game") || strings.Contains(message, "left the game") {
		parts := strings.SplitN(message, "]: ", 2)
		if len(parts) < 2 {
			return
		}
		line := parts[1]
		words := strings.Split(line, " ")
		if len(words) >= 1 {
			playerName := words[0]
			if strings.Contains(line, "joined the game") {
				s.PlayerCount++
				if s.onPlayerJoin != nil {
					s.onPlayerJoin(playerName, s.PlayerCount)
				}
			} else if strings.Contains(line, "left the game") {
				if s.PlayerCount > 0 {
					s.PlayerCount--
				}
				if s.onPlayerLeave != nil {
					s.onPlayerLeave(playerName, s.PlayerCount)
				}
			}
		}
	}
}

func (s *Server) internalOnStderr(message string) {
	// Reserved for future error handling/logging
}

// --- Utility functions ---

func getPIDStats(pid int32) (*ServerStats, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return nil, err
	}
	cpu, err := p.Percent(time.Second)
	if err != nil {
		return nil, err
	}
	memInfo, err := p.MemoryInfo()
	if err != nil {
		return nil, err
	}
	threads, err := p.NumThreads()
	if err != nil {
		return nil, err
	}
	createTime, err := p.CreateTime()
	if err != nil {
		return nil, err
	}
	uptime := time.Since(time.UnixMilli(createTime))
	return &ServerStats{
		CPUPercent:  cpu,
		MemoryMB:    float64(memInfo.RSS) / (1024 * 1024),
		ThreadCount: threads,
		Uptime:      uptime,
	}, nil
}

func getTotalMemoryMB() (int, error) {
	vm, err := mem.VirtualMemory()
	if err != nil {
		return 0, err
	}
	return int(vm.Total / 1024 / 1024), nil
}

func (s *Server) Backup(nonBlocking bool) error {
	backupDir := filepath.Join(s.Directory, "backups")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	doBackup := func() error {
		return backup.CreateBackup(s.Directory, backupDir)
	}

	if nonBlocking {
		go func() {
			if err := doBackup(); err != nil {
				fmt.Println("ERR: Backup failed:", err)
			}
		}()
		return nil
	}

	err := doBackup()
	if err != nil {
		return err
	}
	return nil
}
