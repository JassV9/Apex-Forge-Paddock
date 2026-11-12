# The demo's recorded sessions as an image, for platforms without a folder to mount (Azure
# Container Apps). It runs as an init container: it copies the spool into the volume core
# reads, and opens the volumes core writes to its non-root user.
# Build from the repository root: docker build -f deploy/demo/data.Dockerfile .
FROM busybox:stable
COPY datasets/demo/spool /dataset
CMD ["sh", "-c", "cp -r /dataset/. /spool/ && chmod 0777 /data /outbox"]
