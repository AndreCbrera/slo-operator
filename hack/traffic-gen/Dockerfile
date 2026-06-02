FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY bin/traffic-gen .
USER 65532:65532
ENTRYPOINT ["/traffic-gen"]
