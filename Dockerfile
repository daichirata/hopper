FROM gcr.io/distroless/base
COPY hopper /hopper
ENTRYPOINT ["/hopper"]
