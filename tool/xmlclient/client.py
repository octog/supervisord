# !/usr/bin/env python
# -*- coding: UTF-8 -*-
# ******************************************************
# DESC    :
# AUTHOR  : Alex Stocks
# VERSION : 1.0
# LICENCE : Apache License 2.0
# EMAIL   : alexstocks@foxmail.com
# MOD     : 2018-11-28 11:24
# FILE    : client.py
# ******************************************************

import os
import sys
import time

import config
import client_log as log

import xmlrpclib

def test():
    print("supervisord host: %s" % config.conf.SupervisorHost)
    server = xmlrpclib.Server(config.conf.SupervisorHost + '/RPC2')
    process_name = "udp_test1"

    # clearXXX
    # 结果不正确
    # print("\n clearAllProcessLogs:%s" % server.supervisor.clearAllProcessLogs())
    # 结果不正确
    # print("\n clearLog:%s" % server.supervisor.clearLog())
    # 不支持
    # print("\n clearProcessLog:%s" % server.supervisor.clearProcessLog(process_name))
    # print("\n clearProcessLogs:%s" % server.supervisor.clearProcessLogs(process_name))
    # print("\n :%s" % server.supervisor.())

    # getXXX
    # print("\n getAPIVersion:%s" % server.supervisor.getAPIVersion())
    # 不支持
    # print("\n getAllConfigInfo:%s" % server.supervisor.getAllConfigInfo())
    # print("\n getAllProcessInfo:%s" % server.supervisor.getAllProcessInfo())
    # print("\n getIdentification:%s" % server.supervisor.getIdentification())
    # print("\n getPID:%s" % server.supervisor.getPID(process_name))
    # print("\n getProcessInfo:%s" % server.supervisor.getProcessInfo(process_name))
    # 结果不正确
    # print("\n getState:%s" % server.supervisor.getState(process_name))
    # print("\n getSupervisorVersion:%s" % server.supervisor.getSupervisorVersion())
    # print("\n getVersion:%s" % server.supervisor.getVersion())

    # sXXX
    # print("\n startProcess:%s" % server.supervisor.startProcess(process_name))
    # print("\n stopProcess:%s" % server.supervisor.stopProcess(process_name))
    # print("\n :%s" % server.supervisor.())

    # print(server.system.listMethods())
    # go-supervisord 把这个方法放入了 supervisor 里面
    # print("\ngo-supervisord methods: %s" % server.supervisor.listMethods())

    # 暂不支持
    # print(server.system.methodHelp('supervisor.shutdown'))

if __name__ == '__main__':
    cwd = os.path.dirname(os.path.realpath(__file__))
    log = log.init_log(cwd + '/log/client.log')
    log.info('client starting...')

    env = ""
    if len(sys.argv) >= 2:
      env = sys.argv[1]

    # config.conf = config.DevConfig
    config.conf = config.TestConfig
    if env == "prod":
        config.conf = config.ProdConfig
    elif env == "test":
        config.conf = config.TestConfig

    test()
