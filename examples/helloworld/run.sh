#!/bin/sh
mainpid=$(lsof -i:8888|grep 'LISTEN'|awk '{print $2}')
echo $mainpid
if [ $mainpid > 0 ];then
    echo "main process id:$mainpid"
    kill -9 $mainpid
    if [ $? -eq 0 ];then
    echo "kill $mainpid success"
    go run main.go
    else
    echo "kill $mainpid fail"
    fi
else
    go run main.go
fi