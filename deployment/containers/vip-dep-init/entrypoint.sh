#!/bin/sh
TUNL_INTERFACE="${TUNL_INTERFACE-tunl0}"

while true; do
    ip addr ls $TUNL_INTERFACE | grep 'inet\b'
    status=$?
    if [ $status -eq 0 ]; then
        echo 'Tunnel interface has been initialized with a VIP.'
        exit 0;
    fi
    sleep 1
done
