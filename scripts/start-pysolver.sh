#!/bin/bash

cd ~/energy_efficient_serverledge/pysolver

# Check if the virtual enviroment already exists
if [ ! -d "venv" ]; then
    echo "Virtual environment not found. Creating venv and installing requirements..."
    python3 -m venv venv
    source venv/bin/activate
    pip3 install -r requirements.txt
else
    echo "Virtual environment already exists"
    source venv/bin/activate
fi

python3 main.py