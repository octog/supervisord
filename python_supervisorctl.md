# python supervisorctl
---

## 1 python supervisorctl commands intro
---

### 1.1 add
---

- 1. 添加进程


### 1.2 pid
---

- 1. 查看进程 pid

### 1.3 update
---

- 1. 添加管理程序时使用 update或者update all 会将配置文件所在目录下所有未管理的程序全部添加进来并启动，想添加单个程序时可通过update 指定程序名来添加
- 2. 当把配置文件移走时执行update或者update all 会将已经管理并且启动的程序停止并移除管理
- 3. 执行update 或者update all的时候对原先已经管理程序的状态不做处理除非配置文件发生变更

### 1.4 remove
---

- 1. remove的时候不支持all参数
- 2. remove的时候程序的状态是非RUNNING状态的
- 3. remove掉的程序只要配置文件还存在还可以用update重新添加管理
- 4. remove后面可以跟多个项目名

### 1.5 reload
---

- 1. 载入最新的配置文件，停止原有进程并按新的配置启动

### 1.6 remove
---

- 1. 移除某一进程，移除前必须是停止状态

### 1.7 reread
---

- 1. 重新加载守护进程的配置文件

### 1.8 restart
---

- 1. 重启进程

### 1.9 shutdown
---

- 1. 关闭 supervisord

### 1.10 start
---

- 1. 启动一个或者多个进程

### 1.11 status
---

- 1. 查看进程的状态

### 1.12 stop
---

- 1. 停止一个或者多个进程


## 2 supervisorctl command http apis

### 2.1 status
---

  - 1 start


### 2.2 status
---

  - 1 getAllProcessInfo

### 2.3 pid ps
---

  - 1 getProcessInfo

### 2.4 add
---

  - 1 addProcessGroup

### 2.4 start ps
---

  - 1 startProcess


### 2.5 start all
---

  - 1 startAllProcesses

### 2.6 remove ps/all
---

  - 1 removeProcessGroup ps/all


###  2.7 update ps
---

  - 1 reloadconfig

      这个命令返回 add / change / remove 三组结果

  - 2 add/change/remove

    + 2.1 addProcessGroup

      如果 update 的对象 ps 在 add 结果之中，则调用这个 http api

    + 2.2 stopProcess -> removeProcessGroup -> addProcessGroup

      如果 update 的对象 ps 在 change 结果之中，则调用这三个 http api

    + 2.3 stopProcessGroup -> removeProcessGroup

      如果 update 的对象 ps 在 remove 结果之中，则调用这两个 http api

    + 2.4 getAllProcessInfo -> addProcessGroup

      对于已经存在的进程，先调用 reloadConfig，在调用上面的 http apis

## 3 go supervisorctl
---

### 3.1 status
---

  - 1 supervisord -c /home/vagrant/test/supervisor/conf/supervisord.conf ctl status _procs

      查看 go supervisord ProcessManager.procs 内的数据成员

  - 2 supervisord -c /home/vagrant/test/supervisor/conf/supervisord.conf ctl status _infomap

      查看 go supervisord ProcessManager.psInfoMap 内的数据成员

