FROM python:3.10-slim
RUN pip3 install numpy jsonschema

COPY executor.py /
COPY function.py /

WORKDIR /
CMD python executor.py