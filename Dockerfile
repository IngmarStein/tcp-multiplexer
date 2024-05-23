FROM scratch
ENTRYPOINT ["/tcp-multiplexer"]
COPY tcp-multiplexer /
