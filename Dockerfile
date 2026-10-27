# Makeup Radar website: compiles the app and runs it as a small web server.
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /makeup-radar .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 radar
USER radar
COPY --from=build /makeup-radar /usr/local/bin/makeup-radar
ENV MAKEUP_RADAR_WEB=1
EXPOSE 10000
CMD ["makeup-radar", "--web"]
