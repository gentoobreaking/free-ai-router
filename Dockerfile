FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY freemodel-router /usr/local/bin/
EXPOSE 7352
ENTRYPOINT ["freemodel-router", "start"]
