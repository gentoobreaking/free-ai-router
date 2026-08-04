FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY freemodel-router /usr/local/bin/
COPY data /usr/local/bin/data
EXPOSE 7352
ENTRYPOINT ["freemodel-router", "start"]
