package process

import (
	"fmt"
	"os"
	"strings"
	"time"

	sync "github.com/AlexStocks/goext/sync/deadlock"
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
	return pm.psInfoMap.getDeadPrestartProcess()
}

func (pm *ProcessManager) GetFrozenPrestartProcess() (int, []ProcessInfo) {
	return pm.psInfoMap.getFrozenPrestartProcess()
}

// 用于 status _infomap
func (pm *ProcessManager) GetAllInfomapProcess() (int, []ProcessInfo) {
	return pm.psInfoMap.getAllInfomapProcess()
}

// the return value list:
// the first is prestart process number
// the second is the active prestart process name array
func (pm *ProcessManager) GetActivePrestartProcess() (int, []ProcessInfo) {
	return pm.psInfoMap.getActivePrestartProcess()
}

func (pm *ProcessManager) GetPrestartProcess() (int, []ProcessInfo) {
	return pm.psInfoMap.getPrestartProcess()
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

func (pm *ProcessManager) ValidateStartPs() ([]string, []string) {
	return pm.psInfoMap.validateStartPs(pm.psInfoFile, pm.startKillAll)
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

	pm.psInfoMap.store(pm.psInfoFile)

	return proc
}

func (pm *ProcessManager) StartAutoStartPrograms() {
	pm.ForEachProcess(func(proc *Process) {
		if proc.isAutoStart() {
			proc.Start(false, func(p *Process) {
				pm.UpdateProcessInfo(proc) // to defeat dead-lock
				// pm.psInfoMap.AddProcessInfo(proc.ProcessInfo())
			})
		}
	})

	pm.psInfoMap.store(pm.psInfoFile)
}

func (pm *ProcessManager) UpdateProcessInfo(proc *Process) {
	if nil != pm {
		procState := proc.GetState()
		if procState == RUNNING {
			pm.psInfoMap.addProcessInfo(proc.ProcessInfo())
		} else {
			// 20180131 Bug Fix
			// 测试到如下case：程序原来的配置错误导致程序无法启动，然后其对应的 Process.Start() [下面称为 start1] 不断循环运行；
			//               当把配置修改正确，然后再执行 update 指令后， 一个新的 Process.Start() [start2]程序能够正确启动，把新的 Process
			//               信息通过上面的 if 分支保存到 psInfoMap 之中后退出。
			//               start1 会在此时退出，然后会执行这个 else 分支。如果 start1 晚于 start2 退出，则会导致已经被正确添加的进程信息被
			//               删除，所以此处调用 removeProcessInfoByPID 以根据 psName 和 pid 正确校验后再删除进程信息。
			pm.psInfoMap.removeProcessInfoByPID(proc.GetName(), proc.pid.Load())
		}

		pm.psInfoMap.store(pm.psInfoFile)
	}
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
	// pm.psInfoMap.addProcessInfo(info)

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

	pm.psInfoMap.store(pm.psInfoFile)

	log.Info("create event listener:", eventListenerName)
	return evtListener
}

func (pm *ProcessManager) AddProc(proc *Process) {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	name := proc.config.GetProgramName()
	pm.procs[name] = proc
	// pm.psInfoMap.addProcessInfo(proc.ProcessInfo())
	log.Info("add process:", name)

	pm.psInfoMap.store(pm.psInfoFile)
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
		pm.psInfoMap.store(pm.psInfoFile)
	} else {
		pos := strings.Index(name, ":")
		if pos != -1 {
			proc, ok = pm.procs[name[pos+1:]]
			if ok {
				log.Info("remove process:", name[pos+1:])
				proc.Stop(wait)
				pm.psInfoMap.store(pm.psInfoFile)
			}
		}
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
	pm.psInfoMap.removeProcessInfo(name)
	log.Info("remove process:", name)
	pm.psInfoMap.store(pm.psInfoFile)

	return proc
}

func (pm *ProcessManager) RemoveProcessInfo(name string) ProcessInfo {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	log.Info("remove process info:", name)
	info := pm.psInfoMap.removeProcessInfo(name)
	pm.psInfoMap.store(pm.psInfoFile)

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
	return pm.psInfoMap.findProcessInfo(name)
}

func (pm *ProcessManager) StopProcessInfo(name string, wait bool) *ProcessInfo {
	info := pm.psInfoMap.stopProcessInfo(name, wait)
	if info != nil {
		pm.psInfoMap.store(pm.psInfoFile)
	}

	return info
}

// clear all the processes
func (pm *ProcessManager) Clear() {
	pm.lock.Lock()
	defer pm.lock.Unlock()
	pm.procs = make(map[string]*Process)
	pm.psInfoMap.Reset()

	pm.psInfoMap.store(pm.psInfoFile)
}

func (pm *ProcessManager) ForEachProcess(procFunc func(p *Process)) {
	var procs []*Process
	func() {
		pm.lock.Lock()
		defer pm.lock.Unlock()

		// 获取一份 pm.procs 的拷贝，以防止下面 for 循环中再次对 pm.procs 进行增删操作时有死锁问题
		procs = pm.getAllProcess()
	}()
	for _, proc := range procs {
		procFunc(proc)
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
		pm.psInfoMap.removeProcessInfo(proc.config.GetProgramName())
		if procFunc != nil {
			procFunc(proc.ProcessInfo())
		}
	})

	// kill all prestart processes
	pm.psInfoMap.killAllProcess(procFunc)
	pm.psInfoMap.store(pm.psInfoFile)
}

func (pm *ProcessManager) RemoveAllProcesses(procFunc func(ProcessInfo)) {
	pm.ForEachProcess(func(proc *Process) {
		if proc.GetPid() == 0 {
			name := proc.GetName()
			pm.lock.Lock()
			delete(pm.procs, name)
			pm.lock.Unlock()
			pm.psInfoMap.removeProcessInfo(name)
		}
	})

	// remove dead prestart processes info
	pm.psInfoMap.removeAllProcess(procFunc)
	pm.psInfoMap.store(pm.psInfoFile)
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
		pm.psInfoMap.store(pm.psInfoFile)
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
