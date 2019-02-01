#!/usr/bin/env python3
# -*- coding: UTF-8 -*-
# ******************************************************
# DESC    :
# AUTHOR  : Alex Stocks
# VERSION : 1.0
# LICENCE : Apache License 2.0
# EMAIL   : alexstocks@foxmail.com
# MOD     : 2019-02-01 14:27
# FILE    : multicall.py
# ******************************************************

# import xmlrpc.client
from xmlrpc.client import ServerProxy

def testMulticall(proxy):
    marshalled_list = [
      {'methodName': 'supervisor.getSupervisorVersion', 'params': []},
      {'methodName': 'supervisor.getAPIVersion', 'params': []},
      {'methodName': 'supervisor.getState', 'params': []},
      {'methodName': 'supervisor.getPID', 'params': []},
    ]

    supervisor_version, api_version, state, pid = proxy.system.multicall(marshalled_list)

def testMethods(proxy):
	# methods = proxy.supervisor.listMethods()
	methods = proxy.system.listMethods()
	print("supervisord list methods:%s " % (methods))

if __name__ == '__main__':
	proxy = ServerProxy('http://192.168.11.110:19001')
	testMethods(proxy)
