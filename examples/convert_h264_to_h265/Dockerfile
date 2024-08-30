FROM python:3.10-slim

RUN apt-get update && \
    apt-get install -y \
    ffmpeg \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*
RUN pip3 install --no-cache-dir ffmpeg-python jsonschema

COPY executor.py /
COPY function.py /

WORKDIR /
CMD python executor.py
