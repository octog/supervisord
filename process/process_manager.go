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
		_, err := gxprocess.FindProcess(int(info.PID))
		if err != nil {
			// delete(pm5.psInfoMap.InfoMap, name)
			pm.psInfoMap.RemoveProcessInfo(name)
			psArray = append(psArray, name)
		}
	}

	return prestartNum, psArray
}

func (pm *ProcessManager) GetAllInfomapProcess() (int, []ProcessInfo) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(pm.psInfoMap.InfoMap))
	for _, info := range pm.psInfoMap.InfoMap {
		prestartNum++
		if _, err := gxprocess.FindProcess(int(info.PID)); err == nil {
			psArray = append(psArray, info)
		}
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
		prestartNum++
		if _, err := gxprocess.FindProcess(int(info.PID)); err == nil {
			psArray = append(psArray, info)
		}
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

	if config.IsProgram() {
		return pm.createProgram(supervisor_id, config)
	} else if config.IsEventListener() {
		return pm.createEventListener(supervisor_id, config)
	}

	return nil
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
}

func (pm *ProcessManager) UpdateProcessInfo(proc *Process) {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	pm.psInfoMap.AddProcessInfo(proc.ProcessInfo())
}

func (pm *ProcessManager) createProgram(supervisor_id string, config *config.ConfigEntry) *Process {
	var proc *Process

	procName := config.GetProgramName()
	info, ok := pm.psInfoMap.GetProcessInfo(procName)
	if !ok {
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

	return proc
}

func (pm *ProcessManager) RemoveProcessInfo(name string) ProcessInfo {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	log.Info("remove process info:", name)
	return pm.psInfoMap.RemoveProcessInfo(name)
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

// clear all the processes
func (pm *ProcessManager) Clear() {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.procs = make(map[string]*Process)
	pm.psInfoMap.Reset()
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
		pm.psInfoMap.RemoveProcessInfo(proc.config.GetProgramName())
		if procFunc != nil {
			procFunc(proc.ProcessInfo())
		}
	})

	pm.lock.Lock()
	defer pm.lock.Unlock()
	for _, info := range pm.psInfoMap.InfoMap {
		// Fix: remove the proc info from info map before stopped the prestart program.
		// Otherwise the `MonitorPrestartProcess` will restart the stopped program.
		pm.psInfoMap.RemoveProcessInfo(info.Program)
		info.Stop(true)
		if procFunc != nil {
			procFunc(info)
		}
	}
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
