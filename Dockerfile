FROM golang:1.25 AS builder

WORKDIR /app

COPY ./src .

RUN useradd mimic

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o /app/mimic-lens \
    ./cmd/web/main.go

FROM scratch

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

COPY --from=builder --chown=mimic:mimic /app /app

USER mimic
WORKDIR /app

CMD [ "/app/mimic-lens" ]
 