package mpm

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pelletier/go-toml"
)

// Cmd .
type Cmd struct {
	Cmd   string
	Flags []string
	Dir   string
	Env   []string
}

type configFile map[string]Cmd

// Mpm .
type Mpm struct {
	sync.Mutex

	reader io.ReadCloser
	writer io.WriteCloser
	stdin  io.Reader

	fancy string

	procs map[int]*exec.Cmd
}

// New .
func New() *Mpm {
	r, w := io.Pipe()

	go func() {
		_, err := io.Copy(os.Stdout, r)
		defer r.Close()
		if err != nil {
			// @TODO: log
		}
	}()

	return &Mpm{
		stdin:  os.Stdin,
		reader: r,
		writer: w,
		procs:  make(map[int]*exec.Cmd),
	}
}

// LoadFile .
func (m *Mpm) LoadFile(file string) error {
	fc, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}

	cmds := make(configFile)
	err = toml.Unmarshal(fc, &cmds)
	if err != nil {
		return err
	}

	for _, entry := range cmds {
		cmd := strings.Split(entry.Cmd, " ")
		cmd = append(cmd, entry.Flags...)

		proc := m.newProc(cmd[0], cmd[1:])

		if entry.Dir != "" {
			dir, err := filepath.Abs(entry.Dir)
			if err != nil {
				m.killAll()
				return err
			}
			proc.Dir = filepath.Clean(dir)
		}

		proc.Env = os.Environ()
		for _, e := range entry.Env {
			proc.Env = append(proc.Env, e)
		}

		if err := proc.Start(); err != nil {
			m.killAll()
			return fmt.Errorf("error starting process %v: %w", entry.Cmd, err)
		}

		m.add(proc)
	}

	return nil
}

// Stop .
func (m *Mpm) Stop() error {
	return m.killAll()
}

func (m *Mpm) add(p *exec.Cmd) {
	m.Lock()
	defer m.Unlock()
	m.procs[p.Process.Pid] = p
}

func (m *Mpm) newProc(cmd string, flags []string) *exec.Cmd {
	proc := exec.Command(cmd, flags...)
	proc.Stdout = m.writer
	proc.Stderr = m.writer
	proc.Stdin = m.stdin
	proc.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	return proc
}

func (m *Mpm) killAll() error {
	m.Lock()
	defer m.Unlock()

	for _, p := range m.procs {
		m.killUnsafe(p.Process.Pid)
	}

	_ = m.reader.Close()
	return m.writer.Close()
}

func (m *Mpm) kill(pid int) error {
	m.Lock()
	defer m.Unlock()

	return m.killUnsafe(pid)
}

func (m *Mpm) killUnsafe(pid int) error {
	proc, ok := m.procs[pid]
	if !ok {
		return nil
	}

	exited := make(chan bool, 1)
	timeout := make(chan bool, 1)
	go func() {
		time.Sleep(5 * time.Second)
		timeout <- true
	}()

	if err := killChildProcesses(strconv.Itoa(proc.Process.Pid)); err != nil {
		// pm.logger.Warn().Err(err).Msg("error killing grandchild processes")
	}

	proc.Process.Signal(syscall.SIGTERM)
	go func() {
		if err := proc.Wait(); err != nil {
			// pm.logger.Warn().Err(err).Msg("error waiting for app to be killed after SIGTERM")
		}
		exited <- true
	}()

	select {
	case <-exited:
	case <-timeout:
		if err := proc.Process.Kill(); err != nil {
			// pm.logger.Error().Err(err).Msg("error killing app")
		}
	}

	if err := proc.Process.Release(); err != nil {
		// pm.logger.Error().Err(err).Msg("error releasing process")
	}

	return nil
}

func killChildProcesses(pid string) error {
	pgrep := exec.Command("pgrep", "-P", pid)

	childPidsLines, err := pgrep.Output()
	if err != nil {
		return fmt.Errorf("%v %w: %v", pgrep.String(), err, childPidsLines)
	}

	childPids := strings.Split(string(childPidsLines), "\n")
	for _, childPid := range childPids {
		if childPid == "" {
			continue
		}

		kill := exec.Command("kill", "-s", "TERM", childPid)

		_ = killChildProcesses(childPid)

		if o, err := kill.Output(); err != nil {
			return fmt.Errorf("%v %w: %v", kill.String(), err, o)
		}
	}

	return nil
}
