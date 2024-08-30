# ----------------------- CLOUD -----------------------

#!/bin/bash
docker run \
        --name prom \
        -d \
        -p 9090:9090 \
        -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
        prom/prometheus --enable-feature=agent \
        --config.file=/etc/prometheus/prometheus.yml


#!/bin/sh
docker run \
        --name promRemote \
        -d \
        -p 9091:9090 \
        -v $(pwd)/prometheus_remote.yml:/etc/prometheus/prometheus.yml \
        -v $(pwd)/rules:/etc/prometheus/rules \
        prom/prometheus \
        --web.enable-remote-write-receiver \
        --config.file=/etc/prometheus/prometheus.yml

# ----------------------- AGENT -----------------------

#!/bin/bash
docker run \
        --name prometheusLocal \
        -d --rm \
        -p 9090:9090 \
        -v $(pwd)/prometheus.yml:/etc/prometheus/prometheus.yml \
        prom/prometheus:v2.37.1  \
        --config.file=/etc/prometheus/prometheus.yml
