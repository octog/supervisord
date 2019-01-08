python supervisor bug list:
---

    - 1 python版本不明原因、或者程序过多时会崩溃, 出现问题时要有明确错误提示;
    - 2 有多级日志目录没创建会崩溃，支持自动创建多级日志目录;
    - 3 每次崩溃后必须将之前启动的服务重启一遍再加入到supervisor管理, 可以崩溃但重启后不能再将现有已经运行的服务重启一遍;
    - 4 kill -2 不能将已经运行的服务全部kill掉.


Related Work:

    - 1 https://github.com/arvenil/supervisord
    - 2 https://github.com/gwaycc/supd