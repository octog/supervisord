package process

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/AlexStocks/supervisord/config"
	"github.com/AlexStocks/supervisord/events"
	"github.com/AlexStocks/supervisord/logger"
	"github.com/AlexStocks/supervisord/signals"
	"github.com/AlexStocks/supervisord/types"
	"github.com/alexstocks/goext/sync/atomic"
	"github.com/alexstocks/goext/sync/deadlock"
	log "github.com/sirupsen/logrus"
)

type ProcessState int64

const (
	STOPPED  ProcessState = iota
	STARTING              = 10
	RUNNING               = 20
	BACKOFF               = 30
	STOPPING              = 40
	EXITED                = 100
	FATAL                 = 200
	UNKNOWN               = 1000
)

func (p ProcessState) String() string {
	switch p {
	case STOPPED:
		return "STOPPED"
	case STARTING:
		return "STARTING"
	case RUNNING:
		return "RUNNING"
	case BACKOFF:
		return "BACKOFF"
	case STOPPING:
		return "STOPPING"
	case EXITED:
		// return "EXITED"
		return "STOPPED"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

type Process struct {
	supervisor_id string
	config        *config.ConfigEntry
	cmd           *exec.Cmd
	startTime     time.Time
	stopTime      time.Time
	// state         ProcessState
	state *gxatomic.Int64
	//true if process is starting
	// inStart bool
	// 20190130 Bug Fix:
	// ?????????????????.
	// ??????????????? Start() -> run() ???run() ??????? lock,
	// ???? Process ????lock???????.
	inStart *gxatomic.Bool
	//true if the process is stopped by user
	stopByUser bool
	pid        *gxatomic.Int64
	retryTimes *int32
	// lock       sync.RWMutex
	lock      gxdeadlock.RWMutex
	stdin     io.WriteCloser
	StdoutLog logger.Logger
	StderrLog logger.Logger
	startKill bool
	exitKill  bool
	updateCb  func(*Process)
}

func NewProcess(supervisor_id string, config *config.ConfigEntry) *Process {
	proc := &Process{
		supervisor_id: supervisor_id,
		config:        config,
		cmd:           nil,
		startTime:     time.Unix(0, 0),
		stopTime:      time.Unix(0, 0),
		state:         gxatomic.NewInt64(int64(STOPPED)),
		inStart:       gxatomic.NewBool(false),
		pid:           gxatomic.NewInt64(0),
		stopByUser:    false,
		retryTimes:    new(int32),
	}

	return proc
}

func (p *Process) SetKillAttr(startKill, exitKill bool) {
	p.startKill = startKill
	p.exitKill = exitKill
}

func (p *Process) ProcessInfo() ProcessInfo {
	return ProcessInfo{
		StartTime: uint64(p.startTime.UnixNano()),
		PID:       int64(p.GetPid()),
		Program:   p.GetName(),
		config:    p.config,
	}
}

func (p *Process) TypeProcessInfo() types.ProcessInfo {
	return types.ProcessInfo{
		Name:           p.GetName(),
		Group:          p.GetGroup(),
		Description:    p.GetDescription(),
		Start:          int(p.GetStartTime().Unix()),
		Stop:           int(p.GetStopTime().Unix()),
		Now:            int(time.Now().Unix()),
		State:          int(p.GetState()),
		Statename:      p.GetState().String(),
		Spawnerr:       "",
		Exitstatus:     p.GetExitstatus(),
		Logfile:        p.GetStdoutLogfile(),
		Stdout_logfile: p.GetStdoutLogfile(),
		Stderr_logfile: p.GetStderrLogfile(),
		Pid:            p.GetPid(),
	}
}

// start the process
// Args:
//  wait - true, wait the program started or failed
func (p *Process) Start(wait bool, updateCb func(*Process)) {
	log.WithFields(log.Fields{"program": p.GetName()}).Info("try to start program")
	p.updateCb = updateCb

	preStart := false
	if p.inStart.Load() {
		log.WithFields(log.Fields{"program": p.GetName()}).Info("Don't start program again, program is already started")
		return
	}

	p.inStart.Store(true)
	p.lock.Lock()
	p.stopByUser = false
	p.lock.Unlock()
	if preStart {
		fmt.Println("Start game over!")
		return
	}

	var runCond *sync.Cond
	finished := false
	if wait {
		runCond = sync.NewCond(&sync.Mutex{})
		runCond.L.Lock()
	}

	go func() {
		sleepInterval := 1e8
		for {
			if wait {
				runCond.L.Lock()
			}
			p.run(func() {
				finished = true
				if wait {
					runCond.L.Unlock()
					runCond.Signal()
				}
			})
			if p.stopByUser {
				log.WithFields(log.Fields{"program": p.GetName()}).Info(
					"Stopped by user, don't start it again")
				fmt.Printf("$$$$ program %s Stopped by user, don't start it again", p.GetName())
				p.changeStateTo(EXITED)
				break
			}
			if !p.isAutoRestart() {
				log.WithFields(log.Fields{"program": p.GetName()}).Info(
					"Don't start the stopped program because its autorestart flag is false")
				break
			}
			if sleepInterval < 15e8 {
				sleepInterval *= 2
			}
			time.Sleep(time.Duration(sleepInterval))
		}
		// p.inStart.Store(false)
	}()
	if wait && !finished {
		runCond.Wait()
		runCond.L.Unlock()
	}
}

func (p *Process) GetName() string {
	if p.config.IsProgram() {
		return p.config.GetProgramName()
	} else if p.config.IsEventListener() {
		return p.config.GetEventListenerName()
	}

	return ""
}

func (p *Process) GetGroup() string {
	return p.config.Group
}

func (p *Process) GetDescription() string {
	if p.inStart.Load() {
		return "process starting now, uptime 0:0:0"
	}

	// p.lock.RLock()
	// defer p.lock.RUnlock()
	state := p.GetState()
	if state == RUNNING {
		seconds := int(time.Now().Sub(p.startTime).Seconds())
		minutes := seconds / 60
		hours := minutes / 60
		days := hours / 24
		if days > 0 {
			return fmt.Sprintf("pid %d, uptime %d days, %d:%02d:%02d", p.GetPid(), days, hours%24, minutes%60, seconds%60)
		}
		return fmt.Sprintf("pid %d, uptime %d:%02d:%02d", p.GetPid(), hours%24, minutes%60, seconds%60)
	} else if state != STOPPED {
		return p.stopTime.String()
	}

	return ""
}

func (p *Process) GetExitstatus() int {
	if p.inStart.Load() {
		return STARTING
	}

	// p.lock.RLock()
	// defer p.lock.RUnlock()
	state := p.GetState()
	if state == EXITED || state == BACKOFF {
		if p.cmd.ProcessState == nil {
			return 0
		}
		status, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
		if ok {
			return status.ExitStatus()
		}
	}

	return 0
}

func (p *Process) GetPid() int {
	state := p.GetState()
	if state == STOPPED ||
		state == FATAL ||
		state == UNKNOWN ||
		state == EXITED ||
		state == BACKOFF {

		return 0
	}

	// p.lock.RLock()
	// defer p.lock.RUnlock()
	// if p.cmd != nil {
	// 	return p.cmd.Process.Pid
	// }

	return int(p.pid.Load())
}

// Get the process state
func (p *Process) GetState() ProcessState {
	// return p.state
	return ProcessState(p.state.Load())
}

func (p *Process) GetStartTime() time.Time {
	return p.startTime
}

func (p *Process) GetStopTime() time.Time {
	switch p.GetState() {
	case STARTING:
		fallthrough
	case RUNNING:
		fallthrough
	case STOPPING:
		return time.Unix(0, 0)
	default:
		return p.stopTime
	}
}

func (p *Process) GetStdoutLogfile() string {
	if p.inStart.Load() {
		return ""
	}
	return getStdoutLogfile(p.config)
}

func (p *Process) GetStderrLogfile() string {
	if p.inStart.Load() {
		return ""
	}
	return getStderrLogfile(p.config)
}

func (p *Process) getStartSeconds() int {
	if p.inStart.Load() {
		return 0
	}
	return p.config.GetInt("startsecs", 1)
}

func (p *Process) getStartRetries() int32 {
	return int32(p.config.GetInt("startretries", 3))
}

func (p *Process) isAutoStart() bool {
	return p.config.GetString("autostart", "true") == "true"
}

func (p *Process) GetPriority() int {
	return p.config.GetInt("priority", 999)
}

func (p *Process) getNumberProcs() int {
	return p.config.GetInt("numprocs", 1)
}

func (p *Process) SendProcessStdin(chars string) error {
	if p.stdin != nil {
		_, err := p.stdin.Write([]byte(chars))
		return err
	}
	return fmt.Errorf("NO_FILE")
}

// check if the process should be
func (p *Process) isAutoRestart() bool {
	autoRestart := p.config.GetString("autorestart", "unexpected")

	if autoRestart == "false" {
		return false
	} else if autoRestart == "true" {
		return true
	} else {
		p.lock.RLock()
		defer p.lock.RUnlock()
		if p.cmd != nil && p.cmd.ProcessState != nil {
			exitCode, err := p.getExitCode()
			//If unexpected, the process will be restarted when the program exits
			//with an exit code that is not one of the exit codes associated with
			//this process’ configuration (see exitcodes).
			return err == nil && !p.inExitCodes(exitCode)
		}
	}

	return false
}

func (p *Process) inExitCodes(exitCode int) bool {
	for _, code := range p.getExitCodes() {
		if code == exitCode {
			return true
		}
	}
	return false
}

func (p *Process) getExitCode() (int, error) {
	if p.cmd.ProcessState == nil {
		return -1, fmt.Errorf("no exit code")
	}
	if status, ok := p.cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
		return status.ExitStatus(), nil
	}

	return -1, fmt.Errorf("no exit code")
}

func (p *Process) getExitCodes() []int {
	strExitCodes := strings.Split(p.config.GetString("exitcodes", "0,2"), ",")
	result := make([]int, 0)
	for _, val := range strExitCodes {
		i, err := strconv.Atoi(val)
		if err == nil {
			result = append(result, i)
		}
	}
	return result
}

// check if the process is running or not
//
func (p *Process) isRunning() bool {
	if p.cmd != nil && p.cmd.ProcessState != nil {
		if runtime.GOOS == "windows" {
			proc, err := os.FindProcess(p.cmd.Process.Pid)
			return proc != nil && err == nil
		} else {
			return p.cmd.Process.Signal(syscall.Signal(0)) == nil
		}
	}

	return false
}

// create Command object for the program
func (p *Process) createProgramCommand() error {
	args, err := parseCommand(p.config.GetStringExpression("command", ""))

	if err != nil {
		return err
	}
	p.cmd = exec.Command(args[0])
	if len(args) > 1 {
		p.cmd.Args = args
	}
	p.cmd.SysProcAttr = &syscall.SysProcAttr{}
	if p.setUser() != nil {
		log.WithFields(log.Fields{"user": p.config.GetString("user", "")}).Error("fail to run as user")
		return fmt.Errorf("fail to set user")
	}
	set_deathsig(p.cmd.SysProcAttr, p.exitKill)
	p.setEnv()
	p.setDir()
	// If setLog is invoked, supervisord will send syscall.SIGPIPE
	// to its child processes when it exits.
	p.setLog()

	p.stdin, _ = p.cmd.StdinPipe()
	return nil
}

// wait for the started program exit
func (p *Process) waitForExit(startSecs int64) {
	err := p.cmd.Wait()
	if err != nil {
		log.WithFields(log.Fields{"program": p.GetName()}).Info("fail to wait for program exit")
	} else if p.cmd.ProcessState != nil {
		log.WithFields(log.Fields{"program": p.GetName()}).Infof("program stopped with status:%v", p.cmd.ProcessState)
	} else {
		log.WithFields(log.Fields{"program": p.GetName()}).Info("program stopped")
	}

	p.lock.Lock()
	defer p.lock.Unlock()

	p.stopTime = time.Now()
	if p.stopTime.Unix()-p.startTime.Unix() < startSecs {
		p.changeStateTo(BACKOFF)
	} else {
		p.changeStateTo(EXITED)
	}
}

// fail to start the program
func (p *Process) failToStartProgram(reason string, finishCb func()) {
	log.WithFields(log.Fields{"program": p.GetName()}).Errorf(reason)
	p.changeStateTo(FATAL)
	finishCb()
}

func (p *Process) monitorProgramIsRunning(endTime time.Time, monitorExited *int32) {
	// if time is not expired
	for time.Now().Before(endTime) {
		time.Sleep(time.Duration(100) * time.Millisecond)
		if atomic.LoadInt32(p.retryTimes) > p.getStartRetries() {
			break
		}
	}
	atomic.StoreInt32(monitorExited, 1)

	state := p.GetState()
	p.lock.Lock()
	defer p.lock.Unlock()
	if state == STARTING {
		log.WithFields(log.Fields{"program": p.GetName()}).Info("success to start program (monitor)")
		p.changeStateTo(RUNNING)
	} else {
		log.WithFields(log.Fields{"program": p.GetName()}).Info("fail to start program (monitor)")
		p.changeStateTo(FATAL)
	}
}

func (p *Process) run(finishCb func()) {
	p.lock.Lock()
	defer p.lock.Unlock()

	// check if the program is in running state
	if p.isRunning() {
		log.WithFields(log.Fields{"program": p.GetName()}).Info("Don't start program because it is running (run)")
		finishCb()
		return
	}

	p.startTime = time.Now()
	p.changeStateTo(STARTING)
	atomic.StoreInt32(p.retryTimes, 0)
	startSecs := int64(p.config.GetInt("startsecs", 1))
	endTime := time.Now().Add(time.Duration(startSecs) * time.Second)
	monitorExited := int32(1)
	var once sync.Once

	// finishCb can be only called one time
	finishCbWrapper := func() {
		once.Do(finishCb)
	}
	//process is not expired and not stopped by user
	for !p.stopByUser {
		atomic.StoreInt32(&monitorExited, 1)
		// if the start-up time reaches startSecs
		if startSecs > 0 && time.Now().After(endTime) {
			p.failToStartProgram(fmt.Sprintf("fail to start program because the start-up time reaches the startsecs %d (run)", startSecs), finishCbWrapper)
			break
		}
		// The number of serial failure attempts that supervisord will allow when attempting to
		// start the program before giving up and putting the process into an FATAL state
		// first start time is not the retry time
		if atomic.LoadInt32(p.retryTimes) > 0 && atomic.LoadInt32(p.retryTimes) > p.getStartRetries() {
			p.failToStartProgram(fmt.Sprintf("fail to start program because retry times is greater than %d (run)", p.getStartRetries()), finishCbWrapper)
			break
		}
		atomic.AddInt32(p.retryTimes, 1)
		err := p.createProgramCommand()
		if err != nil {
			p.failToStartProgram("fail to create program", finishCbWrapper)
			break
		}

		err = p.cmd.Start()
		if err != nil {
			p.failToStartProgram(fmt.Sprintf("fail to start program with error:%v (run)", err), finishCbWrapper)
			continue
		}
		if p.StdoutLog != nil {
			p.StdoutLog.SetPid(p.cmd.Process.Pid)
		}
		if p.StderrLog != nil {
			p.StderrLog.SetPid(p.cmd.Process.Pid)
		}
		p.inStart.Store(false)
		p.pid.Store(int64(p.cmd.Process.Pid))
		//Set startsec to 0 to indicate that the program needn't stay
		//running for any particular amount of time.
		if startSecs <= 0 {
			log.WithFields(log.Fields{"program": p.GetName()}).Info("success to start program (run)")
			p.changeStateTo(RUNNING)
			go finishCbWrapper()
		} else if atomic.LoadInt32(p.retryTimes) == 1 { // only start monitor for first try
			atomic.StoreInt32(&monitorExited, 0)
			go func() {
				p.monitorProgramIsRunning(endTime, &monitorExited)
				finishCbWrapper()
			}()
		}
		log.WithFields(log.Fields{"program": p.GetName()}).Debug("wait program exit (run)")
		p.lock.Unlock() // ???????????? waitForExit ??????
		p.waitForExit(startSecs)
		p.lock.Lock()
		// fmt.Printf("ps %s exit now!\n", p.config.GetProgramName())
	}
	// wait for monitor thread exit
	for atomic.LoadInt32(&monitorExited) == 0 {
		time.Sleep(time.Duration(100) * time.Millisecond)
	}
}

func (p *Process) changeStateTo(procState ProcessState) {
	// 20190130 do not use event.
	// state := p.GetState()
	// if p.config.IsProgram() {
	// 	progName := p.config.GetProgramName()
	// 	groupName := p.config.GetGroupName()
	// 	if procState == STARTING {
	// 		events.EmitEvent(events.CreateProcessStartingEvent(progName, groupName, state.String(), int(atomic.LoadInt32(p.retryTimes))))
	// 	} else if procState == RUNNING {
	// 		events.EmitEvent(events.CreateProcessRunningEvent(progName, groupName, state.String(), p.cmd.Process.Pid))
	// 	} else if procState == BACKOFF {
	// 		events.EmitEvent(events.CreateProcessBackoffEvent(progName, groupName, state.String(), int(atomic.LoadInt32(p.retryTimes))))
	// 	} else if procState == STOPPING {
	// 		events.EmitEvent(events.CreateProcessStoppingEvent(progName, groupName, state.String(), p.cmd.Process.Pid))
	// 	} else if procState == EXITED {
	// 		exitCode, err := p.getExitCode()
	// 		expected := 0
	// 		if err == nil && p.inExitCodes(exitCode) {
	// 			expected = 1
	// 		}
	// 		events.EmitEvent(events.CreateProcessExitedEvent(progName, groupName, state.String(), expected, p.cmd.Process.Pid))
	// 	} else if procState == FATAL {
	// 		events.EmitEvent(events.CreateProcessFatalEvent(progName, groupName, state.String()))
	// 	} else if procState == STOPPED {
	// 		events.EmitEvent(events.CreateProcessStoppedEvent(progName, groupName, state.String(), p.cmd.Process.Pid))
	// 	} else if procState == UNKNOWN {
	// 		events.EmitEvent(events.CreateProcessUnknownEvent(progName, groupName, state.String()))
	// 	}
	// }

	// p.state = procState
	p.state.Store(int64(procState))
	if p.updateCb != nil {
		p.updateCb(p)
	}
}

// send signal to the process
//
// Args:
//   sig - the signal to the process
//   sigChildren - true: send the signal to the process and its children proess
//
func (p *Process) Signal(sig os.Signal, sigChildren bool) error {
	p.lock.RLock()
	defer p.lock.RUnlock()

	return p.sendSignal(sig, sigChildren)
}

// send signal to the process
//
// Args:
//    sig - the signal to be sent
//    sigChildren - true if the signal also need to be sent to children process
//
func (p *Process) sendSignal(sig os.Signal, sigChildren bool) error {
	if p.cmd != nil && p.cmd.Process != nil {
		err := signals.Kill(p.cmd.Process, sig, sigChildren)
		return err
	}

	return fmt.Errorf("process is not running")
}

func (p *Process) setEnv() {
	env := p.config.GetEnv("environment")
	if len(env) != 0 {
		p.cmd.Env = append(os.Environ(), env...)
	} else {
		p.cmd.Env = os.Environ()
	}
}

func (p *Process) setDir() {
	dir := p.config.GetStringExpression("directory", "")
	if dir != "" {
		p.cmd.Dir = dir
	}
}

func (p *Process) setLog() {
	if p.config.IsProgram() {
		p.StdoutLog = p.createLogger(p.GetStdoutLogfile(),
			int64(p.config.GetBytes("stdout_logfile_maxbytes", 50*1024*1024)),
			p.config.GetInt("stdout_logfile_backups", 10),
			p.createStdoutLogEventEmitter())
		capture_bytes := p.config.GetBytes("stdout_capture_maxbytes", 0)
		if capture_bytes > 0 {
			log.WithFields(log.Fields{"program": p.config.GetProgramName()}).Info("capture stdout process communication")
			p.StdoutLog = logger.NewLogCaptureLogger(p.StdoutLog,
				capture_bytes,
				"PROCESS_COMMUNICATION_STDOUT",
				p.GetName(),
				p.GetGroup())
		}

		p.cmd.Stdout = p.StdoutLog

		if p.config.GetBool("redirect_stderr", false) {
			p.StderrLog = p.StdoutLog
		} else {
			p.StderrLog = p.createLogger(p.GetStderrLogfile(),
				int64(p.config.GetBytes("stderr_logfile_maxbytes", 50*1024*1024)),
				p.config.GetInt("stderr_logfile_backups", 10),
				p.createStderrLogEventEmitter())
		}

		capture_bytes = p.config.GetBytes("stderr_capture_maxbytes", 0)

		if capture_bytes > 0 {
			log.WithFields(log.Fields{"program": p.config.GetProgramName()}).Info("capture stderr process communication")
			p.StderrLog = logger.NewLogCaptureLogger(p.StdoutLog,
				capture_bytes,
				"PROCESS_COMMUNICATION_STDERR",
				p.GetName(),
				p.GetGroup(),
			)
		}

		p.cmd.Stderr = p.StderrLog
	} else if p.config.IsEventListener() {
		in, err := p.cmd.StdoutPipe()
		if err != nil {
			log.WithFields(log.Fields{"eventListener": p.config.GetEventListenerName()}).Error("fail to get stdin")
			return
		}
		out, err := p.cmd.StdinPipe()
		if err != nil {
			log.WithFields(log.Fields{"eventListener": p.config.GetEventListenerName()}).Error("fail to get stdout")
			return
		}
		events := strings.Split(p.config.GetString("events", ""), ",")
		for i, event := range events {
			events[i] = strings.TrimSpace(event)
		}
		p.cmd.Stderr = os.Stderr

		p.registerEventListener(p.config.GetEventListenerName(),
			events,
			in,
			out)
	}
}

func (p *Process) createStdoutLogEventEmitter() logger.LogEventEmitter {
	if p.config.GetBytes("stdout_capture_maxbytes", 0) <= 0 && p.config.GetBool("stdout_events_enabled", false) {
		return logger.NewStdoutLogEventEmitter(p.config.GetProgramName(), p.config.GetGroupName(), func() int {
			return p.GetPid()
		})
	}
	return logger.NewNullLogEventEmitter()
}

func (p *Process) createStderrLogEventEmitter() logger.LogEventEmitter {
	if p.config.GetBytes("stderr_capture_maxbytes", 0) <= 0 && p.config.GetBool("stderr_events_enabled", false) {
		return logger.NewStdoutLogEventEmitter(p.config.GetProgramName(), p.config.GetGroupName(), func() int {
			return p.GetPid()
		})
	}
	return logger.NewNullLogEventEmitter()
}

func (p *Process) registerEventListener(eventListenerName string,
	_events []string,
	stdin io.Reader,
	stdout io.Writer) {
	eventListener := events.NewEventListener(eventListenerName,
		p.supervisor_id,
		stdin,
		stdout,
		p.config.GetInt("buffer_size", 100))
	events.RegisterEventListener(eventListenerName, _events, eventListener)
}

func (p *Process) unregisterEventListener(eventListenerName string) {
	events.UnregisterEventListener(eventListenerName)
}

func (p *Process) createLogger(logFile string, maxBytes int64, backups int, logEventEmitter logger.LogEventEmitter) logger.Logger {
	return logger.NewLogger(p.GetName(), logFile, logger.NewNullLocker(), maxBytes, backups, logEventEmitter)
}

func (p *Process) setUser() error {
	userName := p.config.GetString("user", "")
	if len(userName) == 0 {
		return nil
	}

	//check if group is provided
	pos := strings.Index(userName, ":")
	groupName := ""
	if pos != -1 {
		groupName = userName[pos+1:]
		userName = userName[0:pos]
	}
	u, err := user.Lookup(userName)
	if err != nil {
		return err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil && groupName == "" {
		return err
	}
	if groupName != "" {
		g, err := user.LookupGroup(groupName)
		if err != nil {
			return err
		}
		gid, err = strconv.ParseUint(g.Gid, 10, 32)
		if err != nil {
			return err
		}
	}
	set_user_id(p.cmd.SysProcAttr, uint32(uid), uint32(gid))
	return nil
}

//send signal to process to stop it
func (p *Process) Stop(wait bool) {
	p.lock.Lock()
	p.stopByUser = true
	p.lock.Unlock()
	log.WithFields(log.Fields{"program": p.GetName()}).Info("stop the process program")
	if p.GetPid() == 0 {
		return
	}

	sigs := strings.Fields(p.config.GetString("stopsignal", ""))
	waitsecs := time.Duration(p.config.GetInt("stopwaitsecs", 10)) * time.Second
	stopasgroup := p.config.GetBool("stopasgroup", false)
	killasgroup := p.config.GetBool("killasgroup", stopasgroup)
	if stopasgroup && !killasgroup {
		log.WithFields(log.Fields{"program": p.GetName()}).Error("Cannot set process stopasgroup=true and killasgroup=false")
	}

	go func() {
		stopped := false
		for i := 0; i < len(sigs) && !stopped; i++ {
			// send signal to process
			sig, err := signals.ToSignal(sigs[i])
			if err != nil {
				continue
			}
			log.WithFields(log.Fields{"program": p.GetName(), "signal": sigs[i], "pid": p.GetPid()}).Info("send stop signal to process program")
			p.Signal(sig, stopasgroup)
			endTime := time.Now().Add(waitsecs)
			//wait at most "stopwaitsecs" seconds for one signal
			for endTime.After(time.Now()) {
				//if it already exits
				state := p.GetState()
				if state != STARTING && state != RUNNING && state != STOPPING {
					stopped = true
					break
				}
				time.Sleep(1 * time.Second)
			}
		}
		if !stopped {
			log.WithFields(log.Fields{"program": p.GetName(), "signal": "KILL", "pid": p.GetPid()}).Info("force to kill the process program")
			p.Signal(syscall.SIGKILL, killasgroup)
		}
	}()
	if wait {
		for {
			// if the program exits
			// p.lock.RLock()
			state := p.GetState()
			if state != STARTING && state != RUNNING && state != STOPPING {
				// p.lock.RUnlock()
				break
			}
			// p.lock.RUnlock()
			time.Sleep(1 * time.Second)
		}
	}
}

func (p *Process) GetStatus() string {
	if p.inStart.Load() {
		return "starting"
	}

	if p.cmd == nil || p.cmd.ProcessState == nil {
		return "starting"
	}

	if p.cmd.ProcessState.Exited() {
		return p.cmd.ProcessState.String()
	}

	return "running"
}
