package daemon

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"time"
)

const EnvName = "XW_DAEMON_IDX"

// The number of times background was called at runtime
var runIdx int = 0

// Daemon
type Daemon struct {
	// records the standard output and error output of daemons and child processes
	LogFile string
	// The maximum number of cycle restarts. If it is 0, it will restart indefinitely
	MaxCount int
	// The maximum number of consecutive startup failures or abnormal exits.
	// Exceed this number, the daemon exits and the child process will not be restarted
	MaxError int
	// The minimum time (in seconds) for a child process to exit normally
	// Less than this time is considered as abnormal exit
	MinExitTime int64
	// validator key for daemon
	ValidatorKey string
}

// Turn the the main program into background running (start a child process, and then exit)
// logFile - If it is not empty, the standard output and error output of the child process will be recorded in this file
// isExit - Whether to exit the main program directly after the child process started.
//          If false, the main program returns * OS Process, the child process returns nil.
//          It needs to be handled by the caller.
func Background(logFile string, priKey string, isExit bool) (*exec.Cmd, error) {
	// Check whether the child process or the parent process
	runIdx++

	envIdx, err := strconv.Atoi(os.Getenv(EnvName))

	if err != nil {
		envIdx = 0
	}

	if runIdx <= envIdx { //child process, exit
		return nil, nil
	}

	// Setting child process environment variables
	env := os.Environ()
	env = append(env, fmt.Sprintf("%s=%d", EnvName, runIdx))

	// Start child process
	cmd, err := startProc(os.Args, env, logFile, priKey)
	if err != nil {
		log.Println(os.Getpid(), "Start child process failed :", err)

		return nil, err
	} else {
		log.Println(os.Getpid(), ":", "Start child process succeed:", "->", cmd.Process.Pid, "\n ")
	}

	if isExit {
		os.Exit(0)
	}

	return cmd, nil
}

func NewDaemon(logFile string) *Daemon {
	return &Daemon{
		LogFile:     logFile,
		MaxCount:    0,
		MaxError:    3,
		MinExitTime: 10,
	}
}

// Run Start the background daemon
func (d *Daemon) Run() {
	// Start a daemon and exit
	_, _ = Background(d.LogFile, d.ValidatorKey, true)

	// The daemon starts a child process and circularly monitors it
	var t int64

	count := 1
	errNum := 0

	for {
		//daemon Information description
		dInfo := fmt.Sprintf("Daemon (pid:%d; count:%d/%d; errNum:%d/%d):",
			os.Getpid(), count, d.MaxCount, errNum, d.MaxError)
		if errNum > d.MaxError {
			log.Println(dInfo, "Failed to start the subprocess too many times, quit")
			os.Exit(1)
		}

		if (d.MaxCount > 0) && (count > d.MaxCount) {
			log.Println(dInfo, "Too many restarts, exit")
			os.Exit(0)
		}
		count++

		t = time.Now().Unix() // Start timestamp
		cmd, err := Background(d.LogFile, d.ValidatorKey, false)

		if err != nil {
			log.Println(dInfo, "The child process failed to start, ", "err:", err)
			errNum++

			continue
		}

		// child process
		if cmd == nil {
			log.Printf("Child process pid=%d: start running...", os.Getpid())

			break
		}

		// Parent process: wait for the child process to exit
		err = cmd.Wait()
		duration := time.Now().Unix() - t // Child process running seconds

		if duration < d.MinExitTime { // Abnormal exit
			errNum++
		} else { // Normal exit
			errNum = 0
		}

		log.Printf("%s Monitoring child process(%d) quit, it has been running for %d seconds: %v\n",
			dInfo, cmd.ProcessState.Pid(), duration, err)
	}
}

func startProc(args, env []string, logFile string, pipeData string) (*exec.Cmd, error) {
	cmd := &exec.Cmd{
		Path:        args[0],
		Args:        args,
		Env:         env,
		SysProcAttr: NewSysProcAttr(),
	}

	if logFile != "" {
		stdout, err := os.OpenFile(logFile, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
		if err != nil {
			log.Println(os.Getpid(), ": Error opening log file:", err)

			return nil, err
		}

		cmd.Stderr = stdout
		cmd.Stdout = stdout
	}

	cmdIn, _ := cmd.StdinPipe()
	_, _ = cmdIn.Write([]byte(pipeData))
	_ = cmdIn.Close()

	err := cmd.Start()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}
