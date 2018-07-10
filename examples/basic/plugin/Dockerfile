# build stage
FROM golang:alpine AS build-go

ENV http_proxy http://ftpproxy.blackrock.com:8080
ENV https_proxy http://ftpproxy.blackrock.com:8080
ENV HTTP_PROXY http://ftpproxy.blackrock.com:8080`
ENV HTTPS_PROXY http://ftpproxy.blackrock.com:8080

#RUN apt-get update && apt-get install -y git
RUN apk --no-cache add git
ENV D=/go/src/github.com/magaldima/intercom
# deps
RUN go get -u github.com/golang/dep/...
COPY ./* $D/app/
RUN cd $D/app && dep init -v
#RUN cd $D && dep ensure -v --vendor-only
# build
RUN cd $D/app && go build -o plugin && cp plugin /tmp/

# final stage
FROM alpine:3.7
RUN apk --no-cache add ca-certificates
WORKDIR /app/server/
COPY --from=build-go /tmp/plugin /app/server/
CMD ["./plugin"]