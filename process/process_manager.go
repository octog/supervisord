package process

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/AlexStocks/goext/os/process"
	"github.com/AlexStocks/supervisord/config"
	log "github.com/sirupsen/logrus"
)

var (
	supervisordStartTime uint64
)

func init() {
	supervisordStartTime = uint64(time.Now().UnixNano())
}

type ProcessManager struct {
	procs          map[string]*Process
	psInfoMap      *ProcessInfoMap // all active process info map
	eventListeners map[string]*Process
	lock           sync.Mutex

	startKillAll bool
	exitKillAll  bool
	psInfoFile   string
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		procs:          make(map[string]*Process),
		eventListeners: make(map[string]*Process),
		psInfoMap:      NewProcessInfoMap(),
	}
}

func (pm *ProcessManager) OutputInfomap() {
}

// the return value list:
// the first is prestart process number
// the second is the dead prestart process name array
func (pm *ProcessManager) GetDeadPrestartProcess() (int, []string) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	prestartNum := 0
	psArray := make([]string, 0, len(pm.psInfoMap.InfoMap))
	for name, info := range pm.psInfoMap.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		prestartNum++
		if info.PID == int64(FROZEN_PID) { // 进程是 supervisorctl 杀掉的，不用重启
			continue
		}
		_, err := gxprocess.FindProcess(int(info.PID))
		if err != nil {
			// delete(pm5.psInfoMap.InfoMap, name)
			pm.psInfoMap.RemoveProcessInfo(name)
			psArray = append(psArray, name)
		}
	}

	return prestartNum, psArray
}

func (pm *ProcessManager) GetFrozenPrestartProcess() (int, []ProcessInfo) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	num := 0
	psArray := make([]ProcessInfo, 0, len(pm.psInfoMap.InfoMap))
	for name, info := range pm.psInfoMap.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		num++
		if info.PID == int64(FROZEN_PID) { // 进程是 supervisorctl 杀掉的，不用重启
			pm.psInfoMap.RemoveProcessInfo(name)
			psArray = append(psArray, info)
		}
	}

	return num, psArray
}

// 用于 status _infomap
func (pm *ProcessManager) GetAllInfomapProcess() (int, []ProcessInfo) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(pm.psInfoMap.InfoMap))
	for _, info := range pm.psInfoMap.InfoMap {
		prestartNum++
		// if _, err := gxprocess.FindProcess(int(info.PID)); err == nil {
		psArray = append(psArray, info)
		// }
	}

	return prestartNum, psArray
}

// the return value list:
// the first is prestart process number
// the second is the active prestart process name array
func (pm *ProcessManager) GetActivePrestartProcess() (int, []ProcessInfo) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(pm.psInfoMap.InfoMap))
	for _, info := range pm.psInfoMap.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		if info.PID == int64(FROZEN_PID) {
			continue
		}
		prestartNum++
		if _, err := gxprocess.FindProcess(int(info.PID)); err == nil {
			psArray = append(psArray, info)
		}
	}

	return prestartNum, psArray
}

func (pm *ProcessManager) GetPrestartProcess() (int, []ProcessInfo) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(pm.psInfoMap.InfoMap))
	for _, info := range pm.psInfoMap.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		prestartNum++
		psArray = append(psArray, info)
	}

	return prestartNum, psArray
}

func (pm *ProcessManager) UpdateConfig(config *config.ConfigEntry) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	pm.startKillAll = config.GetBool("startkillall", true)
	pm.exitKillAll = config.GetBool("exitkillall", true)
	pm.psInfoFile = config.GetString("processinfomapfile", "./supervisor_ps_info_map.yaml")
	log.Info(fmt.Sprintf("startKillAll:%v, exitKillAll:%v, psInfoFile:%s",
		pm.startKillAll, pm.exitKillAll, pm.psInfoFile))
}

func (pm *ProcessManager) ValidateStartPs() {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	pm.psInfoMap.Load(pm.psInfoFile)
	for name, info := range pm.psInfoMap.InfoMap {
		ps, err := gxprocess.FindProcess(int(info.PID))
		if err != nil {
			pm.psInfoMap.RemoveProcessInfo(name)
			continue
		}

		if pm.startKillAll {
			syscall.Kill(ps.Pid(), syscall.SIGKILL)
			pm.psInfoMap.RemoveProcessInfo(name)
			continue
		}
	}
}

func (pm *ProcessManager) CreateProcess(supervisor_id string, config *config.ConfigEntry) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	var proc *Process
	if config.IsProgram() {
		proc = pm.createProgram(supervisor_id, config)
	} else if config.IsEventListener() {
		proc = pm.createEventListener(supervisor_id, config)
	}

	pm.psInfoMap.Store(pm.psInfoFile)

	return proc
}

func (pm *ProcessManager) StartAutoStartPrograms() {
	pm.ForEachProcess(func(proc *Process) {
		if proc.isAutoStart() {
			proc.Start(true, func(p *Process) {
				pm.UpdateProcessInfo(proc) // to defeat dead-lock
				// pm.psInfoMap.AddProcessInfo(proc.ProcessInfo())
			})
		}
	})

	pm.psInfoMap.Store(pm.psInfoFile)
}

func (pm *ProcessManager) UpdateProcessInfo(proc *Process) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	pm.psInfoMap.AddProcessInfo(proc.ProcessInfo())

	pm.psInfoMap.Store(pm.psInfoFile)
}

func (pm *ProcessManager) createProgram(supervisor_id string, config *config.ConfigEntry) *Process {
	var proc *Process

	procName := config.GetProgramName()
	info, ok := pm.psInfoMap.GetProcessInfo(procName)
	if !ok || info.IsFrozen() {
		proc = NewProcess(supervisor_id, config)
		proc.SetKillAttr(pm.startKillAll, pm.exitKillAll)
		pm.procs[procName] = proc
		info = proc.ProcessInfo()
	}
	info.config = config
	pm.psInfoMap.AddProcessInfo(info)

	log.Info("create process:", procName)

	return proc
}

func (pm *ProcessManager) createEventListener(supervisor_id string, config *config.ConfigEntry) *Process {
	eventListenerName := config.GetEventListenerName()
	evtListener, ok := pm.eventListeners[eventListenerName]
	if !ok {
		evtListener = NewProcess(supervisor_id, config)
		evtListener.SetKillAttr(pm.startKillAll, pm.exitKillAll)
		pm.eventListeners[eventListenerName] = evtListener
	}

	pm.psInfoMap.Store(pm.psInfoFile)

	log.Info("create event listener:", eventListenerName)
	return evtListener
}

func (pm *ProcessManager) Add(proc *Process) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.AddProc(proc)
}

func (pm *ProcessManager) AddProc(proc *Process) {
	// pm.lock.Lock()
	// defer pm.lock.Unlock()
	name := proc.config.GetProgramName()
	pm.procs[name] = proc
	pm.psInfoMap.AddProcessInfo(proc.ProcessInfo())
	log.Info("add process:", name)

	pm.psInfoMap.Store(pm.psInfoFile)
}

// stop the process
//
// Arguments:
// name - the name of program
//
// Return the process or nil
func (pm *ProcessManager) StopProcess(name string, wait bool) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	proc, ok := pm.procs[name]
	if ok {
		log.Info("remove process:", name)
		proc.Stop(wait)
		pm.psInfoMap.Store(pm.psInfoFile)
	}

	return proc
}

// remove the process from the manager
//
// Arguments:
// name - the name of program
//
// Return the process or nil
func (pm *ProcessManager) Remove(name string) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	proc, _ := pm.procs[name]
	delete(pm.procs, name)
	pm.psInfoMap.RemoveProcessInfo(name)
	log.Info("remove process:", name)
	pm.psInfoMap.Store(pm.psInfoFile)

	return proc
}

func (pm *ProcessManager) RemoveProcessInfo(name string) ProcessInfo {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	log.Info("remove process info:", name)
	info := pm.psInfoMap.RemoveProcessInfo(name)
	pm.psInfoMap.Store(pm.psInfoFile)

	return info
}

// return process if found or nil if not found
func (pm *ProcessManager) Find(name string) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	proc, ok := pm.procs[name]
	if ok {
		log.Debug("succeed to find process:", name)
	} else {
		//remove group field if it is included
		if pos := strings.Index(name, ":"); pos != -1 {
			proc, ok = pm.procs[name[pos+1:]]
		}
		if !ok {
			log.Info("fail to find process:", name)
		}
	}
	return proc
}

func (pm *ProcessManager) FindMatch(name string) []*Process {
	result := make([]*Process, 0)
	if pos := strings.Index(name, ":"); pos != -1 {
		groupName := name[0:pos]
		programName := name[pos+1:]
		pm.ForEachProcess(func(p *Process) {
			if p.GetGroup() == groupName {
				if programName == "*" || programName == p.GetName() {
					result = append(result, p)
				}
			}
		})
	} else {
		pm.lock.Lock()
		defer pm.lock.Unlock()
		proc, ok := pm.procs[name]
		if ok {
			result = append(result, proc)
		}
	}
	if len(result) <= 0 {
		log.Info("fail to find process:", name)
	}
	return result
}

// return process if found or nil if not found
func (pm *ProcessManager) GetProcsProcessInfo(name string) *Process {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	proc, ok := pm.procs[name]
	if ok {
		log.Debug("succeed to find process:", name)
	} else {
		//remove group field if it is included
		if pos := strings.Index(name, ":"); pos != -1 {
			proc, ok = pm.procs[name[pos+1:]]
		}
		if !ok {
			log.Info("fail to find process:", name)
		}
	}
	return proc
}

func (pm *ProcessManager) FindProcessInfo(name string) *ProcessInfo {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	info, ok := pm.psInfoMap.InfoMap[name]
	if !ok {
		return nil
	}

	log.Debug("succeed to find process info:", info)
	return &info
}

func (pm *ProcessManager) StopProcessInfo(name string, wait bool) *ProcessInfo {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	info, ok := pm.psInfoMap.InfoMap[name]
	if ok {
		info.Stop(wait)
		pm.psInfoMap.InfoMap[name] = info
		pm.psInfoMap.Store(pm.psInfoFile)
		return &info
	}

	return nil
}

// clear all the processes
func (pm *ProcessManager) Clear() {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.procs = make(map[string]*Process)
	pm.psInfoMap.Reset()

	pm.psInfoMap.Store(pm.psInfoFile)
}

func (pm *ProcessManager) ForEachProcess(procFunc func(p *Process)) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	procs := pm.getAllProcess()
	done := make(chan struct{}, 1048576)

	for _, proc := range procs {
		go forOneProcess(proc, procFunc, done)
	}

	for range procs {
		<-done
	}
}

func forOneProcess(process *Process, action func(p *Process), done chan struct{}) {
	action(process)

	done <- struct{}{}
}

func (pm *ProcessManager) getAllProcess() []*Process {
	tmpProcs := make([]*Process, 0)
	for _, proc := range pm.procs {
		tmpProcs = append(tmpProcs, proc)
	}
	return sortProcess(tmpProcs)
}

func (pm *ProcessManager) KillAllProcesses(procFunc func(ProcessInfo)) {
	pm.ForEachProcess(func(proc *Process) {
		proc.Stop(true)
		// 删除 psInfoMap 中对应的进程信息，只在 pm.procs 保存停止的进程的状态即可
		pm.psInfoMap.RemoveProcessInfo(proc.config.GetProgramName())
		if procFunc != nil {
			procFunc(proc.ProcessInfo())
		}
	})

	// prestart processes
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, info := range pm.psInfoMap.InfoMap {
		if info.CheckAlive() {
			info.Stop(true)
		}
		if procFunc != nil {
			procFunc(info)
		}
		pm.psInfoMap.InfoMap[info.Program] = info
	}

	pm.psInfoMap.Store(pm.psInfoFile)
}

func (pm *ProcessManager) RemoveAllProcesses(procFunc func(ProcessInfo)) {
	pm.ForEachProcess(func(proc *Process) {
		if proc.GetPid() == 0 {
			name := proc.GetName()
			delete(pm.procs, name)
			pm.psInfoMap.RemoveProcessInfo(name)
		}
	})

	// remove dead prestart processes info
	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, info := range pm.psInfoMap.InfoMap {
		if !info.CheckAlive() {
			if procFunc != nil {
				procFunc(info)
			}
			pm.psInfoMap.RemoveProcessInfo(info.Program)
		}
	}

	pm.psInfoMap.Store(pm.psInfoFile)
}

func (pm *ProcessManager) RemoveProcessInfoFile() {
	os.Remove(pm.psInfoFile)
}

func (pm *ProcessManager) StopAllProcesses(start bool, exit bool) {
	var flag bool

	if start && pm.startKillAll {
		flag = true
	}

	if exit && pm.exitKillAll {
		flag = true
	}

	if flag {
		pm.KillAllProcesses(nil)
		pm.RemoveProcessInfoFile()
	} else {
		pm.lock.Lock()
		defer pm.lock.Unlock()
		pm.psInfoMap.Store(pm.psInfoFile)
	}
}

func sortProcess(procs []*Process) []*Process {
	prog_configs := make([]*config.ConfigEntry, 0)
	for _, proc := range procs {
		if proc.config.IsProgram() {
			prog_configs = append(prog_configs, proc.config)
		}
	}

	result := make([]*Process, 0)
	p := config.NewProcessSorter()
	for _, config := range p.SortProgram(prog_configs) {
		for _, proc := range procs {
			if proc.config == config {
				result = append(result, proc)
			}
		}
	}

	return result
}
