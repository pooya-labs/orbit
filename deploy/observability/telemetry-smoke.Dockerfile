# The smoke scripts use Python's standard library only. Use the slim upstream
# image instead of carrying optional third-party packages into this test helper.
FROM python:3.13-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /workspace
USER 65532:65532
ENTRYPOINT ["python"]
