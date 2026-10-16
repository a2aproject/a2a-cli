# The binary is built by deploy.sh, so the image needs no Go toolchain.
FROM gcr.io/distroless/static-debian12
COPY a2a /a2a
ENTRYPOINT ["/a2a", "server", "relay", "serve"]
