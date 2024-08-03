FROM python:3.10-slim

WORKDIR /app

COPY requirements.txt requirements.txt
RUN pip install -r requirements.txt

COPY . .

ENTRYPOINT ["python", "main.py"]

# Set default arguments (optional, can be overridden)
CMD ["--port", "5000"]