#!/bin/bash

set -ex

## Upgrade ovsdb
# ovsdb-client convert unix:///var/openlan/confd.sock /etc/openlan/switch/confd.schema.json

## Enable check for DDNS
# [root@centos ~]# crontab -l
# 0,5,10,15,20,25,30,35,40,45,50,55 * * * * /var/openlan/script/update.sh
# [root@centos ~]#

## Update your DDNS
export OL_VERSION=v6
/usr/bin/openlan na ls | grep -w 'name:' | sed 's/name://g' | xargs -i /usr/bin/openlan name add --name {}
