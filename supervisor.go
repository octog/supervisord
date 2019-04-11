package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/AlexStocks/goext/os/process"
	"github.com/AlexStocks/supervisord/config"
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

var NotFoundError = errors.New("no such group")

type Supervisor struct {
	config  *config.Config
	procMgr *process.ProcessManager
	xmlRPC  *XmlRPC
	logger  logger.Logger
	// restarting bool
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
		config:  config.NewConfig(configFile),
		procMgr: process.NewProcessManager(),
		xmlRPC:  NewXmlRPC(),
		// restarting: false,
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

	// s.restarting = true

	// stop all processes
	s.procMgr.KillAllProcesses(nil)
	// remove all processes
	s.procMgr.RemoveAllProcesses(nil)
	err, _, _, _ := s.Reload(false)
	if err == nil {
		reply.Ret = true
	}

	return err
}

// func (s *Supervisor) IsRestarting() bool {
// 	return s.restarting
// }

func getProcessInfo(proc *process.Process) *types.ProcessInfo {
	return &types.ProcessInfo{Name: proc.GetName(),
		Group:          proc.GetGroup(),
		Description:    proc.GetDescription(),
		Start:          int(proc.GetStartTime().Unix()),
		Stop:           int(proc.GetStopTime().Unix()),
		Now:            int(time.Now().Unix()),
		State:          int(proc.GetState()),
		Statename:      proc.GetState().String(),
		Spawnerr:       "",
		Exitstatus:     proc.GetExitstatus(),
		Logfile:        proc.GetStdoutLogfile(),
		Stdout_logfile: proc.GetStdoutLogfile(),
		Stderr_logfile: proc.GetStderrLogfile(),
		Pid:            proc.GetPid()}
}

func (s *Supervisor) GetAllProcessInfo(r *http.Request, args *struct{}, reply *struct{ AllProcessInfo []types.ProcessInfo }) error {
	reply.AllProcessInfo = make([]types.ProcessInfo, 0)
	s.procMgr.ForEachProcess(func(proc *process.Process) {
		procInfo := proc.TypeProcessInfo()
		reply.AllProcessInfo = append(reply.AllProcessInfo, procInfo)
	})
	if _, arr := s.procMgr.GetPrestartProcess(); len(arr) != 0 {
		for _, info := range arr {
			reply.AllProcessInfo = append(reply.AllProcessInfo, info.TypeProcessInfo())
		}
	}
	types.SortProcessInfos(reply.AllProcessInfo)
	log.Info("process num: ", len(reply.AllProcessInfo))
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

func (s *Supervisor) GetProcessInfo(r *http.Request, args *struct{ Name string }, reply *struct{ ProcessInfo types.ProcessInfo }) error {
	log.Info("Get process info of: ", args.Name)

	if len(args.Name) == 0 {
		return errors.New("Arguments Is Empty")
	}
	proc := s.procMgr.Find(args.Name)
	if proc != nil {
		reply.ProcessInfo = proc.TypeProcessInfo()
	} else {
		info := s.procMgr.FindProcessInfo(args.Name)
		if info == nil {
			return fmt.Errorf("no process named %s", args.Name)
		}
		reply.ProcessInfo = info.TypeProcessInfo()
	}

	return nil
}

// func (s *Supervisor) ListMethods(r *http.Request, args *struct{}, reply *struct{ Methods []string }) error {
// 	reply.Methods = xmlCodec.Methods()

// 	return nil
// }

func (s *Supervisor) StartProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	names := s.config.MatchProgramName(args.Name)
	if len(names) == 0 {
		return NotFoundError
	}
	for _, procName := range names {
		proc := s.procMgr.Find(procName)
		if proc == nil {
			psInfo := s.procMgr.FindProcessInfo(procName)
			if psInfo != nil && psInfo.ConfigEntry() != nil {
				proc = s.procMgr.CreateProcess(s.GetSupervisorId(), psInfo.ConfigEntry())
				if proc == nil {
					return fmt.Errorf("fail to create process{config:%#v}", psInfo.ConfigEntry())
				}
			} else {
				proc = s.startProcessByConfig(procName)
				if proc == nil {
					return fmt.Errorf("fail to find process %s in configure file", procName)
				}
			}
		}
		proc.Start(args.Wait, func(p *process.Process) {
			s.procMgr.UpdateProcessInfo(p)
		})
	}
	if len(names) != 0 {
		reply.Success = true
	}
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
		configEntry := info.ConfigEntry()
		group := ""
		if configEntry != nil {
			group = configEntry.GetGroupName()
		}
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        info.Program + " Started ",
			Group:       group, //info.ConfigEntry().Group,
			Status:      faults.SUCCESS,
			Description: "OK",
		})
	}
	_, frozenPrestartProcesses := s.procMgr.GetFrozenPrestartProcess()
	for _, info := range frozenPrestartProcesses {
		configEntry := info.ConfigEntry()
		if configEntry != nil {
			proc := s.procMgr.CreateProcess(s.GetSupervisorId(), info.ConfigEntry())
			if proc != nil {
				proc.Start(true, func(p *process.Process) {
					s.procMgr.UpdateProcessInfo(p)
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
					s.procMgr.UpdateProcessInfo(p)
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

// StopProcess 仅仅把进程停止，不把它的相关信息从 s.procMgr.procs 中删除
func (s *Supervisor) StopProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	names := s.config.MatchProgramName(args.Name)
	log.WithFields(log.Fields{"args": args.Name, "programs": names}).Info("stop process")
	if len(names) == 0 {
		return NotFoundError
	}
	for _, procName := range names {
		proc := s.procMgr.StopProcess(procName, args.Wait)
		if proc == nil {
			psInfo := s.procMgr.FindProcessInfo(procName)
			if psInfo == nil {
				return fmt.Errorf("fail to find process %s", procName)
			}
			s.procMgr.StopProcessInfo(procName, args.Wait)
		}
	}
	if len(names) != 0 {
		reply.Success = true
	}

	return nil
}

// RemoveProcess 仅仅停止工作的进程的相关信息从 s.procMgr.procs 中删除
func (s *Supervisor) RemoveProcess(r *http.Request, args *StartProcessArgs, reply *struct{ Success bool }) error {
	log.WithFields(log.Fields{"program": args.Name}).Info("remove process")
	names := s.config.MatchProgramName(args.Name)
	if len(names) == 0 {
		return NotFoundError
	}
	for _, procName := range names {
		proc := s.procMgr.Find(procName)
		if proc != nil {
			if proc.GetPid() != 0 {
				return fmt.Errorf("process %s is still alive", procName)
			}
		} else {
			psInfo := s.procMgr.FindProcessInfo(procName)
			if psInfo == nil {
				return fmt.Errorf("fail to find process %s", procName)
			}
			if _, err := gxprocess.FindProcess(int(psInfo.PID)); err == nil {
				return fmt.Errorf("process %s is still alive", procName)
			}
		}
		s.procMgr.Remove(procName)
		s.config.RemoveProgram(procName)
	}

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

	prevProgGroup := s.config.ProgramGroup.Clone()
	procs := prevProgGroup.GetAllProcess(args.Name)
	for _, v := range procs {
		proc := s.procMgr.StopProcess(v, args.Wait)
		if proc != nil {
			reply.AllProcessInfo = append(reply.AllProcessInfo, proc.TypeProcessInfo())
		} else {
			psInfo := s.procMgr.FindProcessInfo(v)
			if psInfo == nil {
				return fmt.Errorf("fail to find process %s", args.Name)
			}
			psInfo = s.procMgr.StopProcessInfo(v, args.Wait)
			reply.AllProcessInfo = append(reply.AllProcessInfo, psInfo.TypeProcessInfo())
		}
	}
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

func (s *Supervisor) RemoveAllProcesses(r *http.Request, args *struct {
	Wait bool `default:"true"`
}, reply *struct{ RpcTaskResults []RpcTaskResult }) error {
	s.procMgr.RemoveAllProcesses(func(info process.ProcessInfo) {
		var group string
		if entry := info.ConfigEntry(); entry != nil {
			group = entry.Group
		}
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        info.Program + " removed",
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
	if err == nil {
		err = s.StartAllProcesses(r, args, reply)
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

// func (s *Supervisor) SendRemoteCommEvent(r *http.Request, args *RemoteCommEvent, reply *struct{ Success bool }) error {
// 	events.EmitEvent(events.NewRemoteCommunicationEvent(args.Type, args.Data))
// 	reply.Success = true
// 	return nil
// }

// return err, addedGroup, changedGroup, removedGroup
//
//
func (s *Supervisor) Reload(startup bool) (error, []string, []string, []string) {
	//get the previous loaded programs
	prevPrograms := s.config.GetProgramNames()
	prevProgGroup := s.config.ProgramGroup.Clone()
	var oldActivePrograms, oldDeadPrograms []string

	_, err := s.config.Load()
	if err == nil {
		s.setSupervisordInfo()
		supervisordConf, flag := s.config.GetSupervisord()
		if flag {
			s.procMgr.UpdateConfig(supervisordConf)
			// get previous ps
			oldActivePrograms, oldDeadPrograms = s.procMgr.ValidateStartPs()
			if len(oldActivePrograms) > 0 {
				// yaml中记录的program, 记录是否配置过
				oldActiveProgramsMap := make(map[string]bool)

				// 重新启动supervisor时，需要将在process map info 中的program的配置导入, 要考虑自动remove的情况
				_, _ = s.config.Load()

				for _, n := range oldActivePrograms {
					oldActiveProgramsMap[n] = false
				}
				for _, gName := range s.config.ProgramGroup.GetAllGroup() {
					for _, pName := range s.config.ProgramGroup.GetAllProcess(gName) {
						if _, ok := oldActiveProgramsMap[pName]; ok {
							oldActiveProgramsMap[pName] = true
						}
					}
				}

				for pName, hasConfig := range oldActiveProgramsMap {
					if hasConfig {
						// process info map 中running的program需要监控
						info := s.procMgr.FindProcessInfo(pName)
						if info != nil {
							go func() {
								for {
									_, err := gxprocess.FindProcess(int(info.PID))
									if err == nil {
										time.Sleep(time.Millisecond * 200)
									} else {
										log.WithFields(log.Fields{"process:": pName}).Info("process exit")
										_ = s.StartProcess(&http.Request{}, &StartProcessArgs{pName, false}, &struct{ Success bool }{false})
										break
									}
								}
							}()
						}
					} else {
						log.WithFields(log.Fields{"process:": pName}).Warn("Program is running, but not find configs.")
					}
				}

				log.WithFields(log.Fields{"oldActivePrograms:": oldActivePrograms}).Info("auto-reload programs")
			}

		}
		// s.startEventListeners()
		s.createPrograms(prevPrograms, startup) // create Process
		s.startHttpServer()
		s.startAutoStartPrograms() // start Process: process.Process.Start -> process.Process.run -> process.Process.waitForExit
		if startup {
			loadedPrograms := s.config.GetProgramNames()
			loadedProgGroup := s.config.ProgramGroup.Clone()
			unloadPrograms := util.Sub(loadedPrograms, append(oldActivePrograms, oldDeadPrograms...))
			for _, v := range unloadPrograms {
				s.config.RemoveProgram(v)
				group := loadedProgGroup.GetGroup(v, v)
				s.config.RemoveGroup(group)
			}
			go s.MonitorPrestartProcess()
		}
		if flag {
			if len(oldDeadPrograms) > 0 {
				// program -> group
				oldDeadProgramsMap := make(map[string]string)
				for _, n := range oldDeadPrograms {
					oldDeadProgramsMap[n] = n
				}
				//yaml中记录为running，实际上却挂掉的program需要重新拉起来
				log.WithFields(log.Fields{"oldDeadPrograms:": oldDeadPrograms}).Info("auto-reload programs")
				for name := range oldDeadProgramsMap {
					entries := s.config.GetEntries(func(entry *config.ConfigEntry) bool {
						if entry.IsProgram() && entry.GetProgramName() == name {
							return true
						}
						return false
					})
					for _, e := range entries {
						oldDeadProgramsMap[name] = e.Group
					}
				}
				// 可能是配置了numprocs参数，所以需要先找到program的组, update都是根据组执行的
				for _, n := range oldDeadProgramsMap {
					_ = s.Update(&http.Request{}, &struct{ Process string }{n}, &types.UpdateResult{})
				}
			}

		}
	}
	// fmt.Printf("$$$$ Reload Fin\n")
	addedGroup, changedGroup, removedGroup, _ := s.config.ProgramGroup.Sub(prevProgGroup)

	return err, addedGroup, changedGroup, removedGroup
}

func (s *Supervisor) update(r *http.Request, args *struct{ Process string }, reply *types.UpdateResult) error {
	// 20190320 update的时候，后面参数只能接group，　要把该group下所有的programs都取出来
	//get the previous loaded programs
	gName := args.Process
	var err error
	var prevPrograms, loadedPrograms []string
	var prevEntryArray []config.ConfigEntry

	prevProgGroup := s.config.ProgramGroup.Clone()
	updateAll := false
	if gName == "___all___" {
		updateAll = true
		prevPrograms = s.config.GetProgramNames()
		for _, prevEntry := range s.config.GetEntries(func(*config.ConfigEntry) bool { return true }) {
			prevEntryArray = append(prevEntryArray, prevEntry.Clone())
		}
		loadedPrograms, err = s.config.Load()
	} else {
		prevPrograms = s.config.GetGroupProgramNames(gName)
		for _, prevEntry := range s.config.GetEntries(func(c *config.ConfigEntry) bool { return c.Group == gName }) {
			prevEntryArray = append(prevEntryArray, prevEntry.Clone())
		}
		loadedPrograms = s.config.LoadGroup(gName)
	}

	matchPrograms := s.config.GetGroupProgramNames(gName)
	if len(matchPrograms) == 0 && !updateAll {
		// 不存在的Group
		if !prevProgGroup.GroupExists(gName) {
			return errors.New("BAD_NAME: " + gName)
		}
	}
	removedPrograms := util.Sub(prevPrograms, loadedPrograms)
	for _, removedProg := range removedPrograms {
		if updateAll || util.InStringArray(removedProg, removedPrograms) {
			s.config.RemoveProgram(removedProg)

			// Bugfix 20190118: procMgr.Remove 函数会调用下面的 procMgr.RemoveProcessInfo 函数，
			// 所以把 infoMap 中相应进程的关闭放在上面
			info := s.procMgr.RemoveProcessInfo(removedProg)
			if info.PID != 0 {
				info.Stop(true)
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
	addedPrograms := util.Sub(loadedPrograms, prevPrograms)
	for _, addedProgram := range addedPrograms {

		if updateAll || util.InStringArray(addedProgram, matchPrograms) {

			entries := s.config.GetPrograms()
			startFlag := false
			for j := range entries {
				if entries[j].GetProgramName() == strings.TrimSpace(addedProgram) {
					startFlag = true
					proc := s.procMgr.CreateProcess(s.GetSupervisorId(), entries[j])
					if proc != nil {
						proc.Start(true, func(p *process.Process) {
							s.procMgr.UpdateProcessInfo(p)
						})
					}
				}
			}
			if !startFlag {
				log.Warn("can not find config of program ", addedProgram)
			}
		}
	}
	// 20190321: 仅仅只是名字一样，可能配置改变了，比如组下面的一个program改变了，但是该组是在ChangedGroup，　所以仅仅检测same中program是不够的

	mayUnchangedPrograms := util.Intersection(loadedPrograms, prevPrograms)

	var same []string
	changedGroup := map[string]bool{}
	reply.AddedGroup, reply.ChangedGroup, reply.RemovedGroup, same = s.config.ProgramGroup.Sub(prevProgGroup)

	for _, name := range same {
		if updateAll || name == gName {
			for _, pName := range matchPrograms {
				mayUnchangedPrograms[pName] = true
			}
		}
	}
	// 进一步检测program的配置是否改变
	for pName := range mayUnchangedPrograms {
		entry := s.config.GetProgram(pName)
		for _, prevEntry := range prevEntryArray {
			if prevEntry.Name == entry.Name && prevEntry.Group == entry.Group {
				if !entry.IsSame(prevEntry) {
					name := entry.GetProgramName()
					// stop prestart process
					// 先把 procInfo 删掉，防止 MonitorPrestartProcess goroutine 自动启动这个 processInfo
					info := s.procMgr.RemoveProcessInfo(name)
					if info.PID != 0 {
						info.Stop(true)
						// s.config.RemoveProgram(name)
						log.WithFields(log.Fields{"prestart program": name, "pid": info.PID}).Info(
							"the program is removed and will restart automatically")
					}

					// stop running process
					proc := s.procMgr.Remove(name)
					if proc != nil {
						proc.Stop(false)
						// s.config.RemoveProgram(name)
						log.WithFields(log.Fields{"program": name, "pid": proc.GetPid()}).Info(
							"the program is removed and will restart")
					}

					// restart process
					proc = s.procMgr.CreateProcess(s.GetSupervisorId(), entry)
					if proc != nil {
						proc.Start(true, func(p *process.Process) {
							s.procMgr.UpdateProcessInfo(p)
						})
						changedGroup[entry.Group] = true
						//reply.ChangedGroup = append(reply.ChangedGroup, name)
					} else {
						log.WithFields(log.Fields{"program": name}).Error(
							"the program is removed and can not restart")
					}
				}
			}
		}
	}
	for _, pName := range reply.ChangedGroup {
		changedGroup[pName] = true
	}
	reply.ChangedGroup = nil
	for name := range changedGroup {
		reply.ChangedGroup = append(reply.ChangedGroup, name)
	}
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
							s.procMgr.UpdateProcessInfo(p)
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
		// if s.IsRestarting() {
		// 	log.Info("start to stop all processes and exit")
		// 	s.procMgr.StopAllProcesses(false, true)
		// 	break
		// }
		time.Sleep(10 * time.Second)
	}
}

func (s *Supervisor) createPrograms(prevPrograms []string, flag bool) {
	loadedPrograms := s.config.GetProgramNames()
	// stop old processes and delete its proc info
	removedPrograms := util.Sub(prevPrograms, loadedPrograms)
	for _, removedProg := range removedPrograms {
		// log.WithFields(log.Fields{"program": removedProg}).Info(
		//	"the program is removed and will be stopped")
		s.config.RemoveProgram(removedProg)
		proc := s.procMgr.Remove(removedProg)
		if proc != nil {
			proc.Stop(true)
			log.WithFields(log.Fields{"program": removedProg, "pid": proc.GetPid()}).Info(
				"the program is removed and will be stopped")
		}
		info := s.procMgr.RemoveProcessInfo(removedProg)
		if info.PID != 0 {
			info.Stop(true)
			log.WithFields(log.Fields{"prestart program": removedProg, "pid": info.PID}).Info(
				"the program is removed and will be stopped")
		}
	}
	// create new processes
	if !flag {
		for _, entry := range s.config.GetPrograms() {
			// 如果原 process 还存在，则新的 process 不可能创建成功
			s.procMgr.CreateProcess(s.GetSupervisorId(), entry)
		}
	}
}

func (s *Supervisor) startAutoStartPrograms() {
	s.procMgr.StartAutoStartPrograms()
}

// func (s *Supervisor) startEventListeners() {
// 	eventListeners := s.config.GetEventListeners()
// 	for _, entry := range eventListeners {
// 		if proc := s.procMgr.CreateProcess(s.GetSupervisorId(), entry); proc != nil {
// 			proc.Start(false, func(p *process.Process) {
// 				s.procMgr.UpdateProcessInfo(p)
// 			})
// 		}
// 	}

// 	if len(eventListeners) > 0 {
// 		time.Sleep(1 * time.Second)
// 	}
// }

func (s *Supervisor) startHttpServer() {
	httpServerConfig, ok := s.config.GetInetHttpServer()
	s.xmlRPC.Stop()
	if ok {
		addr := httpServerConfig.GetString("port", "")
		log.Info("start to listen http addr ", addr)
		if addr != "" {
			go s.xmlRPC.StartInetHttpServer(httpServerConfig.GetString("username", ""),
				httpServerConfig.GetString("password", ""), addr, s, gSystem)
		}
	}

	httpServerConfig, ok = s.config.GetUnixHttpServer()
	if ok {
		env := config.NewStringExpression("here", s.config.GetConfigFileDir())
		sockFile, err := env.Eval(httpServerConfig.GetString("file", "/tmp/supervisord.sock"))
		log.Info("start to listen unix addr ", sockFile)
		if err == nil {
			go s.xmlRPC.StartUnixHttpServer(httpServerConfig.GetString("username", ""),
				httpServerConfig.GetString("password", ""), sockFile, s, gSystem)
		}
	}
}

func (s *Supervisor) setSupervisordInfo() {
	supervisordConf, ok := s.config.GetSupervisord()
	if ok {
		//set supervisord log
		env := config.NewStringExpression("here", s.config.GetConfigFileDir())
		logFile, err := env.Eval(supervisordConf.GetString("logfile", "supervisord.log"))
		if err != nil {
			logFile, err = process.Path_expand(logFile)
		}
		if logFile == "/dev/stdout" {
			return
		}
		logEventEmitter := logger.NewNullLogEventEmitter()
		s.logger = logger.NewNullLogger(logEventEmitter)
		if err == nil {
			logfile_maxbytes := int64(supervisordConf.GetBytes("logfile_maxbytes", 50*1024*1024))
			logfile_backups := supervisordConf.GetInt("logfile_backups", 10)
			loglevel := supervisordConf.GetString("loglevel", "info")
			s.logger = logger.NewLogger("supervisord", logFile, &sync.Mutex{}, logfile_maxbytes, logfile_backups, logEventEmitter)
			log.SetLevel(toLogLevel(loglevel))
			log.SetFormatter(&log.TextFormatter{DisableColors: true, FullTimestamp: true})
			log.SetOutput(s.logger)
		}
		//set the pid
		pidfile, err := env.Eval(supervisordConf.GetString("pidfile", "supervisord.pid"))
		if err == nil {
			f, err := os.Create(pidfile)
			if err == nil {
				fmt.Fprintf(f, "%d", os.Getpid())
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

func (s *Supervisor) ReloadConfig(r *http.Request, args *struct{}, replys *types.ReloadConfigResults) error {
	log.Info("start to reload config")
	//get the previous loaded programs
	// prevPrograms := s.config.GetProgramNames()
	prevProgGroup := s.config.ProgramGroup.Clone()
	prevEntries := s.config.GetEntries(func(*config.ConfigEntry) bool { return true })
	var prevEntryArray []config.ConfigEntry
	for i := range prevEntries {
		prevEntryArray = append(prevEntryArray, prevEntries[i].Clone())
	}

	cfg := config.NewConfig(s.config.GetConfigFile())
	_, err := cfg.Load()
	// _, err := s.config.Load()
	// removedPrograms := util.Sub(prevPrograms, loadedPrograms)
	// addedPrograms := util.Sub(loadedPrograms, prevPrograms)
	var same []string
	reply := types.ReloadConfigResult{}
	reply.AddedGroup, reply.ChangedGroup, reply.RemovedGroup, same = cfg.ProgramGroup.Sub(prevProgGroup)
	for _, v := range same {
		entries := cfg.GetEntries(func(conf *config.ConfigEntry) bool {
			if conf.Group == v {
				return true
			}
			return false
		})
		for _, prevEntry := range prevEntryArray {
			for _, v := range entries {
				if prevEntry.Group == v.Group && prevEntry.Name == v.Name {
					if !v.IsSame(prevEntry) {
						reply.ChangedGroup = append(reply.ChangedGroup, v.Group)
					}
				}
			}
		}
	}

	if len(reply.AddedGroup) > 0 {
		log.WithFields(log.Fields{"groups": strings.Join(reply.AddedGroup, ",")}).Info("added groups")
	}
	if len(reply.ChangedGroup) > 0 {
		log.WithFields(log.Fields{"groups": strings.Join(reply.ChangedGroup, ",")}).Info("changed groups")
	}
	if len(reply.RemovedGroup) > 0 {
		log.WithFields(log.Fields{"groups": strings.Join(reply.RemovedGroup, ",")}).Info("removed groups")
	}

	replys.Results = append(replys.Results, []types.ReloadConfigResult{reply})

	return err
}

func (s *Supervisor) AddProcessGroup(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	if err := s.config.UpdateConfigEntry(args.Name); err != nil {
		return err
	}

	//new
	entries := s.config.GetEntries(func(entry *config.ConfigEntry) bool {
		if entry.Group == args.Name {
			return true
		}
		return false
	})
	if len(entries) == 0 {
		return fmt.Errorf("fail to find process %s in configure file", args.Name)
	}

	for _, v := range entries {
		proc := s.procMgr.Find(v.Name)
		var procName string
		if pos := strings.Index(v.Name, ":"); pos != -1 {
			procName = v.Name[pos+1:]
		} else {
			procName = v.Group
		}
		log.WithFields(log.Fields{"procName": procName, "promName": v.Name}).Info("find proc")
		if proc == nil {
			psInfo := s.procMgr.FindProcessInfo(procName)
			if psInfo != nil && psInfo.ConfigEntry() != nil {
				proc = s.procMgr.CreateProcess(s.GetSupervisorId(), psInfo.ConfigEntry())
				if proc == nil {
					return fmt.Errorf("fail to create process{config:%#v}", psInfo.ConfigEntry())
				}
			} else {
				proc = s.startProcessByConfig(procName)
				if proc == nil {
					return fmt.Errorf("fail to find process %s in configure file", v.Name)
				}
			}
		}
		proc.Start(true, func(p *process.Process) {
			s.procMgr.UpdateProcessInfo(p)
		})
	}
	reply.Success = true

	return nil
}

func (s *Supervisor) RemoveProcessGroup(r *http.Request, args *struct{ Name string }, reply *struct{ Success bool }) error {
	log.WithFields(log.Fields{"group": args.Name}).Info("remove process group")

	prevProgGroup := s.config.ProgramGroup.Clone()
	procs := prevProgGroup.GetAllProcess(args.Name)
	if len(procs) == 0 {
		return NotFoundError
	}
	for _, v := range procs {
		proc := s.procMgr.Find(v)
		if proc != nil {
			if proc.GetPid() != 0 {
				return fmt.Errorf("process %s is still alive", v)
			}
		} else {
			psInfo := s.procMgr.FindProcessInfo(v)
			if psInfo != nil {
				if _, err := gxprocess.FindProcess(int(psInfo.PID)); err == nil {
					return fmt.Errorf("process %s is still alive", v)
				}
			}
		}
	}
	for _, v := range procs {
		s.procMgr.Remove(v)
		s.config.RemoveProgram(v)
		s.config.RemoveGroup(args.Name)
	}

	reply.Success = true

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
		procInfo := getProcessInfo(proc)
		reply.RpcTaskResults = append(reply.RpcTaskResults, RpcTaskResult{
			Name:        procInfo.Name,
			Group:       procInfo.Group,
			Status:      faults.SUCCESS,
			Description: "OK",
		})
	})

	return nil
}
