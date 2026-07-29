FROM python:3.12-slim

# Sin esto, los print() del worker quedan en el buffer de stdout y nunca
# se ven en "docker compose logs" — el proceso corre, pero parece mudo.
ENV PYTHONUNBUFFERED=1

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

CMD ["python", "reconciliation_worker.py"]
