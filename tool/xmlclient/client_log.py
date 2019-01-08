#!/usr/bin/env python
# -*- coding: UTF-8 -*-


import os
import socket
import logging
import logging.handlers

logger_map = {}

class HostnameFilter(logging.Filter):
    hostname = ''

    def __init__(self):
        self.hostname = socket.gethostname()

    def filter(self, record):
        record.hostname = self.hostname
        return True

hostname = HostnameFilter()
def init_log(log_path="./log/", level=logging.DEBUG, when="D", backup=7,
             format="%(asctime)s %(thread)d %(filename)s[line:%(lineno)d] %(levelname)s %(message)s",
             datefmt="%Y-%m-%dT%H:%M:%S",
             host=None, port=None, socktype="tcp"):
    """
    init_log - initialize log module

    Args:
      log_path      - Log file path prefix.
                      Log data will go to two files: log_path.log and log_path.log.wf
                      Any non-exist parent directories will be created automatically
      level         - msg above the level will be displayed
                      DEBUG < INFO < WARNING < ERROR < CRITICAL
                      the default value is logging.INFO
      when          - how to split the log file by time interval
                      'S' : Seconds
                      'M' : Minutes
                      'H' : Hours
                      'D' : Days
                      'W' : Week day
                      default value: 'D'
      format        - format of the log
                      default format:
                      %(levelname)s: %(asctime)s: %(filename)s:%(lineno)d * %(thread)d %(message)s
                      INFO: 12-09 18:02:42: log.py:40 * 139814749787872 HELLO WORLD
      backup        - how many backup file to keep
                      default value: 7

      datefmt       - format of the log
      host          - stream/datagram log socket peer host address
      port          - stream/datagram log socket peer host port
      socktype      - stream/datagram log socket type
    Raises:
        OSError: fail to create log directories
        IOError: fail to open log file
    """
    formatter = logging.Formatter(format, datefmt)

    if host and port:
        log_path = "%s:%s:%s" % (socktype, host, port)
    if log_path in logger_map:
        # print ("get %s logger from map" % log_path)
        return logger_map[log_path]

    logger = logging.getLogger(log_path)
    logger.setLevel(level)

    handler = None
    if host and port and socktype:
        # logging.handlers.DatagramHandler
        if socktype == "tcp":
            address=(host, int(port))
            client = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            client.connect(address)
            handler = logging.StreamHandler(client.makefile())
        elif socktype == "udp":
            handler = logging.handlers.DatagramHandler(host, port)
        else:
            raise ValueError("bad socktype, socktype: %s" % (socktype))

        handler.setFormatter(formatter)
        handler.setLevel(level)
        logger.addHandler(handler)
    else:
        dir = os.path.dirname(log_path)
        if not os.path.isdir(dir):
            os.makedirs(dir)

        handler = logging.handlers.TimedRotatingFileHandler(log_path,
                                                            when=when,
                                                            backupCount=backup)
        handler.setLevel(level)
        handler.setFormatter(formatter)
        logger.addHandler(handler)

    logger.propagate = 0
    logger_map[log_path] = logger

    return logger
