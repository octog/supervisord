## 修改bug日志
1. 解决加载*.conf.bak的问题  update能启动后缀名为bak的配置
2. 解决conf配置中环境变量带空格的问题，环境变量不能加载，程序无法启动
3. 解决停止进程，程序奔溃的问题，原因是并发，锁的问题
4. 解决multicall只支持单参数且参数必须是slice的API问题，现在multicall已支持所有API
5. 解决conf配置不支持numprocs项的问题，同一个group配置多个进程，无法update的问题
6. 解决status显示冒号的问题
