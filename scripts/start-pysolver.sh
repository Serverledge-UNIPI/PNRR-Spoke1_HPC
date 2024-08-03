#!/bin/bash
docker build -t pysolver ~/energy_efficient_serverledge/pysolver/
docker run -d --rm --name pysolver-server --publish 5000:5000 pysolver