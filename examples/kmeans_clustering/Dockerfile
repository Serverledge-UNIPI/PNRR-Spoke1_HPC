FROM python:3.10-slim
RUN pip3 install scikit-learn numpy jsonschema

COPY executor.py /
COPY function.py /

WORKDIR /
CMD python executor.py