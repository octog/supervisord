# coding=utf-8

host = None

class ProdConfig(object):
    DEBUG = False
    SupervisorHost = "http://192.168.11.100:19001"
    SupervisorProxy = "http://192.168.11.100:19110"

class TestConfig(object):
    DEBUG = False
    SupervisorHost = "http://192.168.11.110:19001"
    SupervisorProxy = "http://192.168.11.110:19110"

class DevConfig(object):
    DEBUG = True
    SupervisorHost = "http://127.0.0.1:19001"
    SupervisorProxy = "http://10.31.63.62:19110"
