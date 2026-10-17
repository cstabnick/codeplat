# Builds every binary with ./start, then runs ./start in a small Alpine image.
FROM golang:alpine AS build
RUN apk add --no-cache bash make
WORKDIR /src
COPY . .
# --only with no match builds everything and starts nothing.
RUN bash start --only none

FROM alpine
RUN apk add --no-cache bash sed
WORKDIR /app
COPY --from=build /src/bin ./bin
COPY --from=build /src/data ./data
COPY --from=build /src/samples ./samples
COPY start stop ./
# The router needs Ollama and llama-server, and the sandbox needs Docker,
# so neither runs here by default.
# Bind every service to all interfaces so published ports reach it. The
# tracker takes its addresses from config/docker.json.
ENV CODEPLAT_ENV=docker \
    CHAT_ADDR=0.0.0.0:8084 \
    DECIDE_ADDR=0.0.0.0:8085 \
    CHESS_ADDR=0.0.0.0:8086 \
    BOOKMARKS_API_ADDR=0.0.0.0:8081 \
    BOOKMARKS_UI_ADDR=0.0.0.0:8082
EXPOSE 8080 8081 8082 8084 8085 8086 8090
CMD ["bash", "start", "--no-build", "--skip", "router,sandbox"]
