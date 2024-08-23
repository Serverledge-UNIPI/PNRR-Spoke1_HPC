#!/bin/bash
docker build -t pysolver $(pwd)/pysolver/
docker run -d --rm --name pysolver-server --publish 5000:5000 pysolver