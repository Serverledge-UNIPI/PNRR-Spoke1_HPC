FROM python:3.10-slim
RUN pip3 install cryptography jsonschema

COPY executor.py /
COPY function.py /

WORKDIR /
CMD python executor.py

