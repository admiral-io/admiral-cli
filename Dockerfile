FROM gcr.io/distroless/static-debian12:nonroot@sha256:a9329520abc449e3b14d5bc3a6ffae065bdde0f02667fa10880c49b35c109fd1

ARG TARGETOS
ARG TARGETARCH

COPY --chown=nonroot:nonroot ${TARGETOS}/${TARGETARCH}/admiral /usr/bin/admiral

ENTRYPOINT ["/usr/bin/admiral"]
CMD ["--help"]