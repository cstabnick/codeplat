# Builds the farm server as a static binary and runs it alone in an empty image.
# The page and scripts are embedded in the binary, so nothing else is copied.
#
#   docker build -t farm .
#   docker run -d --read-only --cap-drop=ALL --security-opt no-new-privileges \
#     -p 127.0.0.1:8088:8088 farm
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY web ./web
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /farm ./cmd/farm

FROM scratch
COPY --from=build /farm /farm
# nobody: the server needs no privileges and writes nothing.
USER 65534:65534
# Inside the container it must listen on all interfaces for the published port
# to reach it; publish the port on 127.0.0.1 so only the host's proxy can.
ENV FARM_ADDR=0.0.0.0:8088
EXPOSE 8088
ENTRYPOINT ["/farm"]
