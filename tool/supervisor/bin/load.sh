#!/usr/bin/env bash
# ******************************************************
# DESC    :
# AUTHOR  : Alex Stocks
# VERSION : 1.0
# LICENCE : Apache License 2.0
# EMAIL   : alexstocks@foxmail.com
# MOD     : 2018-11-26 20:24
# FILE    : load.sh
# ******************************************************

APP_NAME="supervisord"
APP_BIN="/test/supervisord"
APP_CONF_FILE="/home/vagrant/test/supervisor/conf/supervisord.conf"

usage() {
    echo "Usage: $0 start"
    echo "       $0 stop"
    echo "       $0 restart"
    echo "       $0 list"
    echo "       $0 ctl start program"
    echo "       $0 ctl status"
    exit
}

start() {
	CMD="${APP_BIN} -c ${APP_CONF_FILE} -d"
	eval ${CMD}
    PID=`ps aux | grep -w ${APP_NAME} | grep -v grep | grep -v tail | awk '{print $2}'`
    CUR=`date +%FT%T`
    if [ "${PID}" != "" ]; then
    	for p in ${PID}
    	do
    		echo "start ${APP_NAME} ( pid =" ${p} ") at " ${CUR}
    	done
    fi
}

list() {
    PID=`ps aux | grep -w ${APP_NAME} | grep -v grep | grep -v tail | awk '{printf("%s,%s,%s,%s\n", $1, $2, $9, $10)}'`
    if [ "${PID}" != "" ]; then
        echo "list ${APP_NAME}"

        if [[ ${OS_NAME} == "Linux" || ${OS_NAME} == "Darwin" ]]; then
            echo "index: user, pid, start, duration"
        else
            echo "index: PID, WINPID, UID, STIME, COMMAND"
        fi
        idx=0
        for ps in ${PID}
        do
            echo "${idx}: ${ps}"
            ((idx ++))
        done
    fi
}

stop() {
	PID=`ps aux | grep -w ${APP_NAME} | grep -v grep | grep -v tail | awk '{print $2}'`
    if [ "${PID}" != "" ]; then
        for ps in ${PID}
        do
            echo "kill -SIGINT ${APP_NAME} ( pid =" ${ps} ")"
            kill -2 ${ps}
        done
    fi
}

ctl_start() {
	PROGRAM=$1
	CMD="${APP_BIN} -c ${APP_CONF_FILE} ctl start ${PROGRAM}"
	eval ${CMD}
    PID=`${APP_BIN} -c ${APP_CONF_FILE} ctl status | grep -w ${APP_NAME}`
    echo "start ${PROGRAM} =" ${PID}
}

opt=$1
case C"$opt" in
    Cstart)
        start
        ;;
    Cstop)
        stop
        ;;
    Crestart)
        stop
        start
        ;;
    Clist)
        list
        ;;
    Cctl)
		if [ $# -lt 2 ]; then
            usage
        fi
		opt=$2
        case C"$opt" in
            Cstart)
		        if [ $# != 3 ]; then
                    usage
                fi
                ctl_start $3
                ;;

            Cstatus)
                usage
            ;;

            C*)
                usage
            ;;
        esac
        ;;
    C*)
        usage
        ;;
esac

