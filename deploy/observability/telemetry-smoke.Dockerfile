# The smoke script uses only Python's standard library. Keep the base small and
# update the preinstalled packages that the upstream image ships with.
FROM python:3.13-alpine

RUN python -m pip install --no-cache-dir --upgrade \
    msgpack==1.2.3 \
    setuptools==78.1.1 \
    urllib3==2.8.0

WORKDIR /workspace
ENTRYPOINT ["python"]
