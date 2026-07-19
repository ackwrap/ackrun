#!/bin/sh

uci -q set ackwrap.main.enabled='1'
uci -q commit ackwrap
/etc/init.d/ackwrap restart
