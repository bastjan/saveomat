FROM golang:1 AS builder

COPY . /build
WORKDIR /build
RUN go build

FROM gcr.io/distroless/base-debian10

COPY --from=builder /build/saveomat /

EXPOSE 8080

ENTRYPOINT [ "/saveomat" ]
