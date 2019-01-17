package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlexStocks/supervisord/config"
	"github.com/AlexStocks/supervisord/events"
	"github.com/AlexStocks/supervisord/faults"
	"github.com/AlexStocks/supervisord/logger"
	"github.com/AlexStocks/supervisord/process"
	"github.com/AlexStocks/supervisord/signals"
	"github.com/AlexStocks/supervisord/types"
	"github.com/AlexStocks/supervisord/util"

	log "github.com/sirupsen/logrus"
)

const (
	SUPERVISOR_VERSION   = "3.0"
	MaxSleepTimeInterval = 2e9
)

type Supervisor struct {
	config     *config.Config
	procMgr    *process.ProcessManager
	xmlRPC     *XmlRPC
	logger     logger.Logger
	restarting bool
}

type StartProcessArgs struct {
	Name string
	Wait bool `default:"true"`
}

type ProcessStdin struct {
	Name  string
	Chars string
}

type RemoteCommEvent struct {
	Type string
	Data string
}

type StateInfo struct {
	Statecode int    `xml:"statecode"`
	Statename string `xml:"statename"`
}

type RpcTaskResult struct {
	Name        string `xml:"name"`
	Group       string `xml:"group"`
	Status      int    `xml:"status"`
	Description string `xml:"description"`
}

type LogReadInfo struct {
	Offset int
	Length int
}

type ProcessLogReadInfo struct {
	Name   string
	Offset int
	Length int
}

type ProcessTailLog struct {
	LogData  string
	Offset   int64
	Overflow bool
}

func NewSupervisor(configFile string) *Supervisor {
	s := &Supervisor{
		config:     config.NewConfig(configFile),
		procMgr:    process.NewProcessManager(),
		xmlRPC:     NewXmlRPC(),
		restarting: false,
	}

	return s
}

func (s *Supervisor) GetConfig() *config.Config {
	return s.config
}

func (s *Supervisor) GetVersion(r *http.Request, args *struct{}, reply *struct{ Version string }) error {
	reply.Version = SUPERVISOR_VERSION
	return nil
}

func (s *Supervisor) GetSupervisorVersion(r *http.Request, args *struct{}, reply *struct{ Version string }) error {
	reply.Version = SUPERVISOR_VERSION
	return nil
}

func (s *Supervisor) GetIdentification(r *http.Request, args *struct{}, reply *struct{ Id string }) error {
	reply.Id = s.GetSupervisorId()
	return nil
}

func (s *Supervisor) GetSupervisorId() string {
	entry, ok := s.config.GetSupervisord()
	if !ok {
		return "supervisor"
	}
	return entry.GetString("identifier", "supervisor")
}

func (s *Supervisor) GetState(r *http.Request, args *struct{}, reply *struct{ StateInfo StateInfo }) error {
	//statecode     statename
	//=======================
	// 2            FATAL
	// 1            RUNNING
	// 0            RESTARTING
	// -1           SHUTDOWN
	log.Debug("Get state")
	reply.StateInfo.Statecode = 1
	reply.StateInfo.Statename = "RUNNING"
	return nil
}

// Get all the name of programs
//
// Return the name of all the programs
func (s *Supervisor) GetPrograms() []string {
	return s.config.GetProgramNames()
}
func (s *Supervisor) GetPID(r *http.Request, args *struct{}, reply *struct{ Pid int }) error {
	reply.Pid = os.Getpid()
	return nil
}

func (s *Supervisor) ReadLog(r *http.Request, args *LogReadInfo, reply *struct{ Log string }) error {
	data, err := s.logger.ReadLog(int64(args.Offset), int64(args.Length))
	reply.Log = data
	return err
}

func (s *Supervisor) ClearLog(r *http.Request, args *struct{}, reply *struct{ Ret bool }) error {
	err := s.logger.ClearAllLogFile()
	reply.Ret = err == nil
	return err
}

func (s *Supervisor) Shutdown(r *http.Request, args *struct{}, reply *struct{ Ret bool }) error {
	reply.Ret = true
	log.Info("received rpc request to stop all processes & exit")
	s.procMgr.StopAllProcesses(false, true)
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	return nil
}

func (s *Supervisor) Restart(r *http.Request, args *struct{}, reply *struct{ Ret bool }) error {
	log.Info("Receive instruction to restart")
	s.restarting = true
	reply.Ret = true
	return nil
}

func (s *Supervisor) IsRestarting() bool {
	return s.restarting
}

func (s *Supervisor) GetAllProcessInfo(r *http.Request, args *struct{}, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	reply.AllProcessInfo = make([]types.ProcessInfo, 0)
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		procInfo := proc.TypeProcessInfo()
		reply.AllProcessInfo = append(reply.AllProcessInfo, procInfo)
	})
	if _, arr := s.procMgr.GetActivePrestartProcess(); len(arr) != 0 {
		for _, info := range arr {
			reply.AllProcessInfo = append(reply.AllProcessInfo, info.TypeProcessInfo())
		}
	}
	types.SortProcessInfos(reply.AllProcessInfo)

	return nil
}

func (s *Supervisor) GetAllProcsProcessInfo(r *http.Request, args *struct{}, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	reply.AllProcessInfo = make([]types.ProcessInfo, 0)
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		procInfo := proc.TypeProcessInfo()
		reply.AllProcessInfo = append(reply.AllProcessInfo, procInfo)
	})
	types.SortProcessInfos(reply.AllProcessInfo)

	return nil
}

func (s *Supervisor) GetAllInfomapProcessInfo(r *http.Request, args *struct{}, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	reply.AllProcessInfo = make([]types.ProcessInfo, 0)
	if _, arr := s.procMgr.GetAllInfomapProcess(); len(arr) != 0 {
		for _, info := range arr {
			reply.AllProcessInfo = append(reply.AllProcessInfo, info.TypeProcessInfo())
		}
	}
	types.SortProcessInfos(reply.AllProcessInfo)

	return nil
}

func (s *Supervisor) GetPrestartProcessInfo(r *http.Request, args *struct{}, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	reply.AllProcessInfo = make([]types.ProcessInfo, 0)
	if _, arr := s.procMgr.GetActivePrestartProcess(); len(arr) != 0 {
		for _, info := range arr {
			reply.AllProcessInfo = append(reply.AllProcessInfo, info.TypeProcessInfo())
		}
	}
	types.SortProcessInfos(reply.AllProcessInfo)

	return nil
}

func (s *Supervisor) GetProcessInfo(r *http.Request, args *struct{ Name string }, reply *struct{ ProcInfo types.ProcessInfo }) error {
	log.Info("Get process info of: ", args.Name)
	proc := s.procMgr.Find(args.Name)
	if proc != nil {
		reply.ProcInfo = proc.TypeProcessInfo()
	} else {
		info := s.procMgr.FindProcessInfo(args.Name)
		if info == nil {
			return fmt.Errorf("no process named %s", args.Name)
		}
		reply.ProcInfo = info.TypeProcessInfo()
	}

	return nil
}

func (s *Supervisor) ListMethods(r *http.Request, args *struct{}, reply *struct{ Methods []string }) error {
	reply.Methods = xmlCodec.Methods()

	return nil
}

func (s *Supervisor) StartProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		psInfo := s.procMgr.FindProcessInfo(args.Name)
		if psInfo != nil {
			proc = s.procMgr.CreateProcess(s.GetSupervisorId(), psInfo.ConfigEntry())
			if proc == nil {
				return fmt.Errorf("fail to create process{config:%#v}", psInfo.ConfigEntry())
			}
		} else {
			proc = s.startProcessByConfig(args.Name)
			if proc == nil {
				return fmt.Errorf("fail to find process %s", args.Name)
			}
		}
	}
	proc.Start(args.Wait, func(p *process.Process) {
		s.procMgr.UpdateProcessInfo(proc)
	})
	reply.Success = true
	return nil
}

func (s *Supervisor) startProcessByConfig(program string) *process.Process {
	entries := s.config.GetPrograms()
	for j := range entries {
		if entries[j].GetProgramName() == strings.TrimSpace(program) {
			return s.procMgr.CreateProcess(s.GetSupervisorId(), entries[j])
		}
	}

	return nil
}

func (s *Supervisor) StartAllProcesses(r *http.Request, args *struct {
	Wait bool `default:"true"`
}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		proc.Start(args.Wait, func(p *process.Process) {
			s.procMgr.UpdateProcessInfo(p)
		})
		processInfo := proc.TypeProcessInfo()
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        processInfo.Name + " Started ",
			Group:       processInfo.Group,
			Status:      faults.SUCCESS,
			Description: "OK",
		})
		s.procMgr.AddProc(proc)
	})
	_, activePrestartProcesses := s.procMgr.GetActivePrestartProcess()
	for _, info := range activePrestartProcesses {
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        info.Program + " Started ",
			Group:       info.ConfigEntry().Group,
			Status:      faults.SUCCESS,
			Description: "OK",
		})
	}
	entries := s.config.GetPrograms()
	for i := range entries {
		flag := false
		for j := range reply.RpcTaskResults {
			if entries[i].GetProgramName() == strings.TrimSpace(reply.RpcTaskResults[j].Name) {
				flag = true
				break
			}
		}
		if !flag {
			proc := s.procMgr.CreateProcess(s.GetSupervisorId(), entries[i])
			if proc != nil {
				proc.Start(true, func(p *process.Process) {
					s.procMgr.UpdateProcessInfo(proc)
				})
				processInfo := proc.TypeProcessInfo()
				reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
					Name:        processInfo.Name + " Started ",
					Group:       processInfo.Group,
					Status:      faults.SUCCESS,
					Description: "OK",
				})
			}
		}
	}
	return nil
}

func (s *Supervisor) StartProcessGroup(r *http.Request, args *StartProcessArgs, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	log.WithFields(log.Fields{"group": args.Name}).Info("start process group")
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		if proc.GetGroup() == args.Name {
			proc.Start(args.Wait, nil)
			reply.AllProcessInfo = append(reply.AllProcessInfo, proc.TypeProcessInfo())
		}
	})

	return nil
}

func (s *Supervisor) StopProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	log.WithFields(log.Fields{"program": args.Name}).Info("stop process")
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		psInfo := s.procMgr.FindProcessInfo(args.Name)
		if psInfo == nil {
			return fmt.Errorf("fail to find process %s", args.Name)
		}
		psInfo.Stop(args.Wait)
	} else {
		proc.Stop(args.Wait)
	}
	// Do not invoke s.procMgr.Remove func here. s.procMgr.procs stores all subprocesses state
	s.procMgr.RemoveProcessInfo(args.Name)
	reply.Success = true
	return nil
}

func (s *Supervisor) RestartProcess(r *http.Request, args *StartProcessArgs, reply *struct{ StopSuccess, StartSuccess bool }) error {
	log.WithFields(log.Fields{"program": args.Name}).Info("restart process")

	var stopReply struct{ Success bool }
	err := s.StopProcess(r, args, &stopReply)
	if err != nil {
		return err
	}
	reply.StopSuccess = true

	var startReply struct{ Success bool }
	err = s.StartProcess(r, args, &startReply)
	if err != nil {
		return err
	}
	reply.StartSuccess = true

	return nil
}

func (s *Supervisor) StopProcessGroup(r *http.Request, args *StartProcessArgs, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	log.WithFields(log.Fields{"group": args.Name}).Info("stop process group")
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		if proc.GetGroup() == args.Name {
			proc.Stop(args.Wait)
			reply.AllProcessInfo = append(reply.AllProcessInfo, proc.TypeProcessInfo())
		}
	})
	return nil
}

func (s *Supervisor) StopAllProcesses(r *http.Request, args *struct {
	Wait bool `default:"true"`
}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {
	s.procMgr.KillAllProcesses(func(info process.ProcessInfo) {
		var group string
		if entry := info.ConfigEntry(); entry != nil {
			group = entry.Group
		}
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        info.Program + " Stopped ",
			Group:       group,
			Status:      faults.SUCCESS,
			Description: "OK",
		})
	})

	return nil
}

func (s *Supervisor) RestartAllProcesses(r *http.Request, args *struct {
	Wait bool `default:"true"`
}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {
	err := s.StopAllProcesses(r, args, reply)
	// fmt.Printf("StopAllProcesses reply: %#v, err:%#v\n", reply.stopReply, err)
	if err == nil {
		err = s.StartAllProcesses(r, args, reply)
		// fmt.Printf("StartAllProcesses reply: %#v, err:%#v\n", reply.startReply, err)
	}
	reply = nil

	return err
}

func (s *Supervisor) SignalProcess(r *http.Request, args *types.ProcessSignal, reply *struct{ Success bool }) error {
	reply.Success = true

	sig, err := signals.ToSignal(args.Signal)
	if err != nil {
		return nil
	}

	proc := s.procMgr.Find(args.Name)
	if proc != nil {
		proc.Signal(sig, false)
	} else {
		info := s.procMgr.FindProcessInfo(args.Name)
		if info == nil {
			reply.Success = false
			return fmt.Errorf("No process named %s", args.Name)
		}
		signals.KillPid(int(info.PID), sig, false)
	}

	return nil
}

func (s *Supervisor) SignalProcessGroup(r *http.Request, args *types.ProcessSignal, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		if proc.GetGroup() == args.Name {
			sig, err := signals.ToSignal(args.Signal)
			if err == nil {
				proc.Signal(sig, false)
			}
		}
	})

	s.procMgr.ForEachProcess(func(proc *process.Process) {
		if proc.GetGroup() == args.Name {
			reply.AllProcessInfo = append(reply.AllProcessInfo, proc.TypeProcessInfo())
		}
	})
	return nil
}

func (s *Supervisor) SignalAllProcesses(r *http.Request, args *types.ProcessSignal, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	sig, err := signals.ToSignal(args.Signal)
	if err != nil {
		return nil
	}

	s.procMgr.ForEachProcess(func(proc *process.Process) {
		proc.Signal(sig, false)
		reply.AllProcessInfo = append(reply.AllProcessInfo, proc.TypeProcessInfo())
	})

	if _, arr := s.procMgr.GetActivePrestartProcess(); len(arr) != 0 {
		for _, info := range arr {
			signals.KillPid(int(info.PID), sig, false)
			reply.AllProcessInfo = append(reply.AllProcessInfo, info.TypeProcessInfo())
		}
	}

	return nil
}

func (s *Supervisor) SendProcessStdin(r *http.Request, args *ProcessStdin, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		log.WithFields(log.Fields{"program": args.Name}).Error("program does not exist")
		return fmt.Errorf("NOT_RUNNING")
	}
	if proc.GetState() != process.RUNNING {
		log.WithFields(log.Fields{"program": args.Name}).Error("program does not run")
		return fmt.Errorf("NOT_RUNNING")
	}
	err := proc.SendProcessStdin(args.Chars)
	if err == nil {
		reply.Success = true
	} else {
		reply.Success = false
	}
	return err
}

func (s *Supervisor) SendRemoteCommEvent(r *http.Request, args *RemoteCommEvent, reply *struct{ Success bool }) error {
	events.EmitEvent(events.NewRemoteCommunicationEvent(args.Type, args.Data))
	reply.Success = true
	return nil
}

// return err, addedGroup, changedGroup, removedGroup
//
//
func (s *Supervisor) Reload(startup bool) (error, []string, []string, []string) {
	//get the previous loaded programs
	prevPrograms := s.config.GetProgramNames()
	prevProgGroup := s.config.ProgramGroup.Clone()

	loaded_programs, err := s.config.Load()
	if err == nil {
		s.startLogger()
		supervisordConf, flag := s.config.GetSupervisord()
		if flag {
			s.procMgr.UpdateConfig(supervisordConf)
			// get previous ps
			s.procMgr.ValidateStartPs()
		}
		s.startEventListeners()
		s.createPrograms(prevPrograms) // create Process
		s.startHttpServer()
		s.startAutoStartPrograms() // start Process: process.Process.Start -> process.Process.run -> process.Process.waitForExit
		if startup {
			go s.MonitorPrestartProcess() // k
		}
	}

	removedPrograms := util.Sub(prevPrograms, loaded_programs)
	for _, removedProg := range removedPrograms {
		// log.WithFields(log.Fields{"program": removedProg}).Info(
		//	"the program is removed and will be stopped")
		s.config.RemoveProgram(removedProg)
		proc := s.procMgr.Remove(removedProg)
		if proc != nil {
			proc.Stop(false)
			log.WithFields(log.Fields{"program": removedProg, "pid": proc.GetPid()}).Info(
				"the program is removed and will be stopped")
		}
		info := s.procMgr.RemoveProcessInfo(removedProg)
		if info.PID != 0 {
			info.Stop(false)
			log.WithFields(log.Fields{"prestart program": removedProg, "pid": info.PID}).Info(
				"the program is removed and will be stopped")
		}
	}

	addedGroup, changedGroup, removedGroup := s.config.ProgramGroup.Sub(prevProgGroup)
	return err, addedGroup, changedGroup, removedGroup
}

func (s *Supervisor) update(r *http.Request, args *struct{ Process string }, reply *types.UpdateResult) error {
	//get the previous loaded programs
	prevPrograms := s.config.GetProgramNames()
	prevProgGroup := s.config.ProgramGroup.Clone()
	loaded_programs, err := s.config.Load()
	removedPrograms := util.Sub(prevPrograms, loaded_programs)
	for _, removedProg := range removedPrograms {
		if removedProg == args.Process || "___all___" == args.Process {
			s.config.RemoveProgram(removedProg)

			// Bugfix 20190118: procMgr.Remove 函数会调用下面的 procMgr.RemoveProcessInfo 函数，
			// 所以把 infoMap 中相应进程的关闭放在上面
			info := s.procMgr.RemoveProcessInfo(removedProg)
			if info.PID != 0 {
				info.Stop(false)
				fmt.Printf("start to stop ps %s by info\n", removedProg)
				s.config.RemoveProgram(removedProg)
				log.WithFields(log.Fields{"prestart program": removedProg, "pid": info.PID}).Info(
					"the program is removed and will be stopped")
			}

			proc := s.procMgr.Remove(removedProg)
			if proc != nil {
				proc.Stop(false)
				s.config.RemoveProgram(removedProg)
				log.WithFields(log.Fields{"program": removedProg, "pid": proc.GetPid()}).Info(
					"the program is removed and will be stopped")
			}
		}
	}

	addedPrograms := util.Sub(loaded_programs, prevPrograms)
	for _, addedProgram := range addedPrograms {
		if addedProgram == args.Process || "___all___" == args.Process {
			entries := s.config.GetPrograms()
			startFlag := false
			for j := range entries {
				if entries[j].GetProgramName() == strings.TrimSpace(addedProgram) {
					startFlag = true
					proc := s.procMgr.CreateProcess(s.GetSupervisorId(), entries[j])
					if proc != nil {
						proc.Start(true, func(p *process.Process) {
							s.procMgr.UpdateProcessInfo(proc)
						})
					}
				}
			}
			if !startFlag {
				log.Warn("can not find config of program ", addedProgram)
			}
		}
	}

	reply.AddedGroup, reply.ChangedGroup, reply.RemovedGroup = s.config.ProgramGroup.Sub(prevProgGroup)

	return err
}

func (s *Supervisor) Update(r *http.Request, args *struct{ Process string }, reply *types.UpdateResult) error {
	return s.update(r, args, reply)
}

func (s *Supervisor) UpdateAll(r *http.Request, args *struct{}, reply *types.UpdateResult) error {
	return s.update(r, &struct{ Process string }{Process: "___all___"}, reply)
}

func (s *Supervisor) MonitorPrestartProcess() {
	sleepInterval := 1e9
	for {
		num, arr := s.procMgr.GetDeadPrestartProcess()
		if num == 0 {
			break
		}
		if len(arr) == 0 {
			time.Sleep(time.Duration(sleepInterval))
			sleepInterval *= 2
			if MaxSleepTimeInterval < sleepInterval {
				sleepInterval = MaxSleepTimeInterval
			}
			continue
		}
		entries := s.config.GetPrograms()
		for i := range arr {
			flag := false
			for j := range entries {
				if entries[j].GetProgramName() == strings.TrimSpace(arr[i]) {
					flag = true
					proc := s.procMgr.CreateProcess(s.GetSupervisorId(), entries[j])
					if proc != nil {
						proc.Start(true, func(p *process.Process) {
							s.procMgr.UpdateProcessInfo(proc)
						})
					}
				}
			}
			if !flag {
				log.Warn("can not find config of program ", arr[i])
			}
		}
		sleepInterval = 1e9
		time.Sleep(time.Duration(sleepInterval))
	}
}

func (s *Supervisor) WaitForExit() {
	for {
		// log.Info("wait for exit")
		if s.IsRestarting() {
			log.Info("start to stop all processes and exit")
			s.procMgr.StopAllProcesses(false, true)
			break
		}
		time.Sleep(10 * time.Second)
	}
}

func (s *Supervisor) createPrograms(prevPrograms []string) {
	programs := s.config.GetProgramNames()
	for _, entry := range s.config.GetPrograms() {
		s.procMgr.CreateProcess(s.GetSupervisorId(), entry)
	}
	removedPrograms := util.Sub(prevPrograms, programs)
	for _, p := range removedPrograms {
		s.procMgr.Remove(p)
	}
}

func (s *Supervisor) startAutoStartPrograms() {
	s.procMgr.StartAutoStartPrograms()
}

func (s *Supervisor) startEventListeners() {
	eventListeners := s.config.GetEventListeners()
	for _, entry := range eventListeners {
		if proc := s.procMgr.CreateProcess(s.GetSupervisorId(), entry); proc != nil {
			proc.Start(false, func(p *process.Process) {
				s.procMgr.UpdateProcessInfo(p)
			})
		}
	}

	if len(eventListeners) > 0 {
		time.Sleep(1 * time.Second)
	}
}

func (s *Supervisor) startHttpServer() {
	httpServerConfig, ok := s.config.GetInetHttpServer()
	s.xmlRPC.Stop()
	if ok {
		addr := httpServerConfig.GetString("port", "")
		log.Info("start to listen http addr ", addr)
		if addr != "" {
			go s.xmlRPC.StartInetHttpServer(httpServerConfig.GetString("username", ""), httpServerConfig.GetString("password", ""), addr, s)
		}
	}

	httpServerConfig, ok = s.config.GetUnixHttpServer()
	if ok {
		env := config.NewStringExpression("here", s.config.GetConfigFileDir())
		sockFile, err := env.Eval(httpServerConfig.GetString("file", "/tmp/supervisord.sock"))
		log.Info("start to listen unix addr ", sockFile)
		if err == nil {
			go s.xmlRPC.StartUnixHttpServer(httpServerConfig.GetString("username", ""), httpServerConfig.GetString("password", ""), sockFile, s)
		}
	}
}

func (s *Supervisor) startLogger() {
	supervisordConf, ok := s.config.GetSupervisord()
	if ok {
		//set supervisord log
		env := config.NewStringExpression("here", s.config.GetConfigFileDir())
		logFile, err := env.Eval(supervisordConf.GetString("logfile", "supervisord.log"))
		if err != nil {
			logFile, err = process.Path_expand(logFile)
		}
		logEventEmitter := logger.NewNullLogEventEmitter()
		s.logger = logger.NewNullLogger(logEventEmitter)
		if err == nil {
			logfile_maxbytes := int64(supervisordConf.GetBytes("logfile_maxbytes", 50*1024*1024))
			logfile_backups := supervisordConf.GetInt("logfile_backups", 10)
			loglevel := supervisordConf.GetString("loglevel", "info")
			s.logger = logger.NewLogger("supervisord", logFile, &sync.Mutex{}, logfile_maxbytes, logfile_backups, logEventEmitter)
			log.SetOutput(s.logger)
			log.SetLevel(toLogLevel(loglevel))
			log.SetFormatter(&log.TextFormatter{DisableColors: true})
		}
		//set the pid
		pidfile, err := env.Eval(supervisordConf.GetString("pidfile", "supervisord.pid"))
		if err == nil {
			f, err := os.Create(pidfile)
			if err == nil {
				f.Close()
			}
		}
	}
}

func toLogLevel(level string) log.Level {
	switch strings.ToLower(level) {
	case "critical":
		return log.FatalLevel
	case "error":
		return log.ErrorLevel
	case "warn":
		return log.WarnLevel
	case "info":
		return log.InfoLevel
	default:
		return log.DebugLevel
	}
}

func (s *Supervisor) ReloadConfig(r *http.Request, args *struct{}, reply *types.ReloadConfigResult) error {
	log.Info("start to reload config")
	s.procMgr.RemoveProcessInfoFile()
	s.procMgr.KillAllProcesses(nil)
	err, addedGroup, changedGroup, removedGroup := s.Reload(false)
	if len(addedGroup) > 0 {
		log.WithFields(log.Fields{"groups": strings.Join(addedGroup, ",")}).Info("added groups")
	}

	if len(changedGroup) > 0 {
		log.WithFields(log.Fields{"groups": strings.Join(changedGroup, ",")}).Info("changed groups")
	}

	if len(removedGroup) > 0 {
		log.WithFields(log.Fields{"groups": strings.Join(removedGroup, ",")}).Info("removed groups")
	}
	reply.AddedGroup = addedGroup
	reply.ChangedGroup = changedGroup
	reply.RemovedGroup = removedGroup
	return err
}

func (s *Supervisor) AddProcessGroup(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	reply.Success = false
	return nil
}

func (s *Supervisor) RemoveProcessGroup(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	reply.Success = false
	return nil
}

func (s *Supervisor) ReadProcessStdoutLog(r *http.Request, args *ProcessLogReadInfo, reply *struct{ LogData string }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	var err error
	reply.LogData, err = proc.StdoutLog.ReadLog(int64(args.Offset), int64(args.Length))
	return err
}

func (s *Supervisor) ReadProcessStderrLog(r *http.Request, args *ProcessLogReadInfo, reply *struct{ LogData string }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	var err error
	reply.LogData, err = proc.StderrLog.ReadLog(int64(args.Offset), int64(args.Length))
	return err
}

func (s *Supervisor) TailProcessStdoutLog(r *http.Request, args *ProcessLogReadInfo, reply *ProcessTailLog) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	var err error
	reply.LogData, reply.Offset, reply.Overflow, err = proc.StdoutLog.ReadTailLog(int64(args.Offset), int64(args.Length))
	return err
}

func (s *Supervisor) ClearProcessLogs(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	proc := s.procMgr.Find(args.Name)
	if proc == nil {
		return fmt.Errorf("No such process %s", args.Name)
	}
	err1 := proc.StdoutLog.ClearAllLogFile()
	err2 := proc.StderrLog.ClearAllLogFile()
	reply.Success = err1 == nil && err2 == nil
	if err1 != nil {
		return err1
	}
	return err2
}

func (s *Supervisor) ClearAllProcessLogs(r *http.Request, args *struct{}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {

	s.procMgr.ForEachProcess(func(proc *process.Process) {
		proc.StdoutLog.ClearAllLogFile()
		proc.StderrLog.ClearAllLogFile()
		procInfo := proc.TypeProcessInfo()
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        procInfo.Name,
			Group:       procInfo.Group,
			Status:      faults.SUCCESS,
			Description: "OK",
		})
	})

	return nil
}
